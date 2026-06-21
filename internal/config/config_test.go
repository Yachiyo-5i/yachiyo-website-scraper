package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"yachiyo-website-scraper/internal/config"
)

func TestLoadBuiltinSite(t *testing.T) {
	cfg, err := config.Load("avbase")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.ID != "avbase" {
		t.Fatalf("unexpected site id: %s", cfg.Site.ID)
	}
	if _, err := cfg.Task("search_work"); err != nil {
		t.Fatal(err)
	}

	cfg, err = config.Load("javlibrary")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.ID != "javlibrary" {
		t.Fatalf("unexpected site id: %s", cfg.Site.ID)
	}
	if _, err := cfg.Task("work_detail"); err != nil {
		t.Fatal(err)
	}

	cfg, err = config.Load("javbus")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.ID != "javbus" {
		t.Fatalf("unexpected site id: %s", cfg.Site.ID)
	}
	if _, err := cfg.Task("work_detail"); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinSites(t *testing.T) {
	sites, err := config.BuiltinSites()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(sites, "avbase") {
		t.Fatalf("expected avbase in builtin sites, got %#v", sites)
	}
	if !slices.Contains(sites, "javlibrary") {
		t.Fatalf("expected javlibrary in builtin sites, got %#v", sites)
	}
	if !slices.Contains(sites, "javbus") {
		t.Fatalf("expected javbus in builtin sites, got %#v", sites)
	}
}

func TestLoadFileStillWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "site.yml")
	if err := os.WriteFile(path, []byte(`
site:
  id: file_site
  base_url: http://example.test
tasks:
  search:
    request:
      path: /search
    extract:
      fields:
        title:
          xpath: "//title"
    output:
      format:
        title: "{title}"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.ID != "file_site" {
		t.Fatalf("unexpected site id: %s", cfg.Site.ID)
	}
}

func TestValidateParamRegex(t *testing.T) {
	_, err := config.Parse([]byte(`
site:
  id: bad_regex
  base_url: http://example.test
tasks:
  work_detail:
    params:
      code:
        required: true
        regex: "("
    request:
      path: /article/{code}
    extract:
      fields:
        title:
          xpath: "//title"
    output:
      format:
        title: "{title}"
`))
	if err == nil {
		t.Fatal("expected invalid param regex error")
	}
}
