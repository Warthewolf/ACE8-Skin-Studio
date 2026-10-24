# Architecture

ACE8 Skin Studio is one Go program with no outside Go libraries. It serves its own window as a local web page, does the work in background jobs, and calls **retoc** to take files out of the game and to pack mods. Everything else — reading and writing the game's packages, textures, tables and saves — is done in its own code.

## How it runs

```
ACE8SkinStudio(.exe)
  ├─ local web server on 127.0.0.1 (random port, random token)   server.go
  │    └─ the window: one page (ui/index.html, embedded)         ui/
  ├─ background jobs, one at a time, with progress and a log     jobs.go
  │    └─ retoc (to-legacy: extract, to-zen: pack, list)         work.go, slots.go
  └─ settings.json and the workspace folder                      settings.go
```

1. `main.go` starts the server (`runApp` in `server.go`). It listens on `127.0.0.1` only, makes a random access token, and opens the window (`openURL`: an Edge app window on Windows, a Chromium app window or the default browser on Linux).
2. Every `/api/...` request must carry the token (`guard` in `server.go`), so other pages in the browser cannot talk to the program.
3. Long work runs as a **job** (`jobs.Start`): the window polls `/api/job` for progress, the current step and the log. Only one job runs at a time.
4. The program leaves a few seconds after its last window closes (`presence.go`).
5. Starting the program again shows the running copy's window instead of starting a second one.

`ACE8SkinStudio cli <command>` runs a few operations without the window, for testing (`cli.go`: `info`, `export`, `split`, `rebuild`, `model`).

## Code map

### The window and its requests
| File | What it does |
|---|---|
| `server.go` | Web server, token check, the window's requests (`/api/...`), `/api/state` (settings + checks) |
| `slots_api.go` | Requests for skin slots, sharing and importing |
| `jobs.go` | Background jobs; running retoc and explaining its failures in plain words |
| `presence.go` | Knowing when the last window has closed |
| `report.go` | The support report (never holds the AES key; user name taken out of paths) |
| `ui/` | The page: `head.html` (styles), `body.html` (layout), `app.js` + one script per area; `build.py` joins them into `index.html` |

### Settings and platform
| File | What it does |
|---|---|
| `settings.go` | `settings.json`, defaults, the setup checks shown in Settings, the workspace check |
| `os_windows.go`, `pick_windows.go` | Windows: Edge window, Explorer, the file chooser (in a helper process), finding the running game |
| `os_other.go` | Linux: browser window, zenity/kdialog chooser, Steam libraries and Proton's save folder |
| `oodle.go` | Making sure retoc has the Oodle library before it runs |

### Game packages and data
| File | What it does |
|---|---|
| `uasset.go` | Cooked packages in the legacy layout (`.uasset` + `.uexp`): read, rename, rewrite |
| `fname.go` | How names with numbers are stored (`X_602` is `X` + 603) and name hashes |
| `props.go`, `schema.json`, `usmap.go` | Unversioned properties: reading and writing objects using built-in layouts or a `.usmap` |
| `zen.go` | Reading packages straight out of a mod's `.utoc`/`.ucas` without retoc |
| `skintable.go` | The skin list and unlock list (and the emblem pair) |
| `savegame.go` | The game's save files (GVAS) |

### Textures and pictures
| File | What it does |
|---|---|
| `vt.go` | Streamed UDIM textures: read, decode, rebuild |
| `vtformat.go` | Storing a rebuilt texture as BC7 and/or without mipmaps |
| `vtplain.go` | Aircraft whose skins are ordinary textures (the Ladon) |
| `tex2d.go` | Ordinary textures (helmets, portraits) |
| `bc1.go`, `bc7.go`, `bc7part.go` | Block compression: decoders and encoders |
| `image.go` | PNG and TGA in and out |
| `icons.go` | Skin icons (180 × 180) |
| `ops.go` | Splitting into tiles, paint-ready tiles, matching painted files, composing them back |
| `guess.go` | Guessing the game skin from painted file names |
| `library.go` | Raw textures on disk grouped into skins |
| `glb.go` | Cleaning FModel `.glb` models for painting |

### The screens' work
| File | What it does |
|---|---|
| `work.go` | Split, Extract, paint-ready tiles, Repack builds |
| `base.go` | Game files fetched once for building new skins (refreshed after a game update) |
| `extras.go` | Repack's new name, description and icon for the skin it replaces |
| `slots.go`, `slotbuild.go`, `slotfiles.go` | New skin slots and the merged skin list (see [skin-slots.md](skin-slots.md)) |
| `adopt.go` | Folding other mods' skin lists into the merged one |
| `emblems.go` | New emblems |
| `share.go` | Sharing and importing skin mods as `.zip` |
| `mods.go` | What is installed and which mods clash |
| `pilots.go`, `pilotsaved.go`, `pilotnames.go` | Helmets, radio portraits, names on the radio |
| `savetools.go` | Save backups, Unlock now, restoring, the game-start backup mod |
| `skinslottext.go`, `embed/` | The UE4SS mods the program installs (SkinSlotText, SkinStudioSaveBackup) |
| `skinslottext-source/` | Source of SkinSlotText's DLL and script |

## Principles the code follows

- **Never change the game's own files.** Everything goes into mods.
- **Read, then check by writing back.** Tables and packages are read completely; writing an unchanged one must give the same bytes, so a game update that changes a layout is caught before anything is built.
- **Messages are for players.** Errors say what happened and what to do, in plain words, never a stack trace.
- **The AES key never leaves `settings.json`**: it is passed to retoc, left out of logs and reports, and hidden in the window unless shown on purpose.
- **Safe writes.** Saves are written to a new file and read back before replacing the old one; mods taken out are moved, not deleted.
