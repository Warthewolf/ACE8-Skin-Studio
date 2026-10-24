# Troubleshooting

Every failed task says what most likely went wrong. **Show details** at the bottom of the window shows exactly what retoc printed.

## Asking for help

Press **Copy report** next to a failed task, or **Copy a support report** under Settings → About, and paste it into your message to **@voidcallsreaper** on Discord. The report holds the program's version, your settings, the checks, the installed mods and the last task's log. It **never** contains the AES key, and your Windows user name is taken out of folder paths.

## Common messages

| What you see | What it means | What to do |
|---|---|---|
| **Setup needed** at the top | A setting is missing or wrong | Open Settings: each red note says what to fix |
| **Extracted 0 (6 failed)**, "none of this aircraft's skin textures could be shown", or a list "was not found in the game files" | retoc could not read the game's compressed files: it needs Oodle (`oo2core_9_win64.dll`) next to `retoc.exe` | 2.4.4 and later fetch it for you before each extract. If the message says it could not be fetched: check your internet connection and antivirus, keep `retoc.exe` next to the program (not in Program Files), or download the file from the link in the message and put it next to `retoc.exe` |
| "retoc does not accept the AES key" | The key is not 64 hexadecimal characters | Check it in Settings |
| "The AES key does not open the game's files" | Wrong key, or the game updated and changed its key | Get the current key |
| "This version of retoc does not understand the game's files" | The game updated | Get the newest retoc from [its releases](https://github.com/trumank/retoc/releases) |
| "The disk is full" | One aircraft's textures take about 2–4 GB | Free space, or move the workspace in Settings |
| "A file is in use by another program" | The game or FModel has the file open | Close them and try again |
| "Windows refused access to a file" | The workspace or retoc is in a protected folder, or antivirus blocks retoc | Use a folder you can write to; allow retoc in your antivirus |
| "The workspace folder cannot be used" / "drive E: is not on this computer" | The workspace is on a drive that is unplugged, or settings were copied from another PC | Settings → Workspace folder: choose another, or press **Use the folder next to the program** |
| The build button stays grey | Something the build needs is missing | The text beside the button says what; click it to go there |

## In the game

| What you see | What to do |
|---|---|
| A new skin or emblem does not appear | Finish a campaign mission (the game unlocks them when a mission ends), or close the game and press **Unlock now** on Save tools. Still missing: check the Skin slots screen (mod loader 1.1.0 or newer, skin list up to date) |
| A new skin shows without its paint or icon | The mod loader is older than 1.1.0, or `AC8IoStoreLoader.log` (in the loader's folder) does not say "new files" after a game start |
| A new skin shows its text key instead of its name | SkinSlotText is missing or UE4SS is not loading; the Skin slots screen says which |
| The save looks wrong after a change | Close the game, open Save tools and press **Put this back** next to the backup made before the change |
| Two mods fight over the same skin | The Mods screen lists mods that replace the same files; only one can win |

## The program itself

| What you see | What to do |
|---|---|
| No window appears | Open `ACE8SkinStudio.log` next to the program. The window is an Edge app window; tick "Open in my default browser" in Settings to use another browser |
| "Windows protected your PC" | More info → Run anyway (the program is not signed) |
| The window is still open after closing the program | Use Settings → Quit; the program also leaves a few seconds after its last window closes |
