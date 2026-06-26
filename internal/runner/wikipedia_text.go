package runner

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

func parseWikipediaTextContent(content map[string]interface{}) map[string]interface{} {
	if len(content) == 0 {
		return nil
	}
	wikitext := strings.TrimSpace(stringValue(content["wikitext"]))
	if wikitext == "" {
		return nil
	}
	wikitext = strings.ReplaceAll(wikitext, `\n`, "\n")

	text := map[string]interface{}{}
	if intro := wikipediaIntro(wikitext); intro != "" {
		text["intro"] = intro
	}
	if profile := wikipediaInfoboxProfile(wikitext); len(profile) > 0 {
		text["profile"] = profile
	}
	if person := wikipediaSection(wikitext, "人物"); person != "" {
		text["person"] = cleanWikitext(person)
	}
	links := wikipediaExternalLinks(wikitext)
	if len(links) == 0 {
		links = fallbackExternalLinks(content["external_links"])
	}
	if len(links) > 0 {
		text["external_links"] = links
	}
	return text
}

func wikipediaIntro(wikitext string) string {
	rest := strings.TrimSpace(stripLeadingTemplate(wikitext))
	if idx := strings.Index(rest, "\n=="); idx >= 0 {
		rest = rest[:idx]
	}
	return cleanWikitext(rest)
}

func wikipediaInfoboxProfile(wikitext string) map[string]interface{} {
	body, ok := leadingTemplateBody(wikitext)
	if !ok {
		return nil
	}
	raw := parseTemplateFields(body)
	profile := map[string]interface{}{}

	assignClean := func(outKey string, keys ...string) {
		for _, key := range keys {
			if value := cleanWikitext(raw[key]); value != "" {
				profile[outKey] = value
				return
			}
		}
	}

	assignClean("name", "名前")
	assignClean("ruby", "ふりがな")
	assignClean("nickname", "愛称")
	assignClean("birth_place", "出身地")
	assignClean("blood_type", "血液型")
	assignClean("hair_color", "毛髪の色")
	assignClean("body_as_of", "時点")
	assignClean("cup", "カップ")
	assignClean("genre", "ジャンル")
	assignClean("activity_period", "AV出演期間")
	assignClean("exclusive_contract", "専属契約")

	if birthDate := infoboxBirthDate(raw); birthDate != "" {
		profile["birth_date"] = birthDate
	}
	if value := cleanNumber(raw["身長"]); value != "" {
		profile["height_cm"] = value
	}
	if value := cleanNumber(raw["体重"]); value != "" {
		profile["weight_kg"] = value
	}
	bust := cleanNumber(raw["バスト"])
	waist := cleanNumber(raw["ウエスト"])
	hips := cleanNumber(raw["ヒップ"])
	if bust != "" || waist != "" || hips != "" {
		profile["measurements"] = strings.TrimSpace(fmt.Sprintf("%s - %s - %s cm", bust, waist, hips))
	}
	return profile
}

func leadingTemplateBody(wikitext string) (string, bool) {
	start := strings.Index(wikitext, "{{")
	if start < 0 {
		return "", false
	}
	depth := 0
	for i := start; i < len(wikitext)-1; i++ {
		switch wikitext[i : i+2] {
		case "{{":
			depth++
			i++
		case "}}":
			depth--
			i++
			if depth == 0 {
				return wikitext[start+2 : i-1], true
			}
		}
	}
	return "", false
}

func stripLeadingTemplate(wikitext string) string {
	start := strings.Index(wikitext, "{{")
	if start < 0 {
		return wikitext
	}
	depth := 0
	for i := start; i < len(wikitext)-1; i++ {
		switch wikitext[i : i+2] {
		case "{{":
			depth++
			i++
		case "}}":
			depth--
			i++
			if depth == 0 {
				return wikitext[i+1:]
			}
		}
	}
	return wikitext
}

func parseTemplateFields(body string) map[string]string {
	lines := strings.Split(body, "\n")
	fields := map[string]string{}
	var currentKey string
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "|"))
			key, value, ok := strings.Cut(trimmed, "=")
			if !ok {
				continue
			}
			currentKey = strings.TrimSpace(key)
			fields[currentKey] = strings.TrimSpace(value)
			continue
		}
		if currentKey != "" {
			fields[currentKey] = strings.TrimSpace(fields[currentKey] + "\n" + strings.TrimSpace(line))
		}
	}
	return fields
}

func infoboxBirthDate(fields map[string]string) string {
	year := cleanNumber(fields["生年"])
	month := cleanNumber(fields["生月"])
	day := cleanNumber(fields["生日"])
	if year == "" || month == "" || day == "" {
		return ""
	}
	return fmt.Sprintf("%s-%02s-%02s", year, month, day)
}

func cleanNumber(value string) string {
	value = cleanWikitext(value)
	re := regexp.MustCompile(`\d+`)
	return re.FindString(value)
}

