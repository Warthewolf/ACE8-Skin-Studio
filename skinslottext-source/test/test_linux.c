/* Linux tests of SkinSlotText.dll's logic: Windows functions replaced by stand-ins working on real files. */
#define SST_TEST 1
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <dirent.h>
#include <strings.h>
typedef void *HANDLE; typedef int BOOL; typedef unsigned long DWORD; typedef unsigned short WORD;
typedef struct { WORD wYear, wMonth, wDayOfWeek, wDay, wHour, wMinute, wSecond, wMilliseconds; } SYSTEMTIME;
typedef uint16_t WCH_;
typedef struct { DWORD attr; DWORD ft[6]; DWORD sizeHigh, sizeLow; DWORD res0, res1; WCH_ name[260]; WCH_ alt[14]; } FINDDATA;
#define EXPORT
static uint8_t *FAKE; static size_t FAKESIZE;
static int g_rogLoaded = 0; static uint64_t g_now = 1000;
static char TESTDIR[512];
static void w2u(const uint16_t *w, char *out) { /* UTF-16 -> UTF-8, backslashes -> slashes */
    char *o = out;
    for (; *w; w++) {
        uint32_t c = *w;
        if (c >= 0xD800 && c < 0xDC00 && w[1] >= 0xDC00 && w[1] < 0xE000) { c = 0x10000 + ((c - 0xD800) << 10) + (w[1] - 0xDC00); w++; }
        if (c == '\\') c = '/';
        if (c < 0x80) *o++ = (char)c;
        else if (c < 0x800) { *o++ = (char)(0xC0 | c >> 6); *o++ = (char)(0x80 | (c & 63)); }
        else if (c < 0x10000) { *o++ = (char)(0xE0 | c >> 12); *o++ = (char)(0x80 | ((c >> 6) & 63)); *o++ = (char)(0x80 | (c & 63)); }
        else { *o++ = (char)(0xF0 | c >> 18); *o++ = (char)(0x80 | ((c >> 12) & 63)); *o++ = (char)(0x80 | ((c >> 6) & 63)); *o++ = (char)(0x80 | (c & 63)); }
    }
    *o = 0;
}
static HANDLE GetModuleHandleW(const uint16_t *n) {
    if (!n) return FAKE;
    char b[64]; w2u(n, b);
    return (!strcmp(b, "RoGText.dll") && g_rogLoaded) ? (HANDLE)1 : 0;
}
static BOOL GetModuleHandleExW(DWORD a, const void *b, HANDLE *c) { (void)a; (void)b; *c = 0; return 1; }
static DWORD GetModuleFileNameW(HANDLE h, uint16_t *b, DWORD n) { (void)h; (void)n; char p[600]; snprintf(p, sizeof p, "%s/SkinSlotText.dll", TESTDIR); size_t k = 0; for (; p[k]; k++) b[k] = (uint16_t)(unsigned char)p[k]; b[k] = 0; return (DWORD)k; }
static void *VirtualAlloc(void *a, size_t n, DWORD t, DWORD p) { (void)a; (void)t; (void)p; return calloc(1, n); }
static int protects = 0;
static BOOL VirtualProtect(void *a, size_t n, DWORD p, DWORD *o) { (void)a; (void)n; (void)p; *o = 0x20; protects++; return 1; }
static BOOL FlushInstructionCache(HANDLE a, const void *b, size_t c) { (void)a; (void)b; (void)c; return 1; }
static HANDLE GetCurrentProcess(void) { return 0; }
static HANDLE CreateFileW(const uint16_t *p, DWORD a, DWORD s, void *x, DWORD d, DWORD f, HANDLE t) {
    (void)s; (void)x; (void)d; (void)f; (void)t; char b[1200]; w2u(p, b);
    FILE *fp = fopen(b, (a & 0x40000000) ? "wb" : "rb"); return fp ? (HANDLE)fp : (HANDLE)(intptr_t)-1;
}
static BOOL ReadFile(HANDLE h, void *b, DWORD n, DWORD *g, void *o) { (void)o; *g = (DWORD)fread(b, 1, n, (FILE *)h); return 1; }
static BOOL WriteFile(HANDLE h, const void *b, DWORD n, DWORD *w, void *o) { (void)o; *w = (DWORD)fwrite(b, 1, n, (FILE *)h); fflush((FILE *)h); return 1; }
static BOOL CloseHandle(HANDLE h) { fclose((FILE *)h); return 1; }
static DWORD GetFileSize(HANDLE h, DWORD *x) { (void)x; FILE *f = (FILE *)h; long c = ftell(f); fseek(f, 0, SEEK_END); long e = ftell(f); fseek(f, c, SEEK_SET); return (DWORD)e; }
static int MultiByteToWideChar(unsigned cp, DWORD fl, const char *s, int n, uint16_t *w, int wn) {
    (void)cp; (void)fl; int k = 0; const unsigned char *u = (const unsigned char *)s, *e = u + n;
    while (u < e && k < wn) {
        uint32_t c = *u++;
        if (c >= 0xF0) { c = ((c & 7) << 18) | ((u[0] & 63) << 12) | ((u[1] & 63) << 6) | (u[2] & 63); u += 3; }
        else if (c >= 0xE0) { c = ((c & 15) << 12) | ((u[0] & 63) << 6) | (u[1] & 63); u += 2; }
        else if (c >= 0xC0) { c = ((c & 31) << 6) | (u[0] & 63); u += 1; }
        if (c >= 0x10000) { c -= 0x10000; w[k++] = (uint16_t)(0xD800 + (c >> 10)); if (k < wn) w[k++] = (uint16_t)(0xDC00 + (c & 1023)); }
        else w[k++] = (uint16_t)c;
    }
    return k;
}
static int WideCharToMultiByte(unsigned cp, DWORD f, const uint16_t *w, int n, char *b, int bn, const char *d, BOOL *u) {
    (void)cp; (void)f; (void)d; (void)u; uint16_t tmp[4096]; if (n > 4095) n = 4095; memcpy(tmp, w, n * 2); tmp[n] = 0;
    char out[16384]; w2u(tmp, out); int k = (int)strlen(out); if (k > bn) k = bn; memcpy(b, out, k); return k;
}
static void GetLocalTime(SYSTEMTIME *t) { memset(t, 0, sizeof *t); }
static uint64_t GetTickCount64(void) { return g_now; }
static char *g_found[64]; static int g_nfound, g_ifound;
static HANDLE FindFirstFileW(const uint16_t *pat, FINDDATA *fd) {
    char b[1200]; w2u(pat, b); char *star = strrchr(b, '/'); *star = 0;
    DIR *d = opendir(b); g_nfound = g_ifound = 0; if (!d) return (HANDLE)(intptr_t)-1;
    struct dirent *e; while ((e = readdir(d))) { size_t L = strlen(e->d_name); if (L > 4 && !strcasecmp(e->d_name + L - 4, ".ini")) g_found[g_nfound++] = strdup(e->d_name); }
    closedir(d);
    if (!g_nfound) return (HANDLE)(intptr_t)-1;
    memset(fd, 0, sizeof *fd); for (int i = 0; g_found[0][i]; i++) fd->name[i] = (uint16_t)g_found[0][i]; g_ifound = 1; return (HANDLE)2;
}
static BOOL FindNextFileW(HANDLE h, FINDDATA *fd) { (void)h; if (g_ifound >= g_nfound) return 0; memset(fd, 0, sizeof *fd); for (int i = 0; g_found[g_ifound][i]; i++) fd->name[i] = (uint16_t)g_found[g_ifound][i]; g_ifound++; return 1; }
static BOOL FindClose(HANDLE h) { (void)h; return 1; }

