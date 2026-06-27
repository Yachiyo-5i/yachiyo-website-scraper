package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func minimalConfig() Config {
	return Config{
		Site: SiteConfig{
			ID:      "local",
			BaseURL: "https://example.test",
		},
		Tasks: map[string]Task{
			"search": {
				Request: RequestConfig{Path: "/search"},
				Extract: ExtractConfig{
					Fields: map[string]FieldConfig{
						"title": {XPath: "//h1"},
					},
				},
				Output: OutputConfig{
					Format: map[string]interface{}{"title": "{title}"},
				},
			},
		},
	}
}

func TestLoadFileReadError(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "missing.yml"))
	if err == nil {
		t.Fatal("expected read error")
	}
}

func TestLoadUsesBuiltinOnlyForBareReferences(t *testing.T) {
	if !isBuiltinRef("avbase") {
		t.Fatal("bare site name should be treated as builtin reference")
	}
	if isBuiltinRef("configs/avbase.yml") {
		t.Fatal("paths should not be treated as builtin references")
	}
	if isBuiltinRef("avbase.yml") {
		t.Fatal("yml files should not be treated as builtin references")
	}
	if isBuiltinRef(" ") {
		t.Fatal("blank config path should not be treated as builtin reference")
	}
}

func TestReadBuiltinNormalizesNames(t *testing.T) {
	for _, name := range []string{" avbase.yml ", "javlibrary.yaml"} {
		t.Run(name, func(t *testing.T) {
			data, err := ReadBuiltin(name)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "site:") {
				t.Fatalf("unexpected builtin data for %q", name)
			}
		})
	}
}

func TestReadBuiltinRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", "../avbase", ".", "..", `..\avbase`} {
		t.Run(name, func(t *testing.T) {
			_, err := ReadBuiltin(name)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestBuiltinSitesAreSortedAndOnlyYAML(t *testing.T) {
	sites, err := BuiltinSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) < 4 {
		t.Fatalf("expected bundled sites, got %#v", sites)
	}
	for i := 1; i < len(sites); i++ {
		if sites[i-1] > sites[i] {
			t.Fatalf("sites are not sorted: %#v", sites)
		}
	}
	for _, site := range sites {
		if strings.HasSuffix(site, ".yml") {
			t.Fatalf("site name should not include extension: %q", site)
		}
	}
}

func TestParseRejectsInvalidYAML(t *testing.T) {
	_, err := Parse([]byte("site: ["))
	if err == nil {
		t.Fatal("expected YAML parse error")
	}
}

func TestValidateReportsConfigErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name: "site id",
			mutate: func(c *Config) {
				c.Site.ID = " "
			},
			want: "site.id is required",
		},
		{
			name: "base url",
			mutate: func(c *Config) {
				c.Site.BaseURL = ""
			},
			want: "site.base_url is required",
		},
		{
			name: "no tasks",
			mutate: func(c *Config) {
				c.Tasks = nil
			},
			want: "tasks must contain at least one task",
		},
		{
			name: "empty task name",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				c.Tasks = map[string]Task{"": task}
			},
			want: "task name cannot be empty",
		},
		{
			name: "request",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.Request = RequestConfig{}
				c.Tasks["search"] = task
			},
			want: `request.url or request.path is required`,
		},
		{
			name: "extract fields",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.Extract.Fields = nil
				c.Tasks["search"] = task
			},
			want: `extract.fields must contain at least one field`,
		},
		{
			name: "output format",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.Output.Format = nil
				c.Tasks["search"] = task
			},
			want: `output.format must contain at least one field`,
		},
		{
			name: "pagination param",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.Pagination = &PaginationConfig{}
				c.Tasks["search"] = task
			},
			want: `pagination.param is required`,
		},
		{
			name: "missing gfriends type",
			mutate: func(c *Config) {
				c.Tasks["search"] = Task{
					Params: map[string]ParamSpec{"name": {Required: true}},
					Gfriends: &GfriendsTaskConfig{
						NameParam: "name",
					},
				}
			},
			want: `gfriends.type is required`,
		},
		{
			name: "unsupported gfriends type",
			mutate: func(c *Config) {
				c.Tasks["search"] = Task{
					Params: map[string]ParamSpec{"name": {Required: true}},
					Gfriends: &GfriendsTaskConfig{
						Type: "actor_profile",
					},
				}
			},
			want: `unsupported gfriends.type "actor_profile"`,
		},
		{
			name: "resolve empty target",
			mutate: func(c *Config) {
				c.Indexes = map[string]Index{"actors": {Path: "actors.json"}}
				task := c.Tasks["search"]
				task.ResolveParams = map[string]ParamResolver{"": {Index: "actors", From: "name"}}
				c.Tasks["search"] = task
			},
			want: `resolve_params key cannot be empty`,
		},
		{
			name: "resolve missing index name",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.ResolveParams = map[string]ParamResolver{"id": {From: "name"}}
				c.Tasks["search"] = task
			},
			want: `resolve_params.id.index is required`,
		},
		{
			name: "resolve missing from",
			mutate: func(c *Config) {
				c.Indexes = map[string]Index{"actors": {Path: "actors.json"}}
				task := c.Tasks["search"]
				task.ResolveParams = map[string]ParamResolver{"id": {Index: "actors"}}
				c.Tasks["search"] = task
			},
			want: `resolve_params.id.from is required`,
		},
		{
			name: "resolve unknown index",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.ResolveParams = map[string]ParamResolver{"id": {Index: "actors", From: "name"}}
				c.Tasks["search"] = task
			},
			want: `references unknown index "actors"`,
		},
		{
			name: "invalid param regex",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.Params = map[string]ParamSpec{"code": {Regex: "("}}
				c.Tasks["search"] = task
			},
			want: `params.code.regex`,
		},
		{
			name: "negative regex group",
			mutate: func(c *Config) {
				task := c.Tasks["search"]
				task.Params = map[string]ParamSpec{"code": {Regex: ".*", RegexGroup: -1}}
				c.Tasks["search"] = task
			},
			want: `regex_group cannot be negative`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestTaskReportsMissingTask(t *testing.T) {
	cfg := minimalConfig()

	_, err := cfg.Task("missing")
	if err == nil {
		t.Fatal("expected missing task error")
	}
	if !strings.Contains(err.Error(), `task "missing" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHeadersForMergesDefaultsAndTaskHeaders(t *testing.T) {
	cfg := minimalConfig()
	cfg.Defaults.Headers = map[string]string{
		"User-Agent": "DefaultAgent",
		"X-Default":  "yes",
	}
	task := cfg.Tasks["search"]
	task.Headers = map[string]string{
		"User-Agent": "TaskAgent",
		"X-Task":     "yes",
	}

	got := cfg.HeadersFor(task)
	if got["User-Agent"] != "TaskAgent" || got["X-Default"] != "yes" || got["X-Task"] != "yes" {
		t.Fatalf("unexpected merged headers: %#v", got)
	}

	got["X-Default"] = "changed"
	if cfg.Defaults.Headers["X-Default"] != "yes" {
		t.Fatal("HeadersFor should return a copy")
	}
}

func TestLoadFileParsesValidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.yaml")
	if err := os.WriteFile(path, []byte(`
site:
  id: local
  base_url: https://example.test
tasks:
  search:
    request:
      url: https://example.test/search
    extract:
      fields:
        title:
          xpath: //h1
    output:
      format:
        title: "{title}"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site.ID != "local" {
		t.Fatalf("unexpected loaded config: %+v", cfg.Site)
	}
}
