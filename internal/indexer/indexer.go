package indexer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	builtin "yachiyo-website-scraper/indexes"
)

type Index struct {
	Path      string
	ItemsKey  string
	Items     []map[string]interface{}
	lookupMu  sync.Mutex
	lookupMap map[lookupKey]map[string][]map[string]interface{}
}

type lookupKey struct {
	Field         string
	CaseSensitive bool
}

var (
	cacheMu sync.Mutex
	cache   = map[string]*Index{}
)

func Load(path, itemsKey string) (*Index, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("index path is required")
	}
	if strings.TrimSpace(itemsKey) == "" {
		itemsKey = "items"
	}

	cacheKey := indexCacheKey(path, itemsKey)
	cacheMu.Lock()
	if idx, ok := cache[cacheKey]; ok {
		cacheMu.Unlock()
		return idx, nil
	}
	cacheMu.Unlock()

	data, err := readIndex(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse index %q: %w", path, err)
	}
	rawItems, ok := doc[itemsKey]
	if !ok {
		return nil, fmt.Errorf("index %q does not contain items key %q", path, itemsKey)
	}
	var items []map[string]interface{}
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return nil, fmt.Errorf("parse index %q key %q: %w", path, itemsKey, err)
	}
	idx := &Index{Path: path, ItemsKey: itemsKey, Items: items}
	cacheMu.Lock()
	cache[cacheKey] = idx
	cacheMu.Unlock()
	return idx, nil
}

func (idx *Index) FindAll(field, value string, caseSensitive bool) []map[string]interface{} {
	expected := normalize(value, caseSensitive)
	if expected == "" {
		return nil
	}

	byValue := idx.lookup(field, caseSensitive)
	matches := byValue[expected]
	out := make([]map[string]interface{}, len(matches))
	copy(out, matches)
	return out
}

func (idx *Index) lookup(field string, caseSensitive bool) map[string][]map[string]interface{} {
	key := lookupKey{Field: field, CaseSensitive: caseSensitive}
	idx.lookupMu.Lock()
	defer idx.lookupMu.Unlock()

	if idx.lookupMap == nil {
		idx.lookupMap = map[lookupKey]map[string][]map[string]interface{}{}
	}
	if existing, ok := idx.lookupMap[key]; ok {
		return existing
	}

	byValue := map[string][]map[string]interface{}{}
	for _, item := range idx.Items {
		raw, ok := item[field]
		if !ok {
			continue
		}
		for _, value := range fieldValues(raw, caseSensitive) {
			if value == "" {
				continue
			}
			byValue[value] = append(byValue[value], item)
		}
	}
	idx.lookupMap[key] = byValue
	return byValue
}

func LookupOne(idx *Index, matchField, valueField, value string, caseSensitive bool) (string, []map[string]interface{}, error) {
	matches := idx.FindAll(matchField, value, caseSensitive)
	if len(matches) == 0 {
		return "", nil, nil
	}

	seen := map[string]bool{}
	var values []string
	for _, match := range matches {
		raw, ok := match[valueField]
		if !ok {
			continue
		}
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	if len(values) == 0 {
		return "", matches, fmt.Errorf("matched %d index item(s), but none contains value field %q", len(matches), valueField)
	}
	if len(values) > 1 {
		return "", matches, fmt.Errorf("index lookup is ambiguous: %d distinct %q values matched", len(values), valueField)
	}
	return values[0], matches, nil
}

func readIndex(path string) ([]byte, error) {
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	}

	name := filepath.Base(path)
	data, err := builtin.Files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read index %q: %w", path, err)
	}
	return data, nil
}

func indexCacheKey(path, itemsKey string) string {
	if stat, err := os.Stat(path); err == nil {
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			abs = path
		}
		return fmt.Sprintf("file:%s:%d:%d:%s", abs, stat.Size(), stat.ModTime().UnixNano(), itemsKey)
	}
	return fmt.Sprintf("builtin:%s:%s", filepath.Base(path), itemsKey)
}

func fieldValues(raw interface{}, caseSensitive bool) []string {
	switch typed := raw.(type) {
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, normalize(fmt.Sprint(item), caseSensitive))
		}
		return values
	default:
		return []string{normalize(fmt.Sprint(raw), caseSensitive)}
	}
}

func normalize(value string, caseSensitive bool) string {
	value = strings.TrimSpace(value)
	if !caseSensitive {
		value = strings.ToLower(value)
	}
	return value
}
