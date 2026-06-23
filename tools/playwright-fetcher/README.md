# Playwright Fetcher

This is the scraper HTTP fetch service used by `-playwright`.

It runs a browser inside the same container and exposes:

```text
POST /fetch
GET  /health
```

## Build

From this directory:

```bash
./build-docker.sh --local --tag local
```

This uses normal `docker build` and reuses the local Docker/OrbStack cache.

For a GHCR multi-architecture image:

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u Yachiyo-5i --password-stdin
./build-docker.sh
```

By default this builds and pushes:

```text
ghcr.io/<git-remote-owner>/yachiyo-playwright-fetcher:latest
```

for:

```text
linux/amd64,linux/arm64
```

The OCI author label is read from the repository git account name:

```bash
git config user.name
```

Override the owner or tag when needed:

```bash
GHCR_OWNER=Yachiyo-5i IMAGE_TAG=2026-06-24 ./build-docker.sh
```

The default push uses a GHCR registry cache at `:buildcache`. Disable it with
`--no-cache-registry` if needed.

## Run With Compose

From this directory:

```bash
docker compose up -d --build
```

The local endpoint is:

```text
http://127.0.0.1:3011
```

Use it with the scraper:

```bash
./dist/scraper_darwin_arm64 run \
  -config sehuatang \
  -task forum_threads \
  -param category=高清中文字幕 \
  -param page=1 \
  -playwright http://127.0.0.1:3011
```

In a production compose network, pass the service URL instead:

```text
http://playwright-fetcher:3001
```