#include "skinslottext.c"

/* ---------------------------------------------------------------- tests */
static int fails = 0;
#define CHECK(c, m) do { if (c) printf("ok   %s\n", m); else { printf("FAIL %s\n", m); fails++; } } while (0)
static int origCalls, otherCalls;
static FStr *fake_game(void *t, uint8_t f, FStr *key, uint8_t *found) { (void)t; (void)f; origCalls++; if (found) *found = 0; return key; }
static FStr OTHER_TEXT;
static FStr *fake_other(void *t, uint8_t f, FStr *key, uint8_t *found) {   /* stands in for RoGText's hook */
    (void)t; (void)f; otherCalls++;
    static const uint16_t k[] = { 'W','e','a','p','o','n','_','N','a','m','e','_','o','t','s','s' };
    if (key && key->num == 17 && !memcmp(key->data, k, 32)) { if (found) *found = 1; return &OTHER_TEXT; }
    return fake_game(t, f, key, found);
}
static FStr mk(const char *s) { static uint16_t pool[64][256]; static int n; uint16_t *d = pool[n++]; int k = MultiByteToWideChar(65001, 0, s, (int)strlen(s), d, 255); d[k] = 0; FStr r = { d, k + 1, k + 1 }; return r; }
static int is(const FStr *r, const char *utf8) { FStr e = mk(utf8); return r && r->num == e.num && !memcmp(r->data, e.data, (size_t)e.num * 2); }
static uint8_t *make_fake(uint32_t rva) {
    FAKESIZE = 0x7400000; FAKE = calloc(1, FAKESIZE);
    *(int32_t *)(FAKE + 0x3c) = 0x80; memcpy(FAKE + 0x80, "PE\0\0", 4);
    *(uint16_t *)(FAKE + 0x80 + 6) = 1; *(uint16_t *)(FAKE + 0x80 + 20) = 240;
    *(uint32_t *)(FAKE + 0x80 + 24 + 56) = (uint32_t)FAKESIZE;
    uint8_t *sec = FAKE + 0x80 + 24 + 240; memcpy(sec, ".ecode\0\0", 8);
    *(uint32_t *)(sec + 8) = (uint32_t)FAKESIZE - 0x1000; *(uint32_t *)(sec + 12) = 0x1000; *(uint32_t *)(sec + 36) = 0x60000020;
    memcpy(FAKE + rva, SIG, 48);
    for (int i = 48; i < 64; i++) FAKE[rva + i] = (uint8_t)(0xA0 + i);
    return FAKE + rva;
}
static void write_file(const char *rel, const char *text) { char p[1200]; snprintf(p, sizeof p, "%s/%s", TESTDIR, rel); FILE *f = fopen(p, "wb"); fputs(text, f); fclose(f); }
static void standard_texts(void) {
    write_file("SkinSlotText.ini", "\xEF\xBB\xBF; main file\r\n[Text]\r\nAircraftSkin_Name_su57_601 = Slot Test A\r\nAircraftSkin_Name_su57_603=from the main file\r\n");
    write_file("texts/b_second.ini", "[Text]\nAircraftSkin_Name_su57_603=from b (ignored)\nAircraftSkin_Description_su57_602=Line one.\\nLine two \xE2\x80\x94 \xE6\x97\xA5\xE6\x9C\xAC\n# comment\nno equals sign here\n=no key\n");
    write_file("texts/A_first.ini", "[Text]\nAircraftSkin_Name_su57_602=Navy (new slot)\nAircraftSkin_Description_su57_602=from A (wins over b)\nAircraftSkin_Name_su57_604=\n");
    write_file("texts/notes.txt", "AircraftSkin_Name_su57_609=not an ini file\n");
}
static int ticks_until(uint64_t from, uint64_t to) { for (g_now = from; g_now <= to; g_now += 1000) skinslottext_tick(0); return g_state; }
static void show_log(void) { char p[1200]; snprintf(p, sizeof p, "%s/SkinSlotText.log", TESTDIR); FILE *f = fopen(p, "rb"); char b[8192]; size_t n = fread(b, 1, sizeof b - 1, f); b[n] = 0; fclose(f); printf("---- log\n%s----\n", b); }

