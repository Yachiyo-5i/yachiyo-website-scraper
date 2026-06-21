package extractor_test

import (
	"reflect"
	"strings"
	"testing"

	"yachiyo-website-scraper/internal/config"
	"yachiyo-website-scraper/internal/extractor"
)

func TestExtractPageCoversMetaPageFieldsAndFieldVariants(t *testing.T) {
	page, err := extractor.ExtractPage(`
		<html><body>
			<section id="profile">
				<h1> Alice </h1>
				<img class="avatar" src="/actors/alice.jpg">
			</section>
			<span id="total">Total: 12 works</span>
			<article class="work">
				<a class="title" href="/works/ssis-001"> SSIS-001 Title (2026) </a>
				<div class="summary"><strong>Bold</strong> summary</div>
				<span class="genre">Drama</span>
				<span class="genre">Featured</span>
			</article>
		</body></html>
	`, config.ExtractConfig{
		Scope: &config.ScopeConfig{XPath: "//article[contains(@class, 'work')]"},
		Meta: map[string]config.FieldConfig{
			"total": {
				XPath:      "//span[@id='total']",
				Attr:       "text",
				Regex:      `Total:\s+(\d+)`,
				Type:       "int",
				Trim:       true,
				OnMissing:  "error",
				RegexGroup: 1,
			},
		},
		Page: map[string]config.FieldConfig{
			"actor": {
				XPath: "//section[@id='profile']//h1",
				Attr:  "text",
				Trim:  true,
			},
			"image": {
				XPath:      "//section[@id='profile']//img",
				Attr:       "src",
				ResolveURL: true,
			},
		},
		Fields: map[string]config.FieldConfig{
			"title": {
				XPath: ".//a[contains(@class, 'title')]",
				Attr:  "text",
				Trim:  true,
				Regex: `^(.*?)\s+\(`,
			},
			"url": {
				XPath:      ".//a[contains(@class, 'title')]",
				Attr:       "href",
				ResolveURL: true,
			},
			"year": {
				XPath: ".//a[contains(@class, 'title')]",
				Attr:  "text",
				Regex: `\((\d{4})\)`,
				Type:  "integer",
			},
			"summary_html": {
				XPath: ".//div[contains(@class, 'summary')]",
				Attr:  "html",
			},
			"outer": {
				XPath: ".//div[contains(@class, 'summary')]",
				Attr:  "outer_html",
			},
			"genres": {
				XPath:    ".//span[contains(@class, 'genre')]",
				Attr:     "text",
				Trim:     true,
				Multiple: true,
			},
			"fallback": {
				XPath:   ".//span[contains(@class, 'missing')]",
				Default: "unknown",
			},
			"missing_many": {
				XPath:    ".//span[contains(@class, 'none')]",
				Multiple: true,
			},
		},
	}, extractor.Options{BaseURL: "https://example.test/base/page"})
	if err != nil {
		t.Fatal(err)
	}

	if page.Meta["total"] != 12 {
		t.Fatalf("unexpected meta total: %#v", page.Meta["total"])
	}
	wantPage := extractor.ExtractedItem{
		"actor": "Alice",
		"image": "https://example.test/actors/alice.jpg",
	}
	if !reflect.DeepEqual(page.Page, wantPage) {
		t.Fatalf("unexpected page fields:\nwant: %#v\n got: %#v", wantPage, page.Page)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(page.Items))
	}

	item := page.Items[0]
	if item["title"] != "SSIS-001 Title" {
		t.Fatalf("unexpected title: %#v", item["title"])
	}
	if item["url"] != "https://example.test/works/ssis-001" {
		t.Fatalf("unexpected URL: %#v", item["url"])
	}
	if item["year"] != 2026 {
		t.Fatalf("unexpected year: %#v", item["year"])
	}
	if !strings.Contains(item["summary_html"].(string), "<strong>Bold</strong>") {
		t.Fatalf("unexpected inner HTML: %#v", item["summary_html"])
	}
	if !strings.Contains(item["outer"].(string), `<div class="summary">`) {
		t.Fatalf("unexpected outer HTML: %#v", item["outer"])
	}
	if !reflect.DeepEqual(item["genres"], []string{"Drama", "Featured"}) {
		t.Fatalf("unexpected genres: %#v", item["genres"])
	}
	if item["fallback"] != "unknown" {
		t.Fatalf("unexpected fallback: %#v", item["fallback"])
	}
	if !reflect.DeepEqual(item["missing_many"], []string{}) {
		t.Fatalf("unexpected missing multiple value: %#v", item["missing_many"])
	}
}

