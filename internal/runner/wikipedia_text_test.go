package runner

import "testing"

func TestParseWikipediaTextContentHandlesCompactAVInfobox(t *testing.T) {
	got := parseWikipediaTextContent(map[string]interface{}{
		"wikitext": `{{Expand language}}
{{noteTA|}}
{{AV女優
|原名=小野坂 ゆいか
|假名=おのさか ゆいか
|愛稱=ぴかぴか
|別名=おのゆい
|生年=2002
|生月=2
|生日=9
|血型=A型
|檢查日期=2024年
|身高=164
|體重=48
|バスト=97
|腰圍=72
|下圍=106
|カップ=H
|ジャンル=AV女優
|AV出演期間=2024年－至今（日本）
|専属契約=IdeaPocket
}}
{{Infobox person
| japanese = 小野坂 ゆいか
}}
小野坂唯花（ja，2002年2月9日），是日本的AV女優。
== 简历 ==
2024年出道。
==人物==
興趣包括美食巡禮、旅行、Cosplay。
== 外部链接 ==
* {{Twitter|yuika_onosaka}}
* {{Instagram|yuika_onosaka}}
`,
	})

	profile, ok := got["profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected profile, got %#v", got["profile"])
	}
	if profile["name"] != "小野坂 ゆいか" {
		t.Fatalf("unexpected name: %#v", profile["name"])
	}
	if profile["nickname"] != "ぴかぴか" {
		t.Fatalf("unexpected nickname: %#v", profile["nickname"])
	}
	if profile["alias"] != "おのゆい" {
		t.Fatalf("unexpected alias: %#v", profile["alias"])
	}
	if profile["birth_date"] != "2002-02-09" {
		t.Fatalf("unexpected birth date: %#v", profile["birth_date"])
	}
	if profile["height_cm"] != "164" {
		t.Fatalf("unexpected height: %#v", profile["height_cm"])
	}
	if profile["weight_kg"] != "48" {
		t.Fatalf("unexpected weight: %#v", profile["weight_kg"])
	}
	if profile["bust_cm"] != "97" {
		t.Fatalf("unexpected bust: %#v", profile["bust_cm"])
	}
	if profile["waist_cm"] != "72" {
		t.Fatalf("unexpected waist: %#v", profile["waist_cm"])
	}
	if profile["hips_cm"] != "106" {
		t.Fatalf("unexpected hips: %#v", profile["hips_cm"])
	}
	if profile["measurements"] != "97 - 72 - 106 cm" {
		t.Fatalf("unexpected measurements: %#v", profile["measurements"])
	}
	if text, ok := got["resume"].(string); !ok || text == "" {
		t.Fatalf("expected resume, got %#v", got["resume"])
	}
	if text, ok := got["person"].(string); !ok || text == "" {
		t.Fatalf("expected person, got %#v", got["person"])
	}

	intro := got["intro"].(string)
	if intro == "" || intro[:len("小野坂唯花")] != "小野坂唯花" {
		t.Fatalf("expected intro without infobox markup, got %#v", intro)
	}
}
