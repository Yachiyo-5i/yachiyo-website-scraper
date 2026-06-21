package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yachiyo-website-scraper/internal/config"
	"yachiyo-website-scraper/internal/fetcher"
)

func TestEncodeResult(t *testing.T) {
	encoded, err := EncodeResult(&Result{
		OK:   true,
		Site: "local",
		Task: "search",
		Data: map[string]interface{}{"title": "Example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("expected valid JSON, got %s", encoded)
	}
	if !strings.Contains(string(encoded), "\n  \"ok\": true") {
		t.Fatalf("expected indented result JSON, got %s", encoded)
	}
}

func TestPageMetaAndPaginationMeta(t *testing.T) {
	source := map[string]interface{}{
		"total_results": 42,
	}
	meta := pageMeta(source)
	meta["new"] = "value"
	if _, ok := source["new"]; ok {
		t.Fatal("pageMeta should copy metadata")
	}

	applyPaginationMeta(meta, &config.PaginationConfig{
		Param:      "page",
		TotalField: "total_results",
	}, map[string]string{"page": "last"}, 3)

	if meta["count"] != 3 || meta["page"] != "last" || meta["total"] != 42 {
		t.Fatalf("unexpected pagination meta: %#v", meta)
	}
}

func TestApplyPaginationDefault(t *testing.T) {
	vars := map[string]string{"page": " "}
	applyPaginationDefault(&config.PaginationConfig{Param: "page", Default: "1"}, vars)
	if vars["page"] != "1" {
		t.Fatalf("expected default page, got %#v", vars)
	}

	vars["page"] = "2"
	applyPaginationDefault(&config.PaginationConfig{Param: "page", Default: "1"}, vars)
	if vars["page"] != "2" {
		t.Fatalf("non-empty page should not be overwritten, got %#v", vars)
	}

	applyPaginationDefault(nil, vars)
}

func TestAcceptedStatus(t *testing.T) {
	if !acceptedStatus(http.StatusNoContent, nil) {
		t.Fatal("2xx statuses should be accepted")
	}
	if !acceptedStatus(http.StatusNotFound, []int{http.StatusNotFound}) {
		t.Fatal("configured status should be accepted")
	}
	if acceptedStatus(http.StatusInternalServerError, []int{http.StatusNotFound}) {
		t.Fatal("unconfigured non-2xx status should be rejected")
	}
}

func TestResolveParamsDefaultsRegexAndRequired(t *testing.T) {
	task := config.Task{
		Params: map[string]config.ParamSpec{
			"code": {Required: true, Regex: `(?i)^fc2-(\d+)$`},
			"page": {Default: "1"},
		},
	}

	vars, err := resolveParams(&config.Config{}, task, map[string]string{"code": "FC2-4913917"})
	if err != nil {
		t.Fatal(err)
	}
	if vars["code"] != "4913917" || vars["page"] != "1" {
		t.Fatalf("unexpected resolved params: %#v", vars)
	}

	_, err = resolveParams(&config.Config{}, task, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), `required param "code" is missing`) {
		t.Fatalf("expected missing required param error, got %v", err)
	}
}

func TestNormalizeParamsErrors(t *testing.T) {
	tests := []struct {
		name string
		task config.Task
		vars map[string]string
		want string
	}{
		{
			name: "invalid regex",
			task: config.Task{Params: map[string]config.ParamSpec{"code": {Regex: "("}}},
			vars: map[string]string{"code": "abc"},
			want: "regex",
		},
		{
			name: "mismatch",
			task: config.Task{Params: map[string]config.ParamSpec{"code": {Regex: `^(\d+)$`}}},
			vars: map[string]string{"code": "abc"},
			want: "does not match regex",
		},
		{
			name: "group out of range",
			task: config.Task{Params: map[string]config.ParamSpec{"code": {Regex: `^(\d+)$`, RegexGroup: 2}}},
			vars: map[string]string{"code": "123"},
			want: "regex_group 2 out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := normalizeParams(tt.task, tt.vars)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestResolveIndexedParams(t *testing.T) {
	indexPath := writeRunnerIndex(t, `{
  "actors": [
    {"id": "ra6", "name": "Alice"},
    {"id": "ra7", "name": "Bob"}
  ]
}`)
	cfg := &config.Config{
		Indexes: map[string]config.Index{
			"actors": {Path: indexPath, ItemsKey: "actors", MatchField: "name", ValueField: "id"},
		},
	}
	task := config.Task{
		ResolveParams: map[string]config.ParamResolver{
			"id": {Index: "actors", From: "name"},
		},
	}

	vars := map[string]string{"name": "alice"}
	if err := resolveIndexedParams(cfg, task, vars); err != nil {
		t.Fatal(err)
	}
	if vars["id"] != "ra6" {
		t.Fatalf("unexpected resolved id: %#v", vars)
	}

	vars = map[string]string{"name": "Alice", "id": "manual"}
	if err := resolveIndexedParams(cfg, task, vars); err != nil {
		t.Fatal(err)
	}
	if vars["id"] != "manual" {
		t.Fatalf("existing target param should not be overwritten, got %#v", vars)
	}

	vars = map[string]string{}
	if err := resolveIndexedParams(cfg, task, vars); err != nil {
		t.Fatalf("empty source value should be skipped, got %v", err)
	}
}

func TestResolveIndexedParamsReportsErrors(t *testing.T) {
	indexPath := writeRunnerIndex(t, `{
  "actors": [
    {"id": "a1", "name": "Alice"},
    {"id": "a2", "name": "Alice"}
  ]
}`)
	baseTask := config.Task{
		ResolveParams: map[string]config.ParamResolver{
			"id": {Index: "actors", From: "name"},
		},
	}

	tests := []struct {
		name string
		cfg  *config.Config
		task config.Task
		vars map[string]string
		want string
	}{
		{
			name: "index not found in config",
			cfg:  &config.Config{},
			task: baseTask,
			vars: map[string]string{"name": "Alice"},
			want: `index "actors" not found`,
		},
		{
			name: "no match",
			cfg:  &config.Config{Indexes: map[string]config.Index{"actors": {Path: indexPath, ItemsKey: "actors"}}},
			task: baseTask,
			vars: map[string]string{"name": "Carol"},
			want: "no match",
		},
		{
			name: "ambiguous",
			cfg:  &config.Config{Indexes: map[string]config.Index{"actors": {Path: indexPath, ItemsKey: "actors"}}},
			task: baseTask,
			vars: map[string]string{"name": "Alice"},
			want: "ambiguous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolveIndexedParams(tt.cfg, tt.task, tt.vars)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestBuildURL(t *testing.T) {
	got, err := buildURL("https://example.test/base/", config.RequestConfig{
		Path: "search/{code}",
		Query: map[string]string{
			"page": "{page}",
			"q":    "{code}",
		},
	}, map[string]string{"code": "SSIS-001", "page": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.test/base/search/SSIS-001?page=2&q=SSIS-001" {
		t.Fatalf("unexpected built URL: %q", got)
	}

	got, err = buildURL("https://ignored.test", config.RequestConfig{
		URL: "https://absolute.test/{code}",
	}, map[string]string{"code": "ABC-123"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://absolute.test/ABC-123" {
		t.Fatalf("unexpected absolute URL: %q", got)
	}
}

func TestBuildURLReportsTemplateAndParseErrors(t *testing.T) {
	tests := []struct {
		name string
		base string
		req  config.RequestConfig
		vars map[string]string
		want string
	}{
		{
			name: "missing path variable",
			base: "https://example.test",
			req:  config.RequestConfig{Path: "/{missing}"},
			vars: map[string]string{},
			want: "missing template variables",
		},
		{
			name: "invalid base",
			base: "http://%zz",
			req:  config.RequestConfig{Path: "/ok"},
			vars: map[string]string{},
			want: "invalid URL escape",
		},
		{
			name: "invalid query template",
			base: "https://example.test",
			req:  config.RequestConfig{Path: "/ok", Query: map[string]string{"q": "{missing}"}},
			vars: map[string]string{},
			want: "missing template variables",
		},
		{
			name: "invalid absolute template",
			base: "https://example.test",
			req:  config.RequestConfig{URL: "https://example.test/{missing}"},
			vars: map[string]string{},
			want: "missing template variables",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildURL(tt.base, tt.req, tt.vars)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestRenderHeaders(t *testing.T) {
	got, err := renderHeaders(map[string]string{
		"X-Code": "{code}",
	}, map[string]string{"code": "PRED-886"})
	if err != nil {
		t.Fatal(err)
	}
	if got["X-Code"] != "PRED-886" {
		t.Fatalf("unexpected headers: %#v", got)
	}

	_, err = renderHeaders(map[string]string{"X-Code": "{missing}"}, map[string]string{})
	if err == nil {
		t.Fatal("expected missing header template variable")
	}
}

func TestRuntimeOptionsCookiePrecedence(t *testing.T) {
	cfg := &config.Config{Defaults: config.DefaultsConfig{Cookie: "code={code}"}}
	got, err := runtimeOptions(cfg, fetcher.RuntimeOptions{}, map[string]string{"code": "SSIS-001"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cookie != "code=SSIS-001" {
		t.Fatalf("unexpected default cookie: %#v", got.Cookie)
	}

	got, err = runtimeOptions(cfg, fetcher.RuntimeOptions{Cookie: "runtime=1"}, map[string]string{"code": "SSIS-001"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cookie != "runtime=1" {
		t.Fatalf("runtime cookie should take precedence, got %#v", got.Cookie)
	}

	_, err = runtimeOptions(cfg, fetcher.RuntimeOptions{}, map[string]string{})
	if err == nil {
		t.Fatal("expected missing cookie template variable error")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", " ", "value", "later"); got != "value" {
		t.Fatalf("unexpected first non-empty value: %q", got)
	}
	if got := firstNonEmpty("", " "); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestRunWritesDumpAndReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("<html>server error</html>"))
	}))
	defer server.Close()

	cfg := runnerTestConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  search:
    request:
      path: /search
    extract:
      fields:
        title:
          xpath: //h1
    output:
      format:
        title: "{title}"
`, "__BASE__", server.URL))
	dumpPath := filepath.Join(t.TempDir(), "dump.html")

	res, err := Run(context.Background(), cfg, Options{
		TaskName: "search",
		DumpHTML: dumpPath,
		Runtime:  fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Error == nil || res.Error.Type != "http_error" {
		t.Fatalf("expected http_error result, got %+v", res)
	}
	dumped, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(dumped) != "<html>server error</html>" {
		t.Fatalf("unexpected dump body: %q", dumped)
	}
}

func TestRunReturnsExtractAndOutputErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>Example</h1></body></html>`))
	}))
	defer server.Close()

	t.Run("extract error", func(t *testing.T) {
		cfg := runnerTestConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  detail:
    request:
      path: /detail
    extract:
      fields:
        title:
          xpath: //missing
          required: true
    output:
      type: object
      format:
        title: "{title}"
`, "__BASE__", server.URL))
		res, err := Run(context.Background(), cfg, Options{
			TaskName: "detail",
			Runtime:  fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.OK || res.Error == nil || res.Error.Type != "extract_error" {
			t.Fatalf("expected extract_error result, got %+v", res)
		}
	})

	t.Run("output error", func(t *testing.T) {
		cfg := runnerTestConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  detail:
    request:
      path: /detail
    extract:
      fields:
        title:
          xpath: //h1
    output:
      type: object
      format:
        title: "{missing}"
`, "__BASE__", server.URL))
		res, err := Run(context.Background(), cfg, Options{
			TaskName: "detail",
			Runtime:  fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.OK || res.Error == nil || res.Error.Type != "output_error" {
			t.Fatalf("expected output_error result, got %+v", res)
		}
	})
}

func TestRunObjectOutputWithNoItemsReturnsEmptyObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><p>No items</p></body></html>`))
	}))
	defer server.Close()

	cfg := runnerTestConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  detail:
    request:
      path: /detail
    extract:
      scope:
        xpath: //article
      fields:
        title:
          xpath: .//h1
    output:
      type: object
      format:
        title: "{title}"
`, "__BASE__", server.URL))
	res, err := Run(context.Background(), cfg, Options{
		TaskName: "detail",
		Runtime:  fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got %+v", res.Error)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok || len(data) != 0 {
		t.Fatalf("expected empty object data, got %#v", res.Data)
	}
}

func writeRunnerIndex(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actors.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runnerTestConfig(t *testing.T, yml string) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