func wikipediaSection(wikitext, title string) string {
	re := regexp.MustCompile(`(?m)^==+\s*` + regexp.QuoteMeta(title) + `\s*==+\s*$`)
	loc := re.FindStringIndex(wikitext)
	if loc == nil {
		return ""
	}
	rest := wikitext[loc[1]:]
	next := regexp.MustCompile(`(?m)^==+[^=\n].*?==+\s*$`).FindStringIndex(rest)
	if next != nil {
		rest = rest[:next[0]]
	}
	return strings.TrimSpace(rest)
}

func wikipediaExternalLinks(wikitext string) []map[string]interface{} {
	section := wikipediaSection(wikitext, "外部链接")
	if section == "" {
		section = wikipediaSection(wikitext, "外部連結")
	}
	if section == "" {
		return nil
	}

	var links []map[string]interface{}
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		if line == "" {
			continue
		}
		if link, ok := socialTemplateLink(line); ok {
			links = append(links, link)
			continue
		}
		for _, match := range regexp.MustCompile(`\[(https?://[^\s\]]+)(?:\s+([^\]]+))?\]`).FindAllStringSubmatch(line, -1) {
			label := cleanWikitext(match[2])
			if label == "" {
				label = match[1]
			}
			links = append(links, map[string]interface{}{"label": label, "url": match[1]})
		}
	}
	return links
}

func socialTemplateLink(line string) (map[string]interface{}, bool) {
	match := regexp.MustCompile(`\{\{\s*(Twitter|X|Instagram)\s*\|\s*([^|}]+)`).FindStringSubmatch(line)
	if len(match) != 3 {
		return nil, false
	}
	kind := strings.ToLower(match[1])
	username := strings.TrimSpace(match[2])
	switch kind {
	case "twitter", "x":
		return map[string]interface{}{"label": "Twitter", "url": "https://twitter.com/" + username, "username": username}, true
	case "instagram":
		return map[string]interface{}{"label": "Instagram", "url": "https://www.instagram.com/" + username + "/", "username": username}, true
	default:
		return nil, false
	}
}

func fallbackExternalLinks(value interface{}) []map[string]interface{} {
	values, ok := value.([]interface{})
	if !ok {
		return nil
	}
	var links []map[string]interface{}
	for _, value := range values {
		url := strings.TrimSpace(fmt.Sprint(value))
		if url == "" || strings.Contains(url, "web.archive.org") {
			continue
		}
		links = append(links, map[string]interface{}{"label": url, "url": url})
		if len(links) >= 10 {
			break
		}
	}
	return links
}

func cleanWikitext(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = regexp.MustCompile(`(?s)<ref\b[^>/]*?>.*?</ref>`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`<ref\b[^>]*/>`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(value, "\n")
	value = simplifyKnownTemplates(value)
	value = regexp.MustCompile(`\[\[(?:File|Image|Category):[^\]]+\]\]`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`\[\[[^|\]]+\|([^\]]+)\]\]`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`\[\[([^\]]+)\]\]`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`\[(https?://[^\s\]]+)\s+([^\]]+)\]`).ReplaceAllString(value, "$2")
	value = regexp.MustCompile(`'{2,}`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "&nbsp;", " ")
	value = html.UnescapeString(value)
	value = regexp.MustCompile(`[ \t\r\f]+`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`\n{3,}`).ReplaceAllString(value, "\n\n")
	return strings.TrimSpace(value)
}

func simplifyKnownTemplates(value string) string {
	value = replaceTemplate(value, "bd", func(args []string) string {
		return strings.Join(args, "")
	})
	value = replaceTemplate(value, "fact", func(args []string) string {
		if len(args) == 0 {
			return ""
		}
		return args[0]
	})
	value = replaceTemplate(value, "link-ja", func(args []string) string {
		if len(args) == 0 {
			return ""
		}
		return args[0]
	})
	value = replaceTemplate(value, "jpn", func(args []string) string {
		var out []string
		for _, arg := range args {
			_, raw, ok := strings.Cut(arg, "=")
			if !ok {
				raw = arg
			}
			raw = cleanWikitext(raw)
			if raw != "" {
				out = append(out, raw)
			}
		}
		return strings.Join(out, " / ")
	})
	value = regexp.MustCompile(`\{\{(?:Wayback|Commonscat|DEFAULTSORT)[^{}]*\}\}`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`\{\{[^{}|]+\|([^{}|]+)[^{}]*\}\}`).ReplaceAllString(value, "$1")
	value = regexp.MustCompile(`\{\{[^{}]*\}\}`).ReplaceAllString(value, "")
	return value
}

func replaceTemplate(value, name string, render func([]string) string) string {
	re := regexp.MustCompile(`\{\{\s*` + regexp.QuoteMeta(name) + `\s*\|([^{}]*)\}\}`)
	return re.ReplaceAllStringFunc(value, func(token string) string {
		match := re.FindStringSubmatch(token)
		if len(match) != 2 {
			return ""
		}
		parts := strings.Split(match[1], "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return render(parts)
	})
}
