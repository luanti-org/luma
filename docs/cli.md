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

Named mods with no update, or with no author in their `mod.conf`, are reported and skipped.

#### Dependencies

If the new release of a mod needs mods that are not installed, luma installs those too, along with their own dependencies. Only hard dependencies are installed, never optional ones. `--dry-run` lists them, and `--no-deps` turns this off.

Before installing anything new, luma shows what it will update and install, and asks once whether to continue. Answering anything but `y` or `yes` stops the run with nothing changed. `-y` skips the question. There is no question when no new dependencies are needed, so plain updates run unattended.

When luma is not run from a terminal, such as in a script, it cannot ask. It then stops with an error if new dependencies are needed, unless `-y` or `--no-deps` is given.

A dependency counts as installed if a mod of that name exists in the mods folder, on its own or inside a modpack. Mods that a game provides are not counted unless the game is given with `--game`, which takes the game's folder name. Without it, luma installs such a dependency from ContentDB.

When several packages provide a dependency, luma picks one the same way the Luanti client does: a package named like the dependency, or else the first one ContentDB lists. A package that is already part of the same run is preferred over both. Games are never picked.

To skip a dependency, put its mod name or its package name in `--exclude`. A dependency that is excluded, or that no package compatible with the engine provides, is reported, and the update goes ahead without it.

A dependency is installed into a folder named after its package. If that folder already exists, the dependency is not installed and is counted as a failure.

If an update or a dependency install fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed.

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
