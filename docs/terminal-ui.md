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

The mods screen lists the installed mods and modpacks. Above the list are three actions:

* **Check for updates** asks ContentDB which installed mods have a newer release
* **Update all** updates every mod found by the last check
* **Base game for dependencies** picks the game the mods are used with

Mods in the list are tagged:

| tag | meaning |
|---|---|
| `[U]` | update available |
| `[P]` | modpack |

Press `u` on a mod tagged `[U]` to update only that one. Updating replaces the mod's folder with the new release.

### Dependencies

Before updating, luma asks ContentDB which mods the new releases depend on. Only hard dependencies are considered. If all of them are already installed, the update starts right away.

Otherwise a confirmation screen lists:

* the packages that would be installed, with the mod each one provides and what needs it
* the dependencies no ContentDB package could be found for
* the mods to update

**Install missing dependencies** is checked by default. Uncheck it to update without installing anything new. **Continue** starts the update, and **Cancel** or `esc` goes back with nothing changed.

If the dependencies cannot be looked up, for example because ContentDB is unreachable, the screen shows the error and offers **Update without dependencies** instead.

Dependencies are installed first, then the mods are updated. A dependency never replaces a folder that already exists in the mods folder.

### Base game

Many mods depend on mods that are part of a game, such as `default` in Minetest Game. Select **Base game for dependencies** to choose from the installed games. The mods of the chosen game then count as installed, so they are neither installed again nor listed as not found.

With `<None>`, only the mods folder is considered. The choice lasts until luma is closed.

Mods with no author in their `mod.conf` (or `modpack.conf`) can't be matched to a ContentDB package, so they are never available for update.

## Games and texture packs

These screens list what is installed, and selecting an entry shows its details.

A `[!]` tag means the entry has no valid `game.conf` or `texture_pack.conf`, so its name is guessed from the folder name.
