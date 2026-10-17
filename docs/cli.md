---
title: Command line
---

Passing a command runs it without opening the terminal UI. Commands take the form `<noun> <verb>`:

    luma [global flags] <noun> <verb> [flags] [arguments]

Global flags, such as `--dir`, go before the command. See [Global flags](../global-flags) for the full list.

    luma --dir /path/to/luanti mods list
    luma --flatpak mods outdated

## Help

| command | shows |
|---|---|
| `luma --help` | global flags and command groups |
| `luma mods --help` | the commands of a group |
| `luma mods update --help` | the flags of a command |

## Exit codes

| code | meaning |
|---|---|
| 0 | success |
| 1 | error |
| 2 | usage error, e.g. unknown command or flag |

Results are written to standard output, errors and warnings to standard error.

## mods

### mods list

Lists the installed mods with their author and release number. Modpacks are tagged `[P]`, and `-` stands for a value the mod doesn't declare.

    luma mods list
    luma mods list --names

`--names` prints only the mod names, one per line.

### mods outdated

Lists the mods that have a newer release on ContentDB, with their current and latest release numbers.

    luma mods outdated
    luma mods outdated --names

Takes `--names` too.

Mods with no author in their `mod.conf` can't be matched to a ContentDB package and are not checked. A mod whose folder is a git checkout (it has a `.git` inside) and has no `release` in its `mod.conf` is never listed, since updating would replace the working copy. ContentDB installs always have a `release`, so a mod with one is checked even if it has a `.git`.

### mods update

Updates the named mods, or all outdated mods if none are named. Updating replaces the mod's folder with the new release.

    luma mods update                # update all outdated mods
    luma mods update foo bar        # update only foo and bar
    luma mods update -x foo,bar     # update all except foo and bar
    luma mods update -n             # show what would be updated
    luma mods update --game mineclonia   # dependencies mineclonia provides are not installed

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be updated and installed without changing anything |
| `-x`, `--exclude` | comma-separated mod names to skip |
| `-y`, `--yes` | install new dependencies without asking |
| `--no-deps` | don't install new dependencies of the updated mods |
| `--game` | count this game's mods as installed when resolving dependencies |

Naming a mod that is not installed is an error, and nothing is updated. The same goes for a name in `--exclude` that is neither installed nor a new dependency.

Named mods with no update, with no author in their `mod.conf`, or that are git checkouts with no release, are reported and skipped.

#### Dependencies

