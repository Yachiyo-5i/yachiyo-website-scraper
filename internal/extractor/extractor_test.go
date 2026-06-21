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
