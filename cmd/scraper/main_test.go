package main

import (
	"context"
	"encoding/json"
	"io"
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

func TestParamsFlagSetAndString(t *testing.T) {
	params := paramsFlag{}
	if err := params.Set(" code =PRED-886"); err != nil {
		t.Fatal(err)
	}
	if err := params.Set("page=2"); err != nil {
		t.Fatal(err)
	}
	got := params.String()
	if !strings.Contains(got, "code=PRED-886") || !strings.Contains(got, "page=2") {
		t.Fatalf("unexpected params string: %q", got)
	}

	if err := params.Set("broken"); err == nil {
		t.Fatal("expected bad param format error")
	}
	if err := params.Set(" =value"); err == nil {
		t.Fatal("expected blank param key error")
	}
	if empty := (paramsFlag{}).String(); empty != "" {
		t.Fatalf("empty params should render blank, got %q", empty)
	}
}

func TestMainVersionCommand(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"scraper", "version"}

	stdout, _, err := captureCommandOutput(t, func() error {
		main()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("expected version output")
	}
}

func TestMainDispatchesSuccessfulCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>Example</h1></body></html>`))
	}))
	defer server.Close()

	indexPath := writeCLIFile(t, "actors.json", `{"actors":[{"id":"a1","name":"Alice"}]}`)
	configPath := writeCLIConfig(t, strings.ReplaceAll(strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
indexes:
  actors:
    path: __INDEX__
    items_key: actors
    match_field: name
    value_field: id
tasks:
  detail:
    request:
      path: /detail
    extract:
      fields:
        title:
          xpath: //h1
          attr: text
    output:
      type: object
      format:
        title: "{title}"
`, "__BASE__", server.URL), "__INDEX__", indexPath))

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "run",
			args: []string{"scraper", "run", "-config", configPath, "-task", "detail", "-challenge", "off"},
			want: `"ok": true`,
		},
		{
			name: "validate",
			args: []string{"scraper", "validate", "-config", configPath},
			want: "ok: local",
		},
		{
			name: "tasks",
			args: []string{"scraper", "tasks", "-config", configPath},
			want: "detail",
		},
		{
			name: "sites",
			args: []string{"scraper", "sites"},
			want: "avbase",
		},
		{
			name: "index lookup",
			args: []string{"scraper", "index", "lookup", "-config", configPath, "-name", "Alice"},
			want: `"ok": true`,
		},
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			stdout, _, err := captureCommandOutput(t, func() error {
				main()
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("expected output containing %q, got %q", tt.want, stdout)
			}
		})
	}
}

func TestUsagePrintsCommandSummary(t *testing.T) {
	_, stderr, err := captureCommandOutput(t, func() error {
		usage()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"scraper run", "scraper index build", "scraper version"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("usage output should contain %q, got %q", want, stderr)
		}
	}
}

func TestRunCommandSmoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "PRED-886" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Cookie") != "runtime=1" {
			t.Fatalf("unexpected cookie: %q", r.Header.Get("Cookie"))
		}
		w.Write([]byte(`<html><body><article><a href="/works/PRED-886">PRED-886 Title</a></article></body></html>`))
	}))
	defer server.Close()

	configPath := writeCLIConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
defaults:
  cookie: "default=1"
tasks:
  search:
    params:
      code:
        required: true
    request:
      path: /works
      query:
        q: "{code}"
    extract:
      scope:
        xpath: //article
      fields:
        title:
          xpath: .//a
          attr: text
          trim: true
        url:
          xpath: .//a
          attr: href
          resolve_url: true
    output:
      format:
        title: "{title}"
        url: "{url}"
