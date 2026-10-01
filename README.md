# luma

A terminal UI for managing Luanti mods and games, with ContentDB integration.

**Status: work in progress.** Expect missing features and rough edges.

Until release is made, the only way to use this is to have go installed, at later stages a binary per platform will be made available.

## Usage

    go run . --dir /path/to/luanti
    go run . --flatpak

`--dir` (or `-d`) is the Luanti install folder (the one containing `mods/` and `games/`); it defaults to the current directory.
`--flatpak` uses the Luanti flatpak install instead. If no install is found, luma exits with an error.
`--engine-version` (or `-e`) is the engine version to assume when it can't be detected. Examples: `-e 5.17`, `-e 5.18-dev`
The engine version is read from the engine binary, then from `builtin/game/misc_s.lua`, then from `-e` (checked against ContentDB); if none of these work, luma exits with an error.
Run `go run . -h` for flags.

## Roadmap

The rough goals of the tool are in [ROADMAP.md](ROADMAP.md)

## License

See [LICENSE](LICENSE)
