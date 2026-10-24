# SkinSlotText 1.2

A small UE4SS mod for ACE COMBAT 8 that gives new skin slots their names and descriptions. Skin
mods made with ACE8 Skin Studio add skins in slots the game does not have; the game asks for their
names by text key (for example `SS_ZOESkin_uqbqn_s1_Name`), and this mod answers those keys from
plain text files. Every other text goes to the game unchanged.

Offline and single-player use only.

## What it does

The mod is a Lua script (`mod/Scripts/main.lua`) that UE4SS runs, and a DLL it loads
(`src/skinslottext.c`, about 400 lines of C, no libraries).

1. It reads `SkinSlotText.ini` and `texts\*.ini` next to the DLL (one file per skin mod, `[Text]`
   section, `Key=Text` lines).
2. It finds the game's text lookup function: at its known address in game build 1.1.2.0, or by
   searching the game's executable sections for the function's known bytes in other builds.
3. It replaces the first 15 bytes of that function, in memory only, with a jump to its own lookup.
   Keys from its text files get their text; every other key goes on to the original function
   through a trampoline. No game file is changed, and the patch is gone when the game closes.
4. It gets along with RoGText.dll (the OTSS / laser mods), which hooks the same function: it waits
   for RoGText to hook first and then goes in front of it. If the start of the function holds
   anything it does not recognise, it changes nothing.

Everything it does is written to `SkinSlotText.log` next to the DLL.

The Windows functions it uses, all from kernel32.dll, and why:

| Functions | Used for |
|---|---|
| GetModuleHandleW, GetModuleHandleExW, GetModuleFileNameW | finding the game's executable in memory, and its own folder |
| VirtualProtect, FlushInstructionCache, GetCurrentProcess, VirtualAlloc | writing the 15-byte jump, and the trampoline back to the game's code |
| CreateFileW, ReadFile, WriteFile, GetFileSize, CloseHandle, FindFirstFileW, FindNextFileW, FindClose | reading its text files, writing its log |
| MultiByteToWideChar, WideCharToMultiByte | UTF-8 text files to the game's UTF-16 text |
| GetLocalTime, GetTickCount64 | log times, and waiting for RoGText |

It has no network access, starts no programs, and opens no files outside its own folder.

## Building

Needs LLVM (clang, lld-link, llvm-dlltool, llvm-rc). No Windows SDK or Visual Studio. On Linux or macOS:

    sh build.sh

On Windows, run the four commands in `build.sh` from a shell with LLVM on its PATH. The result is
`build/SkinSlotText.dll`, and `build/SkinSlotText/` is the whole mod, ready to copy into
`ue4ss\Mods`.

The build is reproducible. With LLVM 18 the DLL is byte for byte the one that is shipped:

    SHA-256 baa0e27ebaf42d7c0e65f67011a4bc4b2591c0ff3387d8d409b2c905808f7a85  SkinSlotText.dll

## Tests

`test/test_linux.c` runs the DLL's code on Linux or macOS, with the Windows functions replaced by
stand-ins and a fake game function in memory: reading the text files, hooking an untouched
function, chaining in front of RoGText, giving up when RoGText never hooks or the function holds
unknown bytes, finding the function in another game build, and doing nothing without text files.

    sh test/run_tests.sh

## Files

    build.sh               builds the DLL and the mod folder
    src/skinslottext.c     the DLL
    src/skinslottext.rc    its version information (Properties > Details in Windows)
    src/kernel32.def       the kernel32 functions it links against
    mod/enabled.txt        tells UE4SS to run the mod
    mod/Scripts/main.lua   loads the DLL and gives it a moment each second to decide
    test/                  the Linux tests

Any issues please contact @voidcallsreaper on Discord.
