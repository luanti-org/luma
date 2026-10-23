---
title: Global flags
---

Global flags apply to both the terminal UI and the command line. They go before any command:

| flag | meaning |
|---|---|
| `--dir <path>`, `-d <path>` | use the Luanti install at `<path>`, defaults to [`LUMA_LUANTI_INSTALL_DIR`](#luma_luanti_install_dir) if set, else the current directory |
| `--flatpak` | use the Luanti flatpak install |
| `--engine-version <version>`, `-e <version>` | override the detected engine version |
| `--version` | print the luma version and exit |

`--dir` and `--flatpak` can't be used together. If no install is found, luma exits with an error.

## No flags

Without `--dir` or `--flatpak`, the install is the one in [`LUMA_LUANTI_INSTALL_DIR`](#luma_luanti_install_dir) if set, else the current directory, so luma can be run with no arguments from inside it:

    cd /path/to/luanti
    luma

## LUMA_LUANTI_INSTALL_DIR

Sets the install to use when neither `--dir` nor `--flatpak` is given, so luma can be run from anywhere:

    export LUMA_LUANTI_INSTALL_DIR=/path/to/luanti   # as --dir /path/to/luanti
    export LUMA_LUANTI_INSTALL_DIR=FLATPAK           # as --flatpak

* a relative path is relative to the current directory. A folder named `FLATPAK` must be given as `./FLATPAK`
* it is used even when the current directory holds an install, pass `--dir .` to use that one instead

## --dir

The path is the Luanti folder, the one that holds `bin/`, `builtin/`, `mods/` and `games/`. The engine binary is looked up in `bin/`, or in `bin/Release/` and the other CMake build config folders.

Content is read from the install folder for `RUN_IN_PLACE` builds, otherwise from Luanti's user data folder, or `LUANTI_USER_PATH` if set.

A folder with no engine binary still works if it has content folders, but may need `--engine-version`.

## --flatpak

Requires the `flatpak` command and the Luanti flatpak (`org.luanti.luanti`) to be installed.

## --engine-version

Don't use this flag unless there is no engine binary and you know what you're doing. The engine version is normally detected automatically from the binary and installed files.

The version is used to ask ContentDB for compatible releases, and is resolved in this order:

1. `--engine-version`, if given
2. the output of the engine binary run with `--version`
3. except with `--flatpak`, the newest entry in `builtin/game/misc_s.lua`, with a warning

If none of these work, luma exits with an error asking for `--engine-version`.

    luma -e 5.17 --dir /path/to/luanti
    luma -e 5.18-dev --flatpak

The version must be one ContentDB knows. If its major.minor differs from the detected version, luma prints a warning and uses the override. Content picked for the wrong engine version may not load.
