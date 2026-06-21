# Development Guide

This guide covers local development, tests, coverage, release builds, and docs
maintenance. Read the [Site Adapter Guide](site-adapters.md) first when the
change is primarily a new or updated site config.

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

## Local Checks

Run the full test suite:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test ./... -coverprofile=/tmp/yachiyo-website-scraper.cover.out -covermode=atomic
go tool cover -func=/tmp/yachiyo-website-scraper.cover.out
```

Build the CLI:

```bash
go build -o ./scraper ./cmd/scraper
```

Smoke-check bundled configs:

```bash
./scraper sites
./scraper validate -config avbase
./scraper tasks -config avbase
```

## Test Focus

The tests should lock the intent described in the docs:

- YAML loading and validation
- Template rendering
- XPath, regex, type conversion, missing-field behavior, and URL resolution
- HTTP fetching, cookies, challenge detection, and FlareSolverr integration
- Runner orchestration, pagination metadata, indexed parameter resolution, and
  output formatting
- CLI smoke tests for documented commands

Prefer local `httptest` servers and inline YAML fixtures over real network
requests.

## Release Build

Run:

```bash
./scripts/build.sh
```

The binary is written to `./dist/scraper_<goos>_<goarch>`. The script reads the
root `version` file, verifies it is non-empty, and builds with
`-trimpath -buildvcs=false`. The same `version` file is embedded into the binary
at compile time.

Override the target and output:

```bash
GOOS=linux GOARCH=amd64 OUT_DIR=./release ./scripts/build.sh
```

After build, bundled site YAML files are already embedded in the binary. A
normal run does not need external YAML files.

## Documentation Maintenance

Keep the root README short. Detailed content belongs in `docs/`.

Use this flow:

1. Update [Usage Guide](usage.md) when command syntax changes.
2. Update [Task Reference](tasks.md) when task parameters, supported sites, or
   response shapes change.
3. Update [Runtime Fetching](runtime-fetching.md) when flags, environment
   variables, cookies, challenge handling, or site runtime notes change.
4. Update [YAML Configuration](configuration.md) when the config model changes.
5. Update [Site Adapter Guide](site-adapters.md) when the adapter workflow
   changes.

Docs should stay in English and link to the next relevant page so readers can
move through the workflow without returning to the README.

## Commit Notes

Use the interactive `git-cz` flow when creating commits. Keep the default commit
message format unchanged, use short English descriptions, and do not push unless
explicitly instructed.
