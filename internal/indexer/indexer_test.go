package indexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIndex(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "actors.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadReadsJSONIndexAndCachesByFileMetadata(t *testing.T) {
	path := writeIndex(t, `{
  "actors": [
    {"id": "a1", "name": "Alice"},
    {"id": "b2", "name": "Bob"}
  ]
}`)

	first, err := Load(path, "actors")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(path, "actors")
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatal("expected repeated loads of unchanged file to use cached index")
	}
	if first.Path != path || first.ItemsKey != "actors" || len(first.Items) != 2 {
		t.Fatalf("unexpected loaded index: %+v", first)
	}
}

func TestLoadDefaultsItemsKey(t *testing.T) {
	path := writeIndex(t, `{"items": [{"id": "a1", "name": "Alice"}]}`)

	idx, err := Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if idx.ItemsKey != "items" || len(idx.Items) != 1 {
		t.Fatalf("unexpected index default items key: %+v", idx)
	}
}

func TestLoadReadsBuiltinIndexByBaseName(t *testing.T) {
	idx, err := Load("javbus_actors.json", "actors")
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Items) == 0 {
		t.Fatal("expected builtin javbus actor index to contain items")
	}
}

func TestLoadRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		path string
		key  string
		body string
		want string
	}{
		{
			name: "missing path",
			want: "index path is required",
		},
		{
			name: "invalid json",
			key:  "actors",
			body: `{not-json`,
			want: "parse index",
		},
		{
			name: "missing items key",
			key:  "actors",
			body: `{"items": []}`,
			want: `does not contain items key "actors"`,
		},
		{
			name: "items not list",
			key:  "actors",
			body: `{"actors": {"id": "a1"}}`,
			want: `parse index`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if tt.body != "" {
				path = writeIndex(t, tt.body)
			}
			_, err := Load(path, tt.key)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestFindAllMatchesScalarAndArrayValues(t *testing.T) {
	idx := &Index{Items: []map[string]interface{}{
		{"id": "a1", "name": "Alice", "aliases": []interface{}{"A", "Ace"}},
		{"id": "b2", "name": "alice", "aliases": []interface{}{"B"}},
		{"id": "c3", "name": "Charlie"},
	}}

	matches := idx.FindAll("name", " ALICE ", false)
	if len(matches) != 2 {
		t.Fatalf("expected case-insensitive name matches, got %d", len(matches))
	}
	matches[0]["id"] = "mutated"
	if idx.Items[0]["id"] != "mutated" {
		t.Fatal("FindAll should copy the result slice while preserving item maps")
	}

	if got := idx.FindAll("name", "Alice", true); len(got) != 1 {
		t.Fatalf("expected one case-sensitive match, got %d", len(got))
	}
	if got := idx.FindAll("aliases", "ace", false); len(got) != 1 || got[0]["name"] != "Alice" {
		t.Fatalf("unexpected alias matches: %#v", got)
	}
	if got := idx.FindAll("name", "   ", false); got != nil {
		t.Fatalf("empty lookup value should return nil, got %#v", got)
	}
}

func TestLookupOneReturnsUniqueValue(t *testing.T) {
	idx := &Index{Items: []map[string]interface{}{
		{"id": "a1", "name": "Alice", "aliases": []interface{}{"A", "Ace"}},
		{"id": "a1", "name": "Alice"},
	}}

	value, matches, err := LookupOne(idx, "name", "id", "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	if value != "a1" || len(matches) != 2 {
		t.Fatalf("unexpected lookup result: value=%q matches=%d", value, len(matches))
	}
}

func TestLookupOneNoMatchReturnsEmptyValue(t *testing.T) {
	idx := &Index{Items: []map[string]interface{}{{"id": "a1", "name": "Alice"}}}

	value, matches, err := LookupOne(idx, "name", "id", "Bob", false)
	if err != nil {
		t.Fatal(err)
	}
	if value != "" || matches != nil {
		t.Fatalf("expected empty no-match result, got value=%q matches=%#v", value, matches)
	}
}

func TestLookupOneReportsMissingValueField(t *testing.T) {
	idx := &Index{Items: []map[string]interface{}{{"name": "Alice"}}}

	value, matches, err := LookupOne(idx, "name", "id", "Alice", false)
	if err == nil {
		t.Fatal("expected missing value field error")
	}
	if value != "" || len(matches) != 1 {
		t.Fatalf("unexpected lookup result with error: value=%q matches=%#v", value, matches)
	}
	if !strings.Contains(err.Error(), `none contains value field "id"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLookupOneReportsAmbiguousValues(t *testing.T) {
	idx := &Index{Items: []map[string]interface{}{
		{"id": "a1", "name": "Alice"},
		{"id": "a2", "name": "Alice"},
	}}

	_, _, err := LookupOne(idx, "name", "id", "Alice", false)
	if err == nil {
		t.Fatal("expected ambiguous lookup error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadIndexReportsMissingFileAndBuiltin(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"), "items")
	if err == nil {
		t.Fatal("expected missing index error")
	}
	if !strings.Contains(err.Error(), "read index") {
		t.Fatalf("unexpected error: %v", err)
	}
}
