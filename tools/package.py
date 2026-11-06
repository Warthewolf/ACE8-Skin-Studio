#!/usr/bin/env python3
"""Makes the release packages of ACE8 Skin Studio, each with the retoc for its system.

    python3 tools/package.py                 # build everything into dist/
    python3 tools/package.py --retoc v0.1.5  # another retoc release
    python3 tools/package.py --retoc-dir DIR # retoc archives downloaded by hand (no internet)

Packages (see docs/building.md):
    ACE8SkinStudio-<v>.zip                    Windows: ACE8SkinStudio.exe and retoc.exe
    ACE8SkinStudio-<v>-linux-x86_64.tar.gz    Linux: ACE8SkinStudio and retoc (both executable)
    ACE8SkinStudio-<v>-linux-x86_64.zip       the same as a .zip (permissions kept for unzip)
    ACE8SkinStudio-<v>-with-UE4SS-and-ModLoader-<loader>.zip   with --bundle-win64 DIR
    ACE8SkinStudio-<v>-source.zip             this repository at HEAD (git archive)

retoc comes from https://github.com/trumank/retoc/releases (MIT licence, shipped as
retoc-LICENSE.txt). Each archive is checked against the .sha256 file the release publishes, and
against tools/retoc-lock.json: the first time a retoc release is used its hashes are written
there, and from then on a download that differs is refused. Commit retoc-lock.json.

Oodle is never shipped: it is not ours to redistribute; retoc and Skin Studio fetch it on the
player's computer.
"""
import argparse
import hashlib
import io
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import time
import urllib.request
import zipfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LOCK = os.path.join(ROOT, "tools", "retoc-lock.json")
RETOC_DEFAULT = "v0.1.5"
RETOC_BASE = "https://github.com/trumank/retoc/releases/download/"

# what each package needs from retoc's release
TARGETS = {
    "windows": {"asset": "retoc_cli-x86_64-pc-windows-msvc.zip", "exe": "retoc.exe", "kind": "windows"},
    "linux": {"asset": "retoc_cli-x86_64-unknown-linux-gnu.tar.xz", "exe": "retoc", "kind": "linux-x86_64"},
}


def die(msg):
    print("package: " + msg, file=sys.stderr)
    sys.exit(1)


def sha256(b):
    return hashlib.sha256(b).hexdigest()


def program_kind(b):
    """What a program file is, from its first bytes (as retoccheck.go programKind)."""
    if b[:2] == b"MZ":
        return "windows"
    if b[:4] == b"\x7fELF" and len(b) >= 20:
        machine = int.from_bytes(b[18:20], "big" if b[5] == 2 else "little")
        return {0x3E: "linux-x86_64", 0xB7: "linux-aarch64"}.get(machine, "linux-other")
    return "unknown"


def app_version():
    src = open(os.path.join(ROOT, "settings.go"), encoding="utf-8").read()
    m = re.search(r'const appVersion = "([^"]+)"', src)
    if not m:
        die("appVersion not found in settings.go")
    v = m.group(1)
    first = open(os.path.join(ROOT, "README.txt"), encoding="utf-8").readline().strip()
    if not first.endswith(" " + v):
        die(f"README.txt starts with {first!r}, but settings.go says version {v}; set both (docs/building.md, Releasing)")
    return v


def run(cmd, **kw):
    print("  $ " + " ".join(cmd))
    subprocess.run(cmd, cwd=kw.pop("cwd", ROOT), check=True, **kw)


def build_programs(out):
    run([sys.executable, os.path.join("ui", "build.py")])
    env = dict(os.environ, CGO_ENABLED="0", GOARCH="amd64")
    win = os.path.join(out, "ACE8SkinStudio.exe")
    lin = os.path.join(out, "ACE8SkinStudio")
    run(["go", "build", "-trimpath", "-ldflags=-s -w -H=windowsgui", "-o", win, "."], env=dict(env, GOOS="windows"))
    run(["go", "build", "-trimpath", "-ldflags=-s -w", "-o", lin, "."], env=dict(env, GOOS="linux"))
    return win, lin


def fetch(url):
    print("  downloading " + url)
    req = urllib.request.Request(url, headers={"User-Agent": "ACE8SkinStudio-packager"})
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=120) as r:
                return r.read()
        except Exception as e:  # network trouble: try again a few times
            if attempt == 3:
                die(f"could not download {url} ({e}). Download it by hand and use --retoc-dir")
            time.sleep(2 ** (attempt + 1))


def published_hash(text, asset):
    """The hash in a .sha256 or sha256.sum file for asset."""
    for line in text.splitlines():
        parts = line.replace("*", " ").split()
        if parts and re.fullmatch(r"[0-9a-fA-F]{64}", parts[0]) and (len(parts) == 1 or parts[-1] == asset):
            return parts[0].lower()
    return None


