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
3. Make the packages:

| Package | Contents |
|---|---|
| `ACE8SkinStudio-<v>.zip` | `ACE8SkinStudio/`: `ACE8SkinStudio.exe`, `retoc.exe`, `retoc-LICENSE.txt`, `README.txt`, `CHANGELOG.txt` |
| `ACE8SkinStudio-<v>-with-UE4SS-and-ModLoader-1.1.0.zip` | The above, plus `Win64/` (UE4SS and AC8 Mod Loader, ready to copy into `Game/Binaries/Win64`) and `START HERE.txt` |
| `ACE8SkinStudio-<v>-linux-x86_64.tar.gz` / `.zip` | `ACE8SkinStudio/`: the Linux program (executable), `LINUX.txt`, `README.txt`, `CHANGELOG.txt`, `retoc-LICENSE.txt` (retoc for Linux is downloaded by the player) |
| `ACE8SkinStudio-<v>-source.zip` | This repository's files |

4. Check before publishing: **no `settings.json`** (it holds the AES key) and no Steam files in any package, and the source package builds and passes its tests on its own.

Do not ship **Oodle** (`oo2core_9_win64.dll`): it is not ours to redistribute. retoc and Skin Studio fetch it on the player's computer.
