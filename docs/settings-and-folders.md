# Settings and folders

## settings.json

Settings are kept in `settings.json` next to the program (or in the folder given with `-data`). **It holds your AES key: never share it, and leave it out when you give the program to someone.** The repository's `.gitignore` keeps it out of git.

| Field | Meaning |
|---|---|
| `paksDir` | The game's `Content\Paks` folder (found by itself on Linux) |
| `aesKey` | The game's AES key (64 hexadecimal characters) |
| `usmap` | Optional mappings file (`.usmap`, uncompressed). Only needed if a game update changes the data layouts built into the program |
| `retocPath` | retoc, when it is not next to the program |
| `modsDir` | The mod loader's folder (`…\ue4ss\Mods\IoStoreLoaderMod`); finished mods are installed here |
| `ue4ssMods` | UE4SS's `Mods` folder (holds `mods.txt`); found by itself when the mods folder is inside it |
| `workspace` | Where the program keeps its files (default: `workspace` next to the program) |
| `extraRawDirs` | Extra folders of raw game files to show in the library |
| `engineVersion` | Passed to retoc (`UE5_4`) |
| `project` | The game's project name inside its files (`Live`) |
| `imageFormat` | `png` or `tga` for pictures the program writes |
| `defaultBrowser` | Open the window in the default browser instead of an app window |
| `saveDir` | The game's `SaveGames` folder (found by itself when empty) |
| `tex` | How colour textures were last asked to be stored (BC7, no mipmaps) |
| `theme`, `hideTutorial`, `saveBackupOff` | Appearance and remembered choices |

## The workspace

| Folder | Holds | Safe to delete? |
|---|---|---|
| `raw` | Raw game files from Extract | Yes (extract again) |
| `paint-ready` | Tiles made from game textures; pilots' helmet pictures | Yes, if you have your own copies |
| `split` | Tiles made on the Split screen | Yes |
| `models` | Models made on the Model screen | Yes |
| `mods` | Finished Repack mods | Keep the ones you use |
| `slot-mods` | **The source of your skin mods**: textures, materials, icons | **No** — needed to change your skins later |
| `skin-list` | The shared skin list as last built | Yes (rebuilt when needed) |
| `base` | Game files new skins are built from | Yes (fetched again; refreshed after a game update) |
| `shared` | `.zip` files made with "Share as .zip" | Yes |
| `removed-mods` | Mods taken out of the mods folder | Yes, if you will not put them back |
| `previews` | Small preview pictures | Yes |
| `pilots` | The pilot list, small pictures, and `my-portraits` (your radio portraits) | Keep `my-portraits` |
| `inbox` | Pictures dropped on the window | Yes |

## Next to the game's saves

```
…\ACE COMBAT 8\Saved\SkinStudioSaveBackups        one dated folder per backup
…\ACE COMBAT 8\Saved\SkinStudioSaveBackups\auto   the game-start backups
```

## Other files

| File | Where | What |
|---|---|---|
| `ACE8SkinStudio.log` | Next to the program (or `-data`) | What the program did; read it when no window appears |
| `oo2core_9_win64.dll` | Next to `retoc.exe` | Oodle, which retoc needs to read the game's files (fetched for you) |