`, "__BASE__", server.URL))

	stdout, _, err := captureCommandOutput(t, func() error {
		return run([]string{
			"-config", configPath,
			"-task", "search",
			"-param", "code=PRED-886",
			"-cookie", "runtime=1",
			"-challenge", "off",
			"-timeout", "1s",
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("run should print JSON, got %q: %v", stdout, err)
	}
	if result["ok"] != true || result["site"] != "local" || result["task"] != "search" {
		t.Fatalf("unexpected run result: %#v", result)
	}
}

func TestRunCommandReportsFlagAndConfigErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing config",
			args: []string{"-task", "search"},
			want: "--config is required",
		},
		{
			name: "missing task",
			args: []string{"-config", "avbase"},
			want: "--task is required",
		},
		{
			name: "bad param",
			args: []string{"-config", "avbase", "-task", "search_work", "-param", "broken"},
			want: "key=value",
		},
		{
			name: "bad challenge",
			args: []string{"-config", "avbase", "-task", "search_work", "-challenge", "maybe"},
			want: "--challenge must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := captureCommandOutput(t, func() error {
				return run(tt.args)
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateTasksAndSitesCommands(t *testing.T) {
	configPath := writeCLIConfig(t, `
site:
  id: local
  base_url: https://example.test
tasks:
  detail:
    request:
      path: /detail
    extract:
      fields:
        title:
          xpath: //h1
    output:
      format:
        title: "{title}"
