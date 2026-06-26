package extractor

import (
	"fmt"
	stdhtml "html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"yachiyo-website-scraper/internal/config"
	"yachiyo-website-scraper/internal/templatex"

	"github.com/antchfx/htmlquery"
)

type ExtractedItem map[string]interface{}
type ExtractedMeta map[string]interface{}

type Options struct {
	BaseURL string
}

func Extract(body string, cfg config.ExtractConfig, opts Options) ([]ExtractedItem, error) {
	page, err := ExtractPage(body, cfg, opts)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

type Page struct {
	Items []ExtractedItem
	Meta  ExtractedMeta
	Page  ExtractedItem
}

func ExtractPage(body string, cfg config.ExtractConfig, opts Options) (*Page, error) {
	if strings.EqualFold(strings.TrimSpace(cfg.Type), "json") {
		return ExtractJSONPage(body, cfg)
	}

	doc, err := htmlquery.Parse(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	meta, err := extractMeta(doc, cfg.Meta, opts)
	if err != nil {
		return nil, err
	}
	pageFields, skipPage, err := extractItem(doc, cfg.Page, opts)
	if err != nil {
		return nil, err
	}
	if skipPage {
		pageFields = ExtractedItem{}
	}

	nodes := []*html.Node{doc}
	if cfg.Scope != nil && strings.TrimSpace(cfg.Scope.XPath) != "" {
		nodes, err = htmlquery.QueryAll(doc, cfg.Scope.XPath)
		if err != nil {
			return nil, fmt.Errorf("scope xpath %q: %w", cfg.Scope.XPath, err)
		}
	}

	items := make([]ExtractedItem, 0, len(nodes))
	for _, node := range nodes {
		item, skip, err := extractItem(node, cfg.Fields, opts)
		if err != nil {
			return nil, err
		}
		if skip {
			continue
		}
		items = append(items, item)
	}
	return &Page{Items: items, Meta: meta, Page: pageFields}, nil
}

func extractMeta(doc *html.Node, fields map[string]config.FieldConfig, opts Options) (ExtractedMeta, error) {
	meta := make(ExtractedMeta, len(fields))
	for name, field := range fields {
		value, missing, err := extractField(doc, field, opts)
		if err != nil {
			return nil, fmt.Errorf("meta %q: %w", name, err)
		}
		if missing {
			switch normalizedOnMissing(field) {
			case "error", "skip_item":
				return nil, fmt.Errorf("meta %q is missing", name)
			default:
				value = missingValue(field)
			}
		}
		meta[name] = value
	}
	return meta, nil
}

func extractItem(node *html.Node, fields map[string]config.FieldConfig, opts Options) (ExtractedItem, bool, error) {
	item := make(ExtractedItem, len(fields))
	for name, field := range fields {
		value, missing, err := extractField(node, field, opts)
		if err != nil {
			return nil, false, fmt.Errorf("field %q: %w", name, err)
		}
		if missing {
			switch normalizedOnMissing(field) {
			case "skip_item":
				return nil, true, nil
			case "error":
				return nil, false, fmt.Errorf("field %q is missing", name)
			default:
				item[name] = missingValue(field)
			}
			continue
		}
		item[name] = value
	}
	return item, false, nil
}

func extractField(node *html.Node, field config.FieldConfig, opts Options) (interface{}, bool, error) {
	xpath := strings.TrimSpace(field.XPath)
	if xpath == "" {
		return nil, true, nil
	}

	nodes, err := htmlquery.QueryAll(node, xpath)
	if err != nil {
		return nil, false, err
	}
	if len(nodes) == 0 {
		return nil, true, nil
	}

	if field.Multiple {
		values := make([]string, 0, len(nodes))
		for _, n := range nodes {
			nodeValues, err := valuesFromNode(n, field, opts)
			if err != nil {
				return nil, false, err
			}
			for _, value := range nodeValues {
				values = append(values, fmt.Sprint(value))
			}
		}
		if len(values) == 0 {
			return nil, true, nil
		}
		return values, false, nil
	}

	value, ok, err := valueFromNode(nodes[0], field, opts)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, true, nil
	}
	return value, false, nil
}

func valueFromNode(node *html.Node, field config.FieldConfig, opts Options) (interface{}, bool, error) {
	values, err := valuesFromNode(node, field, opts)
	if err != nil {
		return nil, false, err
	}
	if len(values) == 0 {
		return "", false, nil
	}
	return values[0], true, nil
}

func valuesFromNode(node *html.Node, field config.FieldConfig, opts Options) ([]interface{}, error) {
	attr := field.Attr
	if attr == "" {
		attr = "text"
	}

	var value string
	switch attr {
	case "text":
		value = htmlquery.InnerText(node)
	case "html":
		value = innerHTML(node)
	case "outer_html":
		value = htmlquery.OutputHTML(node, true)
	default:
		value = htmlquery.SelectAttr(node, attr)
	}

	if field.Trim {
		value = strings.TrimSpace(value)
	}

	values := []string{value}
	if field.Regex != "" {
		re, err := regexp.Compile(field.Regex)
		if err != nil {
			return nil, err
		}
		matches := re.FindAllStringSubmatch(value, -1)
		if len(matches) == 0 {
			return nil, nil
		}
		group := field.RegexGroup
		if group == 0 && len(matches[0]) > 1 {
			group = 1
		}
		values = make([]string, 0, len(matches))
		for _, match := range matches {
			if group >= len(match) {
				return nil, fmt.Errorf("regex_group %d out of range for %q", group, field.Regex)
			}
			values = append(values, match[group])
		}
	}

	convertedValues := make([]interface{}, 0, len(values))
	for _, value := range values {
		value = stdhtml.UnescapeString(value)
		if field.Trim {
			value = strings.TrimSpace(value)
		}
		if field.ResolveURL && value != "" {
			resolved, err := resolveURL(opts.BaseURL, value)
			if err != nil {
				return nil, err
			}
			value = resolved
		}
		if value == "" {
			continue
		}
		converted, err := convertValue(value, field.Type)
		if err != nil {
			return nil, err
		}
		convertedValues = append(convertedValues, converted)
	}
	return convertedValues, nil
}

func convertValue(value string, typ string) (interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "", "string":
		return value, nil
	case "int", "integer":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("convert %q to int: %w", value, err)
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("unsupported field type %q", typ)
	}
}

func FormatOutput(format map[string]interface{}, item ExtractedItem) (map[string]interface{}, error) {
	rendered, err := FormatAny(format, item)
	if err != nil {
		return nil, err
	}
	out, ok := rendered.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("output format must render to object")
	}
	return out, nil
}

func FormatAny(format interface{}, item ExtractedItem) (interface{}, error) {
	vars := make(map[string]interface{}, len(item))
	for k, v := range item {
		vars[k] = v
	}
	return templatex.RenderAny(format, vars)
}

func normalizedOnMissing(field config.FieldConfig) string {
	if field.OnMissing != "" {
		return field.OnMissing
	}
	if field.Required {
		return "error"
	}
	return "null"
}

func missingValue(field config.FieldConfig) interface{} {
	if field.Default != "" {
		return field.Default
	}
	if field.Multiple {
		return []string{}
	}
	return nil
}

func resolveURL(baseURL, raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(parsed).String(), nil
}

func innerHTML(node *html.Node) string {
	var b strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(htmlquery.OutputHTML(child, true))
	}
	return b.String()
}