def get_retoc_archives(tag, base, local_dir):
    lock = json.load(open(LOCK)) if os.path.exists(LOCK) else {}
    locked = lock.get(tag, {})
    got = {}
    for name, t in TARGETS.items():
        asset = t["asset"]
        if local_dir:
            p = os.path.join(local_dir, asset)
            if not os.path.exists(p):
                die(f"{asset} is not in {local_dir}")
            data = open(p, "rb").read()
            sums = os.path.join(local_dir, asset + ".sha256")
            want = published_hash(open(sums).read(), asset) if os.path.exists(sums) else None
        else:
            data = fetch(base + tag + "/" + asset)
            want = published_hash(fetch(base + tag + "/" + asset + ".sha256").decode(), asset)
        h = sha256(data)
        if want and h != want:
            die(f"{asset}: SHA-256 {h} is not the one the release publishes ({want})")
        if not want and not locked.get(asset):
            die(f"{asset}: no published SHA-256 to check it against, and none in {os.path.relpath(LOCK, ROOT)}")
        if locked.get(asset) and locked[asset] != h:
            die(f"{asset}: SHA-256 {h} is not the one recorded in {os.path.relpath(LOCK, ROOT)} ({locked[asset]}). "
                "The release changed since it was first used; check it before shipping it")
        locked[asset] = h
        got[name] = data
        print(f"  {asset}: {h} ok")
    lock[tag] = locked
    with open(LOCK, "w") as f:
        json.dump(lock, f, indent=2, sort_keys=True)
        f.write("\n")
    return got


def unpack_retoc(name, archive):
    """The program and its licence from a retoc release archive."""
    t = TARGETS[name]
    files = {}
    if t["asset"].endswith(".zip"):
        with zipfile.ZipFile(io.BytesIO(archive)) as z:
            for info in z.infolist():
                if not info.is_dir():
                    files[info.filename] = z.read(info)
    else:
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:*") as tf:
            for m in tf.getmembers():
                if m.isfile():
                    files[m.name] = tf.extractfile(m).read()
    exe = [v for k, v in files.items() if os.path.basename(k) == t["exe"]]
    lic = [v for k, v in files.items() if os.path.basename(k).upper() in ("LICENSE", "LICENSE.TXT", "LICENSE-MIT")]
    if len(exe) != 1:
        die(f"{t['asset']} holds {len(exe)} files named {t['exe']}, expected 1")
    if not lic:
        die(f"{t['asset']} has no LICENSE file to ship as retoc-LICENSE.txt")
    kind = program_kind(exe[0])
    if kind != t["kind"]:
        die(f"{t['exe']} from {t['asset']} is a {kind} program, expected {t['kind']}; it is not shipped")
    return exe[0], lic[0]


