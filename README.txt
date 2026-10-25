ACE8 Skin Studio 2.4.5
======================

Extract, paint and repack aircraft skins for Ace Combat 8, and add your own
skins in new skin slots, with their own names, descriptions and icons. Add
emblems of your own. Paint pilots' helmets, change the portraits shown next
to radio messages, and give the people on the radio new names. Unlock your
new skins and emblems in your save straight away, and get the save ready
for the game without mods.

Use modded files offline and in single player only.


What you need
-------------
- Windows 10 or 11.
- retoc.exe, the free tool that unpacks and packs the game's containers.
  It is included in this folder and is found automatically as long as it
  stays next to ACE8SkinStudio.exe. retoc is by Truman Kilen and Archengius
  and is shared under the MIT licence (see retoc-LICENSE.txt). Newer
  versions: https://github.com/trumank/retoc/releases
  The first time it reads the game files, retoc may download the Oodle
  decompression library it needs, so be online for the first extraction.
- The game's AES key (64 hexadecimal characters). The program does not
  include it.
- A mod loader that lets the game load unsigned mod containers. New skin
  slots need AC8 Mod Loader 1.1.0 or newer: older loaders do not find files
  that are new to the game, so a new skin would show without its paint and
  icon.
- UE4SS (which the mod loader runs in). New skins get their names and
  descriptions from SkinSlotText, a small UE4SS mod that this program
  installs for you.
- Optional: FModel, to export an aircraft's 3D model as .glb.

ACE8 Skin Studio is a separate program from AC8 Skin Studio 1.x. Both can be
kept and used side by side; they have their own settings and workspace.


Starting it
-----------
Put the ACE8SkinStudio folder somewhere with a short path, for example
C:\ACE8SkinStudio, and double-click ACE8SkinStudio.exe. A window opens. Do
not run it from inside a .zip file.

The first time, Windows may show "Windows protected your PC" because the
program is not signed. Choose "More info", then "Run anyway".

Open Settings. If you used AC8 Skin Studio 1.x, press "Use the settings of
AC8 Skin Studio 1.x" and choose its folder: the game folder, AES key, mods
folder and your extracted skins are copied over. Otherwise fill in:
- Game Paks folder   ...\ACE COMBAT 8\Game\Content\Paks
- AES key
- retoc program      leave empty; the included retoc.exe is used
- Mods folder        the folder your mod loader reads. For AC8 Mod Loader:
                     ...\ACE COMBAT 8\Game\Binaries\Win64\ue4ss\Mods\
                     IoStoreLoaderMod
- UE4SS Mods folder  found by itself when the mods folder is inside it
- Game's save folder found by itself (...\AppData\Local\BANDAI NAMCO
                     Entertainment\ACE COMBAT 8\Saved\SaveGames)
- Mappings (.usmap)  optional; see "After a game update" below

The Appearance switch at the bottom left chooses Dark, Day or System.


The screens
-----------
Extract, Split and Model work as in AC8 Skin Studio 1.x: get a skin's
textures out of the game as paint-ready tiles, cut wide pictures into tiles,
and keep only the parts of a model that wear the skin.

Most aircraft keep each skin texture as tiles (1001, 1002 and so on). Some,
such as the Ladon, keep a picture per part instead (Body and Wing, each
4096 x 4096, ending in _D for colour, _MROS for metal and roughness and _N
for the normal map). Extract writes those as one picture each, without a
tile number, and Repack builds them back. When an aircraft has more than one
part, each picture's name must say which part it is for (Body_01 or
Wing_01, or simply Body or Wing); a picture that does not say is used for
neither. New skin slots need the aircraft to be in the game's skin list, so
for such aircraft use Replace a game skin.

