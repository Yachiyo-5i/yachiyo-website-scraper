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

| Capability | Task | AVBase | JavBus | JavLibrary | FC2 | Sehuatang | Wikipedia | Gfriends |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Work search or work list | `search_work` | Yes, supports `page` | Yes, exact-code detail route; `page` is accepted for a uniform interface but not used | Yes, supports `page` | Yes, exact article route; `page` is accepted for a uniform interface but not used | No | No | No |
| Work detail | `work_detail` | Yes, `code` is the `source_id` returned by `search_work` | Yes, `code` is the work code such as `SSIS-001` | Yes, `code` is the `source_id` returned by `search_work` | Yes, `code` accepts the numeric FC2 code or supported FC2 prefixes | No | No | No |
| Actor detail and actor works | `actor_detail` | Yes, by `name`, supports `page` | Yes, by `name`, supports `page` | No | No | No | No | No |
| Actor candidate search | `actor_search` | No | Yes, by `keyword` | No | No | No | No | No |
| Gfriends actor image lookup | `actor_image` | No | No | No | No | No | No | Yes, by `name` |
| Forum thread list | `forum_threads` | No | No | No | No | Yes | No | No |
| Forum thread detail | `thread_detail` | No | No | No | No | Yes | No | No |
| Wikipedia page search | `page_search` | No | No | No | No | No | Yes | No |
| Wikipedia page summary | `page_summary` | No | No | No | No | No | Yes | No |
| Wikidata entity by title | `entity_by_title` | No | No | No | No | No | Yes | No |
| Wikipedia page content | `page_content` | No | No | No | No | No | Yes | No |
| Wikipedia structured profile | `page_profile` | No | No | No | No | No | Yes | No |

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

For AVBase and JavBus, `data.actor.image` is enhanced by Gfriends when a
matching actor name exists. If Gfriends has no match or its index cannot be
loaded, the site image is kept.

AVBase and JavBus actor detail also attach a best-effort `data.actor.wikipedia`
object from the bundled Wikipedia config. This object does not replace the
site's own actor fields.

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
      "profile": null,
      "wikipedia": {
        "matched": true,
        "title": "Rikka Ono",
        "wikidata_id": "Q97031495",
        "summary": "...",
        "text": {
          "intro": "...",
          "profile": {},
          "person": "...",
          "external_links": []
        }
      }
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

For JavBus, `image` is enhanced by Gfriends when a matching actor name exists.
If Gfriends has no match or its index cannot be loaded, the site image is kept.
The Gfriends index cache is stored beside the scraper binary at
`cache/gfriends/Filetree.json`.

JavBus actor search also attaches a best-effort `wikipedia` object to each actor
candidate.

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
        "image": "https://...",
        "wikipedia": {
          "matched": true,
          "title": "Rikka Ono",
          "wikidata_id": "Q97031495",
          "summary": "...",
          "text": {
            "intro": "...",
            "profile": {},
            "person": "...",
            "external_links": []
          }
        }
      }
    ]
  }
}
```

## `actor_image`

Look up one actor image from Gfriends without fetching a site page.

Supported configs:

```text
gfriends
```

Parameters:

```text
name required. Actor name.
```

Example:

```bash
./scraper run -config gfriends -task actor_image -param name=Alice
```

When the actor is found, the result contains `data.actor.name` and
`data.actor.image`. When the actor is not found or the Gfriends index cannot be
loaded, the task returns `ok: false` with `error.type` set to `not_found`.

Response shape:

```json
{
  "ok": true,
  "site": "gfriends",
  "task": "actor_image",
  "data": {
    "actor": {
      "name": "Alice",
      "image": "https://..."
    }
  }
}
```

## `page_search`

Search Wikipedia page candidates.

Supported sites:

```text
wikipedia
```

Parameters:

```text
keyword required. Search keyword.
lang    optional. Defaults to zh.
```

Example:

```bash
./scraper run -config wikipedia -task page_search -param keyword='Rikka Ono' -param lang=en
```

Response shape:

```json
{
  "ok": true,
  "site": "wikipedia",
  "task": "page_search",
  "data": [
    {
      "title": "Rikka Ono",
      "pageid": 7407438,
      "snippet": "...",
      "timestamp": "2026-06-14T16:28:45Z"
    }
  ]
}
```

## `page_summary`

Fetch Wikipedia REST summary data for one page title.

Supported sites:

```text
wikipedia
```

Parameters:

```text
title required. Exact page title.
lang  optional. Defaults to zh.
```

Example:

```bash
./scraper run -config wikipedia -task page_summary -param title='Rikka Ono' -param lang=en
```

Response fields include `title`, `pageid`, `lang`, `wikidata_id`,
`description`, `summary`, `thumbnail`, `page_url`, `revision`, and `timestamp`.

## `entity_by_title`

Fetch Wikidata entity fields for a Wikipedia page title.

Supported sites:

```text
wikipedia
```

Parameters:

```text
title required. Exact page title on the selected wiki.
lang  optional. Defaults to zh.
```

Example:

```bash
./scraper run -config wikipedia -task entity_by_title -param title='Rikka Ono' -param lang=en
```

Response fields include labels and descriptions for `zh`, `ja`, and `en`, plus
selected Wikidata claims such as `birth_date`, `birth_place_qid`, `height_cm`,
`country_qid`, `occupation_qid`, social usernames, official website, IMDb id,
Commons category, image filename, and activity period years when available.

## `page_content`

Fetch parsed Wikipedia page content needed for richer actor text fields.

Supported sites:

```text
wikipedia
```

Parameters:

```text
title required. Exact page title.
lang  optional. Defaults to zh.
```

Example:

```bash
./scraper run -config wikipedia -task page_content -param title='Rikka Ono' -param lang=en
```

Response fields:

```json
{
  "title": "Rikka Ono",
  "pageid": 7407438,
  "wikitext": "...",
  "external_links": []
}
```

## `page_profile`

Fetch a structured Wikipedia profile assembled from the summary, Wikidata
entity, and page content tasks.

Supported sites:

```text
wikipedia
```

Parameters:

```text
title required. Exact page title or actor name.
lang  optional. Defaults to zh.
```

Example:

```bash
./scraper run -config wikipedia -task page_profile -param title='Rikka Ono' -param lang=en
```

Response fields include the matched title, URL, Wikidata id, summary text,
language variants, profile fields, social fields, media fields, and parsed
text sections when available.

## Next Step

See [Runtime Fetching](runtime-fetching.md) for cookies, challenge handling, and
FlareSolverr.
