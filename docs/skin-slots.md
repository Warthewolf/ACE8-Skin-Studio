# How new skin slots work

A **skin mod made here** ("slot mod") adds skins in slots of their own instead of replacing the game's skins.

## What a skin mod holds

```
Mods/<Folder>/
  <key>_P.utoc / .ucas / .pak     the skins' own textures, under /Game/SkinStudio/<mod key>/<skin key>/
  slotfiles/<skin>/               each skin's material (and damage material) and icon, unpacked
  skinstudio.json                 what the mod is: its skins, emblems and texts
  icons/                          small pictures for the program's lists
```

The mod key is the mod's name plus `_` and five random letters (letters, so the engine never reads the end as a number). Everything a skin owns is under its own folder, so two mods never collide on a file.

## The merged skin list

The game knows its skins from **one** skin list (`DT_Skin`) and **one** unlock list (`DA_Unlock_Skin`). Only one copy of each can be in effect, so the program builds a single merged copy — the **skin list mod**, `zz_SkinStudio_SkinList` — from the game's own lists plus every installed slot mod. It is rebuilt whenever a skin mod is built, installed, imported or taken out (and from the Skin slots screen).

- **Numbers:** new skins get numbers from **601 to 999**. A skin keeps its number once it has one; a skin that arrives wanting a number already taken gets the next free one. Assignments are kept in `skin-list.json` next to the skin list mod.
- **Rows:** each new skin's row is a copy of the row of the game skin it was based on (its template), with a new id, text keys, sort number and its own material and icon.
- **Unlocks:** each new skin gets an unlock rule like the game's first skins (finishing a campaign mission).
- **Material slots:** the skin's material and icon are renamed to its number (`Materials/603/MI_PP0021_su57_603_Decal`, `T_UI_Icon_Skin_su57_603`) and packed with the skin list, so numbers can change without repacking the skin mods. The big textures stay in the skin's own mod and never need renaming.
- **Other tools' skin lists:** mods that carry their own copy of the skin list are folded in (`adopt.go`): the skins they add get free numbers, their changes to the game's skins are kept.
- **Load order:** the skin list mod must load after every other mod that carries a skin list; the program puts it last in the loader's `loadorder.txt`.

## Names and descriptions

The skin list holds text keys (`SS_<mod key>_<skin>_Name`), not texts. **SkinSlotText**, installed in UE4SS's Mods folder, reads the texts from `Scripts/texts/SkinStudio_<mod key>.ini`, one file per mod.

## Emblems

New emblems work the same way: the skin list mod also carries the emblem list and its unlock list; each emblem's pictures are in its mod under `/Game/Emblem/tga/SS/` and `/Game/Emblem/png/SS/`, numbered from 601.

## Sharing (`share.go`)

"Share as .zip" makes a zip people can install with or without Skin Studio:

```
<Folder>.zip
  skinstudio.json, README.txt, icons/
  Mods/<Folder>/                 the mod's own files
  Mods/<Folder>_SkinList/        a skin list with only this mod's skins (for people without Skin Studio)
  ue4ss/Mods/SkinSlotText/       with this mod's texts
```

There is no zip inside the zip (Nexus Mods quarantines those). Importing a shared zip in Skin Studio installs the mod and gives its skins free numbers, so mods from different people never clash.

## Requirements in the game

- **AC8 Mod Loader 1.1.0 or newer** — older loaders do not load files that are new to the game, so new skins show without paint or icon.
- **UE4SS** with **SkinSlotText** for the names.
- Before taking a skin mod out, choose another skin for that aircraft in the game, or use "Prepare my save for the game without mods".