Repack (Replace a game skin)
  Choose the skin to replace and drop the pictures you exported after
  painting. The table shows what each game texture will be built from. Tick
  the textures to rebuild, name the mod and press "Build mod".
  New: "Name, description and icon" can also change what the game shows for
  that skin: a new name and description (through SkinSlotText) and a new
  icon, either the game's icon recoloured or a picture of your own.

  How the colour texture is stored (below the table, both for Repack and
  for New skin slot; the boxes keep your last choice):
  - Sharper colours (BC7): the colour texture is stored as BC7 instead of
    the game's DXT1. Thin lines, small markings, text and soft colour
    gradients come out much closer to your picture. The colour texture's
    file is twice as big (about 120 MB instead of 60 MB for a Su-57 skin)
    and takes longer to build. The game reads BC7 the same way it reads its
    own BC7 textures (the aircraft normal maps are BC7).
  - No mipmaps: only the full-size picture is kept, without the smaller
    copies the game uses for planes further away. Paint stays sharp at a
    distance, but planes far away shimmer, and the whole texture stays in
    video memory. Not yet tried in game: if the skin looks wrong or the
    game has trouble, build it again without this.
  The finish (metal and roughness) and normal map keep the game's own way.

New skin slot
  Adds your paint to the game as a skin of its own, next to the game's
  skins. Nothing of the game is replaced.
  1. Choose the skin to start from: the one you painted on. The new skin
     copies its material, finish, normal map and where emblems may go.
     When your pictures' names say which skin they were painted on (as
     Substance Painter's and this program's do), it is chosen for you as
     soon as you choose the pictures. "Add a folder of raw files" is only
     for raw game files (.uasset, .uexp, .ubulk), not for your pictures.
  2. Drop your painted pictures and tick the textures to rebuild, as for
     Repack. Without pictures the new skin keeps the original's look; only
     its name, description and icon are new.
  3. Give it a name and a description (the hangar shows both). It unlocks
     the way the game unlocks its own first skins: when you finish a
     campaign mission.
  4. Choose its icon: keep the original, recolour the original in your
     paint's main colour or a colour you pick, or use your own picture (its
     middle square is used).
  5. Make a new skin mod, or add the skin to a skin mod you made before.
  6. Press "Build new skin". The skin mod is installed into your mods folder
     straight away, the shared skin list is updated, and the skin gets the
     first free skin number from 601 up.
  7. Start the game and finish a campaign mission. The game unlocks new
     skins when a mission ends; after that the skin is in the hangar for
     good.
  Building again after that updates the same skin; "Start another skin"
  begins a new one.

New emblem
  Adds a picture of yours to the game's emblem list, with its own name and
  description. It goes on any aircraft like the game's emblems.
  1. Drop the picture (PNG, JPEG or TGA). A PNG with a see-through background
     works best: the see-through parts show the aircraft's paint. The whole
     picture is used, fitted into a square; 512 x 512 or larger looks
     sharpest. The preview shows it with checks where it is see-through.
  2. Give it a name and a description (the emblem list shows both).
  3. Make a new skin mod, or add the emblem to a skin mod you made before. A
     skin mod can hold new skins, emblems or both.
  4. Press "Build new emblem". The program makes the five pictures the game
     uses (the decal on the aircraft and the icons in the menus), installs
     the mod, and the emblem gets the first free emblem number from 601 up.
  5. It unlocks when you finish a campaign mission, like the game's emblems,
     or straight away with Unlock now on the Save tools screen.
  "Change picture" and "Edit name" on the Skin slots screen change it later;
  it keeps its number, so aircraft wearing it keep it.

