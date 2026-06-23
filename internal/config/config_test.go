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
	task, err := cfg.Task("actor_detail")
	if err != nil {
		t.Fatal(err)
	}
	assertActorImageEnhancement(t, task, "actor")

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
	task, err = cfg.Task("actor_detail")
	if err != nil {
		t.Fatal(err)
	}
	assertActorImageEnhancement(t, task, "actor")
	task, err = cfg.Task("actor_search")
	if err != nil {
		t.Fatal(err)
	}
	assertActorImageEnhancement(t, task, "actors")

	cfg, err = config.Load("sehuatang")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.ID != "sehuatang" {
		t.Fatalf("unexpected site id: %s", cfg.Site.ID)
	}
	if _, err := cfg.Task("forum_threads"); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Task("thread_detail"); err != nil {
		t.Fatal(err)
	}
}

func assertActorImageEnhancement(t *testing.T, task config.Task, itemsKey string) {
	t.Helper()
	if task.Enhance.ActorImage == nil {
		t.Fatal("expected actor image enhancement")
	}
	if task.Enhance.ActorImage.Source != "gfriends" {
		t.Fatalf("unexpected actor image enhancement source: %q", task.Enhance.ActorImage.Source)
	}
	if task.Enhance.ActorImage.ItemsKey != itemsKey {
		t.Fatalf("unexpected actor image enhancement items key: %q", task.Enhance.ActorImage.ItemsKey)
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
	if !slices.Contains(sites, "sehuatang") {
		t.Fatalf("expected sehuatang in builtin sites, got %#v", sites)
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

func TestParseAutoclickConfig(t *testing.T) {
	cfg, err := config.Parse([]byte(`
site:
  id: autoclick_site
  base_url: http://example.test
defaults:
  autoclick:
    xpath: "//a[contains(@class, 'enter-btn')]"
tasks:
  search:
    request:
      path: /search
      autoclick:
        xpath: "//button[contains(@class, 'confirm')]"
    extract:
      fields:
        title:
          xpath: "//title"
    output:
      format:
        title: "{title}"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Autoclick == nil || cfg.Defaults.Autoclick.XPath != "//a[contains(@class, 'enter-btn')]" {
		t.Fatalf("unexpected default autoclick: %#v", cfg.Defaults.Autoclick)
	}
	task, err := cfg.Task("search")
	if err != nil {
		t.Fatal(err)
	}
	if task.Request.Autoclick == nil || task.Request.Autoclick.XPath != "//button[contains(@class, 'confirm')]" {
		t.Fatalf("unexpected request autoclick: %#v", task.Request.Autoclick)
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

func TestValidateRejectsUnsupportedEnhancementSource(t *testing.T) {
	_, err := config.Parse([]byte(`
site:
  id: unsupported_enhancement
  base_url: http://example.test
tasks:
  actor_search:
    request:
      path: /search
    extract:
      fields:
        name:
          xpath: "//title"
    output:
      format:
        name: "{name}"
    enhance:
      actor_image:
        source: unknown
`))
	if err == nil {
		t.Fatal("expected unsupported enhancement source error")
	}
}
