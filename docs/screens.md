# Screens

The menu on the left is in working order: **Prepare** (get the game's files ready to paint), **Build** (turn your pictures into mods), **Manage** (what is installed, your save), then Settings.

## Prepare

### Extract
Takes an aircraft's skin textures out of the game.
- **Aircraft folder name** — type part of it; matching aircraft are listed. **Refresh the aircraft list** reads the game's file list again (after a game update).
- **Skin folders** — tick only the skins you want; none ticked takes them all. Each is about 130–320 MB of raw files.
- **Preview skins** shows a small picture of each skin before you extract.
- **Paint-ready** turns the textures into tiles named after the texture and tile number (`T_PP0021_su57_001_C_1001.png`): colour, metal/roughness/AO and normal map.
- Most aircraft keep each skin texture as UDIM tiles (1001, 1002…). Some, such as the Ladon, use one square picture per texture (4096 × 4096); those work the same way.

### Split
Cuts a wide picture (all tiles side by side) into separate tiles, one folder per skin. Useful when your paint program exports one long picture.

### Model
Makes a clean model for painting: drop the `.glb` files exported with FModel, keep the skin materials, and save one `.glb` to load in Substance Painter.

## Build

### Repack (replace a game skin)
Turns your painted pictures into a mod that replaces one of the game's skins.
1. Choose the skin you painted on (the program guesses it from your file names).
2. Drop your pictures, or choose their folder. The page shows which game texture each picture is for, and what is missing.
3. Tick the textures to rebuild. Untouched ones stay as the game's.
4. Optional: **BC7** (sharper colour, files twice as big) and **No mipmaps** for the colour texture.
5. Optional: a new name, description or icon for the skin it replaces.
6. **Build**. With "install" ticked, the mod goes straight into your mods folder.

### New skin slot
Adds your skin as a new skin, keeping every game skin as it was.
- **Start from** — the game skin you painted on. The new skin copies its material, finish, normal map and where emblems may go. Type another skin number to start from a different skin of the same aircraft.
- Drop your pictures as for Repack. Without pictures the new skin is a plain copy of the game skin (useful as a base to change later).
- **Name and description** — what the game shows in the hangar.
- **Icon** — keep the original, recolour it in your paint's main colour (or a colour you pick), or use a picture of your own.
- **Mod** — a new skin mod, or add it to one you made before. One mod can hold many skins and emblems and is shared as one `.zip`.
- New skins get numbers from 601 up. They need AC8 Mod Loader 1.1.0 or newer and SkinSlotText (installed for you).

### New emblem
Adds a picture of yours to the game's emblem list, with its own name and description. A PNG with a see-through background works best. It goes into a skin mod, like new skins, and gets an emblem number from 601 up.

### Pilots
- **Helmets** — "Find pilots in the game files" once, pick a pilot, "Get paint-ready pictures", paint, drop them back.
- **Radio portraits** — click a portrait and use a photo of your own (drag to place, scroll to zoom) or another portrait's photo, with the original text or new text in the game's own font. New portraits are kept between sessions.
- **Names on the radio** — new names and callsigns for anyone who talks on the radio. No building: they show the next time the game starts.

## Manage

### Skin slots
Every skin mod installed, with its skins and emblems: change texts or pictures, take one out, share a mod as a `.zip`, or import a `.zip` someone shared (numbers that clash are renumbered for you). **Rebuild the skin list** makes the shared skin list again.

### Mods
Everything in the mods folder: what each mod replaces, and which mods replace the same files (only one of those can win). Mods can be taken out (moved to `workspace\removed-mods`, not deleted).

### Save tools
Work on the game's save (`Campaign.sav`). **Close the game first** — it writes its save when it closes, which would undo the change.
- **Back up now**, and **Put this back** next to any backup — every change makes a backup first.
- **Unlock now** — unlocks new skins and emblems without finishing a mission.
- **Prepare my save for the game without mods** — so the game does not stumble over skins that are no longer installed.
- **Save backup at game start** — a small UE4SS mod that backs the save up each time the game starts.

## Useful links
Pages with templates for the game's skins, opened in your browser:
- [Skin Template Repository - Project Pileus](https://www.nexusmods.com/acecombat8wingsoftheve/mods/87) by Nimbus Custom Works, uploaded by M4rkoz: layered PSD/XCF templates with finish, normal map and dirt mask textures and a Blender preview model.
- [Skin templates by Rythus](https://www.nexusmods.com/acecombat8wingsoftheve/mods/34).

Paint on a template, export one PNG or TGA per tile, and bring it in with Repack or New skin slot. The links are listed in `links.go`; only those pages can be opened from the window.

## Settings
Setup (see [Settings and folders](settings-and-folders.md)), appearance (Dark, Day, System), **Copy a support report**, and **Quit**.
