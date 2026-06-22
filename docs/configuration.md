# YAML Configuration

This page describes the YAML model used by site adapters. Read
[Runtime Fetching](runtime-fetching.md) first if you need cookies, timeouts, or
challenge handling. Continue to the [Site Adapter Guide](site-adapters.md) when
you are adding or changing a site.

Bundled site configs live under:

```text
configs/
```

They are embedded by Go `embed` and included in the binary. Current bundled
configs:

```text
configs/avbase.yml
configs/fc2.yml
configs/javbus.yml
configs/javlibrary.yml
```

To add a bundled site, place the real site YAML in `configs/`, then rebuild.

## Top-Level Shape

```yaml
site:
  id: example
  base_url: https://example.test

defaults:
  cookie: "age=verified"
  headers:
    User-Agent: Mozilla/5.0

indexes:
  actors:
    path: indexes/example_actors.json
    items_key: actors
    match_field: name
    value_field: id

tasks:
  search_work:
    params: {}
    request: {}
    extract: {}
    pagination: {}
    output: {}
```

`site.id`, `site.base_url`, and at least one task are required.

## Defaults

Site-level defaults include optional headers and cookies:

```yaml
defaults:
  cookie: "age=verified; dv=1; existmag=all"
  headers:
    User-Agent: Mozilla/5.0
```

`defaults.cookie` is used only when no runtime cookie is passed by `-cookie` or
`SCRAPER_COOKIE`.

## Task Parameters

Task parameters declare required values, defaults, and optional regex
normalization:

```yaml
params:
  code:
    required: true
    regex: "(?i)^(?:fc2-ppv-|fc2-|ppv-)?([0-9]+)$"
    regex_group: 1
  page:
    default: "1"
```

When `regex` is set, the matched group replaces the raw parameter value before
request templates are rendered. If `regex_group` is not set and the regex has a
capture group, group 1 is used.

## Indexed Parameter Resolution

Use `resolve_params` when a public parameter needs to be translated through a
local JSON index:

```yaml
indexes:
  actors:
    path: indexes/javbus_actors.json
    items_key: actors
    match_field: name
    value_field: id

tasks:
  actor_detail:
    params:
      name:
        required: true
      id: {}
    resolve_params:
      id:
        index: actors
        from: name
```

The runner resolves `id` from `name` before rendering the request.

## Requests

Request-level options include method, URL, path, query, and accepted statuses:

```yaml
request:
  method: GET
  path: /search/{keyword}
  query:
    page: "{page}"
  accept_status: [404]
```

Use `url` for a full URL template, or `path` for a path resolved against
`site.base_url`.

`accept_status` is optional. Use it when a site returns a meaningful HTML page
with a non-2xx status, such as an empty search result page.

## Extraction

Extraction runs in three layers:

- `extract.meta`: evaluated once against the whole page and emitted as result
  metadata.
- `extract.page`: evaluated once against the whole page and made available to
  `output.page_format`.
- `extract.fields`: evaluated for each item node.

`scope.xpath` selects repeated item nodes. Without a scope, the whole document is
treated as a single item.

```yaml
extract:
  meta:
    total:
      xpath: "//script[@id='__NEXT_DATA__']"
      attr: text
      regex: "\"total\":(\\d+)"
      type: int
  scope:
    xpath: "//div[contains(@class, 'work')]"
  fields:
    title:
      xpath: ".//a[contains(@class, 'title')]"
      attr: text
      trim: true
      on_missing: skip_item
```

## Field Options

Supported field options:

```yaml
xpath: string
attr: text | html | outer_html | attribute-name
regex: string
regex_group: int
type: string | int
trim: true | false
multiple: true | false
required: true | false
default: string
on_missing: null | error | skip_item
resolve_url: true | false
```

Notes:

- `resolve_url: true` resolves relative URLs against the final response URL.
- `multiple: true` returns a list.
- When `multiple: true` and `regex` are both set, every regex match inside each
  selected node is returned. This is useful for fields such as `magnet_links`.
- `required: true` is equivalent to `on_missing: error`.
- `on_missing: skip_item` skips the current item. For `extract.page`, it clears
  page-level fields.

## Pagination

Pagination is single-page only. The runner sets a default page parameter and
adds `count`, `page`, and optionally `total` to result metadata.

```yaml
pagination:
  param: page
  default: "1"
  total_field: total
```

## Output

`output.format` maps extracted fields to the final JSON item:

```yaml
output:
  type: list
  format:
    title: "{title}"
    url: "{url}"
```

`output.items_key` wraps repeated items in an object response. `extract.page`
and `output.page_format` can add page-level objects next to those repeated
items:

```yaml
output:
  type: object
  items_key: works
  page_format:
    actor:
      name: "{name}"
      image: "{image}"
  format:
    title: "{title}"
    url: "{url}"
```

## Templates

Templates use `{name}` placeholders. A placeholder that fills the whole string
keeps the original value type; a placeholder inside a larger string is
stringified.

Environment variables can be read with `{env.NAME}`.

## Enhancements

Tasks can optionally enrich formatted output after extraction. Current
enhancement support is limited to actor images from Gfriends:

```yaml
enhance:
  actor_image:
    source: gfriends
    items_key: actors
    name_field: name
    image_field: image
```

When enabled, the runner looks up each actor by `name_field` and replaces
`image_field` with a Gfriends image URL when one is found. `items_key` can point
to either a list of actors, such as `actors`, or a single actor object, such as
`actor`. If Gfriends is unavailable or has no match, the original site image is
kept.

The Gfriends `Filetree.json` index is cached next to the scraper binary at:

```text
cache/gfriends/Filetree.json
```

Only the index is cached; image files are returned as remote URLs.

## Next Step

See the [Site Adapter Guide](site-adapters.md) for a practical workflow.
