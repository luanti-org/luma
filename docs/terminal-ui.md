---
title: Terminal UI
---

Running luma without a command opens the terminal UI:

    luma --dir /path/to/luanti
    luma --flatpak

The main menu shows the folders and engine version in use, and leads to the mods, games and texture packs screens.

## General keys

| key | action |
|---|---|
| `up` / `k`, `down` / `j` | move |
| `enter` / `space` | select, or open details |
| `esc` / `backspace` | go back |
| `q` | quit |
| `ctrl+c` | quit immediately |

`q` is ignored while certain operations are happening to avoid leaving directories in an invalid state.

## Mods

The mods screen lists the installed mods and modpacks. Above the list are two actions:

* **Check for updates** asks ContentDB which installed mods have a newer release
* **Update all** updates every mod found by the last check

Mods in the list are tagged:

| tag | meaning |
|---|---|
| `[U]` | update available |
| `[P]` | modpack |

Press `u` on a mod tagged `[U]` to update only that one. Updating replaces the mod's folder with the new release.

Installing the dependencies that a new release adds is a work in progress in the terminal UI. For now, the [command line](../cli) `mods update` does it.

Mods with no author in their `mod.conf` (or `modpack.conf`) can't be matched to a ContentDB package, so they are never available for update.

## Games and texture packs

These screens list what is installed, and selecting an entry shows its details.

A `[!]` tag means the entry has no valid `game.conf` or `texture_pack.conf`, so its name is guessed from the folder name.