`)

	stdout, _, err := captureCommandOutput(t, func() error {
		return validate([]string{"-config", configPath})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "ok: local (1 tasks)") {
		t.Fatalf("unexpected validate output: %q", stdout)
	}

	stdout, _, err = captureCommandOutput(t, func() error {
		return tasks([]string{"-config", configPath})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != "detail" {
		t.Fatalf("unexpected tasks output: %q", stdout)
	}

	stdout, _, err = captureCommandOutput(t, func() error {
		return sites(nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "avbase") || !strings.Contains(stdout, "javbus") {
		t.Fatalf("unexpected sites output: %q", stdout)
	}
}

func TestValidateTasksAndSitesRequireConfig(t *testing.T) {
	for name, fn := range map[string]func([]string) error{
		"validate": validate,
		"tasks":    tasks,
	} {
		t.Run(name, func(t *testing.T) {
			err := fn(nil)
			if err == nil || !strings.Contains(err.Error(), "--config is required") {
				t.Fatalf("expected --config error, got %v", err)
			}
		})
	}
}

func TestIndexLookupCommandSmoke(t *testing.T) {
	indexPath := writeCLIFile(t, "actors.json", `{
  "actors": [
    {"id": "ra6", "name": "Alice", "url": "https://example.test/star/ra6"}
  ]
}`)
	configPath := writeCLIConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: https://example.test
indexes:
  actors:
    path: __INDEX__
    items_key: actors
    match_field: name
    value_field: id
tasks:
  detail:
    request:
      path: /detail
    extract:
      fields:
        title:
          xpath: //h1
    output:
      format:
        title: "{title}"
`, "__INDEX__", indexPath))

	stdout, _, err := captureCommandOutput(t, func() error {
		return indexLookup([]string{"-config", configPath, "-index", "actors", "-name", "Alice"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("index lookup should print JSON, got %q: %v", stdout, err)
	}
	if result["ok"] != true || result["count"].(float64) != 1 {
		t.Fatalf("unexpected lookup result: %#v", result)
	}

	stdout, _, err = captureCommandOutput(t, func() error {
		return indexCommand([]string{"lookup", "-config", configPath, "-name", "Alice"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"ok": true`) {
		t.Fatalf("unexpected index command output: %q", stdout)
	}
}

func TestIndexLookupReportsInputErrors(t *testing.T) {
	configPath := writeCLIConfig(t, `
site:
  id: local
  base_url: https://example.test
tasks:
  detail:
    request:
      path: /detail
    extract:
      fields:
        title:
          xpath: //h1
    output:
      format:
        title: "{title}"
`)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing config",
			args: []string{"-name", "Alice"},
			want: "--config is required",
		},
		{
			name: "missing name",
			args: []string{"-config", configPath},
			want: "--name is required",
		},
		{
			name: "unknown index",
			args: []string{"-config", configPath, "-name", "Alice"},
			want: `index "actors" not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := captureCommandOutput(t, func() error {
				return indexLookup(tt.args)
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestIndexBuildCommandSmoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			w.Write([]byte(`<html><body>
				<a class="actor" href="/star/a1"><span>Alice</span></a>
			</body></html>`))
		case "2":
			w.Write([]byte(`<html><body>
				<a class="actor" href="/star/a1"><span>Alice Duplicate</span></a>
				<a class="actor" href="/star/b2"><span>Bob</span></a>
			</body></html>`))
		default:
			w.Write([]byte(`<html><body></body></html>`))
		}
	}))
	defer server.Close()

	configPath := writeCLIConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_list:
    params:
      page:
        default: "1"
    request:
      path: /actors
      query:
        page: "{page}"
    extract:
      scope:
        xpath: //a[contains(@class, 'actor')]
      fields:
        id:
          xpath: .
          attr: href
          regex: "/star/([^/]+)"
          on_missing: skip_item
        name:
          xpath: .//span
          attr: text
          trim: true
        url:
          xpath: .
          attr: href
          resolve_url: true
    output:
      type: object
      items_key: actors
      format:
        id: "{id}"
        name: "{name}"
        url: "{url}"
`, "__BASE__", server.URL))
	outPath := filepath.Join(t.TempDir(), "nested", "actors.json")

	_, stderr, err := captureCommandOutput(t, func() error {
		return indexBuild([]string{
			"-config", configPath,
			"-task", "actor_list",
			"-out", outPath,
			"-max-pages", "3",
			"-concurrency", "2",
			"-retries", "0",
			"-challenge", "off",
			"-pretty",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "last page = 2") || !strings.Contains(stderr, "wrote 2 actors") {
		t.Fatalf("unexpected index build stderr: %q", stderr)
	}

	var doc actorIndexDocument
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Site != "local" || doc.TotalPages != 2 || doc.Count != 2 || len(doc.Actors) != 2 {
		t.Fatalf("unexpected index document: %+v", doc)
	}
	if doc.Actors[0]["id"] != "a1" || doc.Actors[1]["id"] != "b2" {
		t.Fatalf("expected sorted deduplicated actors, got %#v", doc.Actors)
	}
}

func TestIndexBuildReportsInputErrors(t *testing.T) {
	validConfigPath := writeCLIConfig(t, `
site:
  id: local
  base_url: https://example.test
tasks:
  actor_list:
    request:
      path: /actors
    extract:
      fields:
        name:
          xpath: //a
    output:
      type: object
      items_key: actors
      format:
        name: "{name}"
`)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing config",
			args: []string{"-out", "actors.json"},
			want: "--config is required",
		},
		{
			name: "missing out",
			args: []string{"-config", "avbase"},
			want: "--out is required",
		},
		{
			name: "max pages",
			args: []string{"-config", "avbase", "-out", "actors.json", "-max-pages", "0"},
			want: "--max-pages must be positive",
		},
		{
			name: "concurrency",
			args: []string{"-config", "avbase", "-out", "actors.json", "-concurrency", "0"},
			want: "--concurrency must be positive",
		},
		{
			name: "bad challenge",
			args: []string{"-config", validConfigPath, "-out", "actors.json", "-challenge", "bad"},
			want: "--challenge must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := captureCommandOutput(t, func() error {
				return indexBuild(tt.args)
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestIndexCommandReportsSubcommandErrors(t *testing.T) {
	for _, args := range [][]string{nil, []string{"unknown"}} {
		err := indexCommand(args)
		if err == nil {
			t.Fatalf("expected error for args %#v", args)
		}
	}
}

func TestFindLastIndexPageReportsEmptyFirstPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body></body></html>`))
	}))
	defer server.Close()

	cfg := mustParseCLIConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_list:
    request:
      path: /actors
    extract:
      scope:
        xpath: //a
      fields:
        name:
          xpath: .
          attr: text
    output:
      type: object
      items_key: actors
      format:
        name: "{name}"
