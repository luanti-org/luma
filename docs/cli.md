---
title: Command line
---

Passing a command runs it without opening the terminal UI. Commands for installed content are `<noun> <verb>`, [install](#install) has no noun:

    luma [global flags] <noun> <verb> [flags] [arguments]
    luma [global flags] install [flags] [arguments]

Global flags such as `--dir` go before the command, see [Global flags](../global-flags):

    luma --dir /path/to/luanti mods list
    luma --flatpak mods outdated

## Help

| command | shows |
|---|---|
| `luma --help` | global flags, commands and command groups |
| `luma mods --help` | the commands of a group |
| `luma mods update --help`, `luma install --help` | the flags of a command |

## Exit codes

| code | meaning |
|---|---|
| 0 | success |
| 1 | error |
| 2 | usage error, e.g. unknown command or flag |

Results go to stdout, errors and warnings to stderr.

## Cache

luma caches some ContentDB data on disk, in `LUMA_CACHE_DIR` if set, else in a `luma` folder in the user's cache folder:

* Linux: `$XDG_CACHE_HOME/luma` or `~/.cache/luma`
* Windows: `%LocalAppData%\luma`
* macOS: `~/Library/Caches/luma`

The folder can be deleted at any time. For now it holds which mods each ContentDB mod and game contains, used for the dependencies of [local packages](#local-packages) and of mods ContentDB can't look up (see [mods list-deps](#mods-list-deps)). It is kept for a day, and fetched again if a required dependency isn't in a copy older than 10 minutes.

## install

Installs mods, modpacks, games and texture packs from ContentDB, at their newest release compatible with the engine, or from local zips and folders.

    luma install mineclonia                  # by package name
    luma install TenPlus1/protector          # by author/name
    luma install animalia mobs_monster       # several at once
    luma install -n animalia                 # show what would be installed
    luma install --game minetest_game animalia   # dependencies minetest_game provides are not installed
    luma install -l ./mymod.zip              # a local zip
    luma install -l ./mymod -l ./other       # local folders
    luma install -l ./mineclonia-main --name mineclonia   # into the folder mineclonia

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be installed without changing anything |
| `-y`, `--yes` | install dependencies without asking |
| `--no-deps` | don't install the dependencies of mods |
| `--game` | count this game's mods as installed when resolving dependencies |
| `-l`, `--local` | install the zip or folder at this path, can be repeated |
| `--name` | the folder name to install the one `--local` package into |

A package is named as `author/name` or by its name alone, ignoring case. These are errors:

* a name alone that several authors use, luma doesn't guess:

      luma: "protector" is ambiguous, use "author/name" format

* a name that matches no package. If it matches the title of exactly one package, luma suggests it:

      luma: "voxelibre" not found on ContentDB, did you mean Wuzzy/mineclone2, "VoxeLibre (formerly MineClone2)"?

* an `author/name` package with no release compatible with the engine
* a package that is already installed, or whose folder already exists, whatever is in it. Use the `update` command of its type, such as [mods update](#mods-update)

If any package has one of these errors, each is reported, nothing is installed, and luma exits with code 1.

Mods and modpacks are installed with their hard dependencies, picked as for [mods update](#dependencies). Games and texture packs have none.

* luma shows what it will install and asks whether to continue, `-y` skips the question
* when not run from a terminal, it stops with an error unless `-y` or `--no-deps` is given
* a dependency that can't be found is reported, the rest are still installed, and luma exits with code 1

As the Luanti client does, luma writes the package's `author` and `release` into its `mod.conf`, `modpack.conf`, `game.conf` or `texture_pack.conf`, so later updates can find it. Mods and texture packs also get their `name`.

If a download or install fails partway, the packages already installed by this run are removed again, and luma exits with code 1.

### Local packages

`-l` takes the path to a zip or folder, and can be mixed with names. An argument that looks like a path, such as `./mymod.zip`, must be given with `-l`.

* if the zip or folder holds a single folder instead of the package itself, that folder is used
* the type comes from the files at the package root: `game.conf`, `texture_pack.conf`, `modpack.conf` or `modpack.txt`, or `init.lua` for a mod
* the package is copied as it is, luma writes nothing into it
* symlinks are not supported
* a zip may unpack to at most 10,000 files and 500 MB

The folder it's installed into is the first of:

1. `--name`
2. the `name` in its conf. Games have none, their folder is the game ID
3. the package's own folder
4. the zip's file name

It must be lowercase letters, digits and `_`, so `mineclonia-main` needs `--name`. `--name` only sets the folder, the conf's `name` still names the mod or texture pack.

A local mod's dependencies come from its `mod.conf`, or for a modpack from those of its mods. Each is installed from:

1. the package ContentDB lists for it, if the conf's `author` and `name` match a ContentDB package
2. the one ContentDB mod with its name
3. the one ContentDB mod that contains a mod of that name, modpacks included. A dependency on `mesecons_mvps` is installed from `Jeija/mesecons`. Of several, the one named like the dependency is picked

Otherwise luma doesn't guess, and the dependency is reported as ambiguous. Games are never picked, a dependency only games contain is reported as such, see `--game`. Step 3 fetches a list of every mod and game on ContentDB, which takes a while the first time, see [Cache](#cache).

## mods

### mods list

Lists the installed mods with their author and release. Modpacks are tagged `[P]`, and `-` stands for a value the mod doesn't declare.

    luma mods list
    luma mods list --names    # only the names, one per line

### mods outdated

Lists the mods with a newer release on ContentDB, with their current and latest release.

    luma mods outdated
    luma mods outdated --names

Not checked:

* mods with no author in their `mod.conf`, as they can't be matched to a ContentDB package
* git checkouts (a `.git` inside) with no `release` in their `mod.conf`, as updating would replace the working copy. ContentDB installs always have a `release`, so they are checked even with a `.git`

### mods update

Updates the named mods, or all outdated mods if none are named, replacing each mod's folder with the new release.

    luma mods update                # all outdated mods
    luma mods update foo bar        # only foo and bar
    luma mods update -x foo,bar     # all except foo and bar
    luma mods update -n             # show what would be updated
    luma mods update --game mineclonia   # dependencies mineclonia provides are not installed

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be updated and installed without changing anything |
| `-x`, `--exclude` | comma-separated mod names to skip |
| `-y`, `--yes` | install new dependencies without asking |
| `--no-deps` | don't install new dependencies of the updated mods |
| `--game` | count this game's mods as installed when resolving dependencies |

* naming a mod that is not installed is an error, and nothing is updated. So is a name in `--exclude` that is neither installed nor a new dependency
* named mods with no update, no author in their `mod.conf`, or that are git checkouts with no release, are reported and skipped
* if an update or a dependency install fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed

#### Dependencies

If a new release needs mods that are not installed, luma installs them too, with their own dependencies. Only hard dependencies are installed, see [mods install-deps](#mods-install-deps) for optional ones. `--dry-run` lists them, `--no-deps` turns this off.

* with new dependencies, luma shows what it will update and install, and asks once whether to continue. Anything but `y` or `yes` stops with nothing changed. `-y` skips the question
* with none, there is no question, so plain updates run unattended
* when not run from a terminal, luma stops with an error if new dependencies are needed, unless `-y` or `--no-deps` is given

A dependency counts as installed if a mod of that name is in the mods folder, on its own or inside a modpack. A game's mods only count if the game's folder name is given with `--game`, otherwise luma installs them from ContentDB.

When several packages provide a dependency, luma picks as the Luanti client does, and never picks a game:

1. a package already part of this run
2. a package named like the dependency
3. the first one ContentDB lists

A dependency is reported and skipped if its mod or package name is in `--exclude`, or if no package compatible with the engine provides it. The update goes ahead without it.

A dependency is installed into a folder named after its package. If that folder already exists, it is not installed and counts as a failure.

### mods list-deps

Lists the required dependencies of an installed mod, or the optional ones with `--optional`, in ContentDB's order, with their status.

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
| `ambiguous` | not installed, several packages provide it and none was picked (see below) |

| flag | meaning |
|---|---|
| `--optional` | list the optional dependencies instead of the required ones |
| `--names` | print only the dependency names, one per line |
| `--game` | count this game's mods as installed |

Installed dependencies and the package shown are decided as for [mods update](#dependencies).

A mod with no author in its `mod.conf`, or with no compatible package on ContentDB, can't be looked up itself. Its dependencies are then listed from its `mod.conf`, or for a modpack from those of its mods, and luma says so on stderr. Their packages are found as for a [local package](#local-packages), and only such a dependency can be `ambiguous`.

### mods install-deps

Installs the missing required dependencies of an installed mod, or its `available` optional ones with `--optional`, with their own required dependencies.

    luma mods install-deps mesecons                # missing required ones
    luma mods install-deps --optional mesecons     # available optional ones
    luma mods install-deps -n mesecons             # show what would be installed

| flag | meaning |
|---|---|
| `--optional` | install the optional dependencies instead of the required ones |
| `-n`, `--dry-run` | show what would be installed without changing anything |
| `-y`, `--yes` | install without asking |
| `--game` | count this game's mods as installed |

* luma shows what it will install and asks whether to continue, `-y` skips the question
* when not run from a terminal, it stops with an error unless `-y` is given
* required dependencies that only games, several packages or no compatible package provide are reported, the rest are still installed, and luma exits with code 1
* optional dependencies that can't be installed are skipped quietly
* dependencies are installed into folders named after their packages, as with [mods update](#dependencies)
* if an install fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed

## games

Games are named by their folder name, such as `minetest_game`.

### games list

Lists the installed games with their author, release and title. `-` stands for a value the game doesn't declare.

    luma games list
    luma games list --names    # only the folder names, one per line

### games outdated

Lists the games with a newer release on ContentDB, with their current and latest release.

    luma games outdated
    luma games outdated --names

Only games with both an `author` and a `release` in their `game.conf` are checked. As in the Luanti client, a Minetest Game without a release, as bundled with engines up to 5.8, counts as outdated unless it is a git checkout.

### games update

Updates the named games, or all outdated games if none are named. The game's folder is replaced with the new release, so changes made inside it are lost. Worlds are not affected.

    luma games update                   # all outdated games
    luma games update mineclonia        # only mineclonia
    luma games update -x mineclonia     # all except mineclonia
    luma games update -n                # show what would be updated

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be updated without changing anything |
| `-x`, `--exclude` | comma-separated game folder names to skip |

* games have no dependencies, so there is nothing to confirm
* naming a game that is not installed, as an argument or in `--exclude`, is an error, and nothing is updated
* named games with no update, or no author or release in their `game.conf`, are reported and skipped
* if an update fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed

## texturepacks

Texture packs are named by the `name` in their `texture_pack.conf`, else by their folder name. The `base` folder holds the engine's own textures and is never listed. `textures` works as a shorter `texturepacks`, as in `luma textures list`.

### texturepacks list

Lists the installed texture packs with their author, release and title. `-` stands for a value the texture pack doesn't declare.

    luma texturepacks list
    luma texturepacks list --names    # only the names, one per line

### texturepacks outdated

Lists the texture packs with a newer release on ContentDB, with their current and latest release.

    luma texturepacks outdated
    luma texturepacks outdated --names

Only texture packs with both an `author` and a `release` in their `texture_pack.conf` are checked.

### texturepacks update

Updates the named texture packs, or all outdated texture packs if none are named. The texture pack's folder is replaced with the new release, so changes made inside it are lost.

    luma texturepacks update                   # all outdated texture packs
    luma texturepacks update soothing32        # only soothing32
    luma texturepacks update -x soothing32     # all except soothing32
    luma texturepacks update -n                # show what would be updated

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be updated without changing anything |
| `-x`, `--exclude` | comma-separated texture pack names to skip |

* texture packs have no dependencies, so there is nothing to confirm
* naming a texture pack that is not installed, as an argument or in `--exclude`, is an error, and nothing is updated
* named texture packs with no update, or no author or release in their `texture_pack.conf`, are reported and skipped
* if an update fails, the rest still go ahead, and luma exits with code 1 after reporting how many failed
