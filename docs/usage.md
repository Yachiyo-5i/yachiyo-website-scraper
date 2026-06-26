# Usage Guide

Start here after reading the root [README](../README.md). This guide covers the
day-to-day CLI workflow. Continue to the [Task Reference](tasks.md) when you
need the exact parameters and response shape for each task.

## Build

Build a local binary:

```bash
go build -o ./scraper ./cmd/scraper
```

Release builds are covered in the [Development Guide](development.md).

## Config Selection

Every command that operates on a site accepts `-config`.

`-config` can be:

- A bundled site name, such as `avbase`
- A YAML file path, useful while developing a new adapter

Examples:

```bash
./scraper validate -config avbase
./scraper validate -config ./configs/avbase.yml
```

Bundled YAML files live under `configs/` and are embedded into the binary.

## Common Commands

List bundled sites:

```bash
./scraper sites
```

Print the compiled version:

```bash
./scraper version
./scraper --version
```

Validate a config:

```bash
./scraper validate -config avbase
```

List tasks in a config:

```bash
./scraper tasks -config avbase
```

Run a task:

```bash
./scraper run -config avbase -task search_work -param code=PRED-886
```

Pass multiple parameters by repeating `-param`:

```bash
./scraper run -config avbase -task search_work -param code=SSIS -param page=1
```

Run a JSON API-backed task:

```bash
./scraper run -config wikipedia -task page_summary -param title='Rikka Ono' -param lang=en -challenge off
```

## Output

Successful `run` commands print formatted JSON:

```json
{
  "ok": true,
  "site": "avbase",
  "task": "search_work",
  "url": "https://...",
  "channel": "http",
  "status": 200,
  "data": []
}
```

When the request is blocked, the server returns an accepted but non-successful
HTML status, or extraction fails, the command still prints a structured result:

```json
{
  "ok": false,
  "site": "avbase",
  "task": "search_work",
  "error": {
    "type": "blocked",
    "reason": "cloudflare_or_antibot_challenge"
  }
}
```

Use the `error.type` field to distinguish blocked pages, fetch errors, HTTP
status errors, extraction errors, and output formatting errors.

## Dumping HTML

Use `-dump-html` when developing selectors or debugging a site response:

```bash
./scraper run -config avbase -task search_work -param code=PRED-886 -dump-html ./debug.html
```

The file is written before extraction. It is useful with the YAML model described
in [YAML Configuration](configuration.md).

## Index Commands

Some site configs use local JSON indexes, for example actor name to actor ID
resolution.

Look up a configured index:

```bash
./scraper index lookup -config javbus -index actors -name Alice
```

Build an actor index from a paginated task:

```bash
./scraper index build -config javbus -task actor_list -out indexes/javbus_actors.json
```

Index building uses the same runtime flags as normal task runs. See
[Runtime Fetching](runtime-fetching.md) before building indexes against sites
that may require cookies or FlareSolverr.

## Next Step

See the [Task Reference](tasks.md) for supported tasks and parameter contracts.
