# Roadmap

Rough goals for luma. Order and scope may change.

The tool covers the ContentDB and world/server management side of Luanti dev tooling.

## Content (mods, games, texture packs)

- [DONE] **Local listing**: scan installed mods and games, allow installation from CDB.
- **Git updates**: allow handling of mods/games/txp that have a .git folder.

- **ContentDB package management**
  - [DONE] Search ContentDB for mods, games and texture packs
  - [DONE] Show a package's ContentDB details (description, license, content warnings, links)
  - [DONE] Install a package along with its dependencies
  - Choose the ContentDB URL, so a mirror can be used
  - Optional "git mode": install and update a mod from its git repository instead of a release zip
  - [DONE] Check installed content for updates
  - Uninstall mods smartly (e.g. if a dep of other mods or used in worlds warn)

- **Games**
  - [DONE] list, search and install games from ContentDB

- **Texture packs**
  - [DONE] install a texture pack from CDB

## Worlds and servers

- **World management**
  - List worlds and see which mods each one enables
  - Enable or disable a mod per world
  - Tell global mods (`mods/`) apart from per-world mods (`worldmods/`)

- **Mod settings editor**
  - Read a mod's `settingtypes.txt`
  - Edit its values in a form and write them to the right config file

- **Non-interactive CLI**
  - [DONE] `apt-get` style command line alongside the terminal UI

## Later

Ideas that are not scoped yet.

- **World backups**: back up worlds whatever their database backend (sqlite3, postgresql, leveldb)
