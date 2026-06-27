package runner

import (
	"context"
	"strings"
	"sync"

	"yachiyo-website-scraper/internal/config"
	"yachiyo-website-scraper/internal/fetcher"
	"yachiyo-website-scraper/internal/gfriends"
)

type ActorImageLookup interface {
	Lookup(context.Context, string) (string, bool)
}

type StaticActorImageLookup map[string]string

func (s StaticActorImageLookup) Lookup(_ context.Context, name string) (string, bool) {
	imageURL, ok := s[strings.TrimSpace(name)]
	return imageURL, ok
}

var (
	defaultGfriendsMu     sync.Mutex
	defaultGfriendsLookup ActorImageLookup
)

func applyEnhancements(ctx context.Context, task config.Task, result *Result, opts Options) {
	actorImage := task.Enhance.ActorImage
	if actorImage != nil && strings.ToLower(strings.TrimSpace(actorImage.Source)) == "gfriends" {
		lookup := opts.Gfriends
		if lookup == nil {
			lookup = defaultGfriends()
		}
		enhanceActorImages(ctx, result.Data, task.Output, *actorImage, lookup)
	}

	if task.Enhance.Wikipedia != nil {
		enhanceActorsWithWikipedia(ctx, result.Data, task.Output, *task.Enhance.Wikipedia, opts.Runtime)
	}
}

func defaultGfriends() ActorImageLookup {
	defaultGfriendsMu.Lock()
	defer defaultGfriendsMu.Unlock()

	if defaultGfriendsLookup == nil {
		defaultGfriendsLookup = gfriends.NewClient(gfriends.Options{})
	}
	return defaultGfriendsLookup
}

func runGfriendsTask(ctx context.Context, cfg *config.Config, task config.Task, opts Options) (*Result, error) {
	vars, err := resolveParams(cfg, task, opts.Params)
	if err != nil {
		return nil, err
	}
	taskType := strings.ToLower(strings.TrimSpace(task.Gfriends.Type))
	switch taskType {
	case "actor_image":
		return runGfriendsActorImageTask(ctx, cfg, task, opts, vars), nil
	default:
		return &Result{
			OK:    false,
			Site:  cfg.Site.ID,
			Task:  opts.TaskName,
			Error: &ErrorInfo{Type: "config_error", Reason: "unsupported gfriends task type"},
		}, nil
	}
}

func runGfriendsActorImageTask(ctx context.Context, cfg *config.Config, task config.Task, opts Options, vars map[string]string) *Result {
	nameParam := firstNonEmpty(task.Gfriends.NameParam, "name")
	name := strings.TrimSpace(vars[nameParam])
	if name == "" {
		return &Result{
			OK:    false,
			Site:  cfg.Site.ID,
			Task:  opts.TaskName,
			Error: &ErrorInfo{Type: "param_error", Reason: "name is required"},
		}
	}

	lookup := opts.Gfriends
	if lookup == nil {
		lookup = defaultGfriends()
	}
	imageURL, ok := lookup.Lookup(ctx, name)
	if !ok || strings.TrimSpace(imageURL) == "" {
		return &Result{
			OK:    false,
			Site:  cfg.Site.ID,
			Task:  opts.TaskName,
			Error: &ErrorInfo{Type: "not_found", Reason: `gfriends actor image not found for "` + name + `"`},
		}
	}

	return &Result{
		OK:   true,
		Site: cfg.Site.ID,
		Task: opts.TaskName,
		Data: map[string]interface{}{
			"actor": map[string]interface{}{
				"name":  name,
				"image": imageURL,
			},
		},
	}
}

func enhanceActorImages(ctx context.Context, data interface{}, output config.OutputConfig, cfg config.ActorImageEnhanceConfig, lookup ActorImageLookup) {
	itemsKey := firstNonEmpty(cfg.ItemsKey, output.ItemsKey, "actors")
	nameField := firstNonEmpty(cfg.NameField, "name")
	imageField := firstNonEmpty(cfg.ImageField, "image")

	switch typed := data.(type) {
	case map[string]interface{}:
		switch actors := typed[itemsKey].(type) {
		case []map[string]interface{}:
			replaceActorImages(ctx, actors, nameField, imageField, lookup)
		case map[string]interface{}:
			replaceActorImage(ctx, actors, nameField, imageField, lookup)
		}
	case []map[string]interface{}:
		replaceActorImages(ctx, typed, nameField, imageField, lookup)
	}
}

func replaceActorImages(ctx context.Context, actors []map[string]interface{}, nameField, imageField string, lookup ActorImageLookup) {
	for _, actor := range actors {
		replaceActorImage(ctx, actor, nameField, imageField, lookup)
	}
}

