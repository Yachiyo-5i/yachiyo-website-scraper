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
	Gfriends   ActorImageLookup
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
	if task.Wikipedia != nil {
		return runWikipediaStructuredTask(ctx, cfg, task, opts)
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
	runtime, err := runtimeOptions(cfg, task, opts.Runtime, vars)
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
	if detectedAgeVerification(resp.Body) {
		result.OK = false
		result.Error = &ErrorInfo{
			Type:    "blocked",
			Reason:  "age_verification_required",
			Matched: []string{"body: safe age verification page"},
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
	applyEnhancements(ctx, task, result, opts)
	return result, nil
}

func runWikipediaStructuredTask(ctx context.Context, cfg *config.Config, task config.Task, opts Options) (*Result, error) {
	wikiTask := firstNonEmpty(task.Wikipedia.Config, "wikipedia")
	wikiCfg, err := config.Load(wikiTask)
	if err != nil {
		return &Result{
			OK:     false,
			Site:   cfg.Site.ID,
			Task:   opts.TaskName,
			Error:  &ErrorInfo{Type: "config_error", Reason: err.Error()},
			Status: 0,
		}, nil
	}

	lang := firstNonEmpty(opts.Params["lang"], task.Wikipedia.Lang, "zh")
	title := firstNonEmpty(opts.Params["title"], opts.Params["name"])
	if strings.TrimSpace(title) == "" {
		return &Result{
			OK:     false,
			Site:   cfg.Site.ID,
			Task:   opts.TaskName,
			Error:  &ErrorInfo{Type: "param_error", Reason: "title is required"},
			Status: 0,
		}, nil
	}

	entityTask := firstNonEmpty(task.Wikipedia.EntityTask, "entity_by_title")
	contentTask := firstNonEmpty(task.Wikipedia.ContentTask, "page_content")

	summary, acceptedTitle, ok := runWikipediaStructuredSummary(ctx, wikiCfg, task, title, lang, opts.Runtime)
	if !ok {
		return &Result{
			OK:    false,
			Site:  cfg.Site.ID,
			Task:  opts.TaskName,
			Error: &ErrorInfo{Type: "summary_error", Reason: "unable to load wikipedia summary"},
		}, nil
	}
	entity, _ := runWikipediaObjectTask(ctx, wikiCfg, entityTask, map[string]string{
		"title": acceptedTitle,
		"lang":  lang,
	}, opts.Runtime)
	content, _ := runWikipediaObjectTask(ctx, wikiCfg, contentTask, map[string]string{
		"title": acceptedTitle,
		"lang":  lang,
	}, opts.Runtime)

	result := &Result{
		OK:      true,
		Site:    cfg.Site.ID,
		Task:    opts.TaskName,
		Status:  200,
		Data:    normalizeWikipediaStructuredResult(title, lang, summary, entity, content),
		Channel: fetcher.ChannelHTTP,
	}
	if pageURL := stringValue(summary["page_url"]); pageURL != "" {
		result.URL = pageURL
	}
	meta := map[string]interface{}{}
	if pageID, ok := summary["pageid"].(int); ok && pageID > 0 {
		meta["pageid"] = pageID
	}
	if wikidataID := stringValue(summary["wikidata_id"]); wikidataID != "" {
		meta["wikidata_id"] = wikidataID
	}
	if len(meta) > 0 {
		result.Meta = meta
	}
	return result, nil
}

func runWikipediaStructuredSummary(ctx context.Context, wikiCfg *config.Config, task config.Task, title, lang string, runtime fetcher.RuntimeOptions) (map[string]interface{}, string, bool) {
	summaryTask := firstNonEmpty(task.Wikipedia.SummaryTask, "page_summary")
	summary, ok := runWikipediaObjectTask(ctx, wikiCfg, summaryTask, map[string]string{
		"title": title,
		"lang":  lang,
	}, runtime)
	if ok && strings.TrimSpace(stringValue(summary["wikidata_id"])) != "" {
		return summary, firstNonEmpty(stringValue(summary["title"]), title), true
	}

	searchTitle, found := fetchWikipediaSearchTitle(ctx, wikiCfg, firstNonEmpty(task.Wikipedia.SearchTask, "page_search"), lang, title, runtime)
	if !found {
		return nil, "", false
	}
	summary, ok = runWikipediaObjectTask(ctx, wikiCfg, summaryTask, map[string]string{
		"title": searchTitle,
		"lang":  lang,
	}, runtime)
	if !ok {
		return nil, "", false
	}
	return summary, firstNonEmpty(stringValue(summary["title"]), searchTitle, title), true
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

func detectedAgeVerification(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "enter-btn") &&
		strings.Contains(lower, "safeid=") &&
		strings.Contains(body, "满18岁")
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

		idx, err := loadIndex(indexCfg, itemsKey)
		if err != nil {
			return err
		}
		resolved, matches, err := indexer.LookupOne(idx, matchField, valueField, sourceValue, indexCfg.CaseSensitive)
		if err != nil {
			if resolver.Optional && isMissingOptionalIndexValue(err) {
				continue
			}
			return fmt.Errorf("resolve param %q from %q=%q: %w", target, resolver.From, sourceValue, err)
		}
		if resolved == "" {
			if resolver.Optional {
				continue
			}
			return fmt.Errorf("resolve param %q from %q=%q: no match in index %q", target, resolver.From, sourceValue, resolver.Index)
		}
		if len(matches) > 0 {
			vars[target] = resolved
		}
	}
	return nil
}

func loadIndex(indexCfg config.Index, itemsKey string) (*indexer.Index, error) {
	if len(indexCfg.Items) > 0 {
		return indexer.FromItems(indexCfg.Items, itemsKey), nil
	}
	return indexer.Load(indexCfg.Path, itemsKey)
}

func isMissingOptionalIndexValue(err error) bool {
	return strings.Contains(err.Error(), "none contains value field")
}

func buildURL(baseURL string, req config.RequestConfig, vars map[string]string) (string, error) {
	if req.URL != "" {
		rendered, err := templatex.Render(req.URL, vars)
		if err != nil {
			return "", err
		}
		finalURL, err := url.Parse(rendered)
		if err != nil {
			return "", err
		}
		if err := applyRequestQuery(finalURL, req, vars); err != nil {
			return "", err
		}
		return finalURL.String(), nil
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
	if err := applyRequestQuery(finalURL, req, vars); err != nil {
		return "", err
	}
	return finalURL.String(), nil
}

func applyRequestQuery(finalURL *url.URL, req config.RequestConfig, vars map[string]string) error {
	query := finalURL.Query()
	for key, valueTemplate := range req.Query {
		value, err := renderQueryValue(valueTemplate, vars, req.OmitEmptyQuery)
		if err != nil {
			return err
		}
		if req.OmitEmptyQuery && strings.TrimSpace(value) == "" {
			continue
		}
		query.Set(key, value)
	}
	finalURL.RawQuery = query.Encode()
	return nil
}

func renderQueryValue(valueTemplate string, vars map[string]string, allowMissing bool) (string, error) {
	if !allowMissing {
		return templatex.Render(valueTemplate, vars)
	}
	missingAsEmpty := make(map[string]string, len(vars))
	for k, v := range vars {
		missingAsEmpty[k] = v
	}
	for _, key := range templatex.Placeholders(valueTemplate) {
		if _, ok := missingAsEmpty[key]; !ok {
			missingAsEmpty[key] = ""
		}
	}
	return templatex.Render(valueTemplate, missingAsEmpty)
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

func runtimeOptions(cfg *config.Config, task config.Task, runtime fetcher.RuntimeOptions, vars map[string]string) (fetcher.RuntimeOptions, error) {
	if strings.TrimSpace(runtime.Cookie) == "" && strings.TrimSpace(cfg.Defaults.Cookie) != "" {
		cookie, err := templatex.Render(cfg.Defaults.Cookie, vars)
		if err != nil {
			return runtime, err
		}
		runtime.Cookie = cookie
	}
	if runtime.Autoclick == nil {
		autoclick := task.Request.Autoclick
		if autoclick == nil {
			autoclick = cfg.Defaults.Autoclick
		}
		if autoclick != nil {
			xpath, err := templatex.Render(autoclick.XPath, vars)
			if err != nil {
				return runtime, err
			}
			runtime.Autoclick = &fetcher.AutoclickConfig{XPath: xpath}
		}
	}
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