class Package:
    """A .zip or .tar.gz whose files sit in one folder, with Unix permissions kept."""

    def __init__(self, path, folder="ACE8SkinStudio"):
        self.path, self.folder, self.files = path, folder, []

    def add(self, name, data, exe=False):
        if os.path.basename(name).lower() == "settings.json":
            die("settings.json holds the AES key and is never shipped")
        self.files.append((self.folder + "/" + name, data, 0o755 if exe else 0o644))

    def add_tree(self, src, under):
        for d, _, names in os.walk(src):
            for n in sorted(names):
                p = os.path.join(d, n)
                rel = os.path.relpath(p, src).replace(os.sep, "/")
                self.add(under + "/" + rel, open(p, "rb").read(), os.access(p, os.X_OK))

    def write(self):
        when = time.localtime()[:6]
        if self.path.endswith(".zip"):
            with zipfile.ZipFile(self.path, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
                for name, data, mode in self.files:
                    zi = zipfile.ZipInfo(name, when)
                    zi.create_system = 3  # Unix, so unzip keeps the permissions below
                    zi.external_attr = (stat.S_IFREG | mode) << 16
                    zi.compress_type = zipfile.ZIP_DEFLATED
                    z.writestr(zi, data)
        else:
            with tarfile.open(self.path, "w:gz") as tf:
                for name, data, mode in self.files:
                    ti = tarfile.TarInfo(name)
                    ti.size, ti.mode, ti.mtime = len(data), mode, time.time()
                    tf.addfile(ti, io.BytesIO(data))
        print(f"  {os.path.basename(self.path)}  {os.path.getsize(self.path) / 1e6:.1f} MB  sha256 {sha256(open(self.path, 'rb').read())}")
        for name, _, mode in self.files:
            print(f"      {oct(mode)[-3:]}  {name}")


def check_package(path, want_retoc):
    """Reads a package back: the right retoc is in it, and nothing that must not be shipped."""
    names = {}
    if path.endswith(".zip"):
        with zipfile.ZipFile(path) as z:
            for i in z.infolist():
                names[i.filename] = (z.read(i), (i.external_attr >> 16) & 0o777)
    else:
        with tarfile.open(path) as tf:
            for m in tf.getmembers():
                if m.isfile():
                    names[m.name] = (tf.extractfile(m).read(), m.mode & 0o777)
    for n in names:
        b = os.path.basename(n).lower()
        if b == "settings.json" or b.startswith("oo2core") or b.startswith("liboo2core") or b.endswith(".sav"):
            die(f"{os.path.basename(path)} holds {n}, which must not be shipped")
    exe, kind = want_retoc
    hits = [(n, v) for n, v in names.items() if os.path.basename(n) == exe]
    if len(hits) != 1:
        die(f"{os.path.basename(path)} holds {len(hits)} {exe}, expected 1")
    data, mode = hits[0][1]
    if program_kind(data) != kind:
        die(f"{os.path.basename(path)}: {exe} is not a {kind} program")
    if kind.startswith("linux") and not mode & 0o100:
        die(f"{os.path.basename(path)}: {exe} is not marked as a program")
    other = "retoc" if exe == "retoc.exe" else "retoc.exe"
    if any(os.path.basename(n) == other for n in names):
        die(f"{os.path.basename(path)} also holds {other}, the retoc for the other system")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", default=os.path.join(ROOT, "dist"))
    ap.add_argument("--retoc", default=RETOC_DEFAULT, help="retoc release tag (default %(default)s)")
    ap.add_argument("--retoc-base", default=RETOC_BASE, help=argparse.SUPPRESS)
    ap.add_argument("--retoc-dir", help="folder with retoc's release archives (and their .sha256 files), instead of downloading")
    ap.add_argument("--bundle-win64", help="a Win64 folder (UE4SS and AC8 Mod Loader) for the with-UE4SS package")
    ap.add_argument("--bundle-loader", default="1.1.0", help="the mod loader version in that package's name")
    ap.add_argument("--start-here", help="START HERE.txt for the with-UE4SS package")
    ap.add_argument("--skip-build", action="store_true", help="use the programs already in --out/build")
    ap.add_argument("--no-source", action="store_true", help="do not make the source package")
    a = ap.parse_args()

    v = app_version()
    os.makedirs(a.out, exist_ok=True)
    build = os.path.join(a.out, "build")
    os.makedirs(build, exist_ok=True)
    print(f"ACE8 Skin Studio {v}, retoc {a.retoc}")
    if a.skip_build:
        win, lin = os.path.join(build, "ACE8SkinStudio.exe"), os.path.join(build, "ACE8SkinStudio")
    else:
        print("Building")
        win, lin = build_programs(build)
    if program_kind(open(win, "rb").read(64)) != "windows" or program_kind(open(lin, "rb").read(64)) != "linux-x86_64":
        die("the built programs are not the expected Windows and Linux x86-64 programs")

    print("retoc")
    archives = get_retoc_archives(a.retoc, a.retoc_base, a.retoc_dir)
    retoc_win, lic = unpack_retoc("windows", archives["windows"])
    retoc_lin, _ = unpack_retoc("linux", archives["linux"])
    rd = lambda n: open(os.path.join(ROOT, n), "rb").read()

    print("Packages")
    made = []
    # Windows
    p = Package(os.path.join(a.out, f"ACE8SkinStudio-{v}.zip"))
    p.add("ACE8SkinStudio.exe", open(win, "rb").read())
    p.add("retoc.exe", retoc_win)
    p.add("retoc-LICENSE.txt", lic)
    p.add("README.txt", rd("README.txt"))
    p.add("CHANGELOG.txt", rd("CHANGELOG.txt"))
    p.write(); made.append((p.path, ("retoc.exe", "windows")))
    if a.bundle_win64:
        b = Package(os.path.join(a.out, f"ACE8SkinStudio-{v}-with-UE4SS-and-ModLoader-{a.bundle_loader}.zip"))
        b.files = list(p.files)
        b.add_tree(a.bundle_win64, "Win64")
        if a.start_here:
            b.add("START HERE.txt", open(a.start_here, "rb").read())
        b.write(); made.append((b.path, ("retoc.exe", "windows")))
    # Linux
    for ext in (".tar.gz", ".zip"):
        p = Package(os.path.join(a.out, f"ACE8SkinStudio-{v}-linux-x86_64{ext}"))
        p.add("ACE8SkinStudio", open(lin, "rb").read(), exe=True)
        p.add("retoc", retoc_lin, exe=True)
        p.add("retoc-LICENSE.txt", lic)
        p.add("LINUX.txt", rd("LINUX.txt"))
        p.add("README.txt", rd("README.txt"))
        p.add("CHANGELOG.txt", rd("CHANGELOG.txt"))
        p.write(); made.append((p.path, ("retoc", "linux-x86_64")))
    # source
    if not a.no_source:
        src = os.path.join(a.out, f"ACE8SkinStudio-{v}-source.zip")
        run(["git", "archive", "--format=zip", "-o", src, "HEAD"])
        print(f"  {os.path.basename(src)}  {os.path.getsize(src) / 1e6:.1f} MB")

    print("Checking")
    for path, want in made:
        check_package(path, want)
        print(f"  {os.path.basename(path)}: ok ({want[0]} for {want[1]})")
    print(f"Done: {a.out}")


if __name__ == "__main__":
    main()
