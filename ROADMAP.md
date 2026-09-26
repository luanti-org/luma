# Roadmap

Rough goals for luma. Order and scope may change.

The tool covers the ContentDB and world/server management side of Luanti dev tooling.

## Content (mods, games, texture packs)

- **Local listing**: scan installed mods and games and show them in the UI
- **Installed-content record**: - TBD, not rushing this - a small local file noting where each mod came from (ContentDB, git, or placed by hand), so updates work and unmanaged mods are left alone. It also caches ContentDB info so installs and updates still work offline for known mods.

- **ContentDB package management**
  - Search ContentDB for mods, games and texture packs
  - Install a package along with its dependencies
  - Choose the ContentDB URL, so a mirror can be used
  - Optional "git mode": install and update a mod from its git repository instead of a release zip
  - Check installed content for updates

- **Games**: list, search and install/uninstall games from ContentDB

- **Texture packs**: install/uninstall a texture pack and enable it for a world or server
  - Possibly support merging texturepacks manually until engine support comes in

## Worlds and servers

- **World management**
  - List worlds and see which mods each one enables
  - Enable or disable a mod per world
  - Tell global mods (`mods/`) apart from per-world mods (`worldmods/`)

- **Mod settings editor**
  - Read a mod's `settingtypes.txt`
  - Edit its values in a form and write them to the right config file

- **Non-interactive CLI**: an `apt-get` style command line alongside the terminal UI

## Later

Ideas that are not scoped yet.

- **World backups**: back up worlds whatever their database backend (sqlite3, postgresql, leveldb)
