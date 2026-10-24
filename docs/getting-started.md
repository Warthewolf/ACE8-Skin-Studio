# Getting started

## What you need

- **Ace Combat 8** on PC (Steam).
- **The game's AES key.** It unlocks the game's files for reading. Skin Studio never shares it: it stays in `settings.json` on your computer and is left out of every support report.
- **retoc** — included with the Windows download (`retoc.exe`). On Linux, see [Linux and Steam Deck](linux.md).
- **For new skin slots and emblems:** UE4SS and **AC8 Mod Loader 1.1.0 or newer**. The "with-UE4SS-and-ModLoader" download has both, ready to copy into the game.
- **A paint program** that exports PNG or TGA, such as Substance Painter, Photoshop, GIMP or Krita.

## Installing

1. Unzip the download somewhere with a short path, such as `C:\ACE8SkinStudio`. Avoid Program Files: the program writes its settings and workspace next to itself.
2. Double-click `ACE8SkinStudio.exe`. A window opens (an Edge app window, or your default browser).
3. If Windows says "Windows protected your PC", choose **More info → Run anyway**. The program is not signed.
4. A short tutorial opens the first time. It can be opened again from **Tutorial** in the menu.

Upgrading: unzip the new version over the old folder (or into a new folder and copy `settings.json` across). Your workspace and settings are kept.

## First-time setup (Settings)

The top bar says **Setup needed** until these are done. Each field shows a green or red note saying what is wrong.

| Setting | What to choose |
|---|---|
| Paks folder | `...\steamapps\common\ACE COMBAT 8\Game\Content\Paks` |
| AES key | 64 hexadecimal characters, with or without `0x` |
| retoc program | Leave empty when `retoc.exe` is next to the program |
| Mods folder | The mod loader's folder: `...\Game\Binaries\Win64\ue4ss\Mods\IoStoreLoaderMod` |
| Workspace folder | Where extracted files and your projects go (default: next to the program) |

Used AC8 Skin Studio 1.x? Press **Use the settings of AC8 Skin Studio 1.x** and choose its folder.

## From a game skin to your own skin

1. **Extract** — type the aircraft folder (for example `PP0021_su57`), pick the skin folders you want (or none for all) and press Extract. Tick "paint-ready" to get tiles you can paint on straight away.
2. **Paint** — paint over the tiles in `workspace\paint-ready`, or paint the aircraft in Substance Painter using a model from the **Model** screen. Export your pictures; their names should say what they are (Substance Painter's names work as they are).
3. **Choose how it goes into the game:**
   - **Repack** replaces one of the game's skins with yours.
   - **New skin slot** adds your skin as an extra skin with its own name, description and icon. It needs the mod loader and UE4SS.
4. **Build.** The mod is installed into your mods folder when the mods folder is set.
5. **Start the game.** New skins unlock when you finish a campaign mission, or straight away with **Unlock now** on the Save tools screen (close the game first).

Next: [Screens](screens.md) explains every screen in detail.
