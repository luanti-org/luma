---
title: Global flags
---

Global flags apply to both the terminal UI and the command line. They go before any command:

| flag | meaning |
|---|---|
| `--dir <path>`, `-d <path>` | use the Luanti install at `<path>`, defaults to the current directory |
| `--flatpak` | use the Luanti flatpak install |
| `--engine-version <version>`, `-e <version>` | override the detected engine version |

`--dir` and `--flatpak` can't be used together. If no install is found, luma exits with an error.

## No flags

Without `--dir` or `--flatpak`, the current directory is assumed to be the Luanti install, so luma can be run with no arguments from inside it:

    cd /path/to/luanti
    luma

## --dir

The path is the Luanti folder, the one that holds `bin/`, `builtin/`, `mods/` and `games/`. The engine binary is looked up in `bin/`.

Content is read from the install folder for `RUN_IN_PLACE` builds, otherwise from Luanti's user data folder.

A folder with no engine binary still works if it has content folders, but may need `--engine-version`.

## --flatpak

Requires the `flatpak` command and the Luanti flatpak to be installed.

## --engine-version

Don't use this flag unless there is no engine binary and you know what you're doing. The engine version is normally detected automatically from the binary and installed files.

The version is used to ask ContentDB for compatible releases, and is resolved in this order:

1. `--engine-version`, if given
2. the output of the engine binary run with `--version`
3. the newest entry in `builtin/game/misc_s.lua`, with a warning

If none of these work, luma exits with an error asking for `--engine-version`.

    luma -e 5.17 --dir /path/to/luanti
    luma -e 5.18-dev --flatpak

The version must be one ContentDB knows. If it differs from the detected version, luma prints a warning and uses the override. Content picked for the wrong engine version may not load.
