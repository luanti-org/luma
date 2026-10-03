---
title: Luma (tool)
bookCollapseSection: true
---

> [!WARNING]
> This tool is a work in progress. Expect missing features, rough edges, changes and docs being out of date until completion.

Luma manages the mods, games and texture packs of a Luanti install, with ContentDB integration. Future work includes managing worlds too.

It has two modes:

* a [terminal UI](terminal-ui), opened when no command is given
* a [command line](cli), for scripts and one-off commands

Both need to know which Luanti install to work on, see [Global flags](global-flags).

## Running

No binaries are released yet, so [Go](https://go.dev/) is needed to run luma from a checkout of the [repository](https://github.com/luanti-org/luma):

    go run . --dir /path/to/luanti
    go run . --flatpak

The rest of these pages write `luma` for the command. Until a binary is available, use `go run .` in its place.

## Roadmap

See the [roadmap](https://github.com/luanti-org/luma/blob/main/ROADMAP.md) for what this tool is aiming to do.
