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
