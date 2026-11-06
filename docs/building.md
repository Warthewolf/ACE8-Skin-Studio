# Building and releasing

## Requirements

- **Go 1.24** or newer (no outside Go modules).
- **Python 3** to put the window's page together.
- Optional: **LLVM** (clang, lld-link, llvm-dlltool, llvm-rc) to rebuild SkinSlotText's DLL.

## Building

```sh
# 1. the window's page: head.html + body.html + app.js and the other scripts -> ui/index.html
cd ui && python3 build.py && cd ..

# 2a. Windows (no console window)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H=windowsgui" -o ACE8SkinStudio.exe .

# 2b. Linux (x86-64, Steam Deck included)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ACE8SkinStudio .
```

Both build from any system (Go cross-compiles). `ui/index.html` is embedded into the program, so **run `build.py` after changing anything in `ui/`**. `rsrc_windows_amd64.syso` gives the Windows program its icon.

Running a build without a window, on a fixed port and token (handy for testing the page in a browser or with Playwright):

```sh
./ACE8SkinStudio -port 48600 -no-open -stay -token test -data ./testdata
# then open http://127.0.0.1:48600/?t=test
```

`-data` chooses where `settings.json`, the log and (by default) the workspace go.

## Tests

```sh
go vet ./...
go test ./...
GOOS=windows go build -o /dev/null .   # the Windows-only files still compile
```

Many tests use real game files (textures, tables, models). They look for them in fixed sample folders and **skip themselves** when the files are not there (`needFile`), so `go test` passes on any machine; the full set runs where the samples are. Tests that run retoc use the test program itself as a stand-in (`FAKE_RETOC`, see `slots_e2e_test.go`).

SkinSlotText's logic has its own tests that run on Linux or macOS:

```sh
cd skinslottext-source/test && ./run_tests.sh
```

## Rebuilding SkinSlotText

```sh
cd skinslottext-source && ./build.sh
```

With LLVM 18 the result is byte for byte the shipped `SkinSlotText.dll` (compare the SHA-256 printed at the end with the one in its README). Copy the result to `embed/SkinSlotText/` to ship it with the program.

## Releasing

1. Set the version: `appVersion` in `settings.go`, the first line of `README.txt`, and a new section at the top of `CHANGELOG.txt`.
2. Build the page and both programs as above; run the tests.
3. Make the packages, each with the retoc for its system:

```sh
python3 tools/package.py            # builds the page and both programs, fetches retoc, writes dist/
```

| Package | Contents |
|---|---|
| `ACE8SkinStudio-<v>.zip` | `ACE8SkinStudio/`: `ACE8SkinStudio.exe`, **`retoc.exe`** (Windows), `retoc-LICENSE.txt`, `README.txt`, `CHANGELOG.txt` |
| `ACE8SkinStudio-<v>-with-UE4SS-and-ModLoader-1.1.0.zip` | The above, plus `Win64/` (UE4SS and AC8 Mod Loader, ready to copy into `Game/Binaries/Win64`) and `START HERE.txt`. Made when you pass `--bundle-win64 <Win64 folder> --start-here <START HERE.txt>` |
| `ACE8SkinStudio-<v>-linux-x86_64.tar.gz` / `.zip` | `ACE8SkinStudio/`: the Linux program and **`retoc`** (Linux x86-64), both executable (the `.zip` keeps that for `unzip`), `LINUX.txt`, `README.txt`, `CHANGELOG.txt`, `retoc-LICENSE.txt` |
| `ACE8SkinStudio-<v>-source.zip` | This repository at `HEAD` (`git archive`) |

What the script does about retoc:
- It takes the retoc release named by `RETOC_DEFAULT` in `tools/package.py` (or `--retoc <tag>`) from [retoc's releases](https://github.com/trumank/retoc/releases): `retoc_cli-x86_64-pc-windows-msvc.zip` for Windows and `retoc_cli-x86_64-unknown-linux-gnu.tar.xz` for Linux. Without internet, download those two (and their `.sha256` files) by hand and pass `--retoc-dir <folder>`.
- Each archive is checked against the `.sha256` the release publishes and against **`tools/retoc-lock.json`**. The first time a release is used its hashes are written there; after that a download that differs is refused. Commit the lock file with the release.
- The program taken from each archive must be the right kind (a Windows program for the Windows package, a Linux x86-64 program for the Linux one), or nothing is packed. Each finished package is read back: exactly one retoc, the right one, executable on Linux, and no retoc for the other system.
- retoc's Linux build needs glibc 2.34 or newer (it is built on Ubuntu 22.04). Skin Studio explains this to players on older Linux and how to build retoc for it (see `LINUX.txt`).

4. Check before publishing: **no `settings.json`** (it holds the AES key) and no Steam files in any package, and the source package builds and passes its tests on its own. The script refuses to pack `settings.json`, Oodle or `.sav` files.

Do not ship **Oodle** (`oo2core_9_win64.dll`): it is not ours to redistribute. retoc and Skin Studio fetch it on the player's computer.
