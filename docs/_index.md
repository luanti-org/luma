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

Download the archive for your platform from the [releases page](https://github.com/luanti-org/luma/releases), extract it, and put `luma` (`luma.exe` on Windows) somewhere on your `PATH`:

    luma --dir /path/to/luanti
    luma --flatpak

With [Go](https://go.dev/) installed, luma can also be installed with `go install github.com/luanti-org/luma@latest`, or run from a checkout of the [repository](https://github.com/luanti-org/luma) by using `go run .` in place of `luma`.

## Roadmap

See the [roadmap](https://github.com/luanti-org/luma/blob/main/ROADMAP.md) for what this tool is aiming to do.
