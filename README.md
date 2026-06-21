# yachiyo-website-scraper

Yachiyo Website Scraper is a Go CLI for config-driven static HTML scraping.

The scraper engine is generic: request rules, XPath selectors, regex cleanup,
pagination metadata, default non-account cookies, and output fields are defined
in YAML. Runtime fetch behavior, including Cloudflare challenge handling and
FlareSolverr, is selected with CLI flags or environment variables.

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

## Documentation

Start with the [Documentation Index](docs/README.md).

## Current Scope

- Static HTML only
- YAML-driven site and task definitions
- Built-in site configs embedded into the binary
- XPath field extraction
- Regex post-processing
- Page-level metadata extraction, such as total result count
- Single-page pagination by request parameter
- Default cookie, runtime cookie override, and FlareSolverr support

## Project Layout

```text
cmd/scraper/        CLI entrypoint
configs/            Built-in site YAML configs, embedded into the binary
indexes/            Built-in index data used by configured site tasks
internal/config/    YAML loading, validation, and embedded config lookup
internal/extractor/ XPath extraction and output formatting
internal/fetcher/   HTTP, challenge detection, and FlareSolverr integration
internal/indexer/   Local JSON index lookup support
internal/runner/    Task execution pipeline
scripts/            Release build scripts
docs/               User and contributor documentation
```

Generated binaries, `dist/`, local dumps, and editor files are ignored by
`.gitignore`. The files under `configs/` and `indexes/` are source assets and
should be committed.

## Development Shortcut

```bash
go test ./...
go test ./... -coverprofile=/tmp/yachiyo-website-scraper.cover.out -covermode=atomic
go tool cover -func=/tmp/yachiyo-website-scraper.cover.out
```
