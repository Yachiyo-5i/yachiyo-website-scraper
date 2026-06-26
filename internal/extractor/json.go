package extractor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"yachiyo-website-scraper/internal/config"
)

type jsonPathToken struct {
	key      string
	index    int
	wildcard bool
}

func ExtractJSONPage(body string, cfg config.ExtractConfig) (*Page, error) {
	var root interface{}
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}

	meta, err := extractJSONMeta(root, cfg.Meta)
	if err != nil {
		return nil, err
	}
	pageFields, skipPage, err := extractJSONItem(root, cfg.Page)
	if err != nil {
		return nil, err
	}
	if skipPage {
		pageFields = ExtractedItem{}
	}

	nodes := []interface{}{root}
	if cfg.Scope != nil && strings.TrimSpace(cfg.Scope.Path) != "" {
		nodes, err = evalJSONPath(root, cfg.Scope.Path)
		if err != nil {
			return nil, fmt.Errorf("scope path %q: %w", cfg.Scope.Path, err)
		}
	}

	items := make([]ExtractedItem, 0, len(nodes))
	for _, node := range nodes {
		item, skip, err := extractJSONItem(node, cfg.Fields)
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

func extractJSONMeta(root interface{}, fields map[string]config.FieldConfig) (ExtractedMeta, error) {
	meta := make(ExtractedMeta, len(fields))
	for name, field := range fields {
		value, missing, err := extractJSONField(root, field)
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

func extractJSONItem(root interface{}, fields map[string]config.FieldConfig) (ExtractedItem, bool, error) {
	item := make(ExtractedItem, len(fields))
	for name, field := range fields {
		value, missing, err := extractJSONField(root, field)
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

func extractJSONField(root interface{}, field config.FieldConfig) (interface{}, bool, error) {
	path := strings.TrimSpace(field.Path)
	if path == "" {
		return nil, true, nil
	}
	values, err := evalJSONPath(root, path)
	if err != nil {
		return nil, false, err
	}
	if len(values) == 0 {
		return nil, true, nil
	}

	if field.Multiple {
		out := make([]interface{}, 0, len(values))
		for _, value := range values {
			converted, ok, err := normalizeJSONFieldValue(value, field)
			if err != nil {
				return nil, false, err
			}
			if ok {
				out = append(out, converted)
			}
		}
		if len(out) == 0 {
			return nil, true, nil
		}
		return out, false, nil
	}

	converted, ok, err := normalizeJSONFieldValue(values[0], field)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, true, nil
	}
	return converted, false, nil
}

func normalizeJSONFieldValue(value interface{}, field config.FieldConfig) (interface{}, bool, error) {
	if value == nil {
		return nil, false, nil
	}

	if needsStringProcessing(field) {
		text := fmt.Sprint(normalizeJSONValue(value))
		if field.Trim {
			text = strings.TrimSpace(text)
		}
		if field.Regex != "" {
			re, err := regexp.Compile(field.Regex)
			if err != nil {
				return nil, false, err
			}
			matches := re.FindAllStringSubmatch(text, -1)
			if len(matches) == 0 {
				return nil, false, nil
			}
			group := field.RegexGroup
			if group == 0 && len(matches[0]) > 1 {
				group = 1
			}
			if group >= len(matches[0]) {
				return nil, false, fmt.Errorf("regex_group %d out of range for %q", group, field.Regex)
			}
			text = matches[0][group]
		}
		if field.Trim {
			text = strings.TrimSpace(text)
		}
		if text == "" {
			return nil, false, nil
		}
		converted, err := convertValue(text, field.Type)
		if err != nil {
			return nil, false, err
		}
		return converted, true, nil
	}

	normalized := normalizeJSONValue(value)
	switch strings.ToLower(strings.TrimSpace(field.Type)) {
	case "", "string":
		return normalized, true, nil
	case "int", "integer":
		converted, err := convertValue(fmt.Sprint(normalized), field.Type)
		if err != nil {
			return nil, false, err
		}
		return converted, true, nil
	default:
		_, err := convertValue("", field.Type)
		return nil, false, err
	}
}

func needsStringProcessing(field config.FieldConfig) bool {
	return field.Trim || field.Regex != "" || strings.TrimSpace(field.Type) != ""
}

func normalizeJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case json.Number:
		if intValue, err := typed.Int64(); err == nil {
			return int(intValue)
		}
		if floatValue, err := typed.Float64(); err == nil {
			return floatValue
		}
		return typed.String()
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, value := range typed {
			out = append(out, normalizeJSONValue(value))
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, value := range typed {
			out[key] = normalizeJSONValue(value)
		}
		return out
	default:
		return value
	}
}

func evalJSONPath(root interface{}, path string) ([]interface{}, error) {
	tokens, err := parseJSONPath(path)
	if err != nil {
		return nil, err
	}
	values := []interface{}{root}
	for _, token := range tokens {
		next := make([]interface{}, 0, len(values))
		for _, value := range values {
			matches, err := applyJSONPathToken(value, token)
			if err != nil {
				return nil, err
			}
			next = append(next, matches...)
		}
		values = next
		if len(values) == 0 {
			break
		}
	}
	return values, nil
}

func parseJSONPath(path string) ([]jsonPathToken, error) {
	path = strings.TrimSpace(path)
	if path == "" || path[0] != '$' {
		return nil, fmt.Errorf("path must start with $")
	}
	if path == "$" {
		return nil, nil
	}

	var tokens []jsonPathToken
	for i := 1; i < len(path); {
		switch path[i] {
		case '.':
			i++
			start := i
			for i < len(path) && path[i] != '.' && path[i] != '[' {
				i++
			}
			key := strings.TrimSpace(path[start:i])
			if key == "" {
				return nil, fmt.Errorf("empty path segment in %q", path)
			}
			if key == "*" {
				tokens = append(tokens, jsonPathToken{wildcard: true})
			} else {
				tokens = append(tokens, jsonPathToken{key: key})
			}
		case '[':
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unclosed array index in %q", path)
			}
			raw := strings.TrimSpace(path[i+1 : i+end])
			if raw == "*" {
				tokens = append(tokens, jsonPathToken{wildcard: true})
			} else {
				index, err := strconv.Atoi(raw)
				if err != nil {
					return nil, fmt.Errorf("invalid array index %q", raw)
				}
				tokens = append(tokens, jsonPathToken{index: index})
			}
			i += end + 1
		default:
			return nil, fmt.Errorf("unexpected path token %q in %q", path[i], path)
		}
	}
	return tokens, nil
}

func applyJSONPathToken(value interface{}, token jsonPathToken) ([]interface{}, error) {
	if token.wildcard {
		switch typed := value.(type) {
		case []interface{}:
			return typed, nil
		case map[string]interface{}:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			out := make([]interface{}, 0, len(keys))
			for _, key := range keys {
				out = append(out, typed[key])
			}
			return out, nil
		default:
			return nil, nil
		}
	}

	if token.key != "" {
		typed, ok := value.(map[string]interface{})
		if !ok {
			return nil, nil
		}
		child, ok := typed[token.key]
		if !ok {
			return nil, nil
		}
		return []interface{}{child}, nil
	}

	typed, ok := value.([]interface{})
	if !ok {
		return nil, nil
	}
	if token.index < 0 || token.index >= len(typed) {
		return nil, nil
	}
	return []interface{}{typed[token.index]}, nil
}
