# luma

A terminal UI for managing Luanti mods and games, with ContentDB integration.

**Status: work in progress.** Expect missing features and rough edges.

Until release is made, the only way to use this is to have go installed, at later stages a binary per platform will be made available.

## Usage

    go run . --dir /path/to/luanti
    go run . --flatpak

`--dir` (or `-d`) is the Luanti install folder (the one containing `mods/` and `games/`); it defaults to the current directory.
`--flatpak` uses the Luanti flatpak install instead. If no install is found, luma exits with an error.
The engine version is read from the engine binary, then from `builtin/game/misc_s.lua`; if neither works, luma exits with an error.
`--engine-version` (or `-e`) overrides the detected engine version and is checked against ContentDB. Examples: `-e 5.17`, `-e 5.18-dev`. Use it only if detection fails or you know what you're doing.
Run `go run . -h` for flags.

### Command line

Passing a command runs it without opening the terminal UI. Global flags such as `--dir` go before the command:

    luma --dir /path/to/luanti mods list
    luma mods outdated
    luma mods update                     # update all outdated mods
    luma mods update -n                  # show what would be updated
    luma mods update -x foo,bar          # update all except foo and bar
    luma mods update foo bar             # update only foo and bar

`mods list` and `mods outdated` take `--names` to print only mod names, one per line.
Run `luma --help` for the command groups, `luma mods --help` for their commands, and `luma mods update --help` for a command's flags.

## Roadmap

The rough goals of the tool are in [ROADMAP.md](ROADMAP.md)

## License

See [LICENSE](LICENSE)
