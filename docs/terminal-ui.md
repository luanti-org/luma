---
title: Terminal UI
---

Running luma without a command opens the terminal UI:

    luma --dir /path/to/luanti
    luma --flatpak

The main menu shows the folders and engine version in use, and leads to the ContentDB, mods, games and texture packs screens.

## General keys

| key | action |
|---|---|
| `up` / `k`, `down` / `j` | move |
| `enter` / `space` | select, or open details |
| `esc` / `backspace` | go back |
| `q` | quit |
| `ctrl+c` | quit immediately |

`q` is ignored during some operations, so folders aren't left half-changed.

## Browse ContentDB

Lists the ContentDB packages compatible with the engine:

* installed packages first, those with an update at the top, then the rest by popularity
* each with its title, `author/name`, type and short description
* fetched on first opening, or taken from the [cache](../cli#cache) if under 10 minutes old

| tag | meaning |
|---|---|
| `[U]` | installed, update available |
| `[I]` | installed |

| key | action |
|---|---|
| `tab` / `shift+tab`, `1`-`4` | filter by type |
| `pgup` / `pgdown`, `home` / `end` | move by page, to the start or end |
| `r` | fetch the list again |
| `/` | search |

### Searching

Press `/` and type to show only packages whose title, `author/name` or short description contain every typed word, ignoring case. The search applies to every type tab, and the order stays the same.

While typing:

* `up` / `down`, `pgup` / `pgdown` and `tab` / `shift+tab` still work
* `left` / `right` and `home` / `end` move the text cursor
* `backspace` and `delete` delete a character, `ctrl+u` clears the search
* `enter` keeps the search and the other keys work again
* `esc` clears the search

With a kept search, `esc` or `backspace` clears it, pressing it again goes back.

### Package details

Selecting a package shows:

* its install status and newest compatible release
* content warnings
* license, development state, downloads, tags, maintainers
* supported games, for mods
* links to its ContentDB page and source

Press `d` to expand or collapse the full description, and `up` / `down` to scroll it.

### Installing

Press `i` on a package that isn't installed:

* games, texture packs and mods with no dependencies install right away
* other mods first show what will be installed, hard dependencies included, and their missing optional dependencies
* **Base game for dependencies** is the same choice as on the [mods screen](#base-game), changing it in either place changes both
* if any part of the install fails, everything it installed is removed

## Mods

The mods screen lists the installed mods and modpacks, below three actions:

* **Check for updates** asks ContentDB which mods have a newer release
* **Update all** updates every mod found by the last check
* **Base game for dependencies** picks the game the mods are used with

| tag | meaning |
|---|---|
| `[U]` | update available |
| `[P]` | modpack |

Press `u` on a mod tagged `[U]` to update only that one. Updating replaces the mod's folder with the new release.

Never available for update:

* mods with no author in their `mod.conf` (or `modpack.conf`), as they can't be matched to a ContentDB package
* git checkouts (a `.git` inside) with no `release` in their `mod.conf`, as updating would replace the working copy

### Dependencies when updating

Before updating, luma asks ContentDB which mods the new releases depend on, hard dependencies only. If all are installed, the update starts right away. Otherwise a confirmation screen lists:

* the packages to install, with the mod each provides and what needs it
* the dependencies no ContentDB package was found for
* the mods to update

**Install missing dependencies** is checked by default, uncheck it to update without installing anything new. **Continue** starts the update, **Cancel** or `esc` goes back with nothing changed.

If the dependencies can't be looked up, for example because ContentDB is unreachable, the screen shows the error and offers **Update without dependencies** instead.

Dependencies are installed first, then the mods are updated. A dependency never replaces a folder that already exists in the mods folder.

### Base game

Many mods depend on mods that are part of a game, such as `default` in Minetest Game. **Base game for dependencies** picks one of the installed games, whose mods then count as installed, so they are neither installed again nor listed as not found.

With `<None>`, only the mods folder counts. The choice lasts until luma is closed.

### Mod details

Selecting a mod shows its title, author, description, folder, path and the dependencies in its `mod.conf`. A modpack lists its mods instead, and selecting one shows its details. A mod tagged `[!]` has no valid `mod.conf`, and its name is guessed from the folder name.

Press `d` to open the dependencies screen of the mod or modpack.

### Dependencies screen

Lists the required dependencies of a mod, then the optional ones, as ContentDB gives them for its latest release, each with a status:

| status | meaning |
|---|---|
| `installed` | a mod of that name is installed |
| `available` | not installed, the package shown would provide it |
| `from game` | not installed, only games on ContentDB provide it |
| `not found` | no package compatible with the engine provides it |
| `ambiguous` | not installed, several packages provide it and none was picked (see below) |

The mods of the [base game](#base-game) count as installed. A hint at the bottom explains `from game` and `ambiguous` when they show.

* `available` dependencies have a checkbox, required ones start checked and optional ones unchecked. `enter` or `space` toggles one
* **Install selected** installs the checked ones with their own required dependencies
* if that pulls in more packages, or some of their dependencies can't be found, the confirmation screen from [updating](#dependencies-when-updating) lists everything first. Otherwise the install starts right away
* when done, the list is refreshed, and the screen shows how many were installed and why any failed
* as when updating, a dependency never replaces a folder that already exists in the mods folder

A mod with no author in its `mod.conf`, or with no compatible package on ContentDB, can't be looked up itself. Its dependencies are then listed from its `mod.conf`, or for a modpack from those of its mods, leaving out mods the pack provides itself, and the screen says which case applies. Their packages are found as for a [local package](../cli#local-packages) on the command line, and only such a dependency can be `ambiguous`. The first time, this fetches a list of every mod and game on ContentDB, which takes a while.

## Games

The games screen lists the installed games, below two actions:

* **Check for updates** asks ContentDB which games have a newer release
* **Update all** updates every game found by the last check

| tag | meaning |
|---|---|
| `[U]` | update available |
| `[!]` | no valid `game.conf`, title guessed from the folder name |

Press `u` on a game tagged `[U]` to update only that one. Updating replaces the game's folder with the new release, without asking.

Games with no author or release in their `game.conf` can't be matched to a ContentDB package, so they are never available for update. The exception is a Minetest Game without a release, as bundled with engines up to 5.8, unless it is a git checkout.

Only one check or update runs at a time, across mods, games and texture packs.

## Texture packs

The texture packs screen lists the installed texture packs, below two actions:

* **Check for updates** asks ContentDB which texture packs have a newer release
* **Update all** updates every texture pack found by the last check

| tag | meaning |
|---|---|
| `[U]` | update available |
| `[!]` | no valid `texture_pack.conf`, name guessed from the folder name |

Press `u` on a texture pack tagged `[U]` to update only that one. Updating replaces the texture pack's folder with the new release, without asking.

Texture packs with no author or release in their `texture_pack.conf` can't be matched to a ContentDB package, so they are never available for update.
