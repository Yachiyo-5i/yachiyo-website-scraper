# Site Adapter Guide

This guide describes how to add or maintain a site adapter. Read
[YAML Configuration](configuration.md) first for the config model. Use the
[Development Guide](development.md) for tests, validation, and release steps.

## Adapter Goal

A site adapter should preserve the unified task contract from the
[Task Reference](tasks.md). Prefer stable public parameters and output keys even
when a site uses different internal routes or identifiers.

## Workflow

1. Identify which unified tasks the site can support.
2. Capture representative static HTML for each page type.
3. Draft or update `configs/<site>.yml`.
4. Validate the config.
5. Run task smoke checks with `-dump-html` when selectors are uncertain.
6. Add or update tests for parser behavior that is easy to regress.
7. Rebuild if the config should be embedded in the binary.

## Map Capabilities to Tasks

Use these task names when possible:

- `search_work`
- `work_detail`
- `actor_detail`
- `actor_search`

If a site cannot support a task, omit it. Do not add a site-specific public task
unless the generic model cannot represent the capability.

## Choose Stable Parameters

Keep public parameter names aligned with existing sites:

- `code` for work search and work detail.
- `page` for single-page pagination.
- `name` for actor detail.
- `keyword` for actor candidate search.

Use `params.regex` to normalize site-specific input forms while keeping the
public interface stable.

## Select Routes and Status Handling

Prefer `request.path` when the route belongs to `site.base_url`:

```yaml
request:
  method: GET
  path: /works/{code}
```

Use `request.url` only for full URLs that cannot be expressed as a path relative
to `site.base_url`.

If a site returns useful HTML with a non-2xx status, declare it explicitly:

```yaml
request:
  accept_status: [404]
```

## Write Extractors

Use `scope.xpath` for repeated items and relative field XPath expressions inside
that scope:

```yaml
extract:
  scope:
    xpath: "//div[contains(@class, 'work')]"
  fields:
    title:
      xpath: ".//a[contains(@class, 'title')]"
      attr: text
      trim: true
      on_missing: skip_item
```

Use `extract.meta` for page counts and pagination hints. Use `extract.page` when
the final output needs a page-level object such as `data.actor`.

Prefer `on_missing: skip_item` for item fields that define whether an item is
valid. Use `default` or the default `null` behavior for optional fields.

## Resolve URLs

Use `resolve_url: true` for links and image URLs that may be relative:

```yaml
url:
  xpath: ".//a"
  attr: href
  resolve_url: true
```

The extractor resolves relative URLs against the final response URL, not just
the configured base URL.

## Use Indexes When Needed

Use a JSON index when a task accepts a human-readable parameter but the site
requires an internal ID:

```yaml
indexes:
  actors:
    path: indexes/example_actors.json
    items_key: actors
    match_field: name
    value_field: id
```

Then resolve the private request parameter:

```yaml
resolve_params:
  id:
    index: actors
    from: name
```

Build index data with:

```bash
./scraper index build -config example -task actor_list -out indexes/example_actors.json
```

Index-building runtime flags are described in [Runtime Fetching](runtime-fetching.md).

## Validate and Smoke Test

Run these checks while developing:

```bash
./scraper validate -config ./configs/example.yml
./scraper tasks -config ./configs/example.yml
./scraper run -config ./configs/example.yml -task search_work -param code=SSIS-001 -dump-html ./debug.html
```

If the site needs challenge bypassing, add the runtime flags from
[Runtime Fetching](runtime-fetching.md).

## Keep Docs and Tests Together

When adding a bundled site:

- Update the support table in [Task Reference](tasks.md).
- Add site-specific runtime notes in [Runtime Fetching](runtime-fetching.md).
- Add tests for any selector behavior that is not obvious from existing tests.
- Run the development checks from [Development Guide](development.md).

## Next Step

See the [Development Guide](development.md) for local checks and release builds.
