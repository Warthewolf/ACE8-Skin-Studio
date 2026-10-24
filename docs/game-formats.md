# Game file formats

What the program knows about Ace Combat 8's files (Unreal Engine 5.4, IoStore containers, unversioned properties). This was worked out from the game's files; it is what the code relies on, not an official description.

## Containers

The game's files are in IoStore containers in `Game/Content/Paks`: `.utoc` (table of contents), `.ucas` (data) and a small `.pak`. They are encrypted with the game's AES key and compressed with **Oodle**. retoc turns them into the legacy layout (`.uasset` + `.uexp` + `.ubulk`) and back.

Mods are containers too. retoc writes mod containers uncompressed and unencrypted, so `zen.go` can read single packages straight out of them. Each chunk in a `.utoc` has a 12-byte id derived from the game file's name: two mods with the same id replace the same file (`mods.go`).

Paths: a game path `/Game/A/B` is `Live/Content/A/B.uasset` on disk (the project is `Live`).

## Packages (`uasset.go`)

A cooked package is a header (`.uasset`: summary, name map, imports, exports, dependencies) and export data (`.uexp`). Notes:
- Names with a trailing number are stored split: `X_602` is the text `X` with number 603; `X_001` (leading zero) stays whole (`fname.go`).
- Each name carries two 16-bit hashes (retoc writes zeros; new names follow whatever the package uses).
- `NamesReferencedFromExportDataCount`: names that export data may use must sit before this index; new names for export data are inserted there.
- The data-resource table at the end of the `.uasset` (44-byte entries) says where bulk data lives; streamed texture chunks and ordinary texture mips are found through it.

## Unversioned properties (`props.go`)

Cooked objects store their properties without names: a header of fragments (skip count, how many present, whether some are zero) and a zero mask, then the present values in the struct's property order. Reading needs each struct's layout; the program carries the ones it needs in `schema.json` (made from the game's `.usmap`). A `.usmap` chosen in Settings can stand in after a game update.

## Streamed UDIM textures (`vt.go`, `vtformat.go`)

Aircraft skin textures are streamed ("virtual") UDIM textures: several squares (tiles 1001, 1002…) in one texture.
- `.uexp` holds the description: pixel format (`PF_DXT1` for colour and masks, `PF_BC7` for normal maps), tile size and border, mip count, per-mip tile tables (Morton-ordered address runs), and chunk records (hash, size, codec, offset).
- `.ubulk` is a run of chunks; each chunk is a small header and fixed-size compressed tiles, each tile `Tile + 2 × Border` pixels square.
- A rebuilt texture keeps its layout byte for byte except the pixels, unless it is stored differently: **BC7** doubles the bytes per tile and chunk sizes; **no mipmaps** keeps only the full-size mip in one chunk.
- Suffixes: `_C` colour, `_MRX` metal, roughness and other finish masks, `_NO` normal map; `_D`, `_MROS`, `_N` on aircraft with ordinary textures.

Some aircraft (the Ladon) keep each skin texture as an **ordinary texture** (`vtplain.go`, `tex2d.go`): one 4096 × 4096 square with a mip chain, the large mips in `.ubulk`, the small ones inside `.uexp`.

## Skin list and unlock list (`skintable.go`)

- `/Game/Datatables/Information/DT_Skin` — one row per skin (`LiveSkinDataTable`): skin id, aircraft and skin string ids, the skin's material (`DecalMaterialRef`), category, emblem permissions (`EmblemProhibition_…` for front, wings, tails, left and right), banner and menu icon, name and description text keys, sort number, DLC id.
- `/Game/Unlock/DA_Unlock_Skin` — "when this happens, unlock that skin" entries.
- Skin ids: `aircraft × 1 000 000 + skin × 1000 + 1` (Su-57 skin 101 is `21101001`); row keys are `I_EM_<id>`.
- Each skin's material is `…/<aircraft>/Materials/<number>/MI_<aircraft>_<number>_Decal`; its icon `/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_<plane>_<number>`.

Emblems use the same pair of files (`DT_Emblem`, `DA_Unlock_Emblem`); emblem pictures are in `/Game/Emblem/tga/` (the decal, 512 × 512) and `/Game/Emblem/png/` (menu icons of 64, 128, 180 and 1024).

## Icons and portraits

- Skin icons: 180 × 180 BC7, one mip inside the `.uexp` (`icons.go`).
- Radio portraits: 428 × 540; helmets: parts of 4096 × 4096 `_C`, `_MRO`, `_N` (`pilots.go`).

## Save files (`savegame.go`)

`Campaign.sav` and `System.sav` are GVAS saves (UE 5.4 tagged properties). The top level has `SavedVersion`, `PackedData` (bytes) and `Checksum`, a CRC-32 of `PackedData` started from `0x41916EBD`. `PackedData` holds property lists ending with `None`; Campaign.sav has the common data (unlock lists) then the campaign data. Unchanged values are kept as bytes, so a save read and written without a change is byte-for-byte the same.

## Texts

Names and descriptions are text keys the game looks up. New skins, emblems and radio names use keys the game does not have; **SkinSlotText** (a UE4SS mod, source in `skinslottext-source/`) answers those keys from `.ini` files and passes every other key to the game.
