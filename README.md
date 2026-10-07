# luma

A terminal UI and command line tool for managing Luanti mods, games and texture packs, with ContentDB integration.

**Status: work in progress.** Expect missing features and rough edges.

No binaries are released yet, so [Go](https://go.dev/) is needed to run luma from a checkout.

## Usage

Open the terminal UI:

    go run . --dir /path/to/luanti
    go run . --flatpak

`--dir` (or `-d`) is the Luanti install folder, the one containing `mods/` and `games/`. It defaults to the current directory. `--flatpak` uses the Luanti flatpak install instead.

Or run a single command without opening it:

    go run . --dir /path/to/luanti mods outdated
    go run . --dir /path/to/luanti mods update

## Documentation

* [Terminal UI](docs/terminal-ui.md): screens, keys, updating mods
* [Command line](docs/cli.md): commands and their flags
* [Global flags](docs/global-flags.md): choosing the install and the engine version

## Roadmap

The rough goals of the tool are in [ROADMAP.md](ROADMAP.md)

## License

See [LICENSE](LICENSE)
