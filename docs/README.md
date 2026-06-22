# Documentation

This directory contains the detailed documentation for
`yachiyo-website-scraper`. The root [README](../README.md) is intentionally
short; use this page to choose the next document based on what you are doing.

## Choose a Guide

New users should start with the [Usage Guide](usage.md). It covers building the
binary, selecting configs, running tasks, reading JSON output, dumping HTML, and
using index commands.

When you need task names, supported sites, parameters, or response shapes, read
the [Task Reference](tasks.md). It defines the public contract that site
adapters should preserve.

When a site needs cookies, anti-bot detection, FlareSolverr, custom timeouts, or
HTML dumps, read [Runtime Fetching](runtime-fetching.md).

When editing YAML files under `configs/`, read
[YAML Configuration](configuration.md). It explains defaults, parameters,
requests, extraction fields, pagination, output formatting, templates, and
indexes.

When adding or maintaining a site adapter, read the
[Site Adapter Guide](site-adapters.md). It turns the config model into a
practical workflow and links back to the task contract.

When changing Go code, tests, release behavior, or documentation structure, read
the [Development Guide](development.md).

## Suggested Flow

```text
Usage Guide
  -> Task Reference
  -> Runtime Fetching
  -> YAML Configuration
  -> Site Adapter Guide
  -> Development Guide
```
