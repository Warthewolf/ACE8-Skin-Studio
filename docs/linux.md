# Linux and Steam Deck

The Linux build is the same program, built for 64-bit Linux (x86-64, Steam Deck included), for the game running through Proton.

## Installing

1. Unpack the `.tar.gz` somewhere with a short path, such as `~/ACE8SkinStudio` (the `.tar.gz` keeps the program runnable; with the `.zip` you may need step 4).
2. **retoc for Linux**: download `retoc-x86_64-unknown-linux-gnu.tar.xz` from [retoc's releases](https://github.com/trumank/retoc/releases), unpack it and put the file `retoc` in the ACE8SkinStudio folder. The Windows `retoc.exe` does not run on Linux.
3. retoc also needs **Oodle for Linux** (`liboo2corelinux64.so.9`) next to it. Skin Studio downloads it there the first time you extract.
4. Run `./ACE8SkinStudio`. If it does not start: `chmod +x ACE8SkinStudio retoc`.

The window opens as an app window in Chrome, Chromium, Edge or Brave when one is installed, otherwise in your default browser.

## Found by itself

- The game's **Paks folder** in any Steam library, including Flatpak Steam:
  `…/steamapps/common/ACE COMBAT 8/Game/Content/Paks`
- The game's **save folder** inside Proton's prefix:
  `…/steamapps/compatdata/<game>/pfx/drive_c/users/steamuser/AppData/Local/BANDAI NAMCO Entertainment/ACE COMBAT 8/Saved/SaveGames`
- Whether the game is running (the Save tools need it closed).

## Choosing files

The Browse and Choose buttons use **zenity** (GNOME) or **kdialog** (KDE, and the Steam Deck's desktop mode). Without either, type or paste paths.

## Mods folder and UE4SS under Proton

Set **Mods folder** in Settings to:
`…/steamapps/common/ACE COMBAT 8/Game/Binaries/Win64/ue4ss/Mods/IoStoreLoaderMod`

UE4SS loads under Proton only when Wine is told to use its DLL. In Steam: the game's **Properties → Launch options**, for current UE4SS (which uses `dwmapi.dll`):

```
WINEDLLOVERRIDES="dwmapi=n,b" %command%
```

## Differences from Windows

- The window is a browser window, not an Edge app window (unless a Chromium browser is installed).
- File choosers need zenity or kdialog.
- The Windows-only parts (Edge window, Windows file dialog) are in `os_windows.go` and `pick_windows.go`; their Linux counterparts are in `os_other.go`.