Pilots
  Helmets, radio portraits and the names on the radio.
  1. Press "Find pilots in the game files" once. It finds every pilot with a
     helmet and every radio portrait, and makes a small picture of each
     helmet (the full-size files it fetched for that are deleted again).
  2. Pick a pilot. Their helmet is shown in parts, each with its own
     pictures: Tasha's has two, the shell (Helmet01_01) and the mask and
     fittings (Helmet01_02).
  3. "Get paint-ready pictures" saves base colour, metallic, roughness,
     ambient occlusion and normal map pictures of every part, full size
     (4096 x 4096), in workspace\paint-ready\Pilots.
  4. Drop your painted pictures on the part they are for, or drop them all
     at once when their names say the part (Helmet01_02, or a material name
     such as MI_PI003_Helmet02 as Substance Painter writes it). The names
     must also say what each picture is: Base_color, Metallic, Roughness,
     AO or Normal, or the game's endings _C, _MRO and _N. Whatever you leave
     out stays as the game's.
  5. Click a radio portrait to change it: a picture of your own (drag it
     into place, scroll to zoom), or the photo of another portrait; and keep
     the text or write new text (name, call sign, rank, unit) in the game's
     own font. When a pilot has two portraits, both can be changed at once.
     New portraits are kept when you close the program (in
     workspace\pilots\my-portraits, with your photo and how you placed it),
     so you can come back later, change them or add more. Portraits made
     with a version before 2.4.2 are inside the mod built from them: the
     Pilots screen offers to bring them back.
  6. Name the mod and press "Build mod". The helmet and the portraits go
     into one mod, which replaces those game files. Every build has all the
     kept portraits, so building again never loses one.
  7. Names on the radio: type a new name for anyone who talks on the radio,
     and a new callsign for the wingmen (Baxter, Tasha, Coster, Kate).
     Flight names (Joker 3, Queen 1 ...) are in their own list: they belong
     to a place in the flight, so whoever flies there gets the new name.
     Press "Save names". Names need no building: SkinSlotText shows them,
     from the file SkinStudioNames.ini in its texts folder, the next time
     the game starts. After a game, each saved name says whether the game
     showed it ("Seen in your last game").
     Baxter, Tasha, Coster and Kate also have a "Hangar name" box, for the
     hangar's pilot list. Left empty, the hangar gets the new name in
     capitals, the way the hangar writes names.
  The text written on a portrait is part of the picture; change it there.
  A few story moments use a name of their own and keep the game's.

Useful links
  Other modders' pages with templates for the game's skins, which open in
  your browser:
  - Skin Template Repository - Project Pileus (Nimbus Custom Works, uploaded
    by M4rkoz): layered PSD and XCF templates with the finish, normal map and
    dirt mask textures and a Blender preview model.
    https://www.nexusmods.com/acecombat8wingsoftheve/mods/87
  - Skin templates by Rythus.
    https://www.nexusmods.com/acecombat8wingsoftheve/mods/34
  Paint on a template, export your pictures (PNG or TGA, one per tile) and
  bring them into the game with Repack or New skin slot.

Skin slots
  Every new skin and emblem you have, grouped by skin mod, with its icon,
  name and number.
  - Import a skin mod: drop a .zip (for example from Nexus Mods). Before
    anything is installed you see its skins and the numbers they will get.
    A skin whose number is already taken by another skin gets the next free
    one; its name, description and icon go with it, and skins that were
    there first keep their numbers. Importing a newer version of a mod you
    have replaces the old one.
  - Share as .zip: makes a file to upload or send. Anyone with ACE8 Skin
    Studio can import it next to their other skins. It also carries what is
    needed to install it by hand without the program (see its README), but
    then only one new-slot skin mod works at a time. There is no zip inside
    the zip, so Nexus Mods takes it as it is.
  - Edit name: changes a skin's or emblem's name and description.
  - Change paint or icon (skin mods made on this computer): opens New skin
    slot set up to rebuild that skin. Its own textures are kept unless new
    pictures replace them.
  - Change picture (emblems made on this computer): opens New emblem set up
    to change that emblem.
  - Remove: takes a skin or emblem out of a mod, or a whole skin mod out of
    the mods folder (it is moved to workspace\removed-mods, not deleted).
    Skin mods you made can be put back from the list at the bottom.
  - Rebuild the skin list: makes the shared skin list again. The screen says
    when this is needed: after a game update, or when skin mods were added
    or removed by hand.

