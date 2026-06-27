package runner_test

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
	"yachiyo-website-scraper/internal/runner"
)

func TestRunExtractsSearchResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "PRED-886" {
			t.Fatalf("unexpected query q: %s", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<div class="work">
					<a class="title" href="/works/PRED-886">PRED-886 Example Title (2026)</a>
					<span class="date">Release: 2026-01-02</span>
					<img class="cover" src="/covers/pred-886.jpg">
				</div>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
defaults:
  headers:
    User-Agent: TestAgent
tasks:
  search_work:
    params:
      code:
        required: true
    request:
      method: GET
      path: /works
      query:
        q: "{code}"
    extract:
      scope:
        xpath: "//div[contains(@class, 'work')]"
      fields:
        title:
          xpath: ".//a[contains(@class, 'title')]"
          attr: text
          trim: true
          regex: "^(.*?)\\s*\\("
        url:
          xpath: ".//a[contains(@class, 'title')]"
          attr: href
          resolve_url: true
        release_date:
          xpath: ".//span[contains(@class, 'date')]"
          attr: text
          regex: "\\d{4}-\\d{2}-\\d{2}"
        cover:
          xpath: ".//img[contains(@class, 'cover')]"
          attr: src
          resolve_url: true
    output:
      type: list
      format:
        title: "{title}"
        url: "{url}"
        release_date: "{release_date}"
        cover: "{cover}"
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "search_work",
		Params:   map[string]string{"code": "PRED-886"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}

	data, ok := res.Data.([]map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	if len(data) != 1 {
		t.Fatalf("expected one result, got %d", len(data))
	}
	got := data[0]
	if got["title"] != "PRED-886 Example Title" {
		t.Fatalf("unexpected title: %#v", got["title"])
	}
	if got["url"] != server.URL+"/works/PRED-886" {
		t.Fatalf("unexpected url: %#v", got["url"])
	}
	if got["cover"] != server.URL+"/covers/pred-886.jpg" {
		t.Fatalf("unexpected cover: %#v", got["cover"])
	}
}

func TestRunUsesDefaultCookieWhenRuntimeCookieIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "age=verified; dv=1; existmag=all" {
			t.Fatalf("unexpected cookie: %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><div class="item"><a href="/ok">OK</a></div></body></html>`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
defaults:
  cookie: "age=verified; dv=1; existmag=all"
tasks:
  search:
    request:
      method: GET
      path: /search
    extract:
      scope:
        xpath: "//div[contains(@class, 'item')]"
      fields:
        title:
          xpath: ".//a"
          attr: text
          trim: true
          on_missing: skip_item
    output:
      type: list
      format:
        title: "{title}"
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "search",
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
}

func TestRunRuntimeCookieOverridesDefaultCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "external=1" {
			t.Fatalf("unexpected cookie: %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><div class="item"><a href="/ok">OK</a></div></body></html>`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
defaults:
  cookie: "age=verified; dv=1; existmag=all"
tasks:
  search:
    request:
      method: GET
      path: /search
    extract:
      scope:
        xpath: "//div[contains(@class, 'item')]"
      fields:
        title:
          xpath: ".//a"
          attr: text
          trim: true
          on_missing: skip_item
    output:
      type: list
      format:
        title: "{title}"
`, "__BASE__", server.URL))

	runtime := fetcher.DefaultRuntimeOptions()
	runtime.Cookie = "external=1"
	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "search",
		Runtime:  runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
}

func TestRunResolvesParamFromIndex(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "actors.json")
	if err := os.WriteFile(indexPath, []byte(`{
  "actors": [
    {
      "id": "ra6",
      "name": "仲村みう",
      "url": "https://www.javbus.com/star/ra6"
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/star/ra6" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<div class="actor"><h1>仲村みう</h1></div>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(strings.ReplaceAll(`
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
  actor_detail:
    params:
      id:
        required: true
      name: {}
    resolve_params:
      id:
        index: actors
        from: name
    request:
      method: GET
      path: /star/{id}
    extract:
      scope:
        xpath: "//div[contains(@class, 'actor')]"
      fields:
        name:
          xpath: ".//h1"
          attr: text
          trim: true
          on_missing: skip_item
    output:
      type: object
      format:
        name: "{name}"
`, "__BASE__", server.URL), "__INDEX__", indexPath))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_detail",
		Params:   map[string]string{"name": "仲村みう"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	if data["name"] != "仲村みう" {
		t.Fatalf("unexpected name: %#v", data["name"])
	}
}

func TestRunAllowsOptionalIndexParamWithoutValue(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "categories.json")
	if err := os.WriteFile(indexPath, []byte(`{
  "categories": [
    {
      "name": "高清中文字幕",
      "fid": "103"
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forum.php" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("fid"); got != "103" {
			t.Fatalf("unexpected fid: %q", got)
		}
		if r.URL.Query().Has("filter") || r.URL.Query().Has("typeid") {
			t.Fatalf("optional empty params should be omitted, got query %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><div class="thread"><a>OK</a></div></body></html>`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
indexes:
  categories:
    path: __INDEX__
    items_key: categories
    match_field: name
tasks:
  forum_threads:
    params:
      category:
        required: true
    resolve_params:
      fid:
        index: categories
        from: category
        value_field: fid
      filter:
        index: categories
        from: category
        value_field: filter
        optional: true
      typeid:
        index: categories
        from: category
        value_field: typeid
        optional: true
    request:
      path: /forum.php
      omit_empty_query: true
      query:
        mod: forumdisplay
        fid: "{fid}"
        filter: "{filter}"
        typeid: "{typeid}"
    extract:
      scope:
        xpath: "//div[contains(@class, 'thread')]"
      fields:
        title:
          xpath: ".//a"
          attr: text
          trim: true
          on_missing: skip_item
    output:
      type: list
      format:
        title: "{title}"
`, "__BASE__", server.URL), "__INDEX__", indexPath))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "forum_threads",
		Params:   map[string]string{"category": "高清中文字幕"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
}

func TestRunBlocksCloudflareChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<html><head><title>Just a moment...</title></head><body>Enable JavaScript and cookies to continue</body></html>"))
	}))
	defer server.Close()

	cfg := basicConfig(t, server.URL)
	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "search_work",
		Params:   map[string]string{"code": "PRED-886"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected blocked result")
	}
	if res.Error == nil || res.Error.Type != "blocked" {
		t.Fatalf("expected blocked error, got: %+v", res.Error)
	}
	if len(res.Error.Matched) == 0 {
		t.Fatal("expected challenge matches")
	}
}

func TestRunBlocksAgeVerificationPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><head><title>SEHUATANG.ORG</title></head><body>
				<script>var safeid='abc';</script>
				<a class="enter-btn" href="./">满18岁，请点此进入</a>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  forum_threads:
    request:
      path: /forum.php
    extract:
      fields:
        title:
          xpath: //a
          attr: text
    output:
      format:
        title: "{title}"
`, "__BASE__", server.URL))
	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "forum_threads",
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected blocked result")
	}
	if res.Error == nil || res.Error.Type != "blocked" || res.Error.Reason != "age_verification_required" {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
}

func TestRunBypassesWithFlareSolverr(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<title>Just a moment...</title>"))
	}))
	defer target.Close()

	fs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Fatalf("unexpected flaresolverr path: %s", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["url"] != target.URL+"/works?q=PRED-886" {
			t.Fatalf("unexpected payload url: %#v", payload["url"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "ok",
			"solution": {
				"url": "` + target.URL + `/works?q=PRED-886",
				"status": 200,
				"response": "<html><body><div class=\"work\"><a class=\"title\" href=\"/works/PRED-886\">Solved Title</a></div></body></html>"
			}
		}`))
	}))
	defer fs.Close()

	cfg := basicConfig(t, target.URL)
	opts := fetcher.DefaultRuntimeOptions()
	opts.Challenge = fetcher.ChallengeBypass
	opts.FlareSolverrURL = fs.URL
	opts.FlareSolverrWait = 5 * time.Second

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "search_work",
		Params:   map[string]string{"code": "PRED-886"},
		Runtime:  opts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok after bypass, got: %+v", res.Error)
	}
	if res.Channel != fetcher.ChannelFlareSolver {
		t.Fatalf("expected flaresolverr channel, got %s", res.Channel)
	}
	data := res.Data.([]map[string]interface{})
	if data[0]["title"] != "Solved Title" {
		t.Fatalf("unexpected title: %#v", data[0]["title"])
	}
}

func TestRunPassesDefaultAutoclickToPlaywright(t *testing.T) {
	targetURL := "https://target.example.test/forum.php"
	playwright := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			URL       string `json:"url"`
			Autoclick *struct {
				XPath string `json:"xpath"`
			} `json:"autoclick"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.URL != targetURL {
			t.Fatalf("unexpected url: %q", payload.URL)
		}
		if payload.Autoclick == nil || payload.Autoclick.XPath != "//a[contains(@class, 'enter-btn')]" {
			t.Fatalf("unexpected autoclick payload: %#v", payload.Autoclick)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    http.StatusOK,
			"final_url": targetURL,
			"body":      `<html><body><div class="thread"><a class="title">Clicked Page</a></div></body></html>`,
		})
	}))
	defer playwright.Close()

	cfg := loadInlineConfig(t, `
site:
  id: local
  base_url: https://target.example.test
defaults:
  autoclick:
    xpath: "//a[contains(@class, 'enter-btn')]"
tasks:
  forum_threads:
    request:
      path: /forum.php
    extract:
      scope:
        xpath: "//div[contains(@class, 'thread')]"
      fields:
        title:
          xpath: ".//a[contains(@class, 'title')]"
          attr: text
          trim: true
          on_missing: skip_item
    output:
      type: list
      format:
        title: "{title}"
`)
	runtime := fetcher.DefaultRuntimeOptions()
	runtime.PlaywrightURL = playwright.URL
	runtime.PlaywrightWait = time.Second

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "forum_threads",
		Runtime:  runtime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
}

func TestRunExtractsMetaAndRequestedPageOnly(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/works" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "SSIS" {
			t.Fatalf("unexpected query q: %s", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("unexpected page: %s", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"total":42}}}</script>
				<div class="work"><a class="title" href="/works/SSIS-003">Page 2 Title A</a></div>
				<div class="work"><a class="title" href="/works/SSIS-004">Page 2 Title B</a></div>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  search_work:
    params:
      code:
        required: true
      page:
        default: "1"
    request:
      method: GET
      path: /works
      query:
        q: "{code}"
        page: "{page}"
    extract:
      meta:
        total:
          xpath: "//script[@id='__NEXT_DATA__']"
          attr: text
          regex: "\"total\":(\\d+)"
          type: int
      scope:
        xpath: "//div[contains(@class, 'work')]"
      fields:
        title:
          xpath: ".//a[contains(@class, 'title')]"
          attr: text
          trim: true
          on_missing: skip_item
        url:
          xpath: ".//a[contains(@class, 'title')]"
          attr: href
          resolve_url: true
          on_missing: skip_item
    pagination:
      param: page
      default: "1"
      total_field: total
    output:
      type: list
      format:
        title: "{title}"
        url: "{url}"
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "search_work",
		Params:   map[string]string{"code": "SSIS", "page": "2"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	if requests != 1 {
		t.Fatalf("expected exactly one request, got %d", requests)
	}
	if res.Meta["total"] != 42 {
		t.Fatalf("unexpected total: %#v", res.Meta["total"])
	}
	if res.Meta["page"] != 2 {
		t.Fatalf("unexpected page: %#v", res.Meta["page"])
	}
	if res.Meta["count"] != 2 {
		t.Fatalf("unexpected count: %#v", res.Meta["count"])
	}
	data := res.Data.([]map[string]interface{})
	if len(data) != 2 {
		t.Fatalf("expected page data only, got %d", len(data))
	}
}

func TestRunAcceptsConfiguredHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/searchstar/Mita" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`
			<html><body>
				<h4>No results</h4>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_search:
    params:
      keyword:
        required: true
    request:
      method: GET
      path: /searchstar/{keyword}
      accept_status: [404]
    extract:
      scope:
        xpath: "//a[contains(@class, 'avatar-box')]"
      fields:
        id:
          xpath: "."
          attr: href
          regex: "/star/([^/]+)"
          on_missing: skip_item
        name:
          xpath: ".//img"
          attr: title
          trim: true
    output:
      type: object
      items_key: actors
      format:
        id: "{id}"
        name: "{name}"
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_search",
		Params:   map[string]string{"keyword": "Mita"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	if res.Status != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", res.Status)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	actors, ok := data["actors"].([]map[string]interface{})
	if !ok {
		t.Fatalf("unexpected actors type: %T", data["actors"])
	}
	if len(actors) != 0 {
		t.Fatalf("expected empty actors, got %#v", actors)
	}
}

func TestRunEnhancesActorSearchImageFromGfriends(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/searchstar/Alice" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<a class="avatar-box" href="/star/a1">
					<img title="Alice" src="/actors/site-alice.jpg">
				</a>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_search:
    params:
      keyword:
        required: true
    request:
      method: GET
      path: /searchstar/{keyword}
    extract:
      scope:
        xpath: "//a[contains(@class, 'avatar-box')]"
      fields:
        id:
          xpath: "."
          attr: href
          regex: "/star/([^/]+)"
          on_missing: skip_item
        name:
          xpath: ".//img"
          attr: title
          trim: true
          on_missing: skip_item
        url:
          xpath: "."
          attr: href
          resolve_url: true
        image:
          xpath: ".//img"
          attr: src
          resolve_url: true
    output:
      type: object
      items_key: actors
      format:
        id: "{id}"
        name: "{name}"
        url: "{url}"
        image: "{image}"
    enhance:
      actor_image:
        source: gfriends
        items_key: actors
        name_field: name
        image_field: image
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_search",
		Params:   map[string]string{"keyword": "Alice"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
		Gfriends: runner.StaticActorImageLookup{"Alice": "https://cdn.example.test/Content/Alice.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	actors, ok := data["actors"].([]map[string]interface{})
	if !ok {
		t.Fatalf("unexpected actors type: %T", data["actors"])
	}
	if len(actors) != 1 {
		t.Fatalf("expected one actor, got %d", len(actors))
	}
	if actors[0]["image"] != "https://cdn.example.test/Content/Alice.jpg" {
		t.Fatalf("expected gfriends image to override site image, got %#v", actors[0]["image"])
	}
}

func TestRunKeepsActorSearchImageWhenGfriendsMisses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/searchstar/Bob" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<a class="avatar-box" href="/star/b1">
					<img title="Bob" src="/actors/site-bob.jpg">
				</a>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_search:
    params:
      keyword:
        required: true
    request:
      method: GET
      path: /searchstar/{keyword}
    extract:
      scope:
        xpath: "//a[contains(@class, 'avatar-box')]"
      fields:
        id:
          xpath: "."
          attr: href
          regex: "/star/([^/]+)"
          on_missing: skip_item
        name:
          xpath: ".//img"
          attr: title
          trim: true
          on_missing: skip_item
        image:
          xpath: ".//img"
          attr: src
          resolve_url: true
    output:
      type: object
      items_key: actors
      format:
        id: "{id}"
        name: "{name}"
        image: "{image}"
    enhance:
      actor_image:
        source: gfriends
        items_key: actors
        name_field: name
        image_field: image
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_search",
		Params:   map[string]string{"keyword": "Bob"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
		Gfriends: runner.StaticActorImageLookup{"Alice": "https://cdn.example.test/Content/Alice.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	data := res.Data.(map[string]interface{})
	actors := data["actors"].([]map[string]interface{})
	want := server.URL + "/actors/site-bob.jpg"
	if actors[0]["image"] != want {
		t.Fatalf("expected site image fallback %q, got %#v", want, actors[0]["image"])
	}
}

func TestRunGfriendsActorImageTask(t *testing.T) {
	cfg := loadInlineConfig(t, `
site:
  id: gfriends
  base_url: https://gfriends.example.test
tasks:
  actor_image:
    params:
      name:
        required: true
    gfriends:
      type: actor_image
      name_param: name
`)

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_image",
		Params:   map[string]string{"name": " Alice "},
		Runtime:  fetcher.DefaultRuntimeOptions(),
		Gfriends: runner.StaticActorImageLookup{"Alice": "https://cdn.example.test/Content/Alice.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	if res.URL != "" {
		t.Fatalf("gfriends task should not fetch a URL, got %q", res.URL)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	actor, ok := data["actor"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected actor type: %T", data["actor"])
	}
	if actor["name"] != "Alice" || actor["image"] != "https://cdn.example.test/Content/Alice.jpg" {
		t.Fatalf("unexpected actor image result: %#v", actor)
	}
}

func TestRunGfriendsActorImageTaskReportsMiss(t *testing.T) {
	cfg := loadInlineConfig(t, `
site:
  id: gfriends
  base_url: https://gfriends.example.test
tasks:
  actor_image:
    params:
      name:
        required: true
    gfriends:
      type: actor_image
      name_param: name
`)

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_image",
		Params:   map[string]string{"name": "Bob"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
		Gfriends: runner.StaticActorImageLookup{"Alice": "https://cdn.example.test/Content/Alice.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatalf("expected not found result, got %+v", res)
	}
	if res.Error == nil || res.Error.Type != "not_found" || res.Error.Reason != `gfriends actor image not found for "Bob"` {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
}

func TestRunEnhancesActorSearchFromWikipediaConfig(t *testing.T) {
	wikiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Api-User-Agent"); got == "" {
			t.Fatal("expected Api-User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/summary/Rikka Ono":
			w.Write([]byte(`{
				"title": "Rikka Ono",
				"pageid": 7407438,
				"lang": "zh",
				"wikibase_item": "Q97031495",
				"description": "Japanese AV actress",
				"extract": "Rikka Ono is a Japanese AV actress.",
				"thumbnail": {"source": "https://upload.example.test/rikka.jpg"},
				"content_urls": {"desktop": {"page": "https://zh.wikipedia.org/wiki/Rikka_Ono"}},
				"revision": "93067322",
				"timestamp": "2026-06-14T16:28:45Z"
			}`))
		case "/entity":
			if got := r.URL.Query().Get("title"); got != "Rikka Ono" {
				t.Fatalf("unexpected entity title: %q", got)
			}
			w.Write([]byte(`{
				"entities": {
					"Q97031495": {
						"id": "Q97031495",
						"labels": {
							"zh": {"value": "Rikka Ono"},
							"ja": {"value": "Rikka Ono"},
							"en": {"value": "Rikka Ono"}
						},
						"claims": {
							"P569": [{"mainsnak": {"datavalue": {"value": {"time": "+2002-01-29T00:00:00Z"}}}}],
							"P27": [{"mainsnak": {"datavalue": {"value": {"id": "Q17"}}}}],
							"P106": [{"mainsnak": {"datavalue": {"value": {"id": "Q1079215"}}}}],
							"P2002": [{"mainsnak": {"datavalue": {"value": "onorikka"}}}],
							"P2003": [{"mainsnak": {"datavalue": {"value": "ono_rikka"}}}],
							"P373": [{"mainsnak": {"datavalue": {"value": "Rikka Ono"}}}],
							"P18": [{"mainsnak": {"datavalue": {"value": "Rikka.jpg"}}}]
						}
					}
				}
			}`))
		case "/content":
			if got := r.URL.Query().Get("title"); got != "Rikka Ono" {
				t.Fatalf("unexpected content title: %q", got)
			}
			w.Write([]byte(`{
				"parse": {
					"title": "Rikka Ono",
					"pageid": 7407438,
					"wikitext": "{{AV女優\\n| 名前 = 小野 六花\\n| ふりがな = おの りっか\\n| 愛称 = りっかたん\\n| 生年 = 2002\\n| 生月 = 2\\n| 生日 = 14\\n| 出身地 = [[滋賀縣]]\\n| 血液型 = {{fact|o}}\\n| 毛髪の色 = black\\n| 身長 = 148\\n| 体重 = 56\\n| バスト = 81\\n| ウエスト = 58\\n| ヒップ = 82\\n| カップ = C[2]\\n| ジャンル = [[成人影片]]\\n| AV出演期間 = [[2020年]] -\\n| 専属契約 = [[MOODYZ]]\\n}}\\n'''小野六花'''（{{jpn|j='''小野六花'''|hg=おの りっか|rm=Ono Rikka}}；{{bd|2002年|2月14日}}），[[日本]][[AV女優]]。所属于[[Allpro]]旗下，身高148cm。\\n==人物==\\n她说出道前从未看过AV，第一次接触AV女星是通过[[社交網路服務|SNS]]认识的[[明日花绮罗]]。\\n== 外部链接 ==\\n* {{Twitter|onorikka}}\\n* {{Instagram|ono_rikka}}\\n",
					"externallinks": ["https://twitter.com/onorikka", "https://www.instagram.com/ono_rikka/"]
				}
			}`))
		default:
			t.Fatalf("unexpected wiki path: %s", r.URL.String())
		}
	}))
	defer wikiServer.Close()

	dir := t.TempDir()
	wikiConfigPath := filepath.Join(dir, "wikipedia.yml")
	if err := os.WriteFile(wikiConfigPath, []byte(strings.ReplaceAll(`
site:
  id: wikipedia
  base_url: __WIKI__
defaults:
  headers:
    User-Agent: yachiyo-website-scraper/1.0 (https://github.com/Yachiyo-5i/yachiyo-website-scraper)
    Api-User-Agent: yachiyo-website-scraper/1.0 (https://github.com/Yachiyo-5i/yachiyo-website-scraper)
    Accept: application/json
tasks:
  page_summary:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /summary/{title}
    extract:
      type: json
      fields:
        title:
          path: "$.title"
          on_missing: error
        pageid:
          path: "$.pageid"
          type: int
        lang:
          path: "$.lang"
        wikidata_id:
          path: "$.wikibase_item"
        description:
          path: "$.description"
        summary:
          path: "$.extract"
        thumbnail:
          path: "$.thumbnail.source"
        page_url:
          path: "$.content_urls.desktop.page"
        revision:
          path: "$.revision"
        timestamp:
          path: "$.timestamp"
    output:
      type: object
      format:
        title: "{title}"
        pageid: "{pageid}"
        lang: "{lang}"
        wikidata_id: "{wikidata_id}"
        description: "{description}"
        summary: "{summary}"
        thumbnail: "{thumbnail}"
        page_url: "{page_url}"
        revision: "{revision}"
        timestamp: "{timestamp}"
  entity_by_title:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /entity
      query:
        title: "{title}"
        lang: "{lang}"
    extract:
      type: json
      fields:
        wikidata_id:
          path: "$.entities.*.id"
        birth_date:
          path: "$.entities.*.claims.P569[0].mainsnak.datavalue.value.time"
          regex: "^\\+([0-9]{4}-[0-9]{2}-[0-9]{2})"
        country_qid:
          path: "$.entities.*.claims.P27[0].mainsnak.datavalue.value.id"
        occupation_qid:
          path: "$.entities.*.claims.P106[0].mainsnak.datavalue.value.id"
        x_username:
          path: "$.entities.*.claims.P2002[0].mainsnak.datavalue.value"
        instagram_username:
          path: "$.entities.*.claims.P2003[0].mainsnak.datavalue.value"
        commons_category:
          path: "$.entities.*.claims.P373[0].mainsnak.datavalue.value"
        wikidata_image:
          path: "$.entities.*.claims.P18[0].mainsnak.datavalue.value"
        zh_title:
          path: "$.entities.*.labels.zh.value"
        ja_title:
          path: "$.entities.*.labels.ja.value"
        en_title:
          path: "$.entities.*.labels.en.value"
    output:
      type: object
      format:
        wikidata_id: "{wikidata_id}"
        birth_date: "{birth_date}"
        country_qid: "{country_qid}"
        occupation_qid: "{occupation_qid}"
        x_username: "{x_username}"
        instagram_username: "{instagram_username}"
        commons_category: "{commons_category}"
        wikidata_image: "{wikidata_image}"
        zh_title: "{zh_title}"
        ja_title: "{ja_title}"
        en_title: "{en_title}"
  page_content:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /content
      query:
        title: "{title}"
        lang: "{lang}"
    extract:
      type: json
      fields:
        title:
          path: "$.parse.title"
        pageid:
          path: "$.parse.pageid"
          type: int
        wikitext:
          path: "$.parse.wikitext"
        external_links:
          path: "$.parse.externallinks.*"
          multiple: true
    output:
      type: object
      format:
        title: "{title}"
        pageid: "{pageid}"
        wikitext: "{wikitext}"
        external_links: "{external_links}"
`, "__WIKI__", wikiServer.URL)), 0o644); err != nil {
		t.Fatal(err)
	}

	actorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/searchstar/Rikka" {
			t.Fatalf("unexpected actor path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<a class="avatar-box" href="/star/r1">
					<img title="Rikka Ono" src="/actors/rikka.jpg">
				</a>
			</body></html>
		`))
	}))
	defer actorServer.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_search:
    params:
      keyword:
        required: true
    request:
      method: GET
      path: /searchstar/{keyword}
    extract:
      scope:
        xpath: "//a[contains(@class, 'avatar-box')]"
      fields:
        name:
          xpath: ".//img"
          attr: title
          trim: true
          on_missing: skip_item
        image:
          xpath: ".//img"
          attr: src
          resolve_url: true
    output:
      type: object
      items_key: actors
      format:
        name: "{name}"
        image: "{image}"
    enhance:
      wikipedia:
        config: __WIKI_CONFIG__
        lang: zh
        title_field: name
        target_field: wikipedia
`, "__BASE__", actorServer.URL), "__WIKI_CONFIG__", wikiConfigPath))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_search",
		Params:   map[string]string{"keyword": "Rikka"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	data := res.Data.(map[string]interface{})
	actors := data["actors"].([]map[string]interface{})
	wiki, ok := actors[0]["wikipedia"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected wikipedia object, got %#v", actors[0]["wikipedia"])
	}
	if wiki["matched"] != true || wiki["wikidata_id"] != "Q97031495" {
		t.Fatalf("unexpected wikipedia match: %#v", wiki)
	}
	if wiki["summary"] != "Rikka Ono is a Japanese AV actress." {
		t.Fatalf("unexpected summary: %#v", wiki["summary"])
	}
	profile := wiki["profile"].(map[string]interface{})
	birthDate := profile["birth_date"].(map[string]interface{})
	if birthDate["value"] != "2002-01-29" || birthDate["property"] != "P569" {
		t.Fatalf("unexpected birth date: %#v", birthDate)
	}
	social := wiki["social"].(map[string]interface{})
	if social["x"] != "onorikka" || social["instagram"] != "ono_rikka" {
		t.Fatalf("unexpected social data: %#v", social)
	}
	text := wiki["text"].(map[string]interface{})
	if _, ok := text["full_text"]; ok {
		t.Fatalf("scraper should not assemble display-only full_text: %#v", text["full_text"])
	}
	if !strings.Contains(text["intro"].(string), "Ono Rikka") {
		t.Fatalf("expected cleaned intro, got %#v", text["intro"])
	}
	if !strings.Contains(text["person"].(string), "SNS") {
		t.Fatalf("expected person section, got %#v", text["person"])
	}
	textProfile := text["profile"].(map[string]interface{})
	if textProfile["nickname"] != "りっかたん" || textProfile["measurements"] != "81 - 58 - 82 cm" {
		t.Fatalf("unexpected text profile: %#v", textProfile)
	}
	if textProfile["name"] != "小野 六花" {
		t.Fatalf("unexpected text profile name: %#v", textProfile["name"])
	}
	externalLinks := text["external_links"].([]map[string]interface{})
	if len(externalLinks) != 2 || externalLinks[0]["url"] != "https://twitter.com/onorikka" {
		t.Fatalf("unexpected external links: %#v", externalLinks)
	}
}

func TestRunWikipediaStructuredTask(t *testing.T) {
	wikiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/summary/皆月光":
			w.Write([]byte(`{
				"title": "皆月光",
				"pageid": 9468033,
				"lang": "zh",
				"wikibase_item": "Q59551818",
				"description": "日本AV女優",
				"extract": "皆月光，日本AV女优。隶属于Bambi Promotion。",
				"thumbnail": {"source": "https://upload.example.test/minazuki.jpg"},
				"content_urls": {"desktop": {"page": "https://zh.wikipedia.org/wiki/%E7%9A%86%E6%9C%88%E5%85%89"}},
				"revision": "92602613",
				"timestamp": "2026-05-10T04:44:23Z"
			}`))
		case "/entity":
			w.Write([]byte(`{
				"entities": {
					"Q59551818": {
						"id": "Q59551818",
						"labels": {
							"zh": {"value": "皆月光"},
							"ja": {"value": "皆月ひかる"},
							"en": {"value": "Hikaru Minazuki"}
						},
						"claims": {
							"P569": [{"mainsnak": {"datavalue": {"value": {"time": "+2000-01-11T00:00:00Z"}}}}],
							"P27": [{"mainsnak": {"datavalue": {"value": {"id": "Q17"}}}}],
							"P106": [{"mainsnak": {"datavalue": {"value": {"id": "Q1079215"}}}}],
							"P2002": [{"mainsnak": {"datavalue": {"value": "hikaru_emo"}}}],
							"P2003": [{"mainsnak": {"datavalue": {"value": "hikaru_emot"}}}]
						}
					}
				}
			}`))
		case "/content":
			w.Write([]byte(`{
				"parse": {
					"title": "皆月光",
					"pageid": 9468033,
					"wikitext": "{{AV女優\\n| 原名 = 皆月 ひかる\\n| 暱稱 = ぴかぴか<br />ひかちゅう\\n| 別名 = ひかる<br />高橋 未来\\n| 生年 = 2000\\n| 生月 = 1\\n| 生日 = 11\\n| 出身地 = [[秋田県]]\\n| 從事年期 = [[2018年]] -\\n| 専属契約 = [[ディープス]]（2018年）\\n| 血型 = \\n| 身高 = 148\\n| 體重 = \\n| バスト = 83\\n| 腰圍 = 55\\n| 下圍 = 85\\n| カップ = B\\n}}\\n'''皆月光'''（{{lang-ja|皆月 ひかる}}，[[2000年]][[1月11日]]—），[[日本]][[AV女优]]。隶属于[[Bambi Promotion]]。\\n== 简历 ==\\n2018年出道。\\n== 人物 ==\\n特长为钢琴。\\n== 外部链接 ==\\n* {{Twitter|hikaru_emo}}\\n",
					"externallinks": ["https://twitter.com/hikaru_emo"]
				}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer wikiServer.Close()

	dir := t.TempDir()
	wikiConfigPath := filepath.Join(dir, "wikipedia.yml")
	if err := os.WriteFile(wikiConfigPath, []byte(strings.ReplaceAll(`
site:
  id: wikipedia
  base_url: __WIKI__
defaults:
  headers:
    User-Agent: yachiyo-website-scraper/1.0
    Api-User-Agent: yachiyo-website-scraper/1.0
    Accept: application/json
tasks:
  page_summary:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /summary/{title}
    extract:
      type: json
      fields:
        title:
          path: "$.title"
        pageid:
          path: "$.pageid"
          type: int
        lang:
          path: "$.lang"
        wikidata_id:
          path: "$.wikibase_item"
        description:
          path: "$.description"
        summary:
          path: "$.extract"
        thumbnail:
          path: "$.thumbnail.source"
        page_url:
          path: "$.content_urls.desktop.page"
        revision:
          path: "$.revision"
        timestamp:
          path: "$.timestamp"
    output:
      type: object
      format:
        title: "{title}"
        pageid: "{pageid}"
        lang: "{lang}"
        wikidata_id: "{wikidata_id}"
        description: "{description}"
        summary: "{summary}"
        thumbnail: "{thumbnail}"
        page_url: "{page_url}"
        revision: "{revision}"
        timestamp: "{timestamp}"
  entity_by_title:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /entity
      query:
        title: "{title}"
        lang: "{lang}"
    extract:
      type: json
      fields:
        wikidata_id:
          path: "$.entities.*.id"
        birth_date:
          path: "$.entities.*.claims.P569[0].mainsnak.datavalue.value.time"
          regex: "^\\+([0-9]{4}-[0-9]{2}-[0-9]{2})"
        country_qid:
          path: "$.entities.*.claims.P27[0].mainsnak.datavalue.value.id"
        occupation_qid:
          path: "$.entities.*.claims.P106[0].mainsnak.datavalue.value.id"
        x_username:
          path: "$.entities.*.claims.P2002[0].mainsnak.datavalue.value"
        instagram_username:
          path: "$.entities.*.claims.P2003[0].mainsnak.datavalue.value"
    output:
      type: object
      format:
        wikidata_id: "{wikidata_id}"
        birth_date: "{birth_date}"
        country_qid: "{country_qid}"
        occupation_qid: "{occupation_qid}"
        x_username: "{x_username}"
        instagram_username: "{instagram_username}"
  page_content:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /content
      query:
        title: "{title}"
        lang: "{lang}"
    extract:
      type: json
      fields:
        title:
          path: "$.parse.title"
        pageid:
          path: "$.parse.pageid"
          type: int
        wikitext:
          path: "$.parse.wikitext"
        external_links:
          path: "$.parse.externallinks.*"
          multiple: true
    output:
      type: object
      format:
        title: "{title}"
        pageid: "{pageid}"
        wikitext: "{wikitext}"
        external_links: "{external_links}"
  page_profile:
    params:
      title:
        required: true
      lang:
        default: zh
    wikipedia:
      config: wikipedia
      lang: zh
      title_field: title
      target_field: wikipedia
`, "__WIKI__", wikiServer.URL)), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: http://example.test
tasks:
  page_profile:
    params:
      title:
        required: true
      lang:
        default: zh
    wikipedia:
      config: __WIKI_CONFIG__
      lang: zh
`, "__WIKI_CONFIG__", wikiConfigPath))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "page_profile",
		Params:   map[string]string{"title": "皆月光", "lang": "zh"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	if data["title"] != "皆月光" {
		t.Fatalf("unexpected title: %#v", data["title"])
	}
	if data["wikidata_id"] != "Q59551818" {
		t.Fatalf("unexpected wikidata id: %#v", data["wikidata_id"])
	}
	text := data["text"].(map[string]interface{})
	if text["resume"] == "" || text["person"] == "" {
		t.Fatalf("expected structured text, got %#v", text)
	}
	if profile := text["profile"].(map[string]interface{}); profile["name"] != "皆月 ひかる" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestRunWikipediaStructuredTaskFallsBackToSearch(t *testing.T) {
	wikiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/summary/小野坂ゆいか":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{}`))
		case "/search":
			if got := r.URL.Query().Get("keyword"); got != "小野坂ゆいか" {
				t.Fatalf("unexpected search keyword: %q", got)
			}
			w.Write([]byte(`[
				{"title": "小野坂唯花", "pageid": 12345, "snippet": "candidate", "timestamp": "2026-06-01T00:00:00Z"}
			]`))
		case "/summary/小野坂唯花":
			w.Write([]byte(`{
				"title": "小野坂唯花",
				"pageid": 12345,
				"lang": "zh",
				"wikibase_item": "Q12345678",
				"description": "日本AV女優",
				"extract": "小野坂唯花，日本AV女优。",
				"thumbnail": {"source": "https://upload.example.test/yuika.jpg"},
				"content_urls": {"desktop": {"page": "https://zh.wikipedia.org/wiki/%E5%B0%8F%E9%87%8E%E5%9D%82%E5%94%AF%E8%8A%B1"}},
				"revision": "1",
				"timestamp": "2026-06-01T00:00:00Z"
			}`))
		case "/entity":
			if got := r.URL.Query().Get("title"); got != "小野坂唯花" {
				t.Fatalf("unexpected entity title: %q", got)
			}
			w.Write([]byte(`{
				"entities": {
					"Q12345678": {
						"id": "Q12345678",
						"labels": {
							"zh": {"value": "小野坂唯花"}
						},
						"claims": {
							"P106": [{"mainsnak": {"datavalue": {"value": {"id": "Q1079215"}}}}]
						}
					}
				}
			}`))
		case "/content":
			if got := r.URL.Query().Get("title"); got != "小野坂唯花" {
				t.Fatalf("unexpected content title: %q", got)
			}
			w.Write([]byte(`{
				"parse": {
					"title": "小野坂唯花",
					"pageid": 12345,
					"wikitext": "{{AV女優\\n| 名前 = 小野坂 唯花\\n| 愛称 = ゆいか\\n}}\\n'''小野坂唯花'''，[[日本]][[AV女优]]。\\n==简历==\\n新人介绍。\\n==人物==\\n人物介绍。\\n",
					"externallinks": []
				}
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer wikiServer.Close()

	dir := t.TempDir()
	wikiConfigPath := filepath.Join(dir, "wikipedia.yml")
	if err := os.WriteFile(wikiConfigPath, []byte(strings.ReplaceAll(`
site:
  id: wikipedia
  base_url: __WIKI__
defaults:
  headers:
    User-Agent: yachiyo-website-scraper/1.0
    Api-User-Agent: yachiyo-website-scraper/1.0
    Accept: application/json
tasks:
  page_search:
    params:
      keyword:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /search
      query:
        keyword: "{keyword}"
        lang: "{lang}"
    extract:
      type: json
      scope:
        path: "$.*"
      fields:
        title:
          path: "$.title"
        pageid:
          path: "$.pageid"
          type: int
        snippet:
          path: "$.snippet"
        timestamp:
          path: "$.timestamp"
    output:
      type: list
      format:
        title: "{title}"
        pageid: "{pageid}"
        snippet: "{snippet}"
        timestamp: "{timestamp}"
  page_summary:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /summary/{title}
      accept_status: [404]
    extract:
      type: json
      fields:
        title:
          path: "$.title"
        pageid:
          path: "$.pageid"
          type: int
        lang:
          path: "$.lang"
        wikidata_id:
          path: "$.wikibase_item"
        description:
          path: "$.description"
        summary:
          path: "$.extract"
        thumbnail:
          path: "$.thumbnail.source"
        page_url:
          path: "$.content_urls.desktop.page"
        revision:
          path: "$.revision"
        timestamp:
          path: "$.timestamp"
    output:
      type: object
      format:
        title: "{title}"
        pageid: "{pageid}"
        lang: "{lang}"
        wikidata_id: "{wikidata_id}"
        description: "{description}"
        summary: "{summary}"
        thumbnail: "{thumbnail}"
        page_url: "{page_url}"
        revision: "{revision}"
        timestamp: "{timestamp}"
  entity_by_title:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /entity
      query:
        title: "{title}"
        lang: "{lang}"
    extract:
      type: json
      fields:
        wikidata_id:
          path: "$.entities.*.id"
        occupation_qid:
          path: "$.entities.*.claims.P106[0].mainsnak.datavalue.value.id"
    output:
      type: object
      format:
        wikidata_id: "{wikidata_id}"
        occupation_qid: "{occupation_qid}"
  page_content:
    params:
      title:
        required: true
      lang:
        default: zh
    request:
      method: GET
      path: /content
      query:
        title: "{title}"
        lang: "{lang}"
    extract:
      type: json
      fields:
        title:
          path: "$.parse.title"
        pageid:
          path: "$.parse.pageid"
          type: int
        wikitext:
          path: "$.parse.wikitext"
        external_links:
          path: "$.parse.externallinks.*"
          multiple: true
    output:
      type: object
      format:
        title: "{title}"
        pageid: "{pageid}"
        wikitext: "{wikitext}"
        external_links: "{external_links}"
  page_profile:
    params:
      title:
        required: true
      lang:
        default: zh
    wikipedia:
      config: wikipedia
      lang: zh
`, `__WIKI__`, wikiServer.URL)), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: http://example.test
tasks:
  page_profile:
    params:
      title:
        required: true
      lang:
        default: zh
    wikipedia:
      config: __WIKI_CONFIG__
      lang: zh
`, "__WIKI_CONFIG__", wikiConfigPath))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "page_profile",
		Params:   map[string]string{"title": "小野坂ゆいか", "lang": "zh"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	data := res.Data.(map[string]interface{})
	if data["title"] != "小野坂唯花" {
		t.Fatalf("unexpected title: %#v", data["title"])
	}
	if res.URL != "https://zh.wikipedia.org/wiki/%E5%B0%8F%E9%87%8E%E5%9D%82%E5%94%AF%E8%8A%B1" {
		t.Fatalf("unexpected url: %#v", res.URL)
	}
}

func TestRunWrapsItemsKeyWithoutPageFormatAndMultipleFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<div class="work">
					<a class="title" href="/works/SSIS-001">SSIS-001 Example</a>
					<a class="actor" href="/talents/Actor-A"><span>Actor A</span></a>
					<a class="actor" href="/talents/Actor-B"><span>Actor B</span></a>
				</div>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  search_work:
    params:
      code:
        required: true
    request:
      method: GET
      path: /works
      query:
        q: "{code}"
    extract:
      scope:
        xpath: "//div[contains(@class, 'work')]"
      fields:
        title:
          xpath: ".//a[contains(@class, 'title')]"
          attr: text
          trim: true
          on_missing: skip_item
        url:
          xpath: ".//a[contains(@class, 'title')]"
          attr: href
          resolve_url: true
        actors:
          xpath: ".//a[contains(@class, 'actor')]/span"
          attr: text
          trim: true
          multiple: true
    output:
      type: object
      items_key: works
      format:
        title: "{title}"
        url: "{url}"
        actors: "{actors}"
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "search_work",
		Params:   map[string]string{"code": "SSIS-001"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}

	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	works, ok := data["works"].([]map[string]interface{})
	if !ok {
		t.Fatalf("unexpected works type: %T", data["works"])
	}
	if len(works) != 1 {
		t.Fatalf("expected one work, got %d", len(works))
	}
	actors, ok := works[0]["actors"].([]string)
	if !ok {
		t.Fatalf("unexpected actors type: %T", works[0]["actors"])
	}
	if len(actors) != 2 || actors[0] != "Actor A" || actors[1] != "Actor B" {
		t.Fatalf("unexpected actors: %#v", actors)
	}
}

func TestRunExtractsPageObjectAndItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/talents/Mita" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("unexpected page: %s", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"total":12,"talent":{"id":64873},"total_followers":43}}}</script>
				<section class="actor">
					<h1>Mita Marin</h1>
					<p class="ruby">mita marin</p>
					<img class="avatar" src="/actors/mita.jpg">
				</section>
				<div class="work"><a class="title" href="/works/AAA-001">First Work</a></div>
				<div class="work"><a class="title" href="/works/AAA-002">Second Work</a></div>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_detail:
    params:
      name:
        required: true
      page:
        default: "1"
    request:
      method: GET
      path: /talents/{name}
      query:
        page: "{page}"
    extract:
      meta:
        total:
          xpath: "//script[@id='__NEXT_DATA__']"
          attr: text
          regex: "\"total\":(\\d+)"
          type: int
      page:
        id:
          xpath: "//script[@id='__NEXT_DATA__']"
          attr: text
          regex: "\"talent\":\\{\"id\":(\\d+)"
          type: int
        name:
          xpath: "//section[contains(@class, 'actor')]/h1"
          attr: text
          trim: true
        ruby:
          xpath: "//section[contains(@class, 'actor')]/p[contains(@class, 'ruby')]"
          attr: text
          trim: true
        image:
          xpath: "//section[contains(@class, 'actor')]//img[contains(@class, 'avatar')]"
          attr: src
          resolve_url: true
        followers:
          xpath: "//script[@id='__NEXT_DATA__']"
          attr: text
          regex: "\"total_followers\":(\\d+)"
          type: int
      scope:
        xpath: "//div[contains(@class, 'work')]"
      fields:
        title:
          xpath: ".//a[contains(@class, 'title')]"
          attr: text
          trim: true
          on_missing: skip_item
        url:
          xpath: ".//a[contains(@class, 'title')]"
          attr: href
          resolve_url: true
          on_missing: skip_item
    pagination:
      param: page
      default: "1"
      total_field: total
    output:
      type: object
      items_key: works
      page_format:
        actor:
          id: "{id}"
          name: "{name}"
          ruby: "{ruby}"
          image: "{image}"
          followers: "{followers}"
      format:
        title: "{title}"
        url: "{url}"
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_detail",
		Params:   map[string]string{"name": "Mita"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	if res.Meta["total"] != 12 || res.Meta["page"] != 1 || res.Meta["count"] != 2 {
		t.Fatalf("unexpected meta: %#v", res.Meta)
	}

	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	actor, ok := data["actor"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected actor type: %T", data["actor"])
	}
	if actor["id"] != 64873 || actor["followers"] != 43 {
		t.Fatalf("expected numeric actor fields, got %#v", actor)
	}
	if actor["image"] != server.URL+"/actors/mita.jpg" {
		t.Fatalf("unexpected actor image: %#v", actor["image"])
	}
	works, ok := data["works"].([]map[string]interface{})
	if !ok {
		t.Fatalf("unexpected works type: %T", data["works"])
	}
	if len(works) != 2 {
		t.Fatalf("expected two works, got %d", len(works))
	}
	if works[1]["url"] != server.URL+"/works/AAA-002" {
		t.Fatalf("unexpected second work url: %#v", works[1]["url"])
	}
}

func TestRunEnhancesActorDetailImageFromGfriends(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/talents/Alice" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
			<html><body>
				<section class="actor">
					<h1>Alice</h1>
					<img class="avatar" src="/actors/site-alice.jpg">
				</section>
				<div class="work"><a class="title" href="/works/AAA-001">First Work</a></div>
			</body></html>
		`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  actor_detail:
    params:
      name:
        required: true
    request:
      method: GET
      path: /talents/{name}
    extract:
      page:
        name:
          xpath: "//section[contains(@class, 'actor')]/h1"
          attr: text
          trim: true
        image:
          xpath: "//section[contains(@class, 'actor')]//img[contains(@class, 'avatar')]"
          attr: src
          resolve_url: true
      scope:
        xpath: "//div[contains(@class, 'work')]"
      fields:
        title:
          xpath: ".//a[contains(@class, 'title')]"
          attr: text
          trim: true
          on_missing: skip_item
    output:
      type: object
      items_key: works
      page_format:
        actor:
          name: "{name}"
          image: "{image}"
      format:
        title: "{title}"
    enhance:
      actor_image:
        source: gfriends
        items_key: actor
        name_field: name
        image_field: image
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "actor_detail",
		Params:   map[string]string{"name": "Alice"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
		Gfriends: runner.StaticActorImageLookup{"Alice": "https://cdn.example.test/Content/Alice.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
	data, ok := res.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data type: %T", res.Data)
	}
	actor, ok := data["actor"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected actor type: %T", data["actor"])
	}
	if actor["image"] != "https://cdn.example.test/Content/Alice.jpg" {
		t.Fatalf("expected gfriends image to override site image, got %#v", actor["image"])
	}
}

func TestRunNormalizesParamWithRegex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/article/4913917/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><body><h1>FC2 Detail</h1></body></html>`))
	}))
	defer server.Close()

	cfg := loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  work_detail:
    params:
      code:
        required: true
        regex: "(?i)^(?:fc2-ppv-|fc2-|ppv-)?([0-9]+)$"
    request:
      method: GET
      path: /article/{code}/
    extract:
      fields:
        title:
          xpath: "//h1"
          attr: text
          trim: true
          on_missing: skip_item
    output:
      type: object
      format:
        title: "{title}"
`, "__BASE__", server.URL))

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "work_detail",
		Params:   map[string]string{"code": "FC2-PPV-4913917"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("expected ok result, got error: %+v", res.Error)
	}
}

func TestRunReturnsParamRegexMismatch(t *testing.T) {
	cfg := loadInlineConfig(t, `
site:
  id: local
  base_url: http://example.test
tasks:
  work_detail:
    params:
      code:
        required: true
        regex: "^([0-9]+)$"
    request:
      method: GET
      path: /article/{code}/
    extract:
      fields:
        title:
          xpath: "//h1"
    output:
      type: object
      format:
        title: "{title}"
`)

	_, err := runner.Run(context.Background(), cfg, runner.Options{
		TaskName: "work_detail",
		Params:   map[string]string{"code": "FC2-PPV-4913917"},
		Runtime:  fetcher.DefaultRuntimeOptions(),
	})
	if err == nil || !strings.Contains(err.Error(), "does not match regex") {
		t.Fatalf("expected regex mismatch, got %v", err)
	}
}

func basicConfig(t *testing.T, baseURL string) *config.Config {
	t.Helper()
	return loadInlineConfig(t, strings.ReplaceAll(`
site:
  id: local
  base_url: __BASE__
tasks:
  search_work:
    params:
      code:
        required: true
    request:
      method: GET
      path: /works
      query:
        q: "{code}"
    extract:
      scope:
        xpath: "//div[contains(@class, 'work')]"
      fields:
        title:
          xpath: ".//a[contains(@class, 'title')]"
          attr: text
          trim: true
          on_missing: skip_item
        url:
          xpath: ".//a[contains(@class, 'title')]"
          attr: href
          resolve_url: true
          on_missing: skip_item
    output:
      type: list
      format:
        title: "{title}"
        url: "{url}"
`, "__BASE__", baseURL))
}

func loadInlineConfig(t *testing.T, yml string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "site.yml")
	if err := os.WriteFile(path, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
