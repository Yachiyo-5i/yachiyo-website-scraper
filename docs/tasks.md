# Task Reference

This page describes the public task contract. Read the [Usage Guide](usage.md)
first if you need command syntax. Continue to [Runtime Fetching](runtime-fetching.md)
when a site requires cookies or challenge bypassing.

Tasks that return the same data type use the same public parameter names and
stable output keys across supported sites. A site may return `null` or `[]` for
fields it cannot extract from a page.

## Site Support

Use `-config` to select a bundled site. Runtime fetch options such as
Cloudflare handling and cookies are passed as flags or environment variables,
not as task parameters.

| Capability | Task | AVBase | JavBus | JavLibrary | FC2 |
| --- | --- | --- | --- | --- | --- |
| Work search or work list | `search_work` | Yes, supports `page` | Yes, exact-code detail route; `page` is accepted for a uniform interface but not used | Yes, supports `page` | Yes, exact article route; `page` is accepted for a uniform interface but not used |
| Work detail | `work_detail` | Yes, `code` is the `source_id` returned by `search_work` | Yes, `code` is the work code such as `SSIS-001` | Yes, `code` is the `source_id` returned by `search_work` | Yes, `code` accepts the numeric FC2 code or supported FC2 prefixes |
| Actor detail and actor works | `actor_detail` | Yes, by `name`, supports `page` | Yes, by `name`, supports `page` | No | No |
| Actor candidate search | `actor_search` | No | Yes, by `keyword` | No | No |

Pagination is single-page only. The CLI requests the page you pass with
`-param page=...` and returns that page's data. It does not automatically fetch
all result pages.

## `search_work`

Search for work summaries.

Supported sites:

```text
avbase, javbus, javlibrary, fc2
```

Parameters:

```text
code    required. Work code, search prefix, or numeric article code, depending on site support.
page    optional. Defaults to 1. Some exact-route sites accept it for compatibility but do not use it.
```

Examples:

```bash
./scraper run -config avbase -task search_work -param code=SSIS -param page=1
./scraper run -config javbus -task search_work -param code=SSIS-001
./scraper run -config javlibrary -task search_work -param code=SSIS-001 -param page=1
./scraper run -config fc2 -task search_work -param code=4913917
```

For FC2, `code` accepts the numeric part directly or these prefixed forms:
`FC2-PPV-{code}`, `FC2-{code}`, and `PPV-{code}`.

Response shape:

```json
{
  "ok": true,
  "site": "avbase",
  "task": "search_work",
  "url": "https://...",
  "channel": "http",
  "status": 200,
  "meta": {
    "count": 30,
    "page": 1,
    "total": 952
  },
  "data": {
    "works": [
      {
        "source_id": "SSIS-001",
        "code": "SSIS-001",
        "title": "...",
        "url": "https://...",
        "cover": "https://...",
        "release_date": "2021-02-18",
        "actors": ["..."],
        "tags": []
      }
    ]
  }
}
```

`meta` is present when the site exposes page metadata. `source_id` is the value
to pass into `work_detail -param code=...` when that site supports work detail.
For AVBase, `source_id` may include a source prefix, such as
`premium:PRED-886` or `secondface:SSIS-001`, because one visible code can map to
multiple source pages.

## `work_detail`

Fetch one full work object.

Supported sites:

```text
avbase, javbus, javlibrary, fc2
```

Parameters:

```text
code    required. Use the site's `source_id` from search_work unless noted in the support table.
```

Examples:

```bash
./scraper run -config javbus -task work_detail -param code=SSIS-001
./scraper run -config javlibrary -task work_detail -param code=javmezzbqu
./scraper run -config fc2 -task work_detail -param code=4913917
./scraper run -config avbase -task work_detail -param code='premium:PRED-886'
```

Response shape:

```json
{
  "ok": true,
  "site": "javbus",
  "task": "work_detail",
  "url": "https://...",
  "channel": "http",
  "status": 200,
  "data": {
    "source_id": "SSIS-001",
    "code": "SSIS-001",
    "title": "...",
    "url": "https://...",
    "cover": "https://...",
    "release_date": "2021-02-18",
    "runtime_minutes": 150,
    "director": "...",
    "maker": "...",
    "label": "...",
    "genres": ["..."],
    "actors": ["..."],
    "sample_images": ["https://..."],
    "magnet_links": ["magnet:?xt=urn:btih:..."],
    "rating": null,
    "wanted_count": null,
    "watched_count": null,
    "owned_count": null
  }
}
```

## `actor_detail`

Fetch actor information and the actor's works on one page.

Supported sites:

```text
avbase, javbus
```

Parameters:

```text
name    required. Actor name.
page    optional. Defaults to 1.
```

Examples:

```bash
./scraper run -config avbase -task actor_detail -param name=Alice -param page=1
./scraper run -config javbus -task actor_detail -param name=Alice -param page=1
```

Response shape:

```json
{
  "ok": true,
  "site": "javbus",
  "task": "actor_detail",
  "url": "https://...",
  "channel": "http",
  "status": 200,
  "meta": {
    "count": 30,
    "page": 1,
    "total": 98,
    "next_page": 2,
    "last_visible_page": 4
  },
  "data": {
    "actor": {
      "id": "w5a",
      "name": "...",
      "url": "https://...",
      "ruby": null,
      "image": "https://...",
      "birthday": null,
      "age": null,
      "height_cm": null,
      "cup": null,
      "bust_cm": null,
      "waist_cm": null,
      "hips_cm": null,
      "followers": null,
      "profile": null
    },
    "works": []
  }
}
```

## `actor_search`

Search actor candidates.

Supported sites:

```text
javbus
```

Parameters:

```text
keyword required. Actor search keyword.
```

Example:

```bash
./scraper run -config javbus -task actor_search -param keyword=Alice
```

Response shape:

```json
{
  "ok": true,
  "site": "javbus",
  "task": "actor_search",
  "data": {
    "actors": [
      {
        "id": "...",
        "name": "...",
        "url": "https://...",
        "image": "https://..."
      }
    ]
  }
}
```

## Next Step

See [Runtime Fetching](runtime-fetching.md) for cookies, challenge handling, and
FlareSolverr.