If the new release of a mod needs mods that are not installed, luma installs those too, along with their own dependencies. Only hard dependencies are installed, never optional ones, see [mods install-deps](#mods-install-deps) for those. `--dry-run` lists them, and `--no-deps` turns this off.

Before installing anything new, luma shows what it will update and install, and asks once whether to continue. Answering anything but `y` or `yes` stops the run with nothing changed. `-y` skips the question. There is no question when no new dependencies are needed, so plain updates run unattended.

When luma is not run from a terminal, such as in a script, it cannot ask. It then stops with an error if new dependencies are needed, unless `-y` or `--no-deps` is given.

A dependency counts as installed if a mod of that name exists in the mods folder, on its own or inside a modpack. Mods that a game provides are not counted unless the game is given with `--game`, which takes the game's folder name. Without it, luma installs such a dependency from ContentDB.

When several packages provide a dependency, luma picks one the same way the Luanti client does: a package named like the dependency, or else the first one ContentDB lists. A package that is already part of the same run is preferred over both. Games are never picked.

To skip a dependency, put its mod name or its package name in `--exclude`. A dependency that is excluded, or that no package compatible with the engine provides, is reported, and the update goes ahead without it.

A dependency is installed into a folder named after its package. If that folder already exists, the dependency is not installed and is counted as a failure.

If an update or a dependency install fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed.

### mods list-deps

Lists the required dependencies of an installed mod, or its optional ones with `--optional`, in the order ContentDB gives them, and whether each is installed.

    luma mods list-deps mesecons
    luma mods list-deps --optional mesecons
    luma mods list-deps --game minetest_game mesecons
    luma mods list-deps --names mesecons

| status | meaning |
|---|---|
| `installed` | a mod of that name is installed |
| `available` | not installed, the package shown would provide it |
| `from game` | not installed, only games on ContentDB provide it |
| `not found` | no package compatible with the engine provides it |
| `missing` | not installed, and not looked up on ContentDB (see below) |

| flag | meaning |
|---|---|
| `--optional` | list the optional dependencies instead of the required ones |
| `--names` | print only the dependency names, one per line |
| `--game` | count this game's mods as installed |

A dependency counts as installed the same way as for [mods update](#dependencies), and the package shown is picked the same way too. Dependencies a game provides show as `from game` unless the game is given with `--game`.

A mod with no author in its `mod.conf`, or with no compatible package on ContentDB, can't be looked up. Its dependencies are then listed from its `mod.conf`, or for a modpack from those of its mods, and luma says so on stderr. Those not installed show as `missing`.

### mods install-deps

Installs the missing required dependencies of an installed mod, or its `available` optional ones with `--optional`, along with their own required dependencies.

    luma mods install-deps mesecons                # install missing required ones
    luma mods install-deps --optional mesecons     # install available optional ones
    luma mods install-deps -n mesecons             # show what would be installed

| flag | meaning |
|---|---|
| `--optional` | install the optional dependencies instead of the required ones |
| `-n`, `--dry-run` | show what would be installed without changing anything |
| `-y`, `--yes` | install without asking |
| `--game` | count this game's mods as installed |

Before installing, luma shows what it will install and asks whether to continue, as `mods update` does for new dependencies. `-y` skips the question, and when luma is not run from a terminal it stops with an error unless `-y` is given.

Required dependencies that can't be installed, because only games or no compatible package provide them, are reported. The rest are still installed, and luma then exits with code 1. This includes every missing dependency of a mod that can't be looked up on ContentDB, see [mods list-deps](#mods-list-deps). Optional dependencies that can't be installed are skipped quietly.

Dependencies are installed into folders named after their packages, as with [mods update](#dependencies). If an install fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed.

## games

Games are named by their folder name in the games folder, such as `minetest_game`.

### games list

Lists the installed games with their author, release number and title. `-` stands for a value the game doesn't declare.

    luma games list
    luma games list --names

`--names` prints only the folder names, one per line.

### games outdated

Lists the games that have a newer release on ContentDB, with their current and latest release numbers.

    luma games outdated
    luma games outdated --names

Takes `--names` too.

Only games installed from ContentDB are checked, which means those with both an `author` and a `release` in their `game.conf`. As in the Luanti client, the one exception is a Minetest Game without a release, as bundled with engines up to 5.8. It counts as outdated unless it is a git checkout.

### games update

Updates the named games, or all outdated games if none are named. Updating replaces the game's folder with the new release, so changes made inside that folder are lost. Worlds are not affected.

    luma games update                   # update all outdated games
    luma games update mineclonia        # update only mineclonia
    luma games update -x mineclonia     # update all except mineclonia
    luma games update -n                # show what would be updated

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be updated without changing anything |
| `-x`, `--exclude` | comma-separated game folder names to skip |

Games have no dependencies to install, so there is nothing to confirm.

Naming a game that is not installed, as an argument or in `--exclude`, is an error, and nothing is updated.

Named games with no update, or with no author or release in their `game.conf`, are reported and skipped.

If an update fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed.

## texturepacks

Texture packs are named by the `name` in their `texture_pack.conf`, or by their folder name if there is none. The `base` folder in the textures folder holds the engine's own textures and is never listed.

`textures` is accepted as a shorter name for `texturepacks`, so `luma textures list` works too.

### texturepacks list

Lists the installed texture packs with their author, release number and title. `-` stands for a value the texture pack doesn't declare.

    luma texturepacks list
    luma texturepacks list --names

`--names` prints only the names, one per line.

### texturepacks outdated

Lists the texture packs that have a newer release on ContentDB, with their current and latest release numbers.

    luma texturepacks outdated
    luma texturepacks outdated --names

Takes `--names` too.

Only texture packs installed from ContentDB are checked, which means those with both an `author` and a `release` in their `texture_pack.conf`.

### texturepacks update

Updates the named texture packs, or all outdated texture packs if none are named. Updating replaces the texture pack's folder with the new release, so changes made inside that folder are lost.

    luma texturepacks update                   # update all outdated texture packs
    luma texturepacks update soothing32        # update only soothing32
    luma texturepacks update -x soothing32     # update all except soothing32
    luma texturepacks update -n                # show what would be updated

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be updated without changing anything |
| `-x`, `--exclude` | comma-separated texture pack names to skip |

Texture packs have no dependencies to install, so there is nothing to confirm.

Naming a texture pack that is not installed, as an argument or in `--exclude`, is an error, and nothing is updated.

Named texture packs with no update, or with no author or release in their `texture_pack.conf`, are reported and skipped.

If an update fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed.
