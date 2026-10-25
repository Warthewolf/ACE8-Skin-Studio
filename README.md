# ACE8 Skin Studio

Extract, paint and repack aircraft skins for **Ace Combat 8**, add new skin slots with their own names, descriptions and icons, add emblems of your own, change pilots' helmets and radio portraits, and manage your mods. It runs as a small local program with its window in the browser.

This repository holds the source of **version 2.4.5** for **Windows and Linux** (x86-64, Steam Deck included). The same code builds both; the platform differences are in `os_windows.go`, `pick_windows.go` and `os_other.go`.

## Documentation

- **Players:** [Getting started](docs/getting-started.md) · [Screens](docs/screens.md) · [Troubleshooting](docs/troubleshooting.md) · [Linux and Steam Deck](docs/linux.md) · [Settings and folders](docs/settings-and-folders.md)
- **Developers:** [Architecture](docs/architecture.md) · [Game file formats](docs/game-formats.md) · [How new skin slots work](docs/skin-slots.md) · [Building and releasing](docs/building.md) · [Contributing](CONTRIBUTING.md)

The player's guide that ships with the program is [README.txt](README.txt); what changed in each version is in [CHANGELOG.txt](CHANGELOG.txt).

> Use modded files offline and in single player only.

## Building

Needs [Go](https://go.dev) 1.24 or newer and Python 3 (to put the window's page together).

```sh
# the window's page: ui/*.html and ui/*.js -> ui/index.html (embedded in the program)
cd ui && python3 build.py && cd ..

# Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -H=windowsgui" -o ACE8SkinStudio.exe .

# Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ACE8SkinStudio .
```

Tests: `go test ./...` (tests that need game sample files skip themselves when the files are not there).

## What it needs at run time

- [retoc](https://github.com/trumank/retoc) by trumank, next to the program (`retoc.exe` on Windows, `retoc` on Linux). retoc reads the game's files with Oodle; Skin Studio makes sure the Oodle library is next to retoc before it runs (copied from the game's folder, or downloaded from the same place retoc uses and checked).
- The game's AES key, entered in Settings.
- For new skin slots: AC8 Mod Loader 1.1.0 or newer and UE4SS.

## Credits

- Container unpacking and packing: retoc, by trumank.
- The BC7 decoder follows the reference tables used in Pillow (HPND licence).

Any issues please contact **@voidcallsreaper** on Discord.