Mods
  Lists the mods in your mods folder with the game files each one replaces,
  and marks mods that replace the same files. Skin mods with new skin slots
  are marked "new skin slots" and the shared skin list "shared skin list";
  they do not conflict with each other.

Save tools
  Changes to your campaign save (Campaign.sav) that the game does not offer.
  Close the game first: it writes its save when it closes, which would undo
  the change. The buttons wait until it is closed.
  - Unlock now: puts every new skin and emblem in the game's lists into your
    save as unlocked (marked new in the hangar), without finishing a
    mission.
  - Prepare my save for the game without mods: do this before you take the
    mods out. The new skins and emblems come out of the unlocked lists, and
    every aircraft wearing one goes back to the game's own skin (or no
    emblem). Your progress, money, aircraft and everything else stay as they
    are. When the mods are back, press Unlock now and choose them again.
  - Backups: before every change both save files (Campaign.sav and
    System.sav) are copied into a dated folder in SkinStudioSaveBackups,
    next to the game's SaveGames folder. "Put this back" restores one; the
    save as it is then is backed up first, so a restore can be undone.
    System.sav (controls, sound and screen settings) is only put back when
    you tick it.
  - Backup at game start: SkinStudioSaveBackup, a small UE4SS mod installed
    with SkinSlotText, copies both saves each time the game starts and keeps
    the last 20. It only reads the save. Turn it off or on here.
  The changed save is read back and its checksum checked before it replaces
  the old one. If Steam reports a cloud conflict the next time the game
  starts, keep the files on this computer.


How new skin slots work
-----------------------
The game knows its skins from one skin list and one unlock list. Only one
copy of each can be in effect, so the program builds one shared copy (the
mod zz_SkinStudio_SkinList) from the game's own lists plus every installed
skin mod, and rebuilds it whenever skins are added, changed or removed.

Each new skin gets a material slot of its own, numbered like the game's
skins: Su-57 skin 603 has its material in PP0021_su57\Materials\603\ and
its icon T_UI_Icon_Skin_su57_603 next to the game's icons. These small files
are made with the shared skin list, because that is where skins get their
numbers; a skin that has to move to another number takes its slot along.
Each skin mod keeps them in its slotfiles folder (2.0.2 to 2.0.8 kept them
in slotfiles.zip, which is still read).

The big files, the textures, stay in the skin's own mod, in a folder of
their own inside the game's content (SkinStudio\<mod>\<skin>\), so two
skin mods never replace each other's files, whatever skin numbers they use.

Mods made with other tools that carry their own copy of the skin list are
found too. Their skins and changes are copied into the shared list, so they
keep working next to your skin mods.