`, "__BASE__", server.URL))

	_, err := findLastIndexPage(context.Background(), cfg, "actor_list", "page", 3, "actors", fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff}, 0)
	if err == nil || !strings.Contains(err.Error(), "first index page returned no data") {
		t.Fatalf("expected empty first page error, got %v", err)
	}
}

func TestRunIndexPageReportsResultItemErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>Example</h1></body></html>`))
	}))
	defer server.Close()

	cfg := mustParseCLIConfig(t, strings.ReplaceAll(`
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
      type: list
      format:
        title: "{title}"
`, "__BASE__", server.URL))

	_, ok, err := runIndexPage(context.Background(), cfg, "detail", "page", 1, "actors", fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff})
	if err == nil || ok {
		t.Fatalf("expected result item error, ok=%v err=%v", ok, err)
	}
}

func TestFetchIndexPagesRetriesFailedPage(t *testing.T) {
	page2Requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "2" {
			page2Requests++
			if page2Requests == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`<html><body>temporary failure</body></html>`))
				return
			}
		}
		w.Write([]byte(`<html><body>
			<a class="actor" href="/star/` + page + `"><span>Actor ` + page + `</span></a>
		</body></html>`))
	}))
	defer server.Close()

	cfg := mustParseCLIConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_list:
    params:
      page:
        default: "1"
    request:
      path: /actors
      query:
        page: "{page}"
    extract:
      scope:
        xpath: //a
      fields:
        id:
          xpath: .
          attr: href
          regex: "/star/([^/]+)"
          on_missing: skip_item
        name:
          xpath: .//span
          attr: text
          trim: true
    output:
      type: object
      items_key: actors
      format:
        id: "{id}"
        name: "{name}"
`, "__BASE__", server.URL))

	pages, err := fetchIndexPages(context.Background(), cfg, "actor_list", "page", 2, "actors", fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff}, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || len(pages[2]) != 1 || pages[2][0]["id"] != "2" {
		t.Fatalf("unexpected retried pages: %#v", pages)
	}
	if page2Requests != 2 {
		t.Fatalf("expected page 2 to be retried once, got %d requests", page2Requests)
	}
}

func TestRunIndexPageWithRetries(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`<html><body>temporary failure</body></html>`))
			return
		}
		w.Write([]byte(`<html><body><a><span>Alice</span></a></body></html>`))
	}))
	defer server.Close()

	cfg := mustParseCLIConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_list:
    request:
      path: /actors
    extract:
      scope:
        xpath: //a
      fields:
        name:
          xpath: .//span
          attr: text
          trim: true
    output:
      type: object
      items_key: actors
      format:
        name: "{name}"
`, "__BASE__", server.URL))

	items, ok, err := runIndexPageWithRetries(context.Background(), cfg, "actor_list", "page", 1, "actors", fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || len(items) != 1 || items[0]["name"] != "Alice" {
		t.Fatalf("unexpected retried result: items=%#v ok=%v", items, ok)
	}

	_, ok, err = runIndexPageWithRetries(context.Background(), cfg, "missing_task", "page", 1, "actors", fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff}, 0)
	if err == nil || ok {
		t.Fatalf("expected missing task error, ok=%v err=%v", ok, err)
	}

	emptyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<html><body>failure</body></html>`))
	}))
	defer emptyServer.Close()
	emptyCfg := mustParseCLIConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_list:
    request:
      path: /actors
    extract:
      fields:
        name:
          xpath: //h1
    output:
      type: object
      items_key: actors
      format:
        name: "{name}"
`, "__BASE__", emptyServer.URL))
	_, ok, err = runIndexPageWithRetries(context.Background(), emptyCfg, "actor_list", "page", 1, "actors", fetcher.RuntimeOptions{Timeout: time.Second, Challenge: fetcher.ChallengeOff}, 0)
	if err != nil || ok {
		t.Fatalf("expected non-ok result without error, ok=%v err=%v", ok, err)
	}
}

