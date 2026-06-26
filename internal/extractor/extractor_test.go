package extractor_test

import (
	"reflect"
	"testing"

	"yachiyo-website-scraper/internal/config"
	"yachiyo-website-scraper/internal/extractor"
)

func TestExtractMultipleRegexFindsAllMatchesInOneNode(t *testing.T) {
	page, err := extractor.ExtractPage(`
		<html><body>
			<article>
				<p>Links:
					magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
					magnet:?xt=urn:btih:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB
				</p>
			</article>
		</body></html>
	`, config.ExtractConfig{
		Fields: map[string]config.FieldConfig{
			"magnet_links": {
				XPath:     "//article",
				Attr:      "text",
				Regex:     `(magnet:\?xt=urn:btih:[A-F0-9]+)`,
				Multiple:  true,
				Trim:      true,
				OnMissing: "error",
			},
		},
	}, extractor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(page.Items))
	}

	got := page.Items[0]["magnet_links"]
	want := []string{
		"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"magnet:?xt=urn:btih:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected magnet links:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExtractPageReadsJSONFieldsAndDynamicEntityKeys(t *testing.T) {
	page, err := extractor.ExtractPage(`{
		"entities": {
			"Q97031495": {
				"id": "Q97031495",
				"labels": {"en": {"value": "Rikka Ono"}},
				"claims": {
					"P569": [
						{"mainsnak": {"datavalue": {"value": {"time": "+2002-01-29T00:00:00Z"}}}}
					],
					"P2002": [
						{"mainsnak": {"datavalue": {"value": "onorikka"}}}
					]
				}
			}
		}
	}`, config.ExtractConfig{
		Type: "json",
		Fields: map[string]config.FieldConfig{
			"wikidata_id": {
				Path:      "$.entities.*.id",
				OnMissing: "error",
			},
			"name": {
				Path: "$.entities.*.labels.en.value",
			},
			"birth_date": {
				Path:  "$.entities.*.claims.P569[0].mainsnak.datavalue.value.time",
				Regex: `^\+([0-9]{4}-[0-9]{2}-[0-9]{2})`,
			},
			"x_username": {
				Path: "$.entities.*.claims.P2002[0].mainsnak.datavalue.value",
			},
		},
	}, extractor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected one JSON item, got %d", len(page.Items))
	}

	got := page.Items[0]
	want := extractor.ExtractedItem{
		"wikidata_id": "Q97031495",
		"name":        "Rikka Ono",
		"birth_date":  "2002-01-29",
		"x_username":  "onorikka",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected JSON item:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExtractPageReadsJSONScopedItems(t *testing.T) {
	page, err := extractor.ExtractPage(`{
		"query": {
			"search": [
				{"title": "Rikka Ono", "pageid": 7407438},
				{"title": "Rikka Ono Works", "pageid": 99}
			]
		}
	}`, config.ExtractConfig{
		Type: "json",
		Scope: &config.ScopeConfig{
			Path: "$.query.search.*",
		},
		Fields: map[string]config.FieldConfig{
			"title": {
				Path:      "$.title",
				OnMissing: "skip_item",
			},
			"pageid": {
				Path: "$.pageid",
				Type: "int",
			},
		},
	}, extractor.Options{})
	if err != nil {
		t.Fatal(err)
	}

	want := []extractor.ExtractedItem{
		{"title": "Rikka Ono", "pageid": 7407438},
		{"title": "Rikka Ono Works", "pageid": 99},
	}
	if !reflect.DeepEqual(page.Items, want) {
		t.Fatalf("unexpected JSON scoped items:\nwant: %#v\n got: %#v", want, page.Items)
	}
}

func TestExtractRegexUnescapesHTMLAttributeMatches(t *testing.T) {
	page, err := extractor.ExtractPage(`
		<html><body>
			<article>
				<a href="magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&amp;dn=sample">Download</a>
			</article>
		</body></html>
	`, config.ExtractConfig{
		Fields: map[string]config.FieldConfig{
			"magnet_links": {
				XPath:     "//article",
				Attr:      "outer_html",
				Regex:     `(magnet:\?xt=urn:btih:[^"'<>[:space:]]+)`,
				Multiple:  true,
				Trim:      true,
				OnMissing: "error",
			},
		},
	}, extractor.Options{})
	if err != nil {
		t.Fatal(err)
	}

	got := page.Items[0]["magnet_links"]
	want := []string{"magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&dn=sample"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected magnet links:\nwant: %#v\n got: %#v", want, got)
	}
}
