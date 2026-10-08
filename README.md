# luma

A terminal UI and command line tool for installing, updating, and managing Luanti packages (mods, games, texture packs), with ContentDB integration. Managing worlds and mod settings is planned.

**Status: work in progress.** Expect missing features and rough edges.

## Install

Download the archive for your platform from the [releases page](https://github.com/luanti-org/luma/releases), extract it, and put `luma` somewhere on your `PATH`.

Builds are available for Linux, macOS and Windows, on x86_64 (`amd64`) and ARM64.

The binaries are not signed, so macOS and Windows may warn before running them.

### Verification

Each release includes a `SHA256SUMS` file. A download can also be checked against the build that produced it with the [GitHub CLI](https://cli.github.com/):

    gh attestation verify luma-<version>-<os>-<arch>.tar.gz --repo luanti-org/luma

### Other methods

With [Go](https://go.dev/) installed, luma can be installed with:

    go install github.com/luanti-org/luma@latest

It can also be run from a checkout of this repository, using `go run .` in place of `luma`.

## Running luma

Run `luma` in your Luanti install directory, or point it at one with `--dir` or `--flatpak`.
Without a command it opens the terminal UI, and with one it runs that command and exits.

Run `luma -h` for help, or see the [online docs](https://docs.luanti.org/for-server-hosts/luma/).

The same pages are also in this repository:

- [Global flags](docs/global-flags.md)
- [Command line](docs/cli.md)
- [Terminal UI](docs/terminal-ui.md)

## Roadmap

The rough goals of the tool are in [ROADMAP.md](ROADMAP.md)

## License

See [LICENSE](LICENSE)