func TestExtractDelegatesToExtractPageItems(t *testing.T) {
	items, err := extractor.Extract(`<html><body><h1>Hello</h1></body></html>`, config.ExtractConfig{
		Fields: map[string]config.FieldConfig{
			"title": {XPath: "//h1", Attr: "text"},
		},
	}, extractor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0]["title"] != "Hello" {
		t.Fatalf("unexpected extract result: %#v", items)
	}
}

func TestExtractSkipsItemsAndPageFieldsWhenConfigured(t *testing.T) {
	page, err := extractor.ExtractPage(`
		<html><body>
			<section id="profile"></section>
			<ul>
				<li><a>First</a></li>
				<li><span>No title</span></li>
			</ul>
		</body></html>
	`, config.ExtractConfig{
		Scope: &config.ScopeConfig{XPath: "//li"},
		Page: map[string]config.FieldConfig{
			"actor": {
				XPath:     "//section[@id='profile']//h1",
				OnMissing: "skip_item",
			},
		},
		Fields: map[string]config.FieldConfig{
			"title": {
				XPath:     ".//a",
				Attr:      "text",
				OnMissing: "skip_item",
			},
			"optional": {
				XPath: ".//span[contains(@class, 'optional')]",
			},
		},
	}, extractor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Page) != 0 {
		t.Fatalf("page fields should be empty after skip_item, got %#v", page.Page)
	}
	if len(page.Items) != 1 || page.Items[0]["title"] != "First" {
		t.Fatalf("unexpected skipped items result: %#v", page.Items)
	}
	if page.Items[0]["optional"] != nil {
		t.Fatalf("missing optional field should render nil, got %#v", page.Items[0]["optional"])
	}
}

func TestExtractReportsMissingRequiredMetaAndFields(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ExtractConfig
		want string
	}{
		{
			name: "required meta",
			cfg: config.ExtractConfig{
				Meta: map[string]config.FieldConfig{
					"total": {XPath: "//missing", Required: true},
				},
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//h1"},
				},
			},
			want: `meta "total" is missing`,
		},
		{
			name: "required field",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//missing", Required: true},
				},
			},
			want: `field "title" is missing`,
		},
		{
			name: "explicit error on missing field",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//missing", OnMissing: "error"},
				},
			},
			want: `field "title" is missing`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractor.ExtractPage(`<html><body><h1>Hello</h1></body></html>`, tt.cfg, extractor.Options{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestExtractReportsExtractionErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ExtractConfig
		opts extractor.Options
		want string
	}{
		{
			name: "invalid scope xpath",
			cfg: config.ExtractConfig{
				Scope: &config.ScopeConfig{XPath: "//*["},
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//h1"},
				},
			},
			want: "scope xpath",
		},
		{
			name: "invalid field xpath",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//*["},
				},
			},
			want: `field "title"`,
		},
		{
			name: "invalid regex",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//h1", Regex: "("},
				},
			},
			want: `field "title"`,
		},
		{
			name: "regex group out of range",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//h1", Regex: `(Hello)`, RegexGroup: 2},
				},
			},
			want: "regex_group 2 out of range",
		},
		{
			name: "int conversion",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"year": {XPath: "//h1", Type: "int"},
				},
			},
			want: `convert "Hello" to int`,
		},
		{
			name: "unsupported type",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"title": {XPath: "//h1", Type: "float"},
				},
			},
			want: `unsupported field type "float"`,
		},
		{
			name: "invalid base URL",
			cfg: config.ExtractConfig{
				Fields: map[string]config.FieldConfig{
					"url": {XPath: "//a", Attr: "href", ResolveURL: true},
				},
			},
			opts: extractor.Options{BaseURL: "http://%zz"},
			want: "invalid URL escape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractor.ExtractPage(`<html><body><h1>Hello</h1><a href="/x">x</a></body></html>`, tt.cfg, tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestFormatOutputRendersNestedValues(t *testing.T) {
	got, err := extractor.FormatOutput(map[string]interface{}{
		"title": "{title}",
		"actor": map[string]interface{}{
			"name": "{actor}",
		},
		"tags": []interface{}{"{tag}", "static"},
	}, extractor.ExtractedItem{
		"title": "Example",
		"actor": "Alice",
		"tag":   "Drama",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{
		"title": "Example",
		"actor": map[string]interface{}{"name": "Alice"},
		"tags":  []interface{}{"Drama", "static"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected formatted output:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestFormatAnyReportsMissingOutputVariable(t *testing.T) {
	_, err := extractor.FormatAny(map[string]interface{}{
		"title": "{missing}",
	}, extractor.ExtractedItem{})
	if err == nil {
		t.Fatal("expected missing output variable error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatAnyCanRenderNonObjectValues(t *testing.T) {
	got, err := extractor.FormatAny("{title}", extractor.ExtractedItem{"title": "Example"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Example" {
		t.Fatalf("unexpected formatted value: %#v", got)
	}
}
