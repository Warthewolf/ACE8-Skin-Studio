# Contributing

Thank you for helping. A few things keep ACE8 Skin Studio safe and easy to use.

## The rules

- **Offline and single player only.** Nothing that touches online play.
- **Never change the game's own files.** Everything goes into mods in the mods folder.
- **Never let the AES key out.** It stays in `settings.json`: not in logs, reports, tests, commits or packages. Never commit a `settings.json`.
- **Write for players.** Messages say what happened and what to do, in plain words. No stack traces, no jargon a player would have to look up. If something can be found or fixed by the program itself, do that instead of asking.
- **Keep the game's data safe.** Read a file completely, and check that writing it back unchanged gives the same bytes before building anything from it. Make a backup before changing a save; write to a new file and read it back before replacing the old one.

## Making a change

1. Read the [architecture](docs/architecture.md) to find where it belongs. File header comments explain each part.
2. Change the code; keep comments in the same plain style (what and why, not how).
3. Changed `ui/`? Run `python3 ui/build.py`.
4. Add or update tests. Tests that need game files must skip themselves when the files are not there (`needFile`).
5. Run `gofmt -l .`, `go vet ./...`, `go test ./...` and `GOOS=windows go build -o /dev/null .` (and the Linux build if you touched platform code).
6. Update `README.txt` (for players) and `CHANGELOG.txt`, and `docs/` if the change affects them.

## Platform code

Windows-only code is in `os_windows.go` and `pick_windows.go`; everything else (Linux) in `os_other.go`. Both sides must offer the same functions.

## Reporting a problem

Use **Copy a support report** in Settings (it never holds the AES key) and contact **@voidcallsreaper** on Discord.
