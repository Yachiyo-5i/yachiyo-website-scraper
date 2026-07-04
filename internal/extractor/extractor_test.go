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

func TestExtractFieldFallbacksUseFirstAvailableValue(t *testing.T) {
	page, err := extractor.ExtractPage(`
		<html><body>
			<div class="item"><time><span>2022-01-27</span></time></div>
			<div class="item"><time><span title="2026-07-05 01:13">25&nbsp;分钟前</span></time></div>
		</body></html>
	`, config.ExtractConfig{
		Scope: &config.ScopeConfig{
			XPath: "//div[contains(@class, 'item')]",
		},
		Fields: map[string]config.FieldConfig{
			"date": {
				XPath: ".//time//span[@title]",
				Attr:  "title",
				Trim:  true,
				Fallbacks: []config.FieldConfig{
					{
						XPath: ".//time",
						Attr:  "text",
						Trim:  true,
					},
				},
			},
		},
	}, extractor.Options{})
	if err != nil {
		t.Fatal(err)
	}

	got := []interface{}{page.Items[0]["date"], page.Items[1]["date"]}
	want := []interface{}{"2022-01-27", "2026-07-05 01:13"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected fallback values:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestSehuatangForumThreadsReadsDateTextWithoutTitle(t *testing.T) {
	cfg, err := config.Load("sehuatang")
	if err != nil {
		t.Fatal(err)
	}
	task, err := cfg.Task("forum_threads")
	if err != nil {
		t.Fatal(err)
	}

	page, err := extractor.ExtractPage(`
		<html><body>
			<table>
				<tbody id="normalthread_754811">
					<tr>
						<th>
							<a class="s xst" href="thread-754811-1-1.html">Korean sample</a>
						</th>
						<td class="by">
							<cite><a href="space-uid-1.html">Hero168</a></cite>
							<em><span>2022-01-27</span></em>
						</td>
						<td class="num"><a>900</a><em>1527951</em></td>
						<td class="by">
							<cite><a href="space-uid-2.html">replyer</a></cite>
							<em><a href="forum.php?mod=redirect&amp;tid=754811&amp;goto=lastpost#lastpost">2026-06-23 17:51</a></em>
						</td>
					</tr>
				</tbody>
				<tbody id="normalthread_3604890">
					<tr>
						<th>
							<em>[<a href="forum.php?mod=forumdisplay&amp;fid=36&amp;filter=typeid&amp;typeid=672">无码破解</a>]</em>
							<a class="s xst" href="thread-3604890-1-1.html">Asia sample</a>
						</th>
						<td class="by">
							<cite><a href="space-uid-3.html">82h8oeddq</a></cite>
							<em><span><span title="2026-07-04">昨天&nbsp;08:37</span></span></em>
						</td>
						<td class="num"><a>101</a><em>28899</em></td>
						<td class="by">
							<cite><a href="space-uid-4.html">czsuperman</a></cite>
							<em><a href="forum.php?mod=redirect&amp;tid=3604890&amp;goto=lastpost#lastpost"><span title="2026-07-05 01:13">25&nbsp;分钟前</span></a></em>
						</td>
					</tr>
				</tbody>
			</table>
		</body></html>
	`, task.Extract, extractor.Options{BaseURL: "https://www.sehuatang.org"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected two items, got %d", len(page.Items))
	}

	gotFirstPostedAt := page.Items[0]["posted_at"]
	if gotFirstPostedAt != "2022-01-27" {
		t.Fatalf("unexpected first posted_at: %#v", gotFirstPostedAt)
	}
	gotFirstLastReplyAt := page.Items[0]["last_reply_at"]
	if gotFirstLastReplyAt != "2026-06-23 17:51" {
		t.Fatalf("unexpected first last_reply_at: %#v", gotFirstLastReplyAt)
	}
	gotSecondPostedAt := page.Items[1]["posted_at"]
	if gotSecondPostedAt != "2026-07-04" {
		t.Fatalf("unexpected second posted_at: %#v", gotSecondPostedAt)
	}
	gotSecondLastReplyAt := page.Items[1]["last_reply_at"]
	if gotSecondLastReplyAt != "2026-07-05 01:13" {
		t.Fatalf("unexpected second last_reply_at: %#v", gotSecondLastReplyAt)
	}
}
