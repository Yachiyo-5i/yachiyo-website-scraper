package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"yachiyo-website-scraper/internal/config"
	"yachiyo-website-scraper/internal/extractor"
	"yachiyo-website-scraper/internal/fetcher"
	"yachiyo-website-scraper/internal/indexer"
	"yachiyo-website-scraper/internal/templatex"
)

type Options struct {
	ConfigPath string
	TaskName   string
	Params     map[string]string
	Runtime    fetcher.RuntimeOptions
	DumpHTML   string
}

type Result struct {
	OK      bool                   `json:"ok"`
	Site    string                 `json:"site"`
	Task    string                 `json:"task"`
	URL     string                 `json:"url,omitempty"`
	Channel fetcher.Channel        `json:"channel,omitempty"`
	Status  int                    `json:"status,omitempty"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
	Data    interface{}            `json:"data,omitempty"`
	Error   *ErrorInfo             `json:"error,omitempty"`
	Debug   map[string]string      `json:"debug,omitempty"`
}

type ErrorInfo struct {
	Type    string   `json:"type"`
	Reason  string   `json:"reason"`
	Matched []string `json:"matched,omitempty"`
}

func Run(ctx context.Context, cfg *config.Config, opts Options) (*Result, error) {
	task, err := cfg.Task(opts.TaskName)
	if err != nil {
		return nil, err
	}

	vars, err := resolveParams(cfg, task, opts.Params)
	if err != nil {
		return nil, err
	}
	applyPaginationDefault(task.Pagination, vars)

	reqURL, err := buildURL(cfg.Site.BaseURL, task.Request, vars)
	if err != nil {
		return nil, err
	}
	headers, err := renderHeaders(cfg.HeadersFor(task), vars)
	if err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(task.Request.Method))
	if method == "" {
		method = "GET"
	}
	runtime, err := runtimeOptions(cfg, opts.Runtime, vars)
	if err != nil {
		return nil, err
	}

	fetchResult, err := fetcher.Fetch(ctx, fetcher.Request{
		Method:  method,
		URL:     reqURL,
		Headers: headers,
	}, runtime)
	if fetchResult == nil {
		return nil, err
	}
	resp := fetchResult.Response
	if opts.DumpHTML != "" {
		if err := os.WriteFile(opts.DumpHTML, []byte(resp.Body), 0o644); err != nil {
			return nil, fmt.Errorf("dump html: %w", err)
		}
	}

	result := &Result{
		Site:    cfg.Site.ID,
		Task:    opts.TaskName,
		URL:     reqURL,
		Channel: resp.Channel,
		Status:  resp.Status,
	}

	if fetchResult.Challenge.Detected {
		result.OK = false
		result.Error = &ErrorInfo{
			Type:    "blocked",
			Reason:  fetchResult.Challenge.Reason,
			Matched: fetchResult.Challenge.Matched,
		}
		return result, nil
	}
	if err != nil {
		result.OK = false
		result.Error = &ErrorInfo{Type: "fetch_error", Reason: err.Error()}
		return result, nil
	}
	if !acceptedStatus(resp.Status, task.Request.AcceptStatus) {
		result.OK = false
		result.Error = &ErrorInfo{Type: "http_error", Reason: fmt.Sprintf("status %d", resp.Status)}
		return result, nil
	}

	page, err := extractor.ExtractPage(resp.Body, task.Extract, extractor.Options{BaseURL: resp.FinalURL})
	if err != nil {
		result.OK = false
		result.Error = &ErrorInfo{Type: "extract_error", Reason: err.Error()}
		return result, nil
	}

	formatted := make([]map[string]interface{}, 0, len(page.Items))
	for _, item := range page.Items {
		out, err := extractor.FormatOutput(task.Output.Format, item)
		if err != nil {
			result.OK = false
			result.Error = &ErrorInfo{Type: "output_error", Reason: err.Error()}
			return result, nil
		}
		formatted = append(formatted, out)
	}

	meta := pageMeta(page.Meta)
	applyPaginationMeta(meta, task.Pagination, vars, len(formatted))
	if len(meta) > 0 {
		result.Meta = meta
	}

	outputType := task.Output.Type
	if outputType == "" {
		outputType = "list"
	}
	result.OK = true
	if task.Output.ItemsKey != "" || len(task.Output.PageFormat) > 0 {
		data, err := buildPageData(task.Output, page.Page, formatted)
		if err != nil {
			result.OK = false
			result.Error = &ErrorInfo{Type: "output_error", Reason: err.Error()}
			return result, nil
		}
		result.Data = data
	} else if outputType == "object" {
		if len(formatted) == 0 {
			result.Data = map[string]interface{}{}
		} else {
			result.Data = formatted[0]
		}
	} else {
		result.Data = formatted
	}
	return result, nil
}

func buildPageData(output config.OutputConfig, pageFields extractor.ExtractedItem, items []map[string]interface{}) (map[string]interface{}, error) {
	data := map[string]interface{}{}
	if len(output.PageFormat) > 0 {
		rendered, err := extractor.FormatAny(output.PageFormat, pageFields)
		if err != nil {
			return nil, err
		}
		renderedData, ok := rendered.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("output.page_format must render to object")
		}
		data = renderedData
	}
	itemsKey := output.ItemsKey
	if itemsKey == "" {
		itemsKey = "items"
	}
	data[itemsKey] = items
	return data, nil
}

func pageMeta(meta extractor.ExtractedMeta) map[string]interface{} {
	out := make(map[string]interface{}, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func applyPaginationMeta(meta map[string]interface{}, cfg *config.PaginationConfig, vars map[string]string, count int) {
	if cfg == nil {
		return
	}

	meta["count"] = count
	if cfg.Param != "" {
		pageValue := vars[cfg.Param]
		if page, err := strconv.Atoi(strings.TrimSpace(pageValue)); err == nil {
			meta["page"] = page
		} else {
			meta["page"] = pageValue
		}
	}
	if cfg.TotalField != "" && cfg.TotalField != "total" {
		if total, ok := meta[cfg.TotalField]; ok {
			meta["total"] = total
		}
	}
}

func applyPaginationDefault(cfg *config.PaginationConfig, vars map[string]string) {
	if cfg == nil || cfg.Param == "" || cfg.Default == "" {
		return
	}
	if strings.TrimSpace(vars[cfg.Param]) == "" {
		vars[cfg.Param] = cfg.Default
	}
}

func acceptedStatus(status int, accepted []int) bool {
	if status >= 200 && status < 300 {
		return true
	}
	for _, allowed := range accepted {
		if status == allowed {
			return true
		}
	}
	return false
}

func EncodeResult(result *Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

func resolveParams(cfg *config.Config, task config.Task, input map[string]string) (map[string]string, error) {
	vars := make(map[string]string, len(input)+len(task.Params))
	for k, v := range input {
		vars[k] = v
	}
	for name, spec := range task.Params {
		if _, ok := vars[name]; !ok && spec.Default != "" {
			vars[name] = spec.Default
		}
	}
	if err := resolveIndexedParams(cfg, task, vars); err != nil {
		return nil, err
	}
	if err := normalizeParams(task, vars); err != nil {
		return nil, err
	}
	for name, spec := range task.Params {
		if spec.Required && strings.TrimSpace(vars[name]) == "" {
			return nil, fmt.Errorf("required param %q is missing", name)
		}
	}
	return vars, nil
}

func normalizeParams(task config.Task, vars map[string]string) error {
	for name, spec := range task.Params {
		if strings.TrimSpace(spec.Regex) == "" {
			continue
		}
		value := strings.TrimSpace(vars[name])
		if value == "" {
			continue
		}
		re, err := regexp.Compile(spec.Regex)
		if err != nil {
			return fmt.Errorf("param %q regex: %w", name, err)
		}
		matches := re.FindStringSubmatch(value)
		if len(matches) == 0 {
			return fmt.Errorf("param %q value %q does not match regex %q", name, value, spec.Regex)
		}
		group := spec.RegexGroup
		if group == 0 && len(matches) > 1 {
			group = 1
		}
		if group >= len(matches) {
			return fmt.Errorf("param %q regex_group %d out of range for %q", name, group, spec.Regex)
		}
		vars[name] = strings.TrimSpace(matches[group])
	}
	return nil
}

func resolveIndexedParams(cfg *config.Config, task config.Task, vars map[string]string) error {
	for target, resolver := range task.ResolveParams {
		if strings.TrimSpace(vars[target]) != "" {
			continue
		}
		sourceValue := strings.TrimSpace(vars[resolver.From])
		if sourceValue == "" {
			continue
		}

		indexCfg, ok := cfg.Indexes[resolver.Index]
		if !ok {
			return fmt.Errorf("index %q not found", resolver.Index)
		}
		itemsKey := firstNonEmpty(indexCfg.ItemsKey, "items")
		matchField := firstNonEmpty(resolver.MatchField, indexCfg.MatchField, resolver.From)
		valueField := firstNonEmpty(resolver.ValueField, indexCfg.ValueField, target)

		idx, err := indexer.Load(indexCfg.Path, itemsKey)
		if err != nil {
			return err
		}
		resolved, matches, err := indexer.LookupOne(idx, matchField, valueField, sourceValue, indexCfg.CaseSensitive)
		if err != nil {
			return fmt.Errorf("resolve param %q from %q=%q: %w", target, resolver.From, sourceValue, err)
		}
		if resolved == "" {
			return fmt.Errorf("resolve param %q from %q=%q: no match in index %q", target, resolver.From, sourceValue, resolver.Index)
		}
		if len(matches) > 0 {
			vars[target] = resolved
		}
	}
	return nil
}

func buildURL(baseURL string, req config.RequestConfig, vars map[string]string) (string, error) {
	if req.URL != "" {
		return templatex.Render(req.URL, vars)
	}

	path, err := templatex.Render(req.Path, vars)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	finalURL := base.ResolveReference(rel)
	query := finalURL.Query()
	for key, valueTemplate := range req.Query {
		value, err := templatex.Render(valueTemplate, vars)
		if err != nil {
			return "", err
		}
		query.Set(key, value)
	}
	finalURL.RawQuery = query.Encode()
	return finalURL.String(), nil
}

func renderHeaders(headers map[string]string, vars map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		rendered, err := templatex.Render(v, vars)
		if err != nil {
			return nil, err
		}
		out[k] = rendered
	}
	return out, nil
}

func runtimeOptions(cfg *config.Config, runtime fetcher.RuntimeOptions, vars map[string]string) (fetcher.RuntimeOptions, error) {
	if strings.TrimSpace(runtime.Cookie) != "" || strings.TrimSpace(cfg.Defaults.Cookie) == "" {
		return runtime, nil
	}
	cookie, err := templatex.Render(cfg.Defaults.Cookie, vars)
	if err != nil {
		return runtime, err
	}
	runtime.Cookie = cookie
	return runtime, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
