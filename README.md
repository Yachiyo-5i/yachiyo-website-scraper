# yachiyo-website-scraper

Yachiyo Website Scraper is a Go CLI for config-driven HTML and JSON scraping.

The scraper engine is generic: request rules, XPath or JSON selectors, regex
cleanup, pagination metadata, default non-account cookies, and output fields
are defined in YAML. Runtime fetch behavior, including Cloudflare challenge
handling, FlareSolverr, and Playwright browser fetching, is selected with CLI
flags or environment variables.

## Site Support

Bundled site configs are embedded into the binary and can be selected with
`-config`.

| Capability | Task | AVBase | JavBus | JavLibrary | FC2 | Sehuatang | Wikipedia |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Work search or work list | `search_work` | Yes | Yes | Yes | Yes | No | No |
| Work detail | `work_detail` | Yes | Yes | Yes | Yes | No | No |
| Actor detail and actor works | `actor_detail` | Yes | Yes | No | No | No | No |
| Actor candidate search | `actor_search` | No | Yes | No | No | No | No |
| Forum thread list | `forum_threads` | No | No | No | No | Yes | No |
| Forum thread detail | `thread_detail` | No | No | No | No | Yes | No |
| Wikipedia page search | `page_search` | No | No | No | No | No | Yes |
| Wikipedia page summary | `page_summary` | No | No | No | No | No | Yes |
| Wikidata entity by title | `entity_by_title` | No | No | No | No | No | Yes |
| Wikipedia page content | `page_content` | No | No | No | No | No | Yes |

Pagination is single-page only. The CLI requests the page you pass with
`-param page=...` and returns that page's data.

## Quick Start

Build the local binary:

```bash
go build -o ./scraper ./cmd/scraper
```

List bundled site configs:

```bash
./scraper sites
```

Validate a bundled site config:

```bash
./scraper validate -config avbase
```

Run a task:

```bash
./scraper run -config avbase -task search_work -param code=PRED-886
```

For sites that need challenge bypassing:

```bash
./scraper run \
  -config avbase \
  -task search_work \
  -param code=PRED-886 \
  -challenge bypass \
  -flaresolverr http://127.0.0.1:8191
```

Browser-backed fetching can use the optional helper under
`tools/playwright-fetcher/`.

## Documentation

Start with the [Documentation Index](docs/README.md).

## License

This project is licensed under the terms in [LICENSE](LICENSE).
