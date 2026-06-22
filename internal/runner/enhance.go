package runner

import (
	"context"
	"strings"
	"sync"

	"yachiyo-website-scraper/internal/config"
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
	if actorImage == nil || strings.ToLower(strings.TrimSpace(actorImage.Source)) != "gfriends" {
		return
	}

	lookup := opts.Gfriends
	if lookup == nil {
		lookup = defaultGfriends()
	}
	enhanceActorImages(ctx, result.Data, task.Output, *actorImage, lookup)
}

func defaultGfriends() ActorImageLookup {
	defaultGfriendsMu.Lock()
	defer defaultGfriendsMu.Unlock()

	if defaultGfriendsLookup == nil {
		defaultGfriendsLookup = gfriends.NewClient(gfriends.Options{})
	}
	return defaultGfriendsLookup
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