Names and descriptions are not in the skin list itself. SkinSlotText (in
UE4SS's Mods folder) reads them from one small file per skin mod, in
SkinSlotText\Scripts\texts\.

New emblems work the same way: the shared list mod also carries one copy
of the game's emblem list and emblem unlock list, with every new emblem
added. Their pictures stay in their own skin mod, in Emblem\tga\SS and
Emblem\png\SS.

Before taking out a skin mod, choose another skin for that aircraft in the
game, so the game does not look for a skin that is gone. Before playing the
game without any mods, use "Prepare my save for the game without mods" on
the Save tools screen.


Where files go
--------------
Everything is kept in the "workspace" folder next to the program (you can
move it in Settings):
  workspace\raw           raw game files from Extract
  workspace\paint-ready   tiles made from game textures, and pilots' helmet
                          pictures (in Pilots)
  workspace\split         tiles made on the Split screen, one folder per skin
  workspace\models        models made on the Model screen
  workspace\mods          finished Repack mods
  workspace\slot-mods     the source files of your skin mods (keep these to
                          change your skins later): textures in input,
                          materials and icons in slotfiles
  workspace\skin-list     the shared skin list as last built
  workspace\base          game files new skins are built from (fetched again
                          after a game update)
  workspace\shared        .zip files made with "Share as .zip"
  workspace\removed-mods  mods taken out of the mods folder
  workspace\previews      small preview pictures (safe to delete)
  workspace\pilots        the list of pilots and portraits, and their small
                          pictures (safe to delete; Find pilots makes it again)
Save backups are kept next to the game's own SaveGames folder:
  ...\ACE COMBAT 8\Saved\SkinStudioSaveBackups   one dated folder per backup
  ...\ACE COMBAT 8\Saved\SkinStudioSaveBackups\auto   the game-start backups
Settings are saved in settings.json next to the program. The file holds your
AES key, so leave it out if you share the program with someone.


After a game update
-------------------
- The program fetches the game's skin list again by itself, and the Skin
  slots screen asks you to rebuild the shared skin list. Do that before
  playing, so the game's own new skins are kept.
- If the update changed how the skin list is stored, building stops with a
  message saying so. A newer mappings file (.usmap, made with UE4SS) set in
  Settings then lets it read the new layout.
- A game update can change the AES key and may need a newer retoc.


Limits
------
- Aircraft: works on the game's streamed UDIM skin textures (colour, finish,
  normal, masks). Ordinary aircraft textures such as the landing gear and
  cockpit are listed but cannot be rebuilt yet. (Pilots' helmets and
  portraits are ordinary textures; the Pilots screen rebuilds those.)
- Rebuilding the normal map is experimental: the result is slightly less
  sharp than the original.
- New skins need AC8 Mod Loader 1.1.0 or newer and SkinSlotText. Without
  SkinSlotText a new skin shows its text key instead of its name.
- Skin mods from others can only be changed (paint, icon) by the person who
  made them; their names and descriptions can be edited.
- Mods from other tools are read when their containers are not encrypted or
  compressed with Oodle (mods packed with retoc are neither).


If something goes wrong
-----------------------
- The build button stays grey: the text beside it says what is missing.
  Click it to go to that step.
- A task fails: the message says what most likely went wrong. "Show details"
  at the bottom of the window shows what retoc printed.
- Asking for help: press "Copy report" next to a failed task, or "Copy a
  support report" under Settings > About. It copies the version, your
  settings and the last task's log. The AES key is never included and your
  Windows user name is taken out of the folder paths.
- A new skin or emblem does not appear in the game: finish a campaign
  mission first (the game unlocks them when a mission ends), or close the
  game and press Unlock now on the Save tools screen. If it is still missing,
  check the Skin slots screen: the mod loader must be 1.1.0 or newer, and
  the skin list must be up to date.
- A new skin appears without its paint or icon: the mod loader is older than
  1.1.0, or AC8IoStoreLoader.log (in the loader's folder) does not say "new
  files" after a game start.
- "Extracted 0 (6 failed)" or "none of this aircraft's skin textures could
  be shown": retoc could not read the game's compressed files. It needs
  oo2core_9_win64.dll (Oodle) next to retoc.exe. Skin Studio copies it from
  the game's folder or downloads it before each extract; if that fails, the
  message says why (usually a firewall, or retoc in a folder that cannot be
  written to: keep retoc.exe next to this program).
- The workspace folder cannot be used (for example on a drive that is not
  plugged in): Settings says so; choose another folder, or press "Use the
  folder next to the program".
- No window appears: open ACE8SkinStudio.log next to the program. The window
  is shown with Microsoft Edge; tick "Open in my default browser" in
  Settings if you prefer another browser.
- The save looks wrong after a change: close the game, open Save tools and
  press "Put this back" next to the backup made before the change.
- To close the program, close its window, or use Settings > Quit.


Credits
-------
Container unpacking and packing is done by retoc, by trumank.
The BC7 decoder follows the reference tables used in Pillow (HPND licence).
New portrait text is written in the game's own font, read from the game's
files on your computer; the font is not part of this program.


Any issues please contact @voidcallsreaper on Discord.
