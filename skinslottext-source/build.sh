#!/bin/sh
# Builds SkinSlotText.dll with LLVM only (clang, lld-link, llvm-dlltool, llvm-rc). No Windows SDK or Visual
# Studio is needed, and it works the same on Linux, macOS or Windows (run the same four commands
# from a shell that has LLVM on its PATH).
#
# With LLVM 18 the result is byte for byte the SkinSlotText.dll that is shipped: compare the
# SHA-256 printed at the end with the one in README.md.
set -e
cd "$(dirname "$0")"
OUT=${OUT:-build}
mkdir -p "$OUT/SkinSlotText/Scripts"

llvm-dlltool -m i386:x86-64 -d src/kernel32.def -l "$OUT/kernel32.lib"

llvm-rc /fo "$OUT/skinslottext.res" src/skinslottext.rc

clang --target=x86_64-pc-windows-msvc -O2 -ffreestanding -fno-builtin -nostdlibinc \
      -fno-stack-protector -mno-stack-arg-probe -funwind-tables \
      -c src/skinslottext.c -o "$OUT/skinslottext.obj"

lld-link /dll /nodefaultlib /entry:DllMain /subsystem:windows /Brepro \
         /out:"$OUT/SkinSlotText.dll" "$OUT/skinslottext.obj" "$OUT/skinslottext.res" "$OUT/kernel32.lib"

# the whole UE4SS mod, ready to copy into ue4ss\Mods
cp mod/enabled.txt "$OUT/SkinSlotText/"
cp mod/Scripts/main.lua "$OUT/SkinSlotText/Scripts/"
cp "$OUT/SkinSlotText.dll" "$OUT/SkinSlotText/Scripts/"

echo "built $OUT/SkinSlotText.dll and the mod folder $OUT/SkinSlotText"
if command -v sha256sum >/dev/null 2>&1; then sha256sum "$OUT/SkinSlotText.dll"
elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$OUT/SkinSlotText.dll"; fi
