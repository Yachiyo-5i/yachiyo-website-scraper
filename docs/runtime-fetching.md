# Runtime Fetching

This page explains runtime fetch behavior. Read the [Task Reference](tasks.md)
first for task parameters. Continue to [YAML Configuration](configuration.md)
when you need to understand how runtime options interact with site configs.

Challenge behavior and FlareSolverr endpoints are not stored in site YAML.
Non-account cookies can be stored as `defaults.cookie`; runtime cookies still
take precedence.

Pass `-playwright` when a site needs a real browser session. Unlike
FlareSolverr, a Playwright URL selects the browser fetch channel directly; the
CLI does not try normal HTTP first.

## Flags

```text
-cookie string
    Cookie header value, overrides defaults.cookie
-challenge string
    Challenge handling: detect, bypass, off
-flaresolverr string
    FlareSolverr base URL
-playwright string
    Playwright fetch service base URL
-timeout duration
    HTTP timeout, default 30s
-flaresolverr-timeout duration
    FlareSolverr timeout, default 60s
-playwright-timeout duration
    Playwright fetch timeout, default 60s
-dump-html string
    Write fetched HTML to a file before extraction
```

Example:

```bash
./scraper run \
  -config avbase \
  -task search_work \
  -param code=SSIS-001 \
  -challenge bypass \
  -flaresolverr http://127.0.0.1:8191
```

Playwright fetching can use the optional helper under
`tools/playwright-fetcher/`.

## Environment Variables

```text
SCRAPER_COOKIE
SCRAPER_CHALLENGE
SCRAPER_FLARESOLVERR_URL
SCRAPER_PLAYWRIGHT_URL
SCRAPER_TIMEOUT
SCRAPER_FLARESOLVERR_TIMEOUT
SCRAPER_PLAYWRIGHT_TIMEOUT
```

Example:

```bash
export SCRAPER_CHALLENGE=bypass
export SCRAPER_FLARESOLVERR_URL=http://127.0.0.1:8191

./scraper run -config avbase -task search_work -param code=SSIS-001
```

## Challenge Modes

- `detect`: detect Cloudflare or anti-bot challenge pages and return a blocked
  result.
- `bypass`: use FlareSolverr when a challenge is detected.
- `off`: skip challenge detection and process the raw HTTP response.

Use `detect` when you want a clear structured error instead of attempting a
bypass. Use `bypass` only when a FlareSolverr service is available.

When `-playwright` or `SCRAPER_PLAYWRIGHT_URL` is set, the runner fetches the
page through the Playwright service for all challenge modes except that `off`
skips challenge detection on the returned browser HTML.

## Playwright Helper

The optional Playwright HTTP helper lives under `tools/playwright-fetcher/`.
That directory contains its Dockerfile, compose file, and usage notes. Use its
HTTP API URL with `-playwright`; the scraper does not depend on the helper unless
that runtime flag or `SCRAPER_PLAYWRIGHT_URL` is set.

## Cookie Precedence

Cookie resolution order:

1. `-cookie`
2. `SCRAPER_COOKIE`
3. `defaults.cookie` from YAML
4. No cookie

Runtime cookies override the YAML default. This lets a bundled config include a
non-account verification cookie while still allowing callers to provide their
own cookie.

## Site Fetch Notes

| Site | Runtime note |
| --- | --- |
| `avbase` | Usually needs `-challenge bypass -flaresolverr http://127.0.0.1:8191` or a valid cookie |
| `javbus` | Includes a non-account default cookie for age and region verification; runtime `-cookie` overrides it |
| `javlibrary` | Usually needs `-challenge bypass -flaresolverr http://127.0.0.1:8191` or a valid cookie |
| `fc2` | Article pages are static enough for normal HTTP fetch in current checks; age modal content may still be present in HTML |
| `sehuatang` | Use `-playwright http://127.0.0.1:3011` for stable browser fetching and category pages |

## Debugging Fetches

Use `-dump-html` to save the fetched response before extraction:

```bash
./scraper run -config javlibrary -task search_work -param code=SSIS-001 -dump-html ./debug.html
```

Then update selectors using the extraction model in
[YAML Configuration](configuration.md).

## Next Step

See [YAML Configuration](configuration.md) for the site config model.