func TestResultItems(t *testing.T) {
	items := []map[string]interface{}{{"id": "a1"}}
	got, err := resultItems(map[string]interface{}{"actors": items}, "actors")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["id"] != "a1" {
		t.Fatalf("unexpected items: %#v", got)
	}

	tests := []struct {
		name string
		data interface{}
		want string
	}{
		{name: "not object", data: []map[string]interface{}{}, want: "expected object data"},
		{name: "missing key", data: map[string]interface{}{}, want: `does not contain key "actors"`},
		{name: "wrong type", data: map[string]interface{}{"actors": []interface{}{}}, want: "unexpected type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resultItems(tt.data, "actors")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestMergeIndexPagesSortsAndDeduplicates(t *testing.T) {
	got := mergeIndexPages(map[int][]map[string]interface{}{
		2: {
			{"id": "b2", "name": "Bob"},
			{"url": "https://example.test/no-id", "name": "No ID"},
		},
		1: {
			{"id": "a1", "name": "Alice"},
			{"id": "b2", "name": "Bob duplicate"},
			{"url": "https://example.test/no-id", "name": "No ID duplicate"},
			{"name": "Nameless"},
		},
	})
	if len(got) != 4 {
		t.Fatalf("expected 4 deduplicated entries, got %#v", got)
	}
	if got[0]["id"] != "a1" || got[1]["id"] != "b2" || got[2]["name"] != "No ID duplicate" || got[3]["name"] != "Nameless" {
		t.Fatalf("unexpected merged order: %#v", got)
	}
}

func TestEnvAndChallengeHelpers(t *testing.T) {
	t.Setenv("SCRAPER_TEST_VALUE", "from-env")
	if got := envDefault("SCRAPER_TEST_VALUE", "fallback"); got != "from-env" {
		t.Fatalf("unexpected env default result: %q", got)
	}
	if got := envDefault("SCRAPER_TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("unexpected fallback result: %q", got)
	}

	t.Setenv("SCRAPER_TEST_DURATION", "250ms")
	if got := envDuration("SCRAPER_TEST_DURATION", time.Second); got != 250*time.Millisecond {
		t.Fatalf("unexpected env duration: %s", got)
	}
	t.Setenv("SCRAPER_TEST_DURATION", "not-a-duration")
	if got := envDuration("SCRAPER_TEST_DURATION", time.Second); got != time.Second {
		t.Fatalf("invalid duration should use fallback, got %s", got)
	}
	t.Setenv("SCRAPER_TEST_DURATION", "")
	if got := envDuration("SCRAPER_TEST_DURATION", time.Second); got != time.Second {
		t.Fatalf("empty duration should use fallback, got %s", got)
	}

	mode, err := challengeMode("bypass")
	if err != nil {
		t.Fatal(err)
	}
	if mode != fetcher.ChallengeBypass {
		t.Fatalf("unexpected challenge mode: %s", mode)
	}
	if _, err := challengeMode("invalid"); err == nil {
		t.Fatal("expected invalid challenge mode error")
	}
}

func captureCommandOutput(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW
	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	runErr := fn()

	stdoutW.Close()
	stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	stdout, readOutErr := io.ReadAll(stdoutR)
	stderr, readErrErr := io.ReadAll(stderrR)
	if readOutErr != nil {
		t.Fatal(readOutErr)
	}
	if readErrErr != nil {
		t.Fatal(readErrErr)
	}
	return string(stdout), string(stderr), runErr
}

func writeCLIConfig(t *testing.T, yml string) string {
	t.Helper()
	return writeCLIFile(t, "site.yml", yml)
}

func writeCLIFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustParseCLIConfig(t *testing.T, yml string) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