func replaceActorImage(ctx context.Context, actor map[string]interface{}, nameField, imageField string, lookup ActorImageLookup) {
	name := strings.TrimSpace(stringValue(actor[nameField]))
	if name == "" {
		return
	}
	imageURL, ok := lookup.Lookup(ctx, name)
	if !ok || strings.TrimSpace(imageURL) == "" {
		return
	}
	actor[imageField] = imageURL
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func enhanceActorsWithWikipedia(ctx context.Context, data interface{}, output config.OutputConfig, cfg config.WikipediaEnhanceConfig, runtime fetcher.RuntimeOptions) {
	actors := collectWikipediaActors(data, output)
	if len(actors) == 0 {
		return
	}

	configPath := firstNonEmpty(cfg.Config, "wikipedia")
	wikiCfg, err := config.Load(configPath)
	if err != nil {
		attachWikipediaMisses(actors, wikipediaTitleField(cfg), wikipediaTargetField(cfg), "config_error")
		return
	}

	cache := map[string]map[string]interface{}{}
	for _, actor := range actors {
		title := strings.TrimSpace(stringValue(actor[wikipediaTitleField(cfg)]))
		if title == "" {
			continue
		}
		cacheKey := wikipediaLang(cfg) + ":" + title
		wiki, ok := cache[cacheKey]
		if !ok {
			wiki = fetchWikipediaActor(ctx, wikiCfg, cfg, title, runtime)
			cache[cacheKey] = wiki
		}
		actor[wikipediaTargetField(cfg)] = wiki
	}
}

func collectWikipediaActors(data interface{}, output config.OutputConfig) []map[string]interface{} {
	switch typed := data.(type) {
	case map[string]interface{}:
		if actor, ok := typed["actor"].(map[string]interface{}); ok {
			return []map[string]interface{}{actor}
		}
		itemsKey := firstNonEmpty(output.ItemsKey, "actors")
		if actors, ok := typed[itemsKey].([]map[string]interface{}); ok {
			return actors
		}
		if actors, ok := typed["actors"].([]map[string]interface{}); ok {
			return actors
		}
	case []map[string]interface{}:
		return typed
	}
	return nil
}

func attachWikipediaMisses(actors []map[string]interface{}, titleField, targetField, reason string) {
	for _, actor := range actors {
		title := strings.TrimSpace(stringValue(actor[titleField]))
		if title == "" {
			continue
		}
		actor[targetField] = wikipediaMiss(title, reason)
	}
}

func fetchWikipediaActor(ctx context.Context, wikiCfg *config.Config, cfg config.WikipediaEnhanceConfig, title string, runtime fetcher.RuntimeOptions) map[string]interface{} {
	lang := wikipediaLang(cfg)
	summary, ok := runWikipediaObjectTask(ctx, wikiCfg, wikipediaSummaryTask(cfg), map[string]string{
		"title": title,
		"lang":  lang,
	}, runtime)
	if !ok || strings.TrimSpace(stringValue(summary["wikidata_id"])) == "" {
		searchTitle, found := fetchWikipediaSearchTitle(ctx, wikiCfg, wikipediaSearchTask(cfg), wikipediaLang(cfg), title, runtime)
		if !found {
			return wikipediaMiss(title, "not_found")
		}
		summary, ok = runWikipediaObjectTask(ctx, wikiCfg, wikipediaSummaryTask(cfg), map[string]string{
			"title": searchTitle,
			"lang":  lang,
		}, runtime)
		if !ok {
			return wikipediaMiss(title, "summary_error")
		}
	}

	acceptedTitle := firstNonEmpty(stringValue(summary["title"]), title)
	entity, _ := runWikipediaObjectTask(ctx, wikiCfg, wikipediaEntityTask(cfg), map[string]string{
		"title": acceptedTitle,
		"lang":  lang,
	}, runtime)
	content, _ := runWikipediaObjectTask(ctx, wikiCfg, wikipediaContentTask(cfg), map[string]string{
		"title": acceptedTitle,
		"lang":  lang,
	}, runtime)

	return normalizeWikipediaActor(title, lang, summary, entity, content)
}

func fetchWikipediaSearchTitle(ctx context.Context, wikiCfg *config.Config, taskName, lang, title string, runtime fetcher.RuntimeOptions) (string, bool) {
	result, err := Run(ctx, wikiCfg, Options{
		TaskName: taskName,
		Params: map[string]string{
			"keyword": title,
			"lang":    lang,
		},
		Runtime: runtime,
	})
	if err != nil || result == nil || !result.OK {
		return "", false
	}

	switch data := result.Data.(type) {
	case []map[string]interface{}:
		if len(data) == 0 {
			return "", false
		}
		return firstNonEmpty(stringValue(data[0]["title"]), title), true
	case map[string]interface{}:
		if actors, ok := data["items"].([]map[string]interface{}); ok && len(actors) > 0 {
			return firstNonEmpty(stringValue(actors[0]["title"]), title), true
		}
	}
	return "", false
}

func runWikipediaObjectTask(ctx context.Context, wikiCfg *config.Config, taskName string, params map[string]string, runtime fetcher.RuntimeOptions) (map[string]interface{}, bool) {
	result, err := Run(ctx, wikiCfg, Options{
		TaskName: taskName,
		Params:   params,
		Runtime:  runtime,
	})
	if err != nil || result == nil || !result.OK {
		return nil, false
	}
	data, ok := result.Data.(map[string]interface{})
	return data, ok
}

func normalizeWikipediaActor(query, lang string, summary, entity, content map[string]interface{}) map[string]interface{} {
	return normalizeWikipediaStructuredResult(query, lang, summary, entity, content)
}

func normalizeWikipediaStructuredResult(query, lang string, summary, entity, content map[string]interface{}) map[string]interface{} {
	wikidataID := firstNonEmpty(stringValue(summary["wikidata_id"]), stringValue(entity["wikidata_id"]))
	occupation := []interface{}{}
	if value := propertyValue(entity["occupation_qid"], "P106", "qid"); value != nil {
		occupation = append(occupation, value)
	}
	out := map[string]interface{}{
		"matched":     true,
		"query":       query,
		"lang":        firstNonEmpty(stringValue(summary["lang"]), lang),
		"title":       summary["title"],
		"pageid":      summary["pageid"],
		"url":         summary["page_url"],
		"wikidata_id": wikidataID,
		"description": summary["description"],
		"summary":     summary["summary"],
		"thumbnail":   summary["thumbnail"],
		"revision":    summary["revision"],
		"timestamp":   summary["timestamp"],
		"languages": map[string]interface{}{
			"zh": entity["zh_title"],
			"ja": entity["ja_title"],
			"en": entity["en_title"],
		},
		"profile": map[string]interface{}{
			"birth_date":  propertyValue(entity["birth_date"], "P569", "value"),
			"birth_place": propertyValue(entity["birth_place_qid"], "P19", "qid"),
			"height_cm":   propertyValue(entity["height_cm"], "P2048", "value"),
			"country":     propertyValue(entity["country_qid"], "P27", "qid"),
			"occupation":  occupation,
		},
		"social": map[string]interface{}{
			"x":                emptyToNil(entity["x_username"]),
			"instagram":        emptyToNil(entity["instagram_username"]),
			"official_website": emptyToNil(entity["official_website"]),
			"imdb_id":          emptyToNil(entity["imdb_id"]),
		},
		"media": map[string]interface{}{
			"commons_category":  emptyToNil(entity["commons_category"]),
			"wikidata_image":    emptyToNil(entity["wikidata_image"]),
			"summary_thumbnail": emptyToNil(summary["thumbnail"]),
		},
	}
	if text := parseWikipediaTextContent(content); len(text) > 0 {
		out["text"] = text
	}
	return out
}

func propertyValue(value interface{}, property string, key string) interface{} {
	if !presentValue(value) {
		return nil
	}
	return map[string]interface{}{
		key:        value,
		"source":   "wikidata",
		"property": property,
	}
}

func emptyToNil(value interface{}) interface{} {
	if !presentValue(value) {
		return nil
	}
	return value
}

func presentValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}

func wikipediaMiss(query, reason string) map[string]interface{} {
	return map[string]interface{}{
		"matched": false,
		"query":   query,
		"reason":  reason,
	}
}

func wikipediaLang(cfg config.WikipediaEnhanceConfig) string {
	return firstNonEmpty(cfg.Lang, "zh")
}

func wikipediaTitleField(cfg config.WikipediaEnhanceConfig) string {
	return firstNonEmpty(cfg.TitleField, "name")
}

func wikipediaTargetField(cfg config.WikipediaEnhanceConfig) string {
	return firstNonEmpty(cfg.TargetField, "wikipedia")
}

func wikipediaSummaryTask(cfg config.WikipediaEnhanceConfig) string {
	return firstNonEmpty(cfg.SummaryTask, "page_summary")
}

func wikipediaSearchTask(cfg config.WikipediaEnhanceConfig) string {
	return firstNonEmpty(cfg.SearchTask, "page_search")
}

func wikipediaEntityTask(cfg config.WikipediaEnhanceConfig) string {
	return firstNonEmpty(cfg.EntityTask, "entity_by_title")
}

func wikipediaContentTask(cfg config.WikipediaEnhanceConfig) string {
	return firstNonEmpty(cfg.ContentTask, "page_content")
}
