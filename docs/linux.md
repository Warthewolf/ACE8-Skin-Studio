# Linux and Steam Deck

The Linux build is the same program, built for 64-bit Linux (x86-64, Steam Deck included), for the game running through Proton.

## Installing

1. Unpack the `.tar.gz` somewhere with a short path, such as `~/ACE8SkinStudio` (the `.tar.gz` keeps the program runnable; with the `.zip` you may need step 4).
2. **retoc for Linux comes with it**: the file `retoc` next to `ACE8SkinStudio` (the Windows package has `retoc.exe` instead; the Windows `retoc.exe` does not run on Linux, and Settings says so when that is the one it finds). To use another retoc, download `retoc_cli-x86_64-unknown-linux-gnu.tar.xz` from [retoc's releases](https://github.com/trumank/retoc/releases) and unpack it in the ACE8SkinStudio folder: `retoc` is also found in the `retoc_cli-x86_64-unknown-linux-gnu` folder the download unpacks to.
3. retoc also needs **Oodle for Linux** (`liboo2corelinux64.so.9`) next to it. Skin Studio downloads it there the first time you extract (next to the real file when retoc is a symbolic link, which is where retoc looks).
4. Run `./ACE8SkinStudio`. If it does not start: `chmod +x ACE8SkinStudio`. retoc is marked as a program by Skin Studio when it is not.

## When retoc does not run

Settings checks retoc before it is used and says what is wrong:

| Settings says | What to do |
|---|---|
| *is the Windows version (retoc.exe)* | Get the Linux download (step 2). |
| *made for ARM computers* | You downloaded `aarch64`; get `x86_64`. |
| *was not found* | retoc was taken out of the folder (or never unpacked). Unpack the Linux package again. |
| *made for a newer Linux than this one: it needs glibc 2.34* | retoc's ready-made Linux build is built on Ubuntu 22.04 and needs glibc 2.34 or newer (Ubuntu 22.04, Debian 12, Fedora 36, Linux Mint 21 and newer). On an older Linux, build retoc for it with Rust ([rustup.rs](https://rustup.rs)): `cargo install --git https://github.com/trumank/retoc retoc_cli`, then choose `~/.cargo/bin/retoc` in Settings. |
| *a library it needs is missing* | Install the library named, or build retoc as above. |
| *Linux refused to start retoc* | The folder is on a drive that does not allow programs (often a USB stick or SD card mounted `noexec`): move the ACE8SkinStudio folder into your home folder. |
| *could not be put in … (Oodle)* | retoc is in a folder you cannot write to (for example `/usr/bin` from a package): copy it into the ACE8SkinStudio folder and choose that copy. |

When retoc fails on one file, Skin Studio tries again one file at a time with retoc's output going to a terminal (a pseudo-terminal), since retoc only prints its reasons there; the reason goes into Show details and the support report.

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
