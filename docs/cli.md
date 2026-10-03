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

| flag | meaning |
|---|---|
| `-n`, `--dry-run` | show what would be updated without changing anything |
| `-x`, `--exclude` | comma-separated mod names to skip |

Naming a mod that is not installed, as an argument or in `--exclude`, is an error, and nothing is updated.

Named mods with no update, or with no author in their `mod.conf`, are reported and skipped.

If an update fails, the remaining mods are still updated, and luma exits with code 1 after reporting how many failed.
