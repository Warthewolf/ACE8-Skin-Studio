#!/bin/sh
# Runs SkinSlotText's logic on Linux or macOS: the Windows functions are replaced by stand-ins that
# work on real files and a fake game function in memory (see test_linux.c). Needs a C compiler.
set -e
cd "$(dirname "$0")"
CC=${CC:-cc}
$CC -O1 -Wall -I../src -o test_linux test_linux.c
fails=0
for sc in texts alone rogtext rogtext_never unknown moved moved_hooked notexts; do
    dir=$(mktemp -d)
    mkdir -p "$dir/texts"
    if ./test_linux "$sc" "$dir" >"$dir/out.txt" 2>&1; then
        echo "ok    $sc"
    else
        echo "FAIL  $sc"; cat "$dir/out.txt"; fails=1
    fi
    rm -rf "$dir"
done
rm -f test_linux
[ $fails = 0 ] && echo "all scenarios passed"
exit $fails