int main(int argc, char **argv) {
    const char *sc = argv[1];
    snprintf(TESTDIR, sizeof TESTDIR, "%s", argv[2]);
    if (!strcmp(sc, "texts")) {
        standard_texts();
        uint8_t *fn = make_fake(EXPECTED_RVA); uint8_t before[64]; memcpy(before, fn, 64);
        luaopen_skinslottext(0);
        CHECK(g_n == 5, "5 texts: main file first, A_first before b_second, duplicates and bad lines skipped, .txt ignored");
        CHECK(memcmp(fn, before, 64) == 0, "loading texts changes no code");
        g_next = fake_game;
        uint8_t found = 9; FStr k;
        k = mk("AircraftSkin_Name_su57_601"); CHECK(is(hook_lookup(0, 0, &k, &found), "Slot Test A") && found == 1 && origCalls == 0, "our key -> our text (spaces round = trimmed), found = 1, game not asked");
        k = mk("AircraftSkin_Name_su57_603"); CHECK(is(hook_lookup(0, 0, &k, &found), "from the main file"), "a key in two files: the first file wins");
        k = mk("AircraftSkin_Description_su57_602"); CHECK(is(hook_lookup(0, 0, &k, &found), "from A (wins over b)"), "texts\\ files are read A-Z (case-insensitive)");
        k = mk("AircraftSkin_Name_su57_602"); CHECK(is(hook_lookup(0, 0, &k, 0), "Navy (new slot)"), "a null found pointer is fine");
        k = mk("AircraftSkin_Name_su57_604"); CHECK(is(hook_lookup(0, 0, &k, &found), "") && found == 1, "an empty value gives an empty text");
        k = mk("AircraftSkin_Name_su57_60"); found = 9; FStr *r = hook_lookup(0, 0, &k, &found);
        CHECK(r == &k && found == 0 && origCalls == 1, "a key that is only a prefix of ours goes to the game");
        k = mk("AircraftSkin_Name_su57_609"); r = hook_lookup(0, 0, &k, &found); CHECK(r == &k && origCalls == 2, "keys from non-.ini files are not used");
        FStr empty = { 0, 0, 0 }; r = hook_lookup(0, 0, &empty, &found); CHECK(r == &empty && origCalls == 3, "an empty key goes to the game");
        /* line breaks and non-ASCII: load a second set with only b to see its description */
        show_log();
    } else if (!strcmp(sc, "alone")) {
        standard_texts();
        uint8_t *fn = make_fake(EXPECTED_RVA); uint8_t before[64]; memcpy(before, fn, 64);
        luaopen_skinslottext(0);
        CHECK(ticks_until(1000, 8000) == 0 && memcmp(fn, before, 64) == 0, "no RoGText: waits about 8 s without touching anything");
        CHECK(ticks_until(9000, 9000) == 1, "then hooks the game's function");
        CHECK(fn[0] == 0xFF && fn[1] == 0x25 && *(uint64_t *)(fn + 6) == (uint64_t)(uintptr_t)&hook_lookup && fn[14] == 0x90, "function start = jmp [rip] -> our hook");
        CHECK(memcmp(fn + 15, before + 15, 49) == 0, "bytes after the 15 patched ones untouched");
        uint8_t *tr = (uint8_t *)(void *)g_next;
        CHECK(memcmp(tr, before, 15) == 0 && tr[15] == 0xFF && tr[16] == 0x25 && *(uint64_t *)(tr + 21) == (uint64_t)(uintptr_t)(fn + 15) && tr[29] == 0x90, "trampoline = the 15 original bytes + jmp back to fn + 15");
        int p = protects; ticks_until(10000, 20000); CHECK(protects == p, "later ticks change nothing");
        show_log();
    } else if (!strcmp(sc, "rogtext")) {
        standard_texts(); g_rogLoaded = 1;
        uint8_t *fn = make_fake(EXPECTED_RVA);
        luaopen_skinslottext(0);
        CHECK(ticks_until(1000, 20000) == 0 && fn[0] == 0x4c, "RoGText loaded but not hooked yet: we wait and leave the function alone");
        /* RoGText installs its hook the way RoGText.dll 1.0-1.9 does */
        uint8_t rog[15]; jmp_patch(rog, (uint64_t)(uintptr_t)&fake_other); memcpy(fn, rog, 15);
        CHECK(ticks_until(21000, 21000) == 1, "as soon as RoGText's hook is there, we go in front of it");
        CHECK(*(uint64_t *)(fn + 6) == (uint64_t)(uintptr_t)&hook_lookup && fn[0] == 0xFF && fn[14] == 0x90, "the start now jumps to our hook");
        CHECK(g_next == fake_other, "keys that are not ours go on to RoGText's hook");
        uint8_t found = 9; FStr k;
        k = mk("AircraftSkin_Name_su57_601"); CHECK(is(hook_lookup(0, 0, &k, &found), "Slot Test A") && otherCalls == 0, "our key: our text, RoGText not asked");
        k = mk("Weapon_Name_otss"); FStr *r = hook_lookup(0, 0, &k, &found); CHECK(r == &OTHER_TEXT && found == 1 && otherCalls == 1 && origCalls == 0, "RoGText's key: RoGText's text");
        k = mk("Weapon_Name_mg"); r = hook_lookup(0, 0, &k, &found); CHECK(r == &k && found == 0 && otherCalls == 2 && origCalls == 1, "other keys: through RoGText to the game");
        show_log();
    } else if (!strcmp(sc, "rogtext_never")) {
        standard_texts(); g_rogLoaded = 1;
        uint8_t *fn = make_fake(EXPECTED_RVA);
        luaopen_skinslottext(0);
        CHECK(ticks_until(1000, 60000) == 0 && fn[0] == 0x4c, "RoGText loaded but never hooks: we wait up to 60 s");
        CHECK(ticks_until(61000, 61000) == 1 && *(uint64_t *)(fn + 6) == (uint64_t)(uintptr_t)&hook_lookup, "then hook the game's function ourselves");
        show_log();
    } else if (!strcmp(sc, "unknown")) {
        standard_texts();
        uint8_t *fn = make_fake(EXPECTED_RVA);
        fn[0] = 0xE9; fn[1] = 0x11; fn[2] = 0x22; fn[3] = 0x33; fn[4] = 0x44;   /* someone else's 5-byte jmp */
        uint8_t before[64]; memcpy(before, fn, 64);
        luaopen_skinslottext(0);
        CHECK(ticks_until(1000, 1000) == 2 && memcmp(fn, before, 64) == 0 && protects == 0, "an unknown change at the start: we give up and change nothing");
        show_log();
    } else if (!strcmp(sc, "moved")) {
        standard_texts();
        uint8_t *fn = make_fake(0x2345670);   /* another game build: the function elsewhere */
        luaopen_skinslottext(0);
        CHECK(ticks_until(1000, 9000) == 1 && fn[0] == 0xFF && *(uint64_t *)(fn + 6) == (uint64_t)(uintptr_t)&hook_lookup, "function found by search in another build and hooked");
        show_log();
    } else if (!strcmp(sc, "moved_hooked")) {
        standard_texts();
        uint8_t *fn = make_fake(0x2345670);
        uint8_t rog[15]; jmp_patch(rog, (uint64_t)(uintptr_t)&fake_other); memcpy(fn, rog, 15);
        luaopen_skinslottext(0);
        CHECK(ticks_until(1000, 1000) == 1 && g_next == fake_other, "another build, already hooked by RoGText: found by search and chained");
    } else if (!strcmp(sc, "notexts")) {
        uint8_t *fn = make_fake(EXPECTED_RVA); uint8_t before[64]; memcpy(before, fn, 64);
        luaopen_skinslottext(0);
        CHECK(ticks_until(1000, 30000) == 2 && memcmp(fn, before, 64) == 0, "no text files: nothing is hooked");
        show_log();
    }
    printf("%s\n", fails ? "SOME CHECKS FAILED" : "all checks passed");
    return fails != 0;
}
