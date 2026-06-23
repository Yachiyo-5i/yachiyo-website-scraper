package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	buildinfo "yachiyo-website-scraper"
	"yachiyo-website-scraper/internal/config"
	"yachiyo-website-scraper/internal/fetcher"
	"yachiyo-website-scraper/internal/indexer"
	"yachiyo-website-scraper/internal/runner"
)

type paramsFlag map[string]string

func (p paramsFlag) String() string {
	if len(p) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p))
	for k, v := range p {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (p paramsFlag) Set(value string) error {
	key, val, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("param must use key=value format")
	}
	p[strings.TrimSpace(key)] = val
	return nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version", "--version", "-version", "-v":
		fmt.Println(buildinfo.Version())
	case "run":
		if err := run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "validate":
		if err := validate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "tasks":
		if err := tasks(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "categories":
		if err := categories(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "sites":
		if err := sites(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "index":
		if err := indexCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func run(args []string) error {
	params := paramsFlag{}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "builtin site name or path to site YAML config")
	taskName := fs.String("task", "", "task name in config")
	fs.Var(params, "param", "task parameter in key=value format; can be repeated")
	cookie := fs.String("cookie", os.Getenv("SCRAPER_COOKIE"), "cookie header value")
	challenge := fs.String("challenge", envDefault("SCRAPER_CHALLENGE", "detect"), "challenge handling: detect, bypass, off")
	flaresolverrURL := fs.String("flaresolverr", os.Getenv("SCRAPER_FLARESOLVERR_URL"), "FlareSolverr base URL")
	playwrightURL := fs.String("playwright", os.Getenv("SCRAPER_PLAYWRIGHT_URL"), "Playwright fetch service base URL")
	timeout := fs.Duration("timeout", envDuration("SCRAPER_TIMEOUT", 30*time.Second), "HTTP timeout")
	flaresolverrTimeout := fs.Duration("flaresolverr-timeout", envDuration("SCRAPER_FLARESOLVERR_TIMEOUT", 60*time.Second), "FlareSolverr timeout")
	playwrightTimeout := fs.Duration("playwright-timeout", envDuration("SCRAPER_PLAYWRIGHT_TIMEOUT", 60*time.Second), "Playwright fetch timeout")
	dumpHTML := fs.String("dump-html", "", "write fetched HTML to this path before extraction")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if *taskName == "" {
		return fmt.Errorf("--task is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	mode := fetcher.ChallengeMode(*challenge)
	switch mode {
	case fetcher.ChallengeDetect, fetcher.ChallengeBypass, fetcher.ChallengeOff:
	default:
		return fmt.Errorf("--challenge must be one of: detect, bypass, off")
	}

	res, err := runner.Run(context.Background(), cfg, runner.Options{
		ConfigPath: *configPath,
		TaskName:   *taskName,
		Params:     params,
		DumpHTML:   *dumpHTML,
		Runtime: fetcher.RuntimeOptions{
			Timeout:          *timeout,
			Cookie:           *cookie,
			Challenge:        mode,
			FlareSolverrURL:  *flaresolverrURL,
			FlareSolverrWait: *flaresolverrTimeout,
			PlaywrightURL:    *playwrightURL,
			PlaywrightWait:   *playwrightTimeout,
		},
	})
	if err != nil {
		return err
	}
	encoded, err := runner.EncodeResult(res)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if !res.OK {
		os.Exit(3)
	}
	return nil
}

func validate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "builtin site name or path to site YAML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	fmt.Printf("ok: %s (%d tasks)\n", cfg.Site.ID, len(cfg.Tasks))
	return nil
}

func tasks(args []string) error {
	fs := flag.NewFlagSet("tasks", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "builtin site name or path to site YAML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	for name := range cfg.Tasks {
		fmt.Println(name)
	}
	return nil
}

func sites(args []string) error {
	fs := flag.NewFlagSet("sites", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	sites, err := config.BuiltinSites()
	if err != nil {
		return err
	}
	for _, site := range sites {
		fmt.Println(site)
	}
	return nil
}

func categories(args []string) error {
	fs := flag.NewFlagSet("categories", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "builtin site name or path to site YAML config")
	indexName := fs.String("index", "categories", "category index name in config")
	field := fs.String("field", "name", "category display field")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	indexCfg, ok := cfg.Indexes[*indexName]
	if !ok {
		return fmt.Errorf("index %q not found", *indexName)
	}
	itemsKey := indexCfg.ItemsKey
	if itemsKey == "" {
		itemsKey = "items"
	}
	idx, err := loadCLIIndex(indexCfg, itemsKey)
	if err != nil {
		return err
	}
	for _, item := range idx.Items {
		value := strings.TrimSpace(fmt.Sprint(item[*field]))
		if value == "" {
			continue
		}
		fmt.Println(value)
	}
	return nil
}

type indexPageResult struct {
	Page  int
	Items []map[string]interface{}
	OK    bool
	Err   error
}

type actorIndexDocument struct {
	Site       string                   `json:"site"`
	Kind       string                   `json:"kind"`
	Source     string                   `json:"source"`
	Task       string                   `json:"task"`
	UpdatedAt  string                   `json:"updated_at"`
	TotalPages int                      `json:"total_pages"`
	Count      int                      `json:"count"`
	Actors     []map[string]interface{} `json:"actors"`
}

func indexCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("index command requires subcommand: build or lookup")
	}
	switch args[0] {
	case "build":
		return indexBuild(args[1:])
	case "lookup":
		return indexLookup(args[1:])
	default:
		return fmt.Errorf("unknown index subcommand %q", args[0])
	}
}

func indexBuild(args []string) error {
	fs := flag.NewFlagSet("index build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "builtin site name or path to site YAML config")
	taskName := fs.String("task", "actor_list", "task that returns index items")
	outPath := fs.String("out", "", "output index JSON path")
	pageParam := fs.String("page-param", "page", "pagination parameter name")
	itemsKey := fs.String("items-key", "", "items key in task object output")
	maxPages := fs.Int("max-pages", 2000, "maximum page probe limit")
	concurrency := fs.Int("concurrency", 8, "concurrent page fetches")
	retries := fs.Int("retries", 3, "retries per page")
	cookie := fs.String("cookie", os.Getenv("SCRAPER_COOKIE"), "cookie header value")
	challenge := fs.String("challenge", envDefault("SCRAPER_CHALLENGE", "detect"), "challenge handling: detect, bypass, off")
	flaresolverrURL := fs.String("flaresolverr", os.Getenv("SCRAPER_FLARESOLVERR_URL"), "FlareSolverr base URL")
	playwrightURL := fs.String("playwright", os.Getenv("SCRAPER_PLAYWRIGHT_URL"), "Playwright fetch service base URL")
	timeout := fs.Duration("timeout", envDuration("SCRAPER_TIMEOUT", 30*time.Second), "HTTP timeout")
	flaresolverrTimeout := fs.Duration("flaresolverr-timeout", envDuration("SCRAPER_FLARESOLVERR_TIMEOUT", 60*time.Second), "FlareSolverr timeout")
	playwrightTimeout := fs.Duration("playwright-timeout", envDuration("SCRAPER_PLAYWRIGHT_TIMEOUT", 60*time.Second), "Playwright fetch timeout")
	pretty := fs.Bool("pretty", false, "write pretty-printed JSON instead of compact JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if *outPath == "" {
		return fmt.Errorf("--out is required")
	}
	if *maxPages < 1 {
		return fmt.Errorf("--max-pages must be positive")
	}
	if *concurrency < 1 {
		return fmt.Errorf("--concurrency must be positive")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	task, err := cfg.Task(*taskName)
	if err != nil {
		return err
	}
	key := *itemsKey
	if key == "" {
		key = task.Output.ItemsKey
	}
	if key == "" {
		key = "items"
	}
	mode, err := challengeMode(*challenge)
	if err != nil {
		return err
	}
	runtime := fetcher.RuntimeOptions{
		Timeout:          *timeout,
		Cookie:           *cookie,
		Challenge:        mode,
		FlareSolverrURL:  *flaresolverrURL,
		FlareSolverrWait: *flaresolverrTimeout,
		PlaywrightURL:    *playwrightURL,
		PlaywrightWait:   *playwrightTimeout,
	}

	ctx := context.Background()
	lastPage, err := findLastIndexPage(ctx, cfg, *taskName, *pageParam, *maxPages, key, runtime, *retries)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "index build: last page = %d\n", lastPage)

	pages, err := fetchIndexPages(ctx, cfg, *taskName, *pageParam, lastPage, key, runtime, *concurrency, *retries)
	if err != nil {
		return err
	}
	actors := mergeIndexPages(pages)
	doc := actorIndexDocument{
		Site:       cfg.Site.ID,
		Kind:       "actors",
		Source:     strings.TrimRight(cfg.Site.BaseURL, "/") + "/actresses",
		Task:       *taskName,
		UpdatedAt:  time.Now().Format(time.RFC3339),
		TotalPages: lastPage,
		Count:      len(actors),
		Actors:     actors,
	}
	var encoded []byte
	if *pretty {
		encoded, err = json.MarshalIndent(doc, "", "  ")
	} else {
		encoded, err = json.Marshal(doc)
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*outPath, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "index build: wrote %d actors to %s\n", len(actors), *outPath)
	return nil
}

func indexLookup(args []string) error {
	fs := flag.NewFlagSet("index lookup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "builtin site name or path to site YAML config")
	indexName := fs.String("index", "actors", "index name in config")
	name := fs.String("name", "", "actor name to look up")
	field := fs.String("field", "", "match field override")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("--name is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	indexCfg, ok := cfg.Indexes[*indexName]
	if !ok {
		return fmt.Errorf("index %q not found", *indexName)
	}
	itemsKey := indexCfg.ItemsKey
	if itemsKey == "" {
		itemsKey = "items"
	}
	matchField := *field
	if matchField == "" {
		matchField = indexCfg.MatchField
	}
	if matchField == "" {
		matchField = "name"
	}
	idx, err := loadCLIIndex(indexCfg, itemsKey)
	if err != nil {
		return err
	}
	matches := idx.FindAll(matchField, *name, indexCfg.CaseSensitive)
	out := map[string]interface{}{
		"ok":      len(matches) > 0,
		"count":   len(matches),
		"matches": matches,
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if len(matches) == 0 {
		os.Exit(3)
	}
	return nil
}

func loadCLIIndex(indexCfg config.Index, itemsKey string) (*indexer.Index, error) {
	if len(indexCfg.Items) > 0 {
		return indexer.FromItems(indexCfg.Items, itemsKey), nil
	}
	return indexer.Load(indexCfg.Path, itemsKey)
}

func findLastIndexPage(ctx context.Context, cfg *config.Config, taskName, pageParam string, maxPages int, itemsKey string, runtime fetcher.RuntimeOptions, retries int) (int, error) {
	ok, err := indexPageOK(ctx, cfg, taskName, pageParam, 1, itemsKey, runtime, retries)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("first index page returned no data")
	}

	low, high := 1, 2
	for high <= maxPages {
		ok, err := indexPageOK(ctx, cfg, taskName, pageParam, high, itemsKey, runtime, retries)
		if err != nil {
			return 0, err
		}
		if !ok {
			break
		}
		low = high
		high *= 2
	}
	if high > maxPages {
		high = maxPages + 1
	}
	for low+1 < high {
		mid := (low + high) / 2
		ok, err := indexPageOK(ctx, cfg, taskName, pageParam, mid, itemsKey, runtime, retries)
		if err != nil {
			return 0, err
		}
		if ok {
			low = mid
		} else {
			high = mid
		}
	}
	return low, nil
}

func indexPageOK(ctx context.Context, cfg *config.Config, taskName, pageParam string, page int, itemsKey string, runtime fetcher.RuntimeOptions, retries int) (bool, error) {
	items, ok, err := runIndexPageWithRetries(ctx, cfg, taskName, pageParam, page, itemsKey, runtime, retries)
	if err != nil {
		return false, err
	}
	return ok && len(items) > 0, nil
}

func fetchIndexPages(ctx context.Context, cfg *config.Config, taskName, pageParam string, lastPage int, itemsKey string, runtime fetcher.RuntimeOptions, concurrency int, retries int) (map[int][]map[string]interface{}, error) {
	jobs := make(chan int)
	results := make(chan indexPageResult)
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range jobs {
				items, ok, err := runIndexPageWithRetries(ctx, cfg, taskName, pageParam, page, itemsKey, runtime, retries)
				results <- indexPageResult{Page: page, Items: items, OK: ok, Err: err}
			}
		}()
	}

	go func() {
		for page := 1; page <= lastPage; page++ {
			jobs <- page
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	pages := make(map[int][]map[string]interface{}, lastPage)
	completed := 0
	failed := map[int]error{}
	for result := range results {
		completed++
		if result.Err != nil {
			failed[result.Page] = result.Err
		} else if !result.OK {
			failed[result.Page] = fmt.Errorf("page %d returned non-ok result", result.Page)
		} else if result.OK {
			pages[result.Page] = result.Items
		}
		if completed == 1 || completed%50 == 0 || completed == lastPage {
			fmt.Fprintf(os.Stderr, "index build: fetched %d/%d pages\n", completed, lastPage)
		}
	}
	if len(failed) > 0 {
		failedPages := make([]int, 0, len(failed))
		for page := range failed {
			failedPages = append(failedPages, page)
		}
		sort.Ints(failedPages)
		fmt.Fprintf(os.Stderr, "index build: retrying %d failed page(s): %v\n", len(failedPages), failedPages)
		for _, page := range failedPages {
			items, ok, err := runIndexPageWithRetries(ctx, cfg, taskName, pageParam, page, itemsKey, runtime, retries+5)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("page %d returned non-ok result after retry", page)
			}
			pages[page] = items
		}
	}
	if len(pages) != lastPage {
		return nil, fmt.Errorf("fetched %d/%d pages", len(pages), lastPage)
	}
	return pages, nil
}

func runIndexPage(ctx context.Context, cfg *config.Config, taskName, pageParam string, page int, itemsKey string, runtime fetcher.RuntimeOptions) ([]map[string]interface{}, bool, error) {
	res, err := runner.Run(ctx, cfg, runner.Options{
		TaskName: taskName,
		Params:   map[string]string{pageParam: strconv.Itoa(page)},
		Runtime:  runtime,
	})
	if err != nil {
		return nil, false, err
	}
	if !res.OK {
		return nil, false, nil
	}
	items, err := resultItems(res.Data, itemsKey)
	if err != nil {
		return nil, false, fmt.Errorf("page %d: %w", page, err)
	}
	return items, true, nil
}

func runIndexPageWithRetries(ctx context.Context, cfg *config.Config, taskName, pageParam string, page int, itemsKey string, runtime fetcher.RuntimeOptions, retries int) ([]map[string]interface{}, bool, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		items, ok, err := runIndexPage(ctx, cfg, taskName, pageParam, page, itemsKey, runtime)
		if err == nil && ok {
			return items, true, nil
		}
		lastErr = err
		if attempt < retries {
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		}
	}
	if lastErr != nil {
		return nil, false, lastErr
	}
	return nil, false, nil
}

func resultItems(data interface{}, itemsKey string) ([]map[string]interface{}, error) {
	object, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected object data, got %T", data)
	}
	raw, ok := object[itemsKey]
	if !ok {
		return nil, fmt.Errorf("data does not contain key %q", itemsKey)
	}
	items, ok := raw.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("data.%s has unexpected type %T", itemsKey, raw)
	}
	return items, nil
}

func mergeIndexPages(pages map[int][]map[string]interface{}) []map[string]interface{} {
	pageNums := make([]int, 0, len(pages))
	for page := range pages {
		pageNums = append(pageNums, page)
	}
	sort.Ints(pageNums)

	seen := map[string]bool{}
	var out []map[string]interface{}
	for _, page := range pageNums {
		for _, item := range pages[page] {
			key := ""
			if raw, ok := item["id"]; ok && raw != nil {
				key = strings.TrimSpace(fmt.Sprint(raw))
			}
			if key == "" {
				if raw, ok := item["url"]; ok && raw != nil {
					key = strings.TrimSpace(fmt.Sprint(raw))
				}
			}
			if key != "" && seen[key] {
				continue
			}
			if key != "" {
				seen[key] = true
			}
			out = append(out, item)
		}
	}
	return out
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  scraper run -config avbase -task search_work -param code=PRED-886 [--cookie "..."] [--challenge detect|bypass|off] [--flaresolverr URL] [--playwright URL] [--dump-html debug.html]
  scraper index build -config javbus -task actor_list -out indexes/javbus_actors.json
  scraper index lookup -config javbus -index actors -name 仲村みう
  scraper categories -config sehuatang
  scraper validate -config avbase
  scraper tasks -config avbase
  scraper sites
  scraper version`)
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func challengeMode(value string) (fetcher.ChallengeMode, error) {
	mode := fetcher.ChallengeMode(value)
	switch mode {
	case fetcher.ChallengeDetect, fetcher.ChallengeBypass, fetcher.ChallengeOff:
		return mode, nil
	default:
		return "", fmt.Errorf("--challenge must be one of: detect, bypass, off")
	}
}
