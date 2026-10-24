/* SkinSlotText - names and descriptions for new skin slots in ACE COMBAT 8 (experimental).
 *
 * The game looks every piece of interface text up by key through one function, for example
 * "AircraftSkin_Name_su57_601". A key the game does not know comes back as the key itself. This DLL answers the keys
 * listed in its text files and passes every other key on unchanged:
 *
 *     <this DLL's folder>\SkinSlotText.ini        (optional)
 *     <this DLL's folder>\texts\*.ini             (one file per skin mod; read in A-Z order)
 *
 *     [Text]
 *     AircraftSkin_Name_su57_601=My skin
 *     AircraftSkin_Description_su57_601=First line\nSecond line
 *
 * A key listed in two files: the first file (A-Z) wins and the log says which one was ignored.
 *
 * Getting along with other text mods: RoGText.dll (the OTSS / laser mods) hooks the same function, and it can only
 * install its hook on the game's untouched function. So this DLL waits until RoGText has installed its hook and then
 * puts itself in front of it: our keys get our text, every other key goes on to RoGText, and RoGText passes what it
 * does not know to the game. Without RoGText it hooks the game's function directly after a short wait. If the start of
 * the function holds anything it does not recognise, it changes nothing and says so in SkinSlotText.log.
 *
 * Loaded by the SkinSlotText UE4SS mod (Scripts\main.lua): luaopen_skinslottext() once, then skinslottext_tick() about
 * once a second on the game thread until it has decided. The patch exists only in memory; no game file is changed.
 */
#include <stdint.h>
#include <stddef.h>

typedef uint16_t WCH;   /* UTF-16 unit: Unreal's TCHAR on Windows */

#ifndef SST_TEST
typedef void *HANDLE;
typedef int BOOL;
typedef unsigned long DWORD;
typedef unsigned short WORD;
typedef struct { WORD wYear, wMonth, wDayOfWeek, wDay, wHour, wMinute, wSecond, wMilliseconds; } SYSTEMTIME;
typedef struct {
    DWORD attr; DWORD ft[6]; DWORD sizeHigh, sizeLow; DWORD res0, res1;
    WCH name[260]; WCH alt[14];
} FINDDATA;
#define WINAPI __stdcall
__declspec(dllimport) HANDLE WINAPI GetModuleHandleW(const WCH *);
__declspec(dllimport) BOOL WINAPI GetModuleHandleExW(DWORD, const void *, HANDLE *);
__declspec(dllimport) DWORD WINAPI GetModuleFileNameW(HANDLE, WCH *, DWORD);
__declspec(dllimport) void *WINAPI VirtualAlloc(void *, size_t, DWORD, DWORD);
__declspec(dllimport) BOOL WINAPI VirtualProtect(void *, size_t, DWORD, DWORD *);
__declspec(dllimport) BOOL WINAPI FlushInstructionCache(HANDLE, const void *, size_t);
__declspec(dllimport) HANDLE WINAPI GetCurrentProcess(void);
__declspec(dllimport) HANDLE WINAPI CreateFileW(const WCH *, DWORD, DWORD, void *, DWORD, DWORD, HANDLE);
__declspec(dllimport) BOOL WINAPI ReadFile(HANDLE, void *, DWORD, DWORD *, void *);
__declspec(dllimport) BOOL WINAPI WriteFile(HANDLE, const void *, DWORD, DWORD *, void *);
__declspec(dllimport) BOOL WINAPI CloseHandle(HANDLE);
__declspec(dllimport) DWORD WINAPI GetFileSize(HANDLE, DWORD *);
__declspec(dllimport) int WINAPI MultiByteToWideChar(unsigned, DWORD, const char *, int, WCH *, int);
__declspec(dllimport) int WINAPI WideCharToMultiByte(unsigned, DWORD, const WCH *, int, char *, int, const char *, BOOL *);
__declspec(dllimport) void WINAPI GetLocalTime(SYSTEMTIME *);
__declspec(dllimport) uint64_t WINAPI GetTickCount64(void);
__declspec(dllimport) HANDLE WINAPI FindFirstFileW(const WCH *, FINDDATA *);
__declspec(dllimport) BOOL WINAPI FindNextFileW(HANDLE, FINDDATA *);
__declspec(dllimport) BOOL WINAPI FindClose(HANDLE);
#define EXPORT __declspec(dllexport)
/* ------------------------------------------------------------------------------------------------ tiny libc */
void *memset(void *d, int c, size_t n) { unsigned char *p = d; while (n--) *p++ = (unsigned char)c; return d; }
void *memcpy(void *d, const void *s, size_t n) { unsigned char *p = d; const unsigned char *q = s; while (n--) *p++ = *q++; return d; }
int _fltused = 0;
#endif
#define INVALID_HANDLE ((HANDLE)(intptr_t)-1)
#define VERSION "1.2"

static size_t wlen(const WCH *s) { size_t n = 0; while (s[n]) n++; return n; }
static void wcat(WCH *d, const char *s) { size_t n = wlen(d); while (*s) d[n++] = (WCH)(unsigned char)*s++; d[n] = 0; }
static void wcatw(WCH *d, const WCH *s) { size_t n = wlen(d); while (*s) d[n++] = *s++; d[n] = 0; }
static void *alloc(size_t n) { return VirtualAlloc(0, n, 0x3000 /*commit|reserve*/, 4 /*PAGE_READWRITE*/); }

/* ------------------------------------------------------------------------------------------------ log */
static WCH g_dir[520];              /* this DLL's folder, with trailing backslash */
static HANDLE g_log = 0;
static void log_open(void) {
    WCH p[600]; p[0] = 0; wcatw(p, g_dir); wcat(p, "SkinSlotText.log");
    g_log = CreateFileW(p, 0x40000000 /*GENERIC_WRITE*/, 1 /*share read*/, 0, 2 /*CREATE_ALWAYS*/, 0x80, 0);
    if (g_log == INVALID_HANDLE) g_log = 0;
}
static void log_raw(const char *s, size_t n) { DWORD w; if (g_log) WriteFile(g_log, s, (DWORD)n, &w, 0); }
static void log_str(const char *s) { size_t n = 0; while (s[n]) n++; log_raw(s, n); }
static void log_w(const WCH *s, int n) {
    char b[2048]; int k = WideCharToMultiByte(65001, 0, s, n, b, sizeof b, 0, 0); if (k > 0) log_raw(b, (size_t)k);
}
static void log_hex(uint64_t v) {
    char b[19]; int i; b[0] = '0'; b[1] = 'x';
    for (i = 0; i < 16; i++) { int d = (int)((v >> (60 - 4 * i)) & 15); b[2 + i] = (char)(d < 10 ? '0' + d : 'a' + d - 10); }
    i = 2; while (i < 17 && b[i] == '0') i++;
    log_raw(b, 2); log_raw(b + i, 18 - i);
}
static void log_dec(int64_t v) {
    char b[24]; int i = 23; b[i] = 0; int neg = v < 0; uint64_t u = neg ? (uint64_t)(-v) : (uint64_t)v;
    do { b[--i] = (char)('0' + u % 10); u /= 10; } while (u);
    if (neg) b[--i] = '-';
    log_str(b + i);
}
static void log_time(void) {
    SYSTEMTIME t; GetLocalTime(&t);
    char b[24] = "0000-00-00 00:00:00.000 "; int v[7] = { t.wYear, t.wMonth, t.wDay, t.wHour, t.wMinute, t.wSecond, t.wMilliseconds };
    int pos[7] = { 0, 5, 8, 11, 14, 17, 20 }, wid[7] = { 4, 2, 2, 2, 2, 2, 3 };
    for (int k = 0; k < 7; k++) { int x = v[k]; for (int j = wid[k] - 1; j >= 0; j--) { b[pos[k] + j] = (char)('0' + x % 10); x /= 10; } }
    log_raw(b, 24);
}
#define LOG(s) do { log_time(); log_str(s); } while (0)

/* ------------------------------------------------------------------------------------------------ text table */
typedef struct { WCH *data; int32_t num; int32_t max; } FStr;     /* Unreal FString (TArray<TCHAR>) */
typedef struct { WCH *key; int keylen; FStr value; int hits; int file; } Entry;
#define MAXE 8192
#define MAXFILES 256
static Entry *g_e = 0;
static int g_n = 0;
static int *g_slot = 0; static uint32_t g_mask = 0;   /* open-addressing hash: index+1 into g_e, 0 = empty */
static WCH g_files[MAXFILES][260]; static int g_nfiles = 0;

static uint32_t hash_key(const WCH *s, int n) {
    uint32_t h = 2166136261u;
    for (int i = 0; i < n; i++) { h ^= s[i]; h *= 16777619u; }
    return h;
}
static int same(const WCH *a, const WCH *b, int n) { for (int i = 0; i < n; i++) if (a[i] != b[i]) return 0; return 1; }
static Entry *find_key(const WCH *k, int n) {
    if (!g_slot || n <= 0) return 0;
    for (uint32_t i = hash_key(k, n) & g_mask;; i = (i + 1) & g_mask) {
        int s = g_slot[i];
        if (!s) return 0;
        Entry *e = &g_e[s - 1];
        if (e->keylen == n && same(e->key, k, n)) return e;
    }
}
static Entry *find_entry(const FStr *k) {
    if (!k || !k->data || k->num <= 1) return 0;
    return find_key(k->data, k->num - 1);
}

static int is_space(WCH c) { return c == ' ' || c == '\t' || c == '\r' || c == 0xFEFF; }

/* one text file (UTF-8): key=value lines in any [section]; ; or # starts a comment line; \n in a value = line break */
static void load_file(const WCH *path, int fileIndex) {
    HANDLE f = CreateFileW(path, 0x80000000 /*GENERIC_READ*/, 1, 0, 3 /*OPEN_EXISTING*/, 0x80, 0);
    if (f == INVALID_HANDLE) return;
    DWORD sz = GetFileSize(f, 0), got = 0;
    if (sz == 0 || sz > 4u << 20) { CloseHandle(f); log_time(); log_str("skipped (empty or over 4 MB): "); log_w(path, (int)wlen(path)); log_str("\r\n"); return; }
    char *raw = alloc(sz + 1); WCH *w = alloc((sz + 1) * 2);
    if (!raw || !w) { CloseHandle(f); LOG("out of memory\r\n"); return; }
    ReadFile(f, raw, sz, &got, 0); CloseHandle(f);
    int wn = MultiByteToWideChar(65001, 0, raw, (int)got, w, (int)sz);
    int i = 0, added = 0;
    while (i < wn) {
        int ls = i; while (i < wn && w[i] != '\n') i++;
        int le = i; i++;
        while (ls < le && is_space(w[ls])) ls++;
        while (le > ls && is_space(w[le - 1])) le--;
        if (le <= ls || w[ls] == ';' || w[ls] == '#' || w[ls] == '[') continue;
        int eq = ls; while (eq < le && w[eq] != '=') eq++;
        if (eq >= le) continue;
        int ke = eq; while (ke > ls && is_space(w[ke - 1])) ke--;
        int vs = eq + 1; while (vs < le && is_space(w[vs])) vs++;
        if (ke <= ls) continue;
        Entry *dup = find_key(w + ls, ke - ls);
        if (dup) {
            log_time(); log_str("duplicate key "); log_w(w + ls, ke - ls); log_str(" in "); log_w(g_files[fileIndex], (int)wlen(g_files[fileIndex]));
            log_str(" ignored - it is already given by "); log_w(g_files[dup->file], (int)wlen(g_files[dup->file])); log_str("\r\n");
            continue;
        }
        if (g_n >= MAXE) { LOG("more than 8192 texts - the rest are ignored\r\n"); break; }
        /* key and value live in w (this file's buffer, kept for the game's lifetime); unescape \n in place */
        int tn = 0; WCH *val = w + vs;
        for (int k = vs; k < le; k++) {
            if (w[k] == '\\' && k + 1 < le && w[k + 1] == 'n') { val[tn++] = '\n'; k++; } else val[tn++] = w[k];
        }
        val[tn] = 0;                        /* overwrites the line end (or the spare unit at the end) */
        Entry *e = &g_e[g_n];
        e->key = w + ls; e->keylen = ke - ls; e->file = fileIndex; e->hits = 0;
        e->value.data = val; e->value.num = tn + 1; e->value.max = tn + 1;
        /* insert into the hash */
        for (uint32_t h = hash_key(e->key, e->keylen) & g_mask;; h = (h + 1) & g_mask) if (!g_slot[h]) { g_slot[h] = g_n + 1; break; }
        g_n++; added++;
        log_time(); log_str("  "); log_w(e->key, e->keylen); log_str(" = "); log_w(val, tn); log_str("\r\n");
    }
    log_time(); log_dec(added); log_str(" texts from "); log_w(g_files[fileIndex], (int)wlen(g_files[fileIndex])); log_str("\r\n");
}

static int wless(const WCH *a, const WCH *b) {          /* case-insensitive A-Z order */
    for (;; a++, b++) {
        WCH x = *a, y = *b;
        if (x >= 'A' && x <= 'Z') x += 32;
        if (y >= 'A' && y <= 'Z') y += 32;
        if (x != y || !x) return x < y;
    }
}

static void load_texts(void) {
    g_e = alloc(sizeof(Entry) * MAXE);
    g_mask = 16384 - 1; g_slot = alloc(sizeof(int) * 16384);
    if (!g_e || !g_slot) { LOG("out of memory\r\n"); g_slot = 0; return; }
    /* SkinSlotText.ini next to the DLL comes first, then texts\*.ini in A-Z order */
    WCH p[600];
    p[0] = 0; wcatw(p, g_dir); wcat(p, "SkinSlotText.ini");
    HANDLE t = CreateFileW(p, 0x80000000, 1, 0, 3, 0x80, 0);
    if (t != INVALID_HANDLE) { CloseHandle(t); g_files[0][0] = 0; wcat(g_files[0], "SkinSlotText.ini"); g_nfiles = 1; }
    int firstText = g_nfiles;
    FINDDATA fd; p[0] = 0; wcatw(p, g_dir); wcat(p, "texts\\*.ini");
    HANDLE h = FindFirstFileW(p, &fd);
    if (h != INVALID_HANDLE) {
        do {
            if (fd.attr & 0x10) continue;                 /* a folder */
            if (g_nfiles >= MAXFILES) { LOG("more than 256 text files - the rest are ignored\r\n"); break; }
            WCH *d = g_files[g_nfiles]; d[0] = 0; wcat(d, "texts\\"); wcatw(d, fd.name);
            /* keep texts\ files sorted */
            int j = g_nfiles++;
            while (j > firstText && wless(g_files[j], g_files[j - 1])) {
                WCH tmp[260]; memcpy(tmp, g_files[j], sizeof tmp); memcpy(g_files[j], g_files[j - 1], sizeof tmp); memcpy(g_files[j - 1], tmp, sizeof tmp); j--;
            }
        } while (FindNextFileW(h, &fd));
        FindClose(h);
    }
    if (g_nfiles == 0) { LOG("no text files (SkinSlotText.ini or texts\\*.ini) next to the DLL\r\n"); return; }
    for (int i = 0; i < g_nfiles; i++) {
        p[0] = 0; wcatw(p, g_dir); wcatw(p, g_files[i]);
        load_file(p, i);
    }
    log_time(); log_dec(g_n); log_str(" texts in all\r\n");
}

/* ------------------------------------------------------------------------------------------------ the hook */
typedef FStr *(*LookupFn)(void *table, uint8_t flag, FStr *key, uint8_t *found);
static LookupFn g_next = 0;        /* what we pass other keys to: our trampoline, or the hook that was there before */

/* For testing: the first time the game asks for any skin name ("AircraftSkin_Name_..."), the log says so. That shows
   which skins a hangar list really holds, the game's own included. */
static uint32_t g_seen[2048]; static int g_seenCount = 0;
static void note_skin_key(const FStr *k) {
    static const char P[] = "AircraftSkin_Name_";
    if (!k || !k->data || k->num <= 19 || g_seenCount >= 1500) return;
    for (int i = 0; i < 18; i++) if (k->data[i] != (WCH)P[i]) return;
    uint32_t h = hash_key(k->data, k->num - 1) | 1u;
    uint32_t i = h & 2047u;
    for (;; i = (i + 1) & 2047u) { if (g_seen[i] == h) return; if (!g_seen[i]) break; }
    g_seen[i] = h; g_seenCount++;
    log_time(); log_str("game asked for: "); log_w(k->data, k->num - 1); log_str("\r\n");
}

static FStr *hook_lookup(void *table, uint8_t flag, FStr *key, uint8_t *found) {
    Entry *e = find_entry(key);
    note_skin_key(key);
    if (!e) return g_next(table, flag, key, found);
    if (found) *found = 1;
    if (e->hits++ == 0) { log_time(); log_str("first use: "); log_w(e->key, e->keylen); log_str("\r\n"); }
    return &e->value;
}

/* the start of the game's lookup function (game 1.1.2.0, RVA 0x73b1750). The first 15 bytes are the ones a hook
   replaces; none of them is position-dependent. */
static const uint8_t SIG[48] = {
    0x4c,0x8b,0xdc, 0x49,0x89,0x5b,0x08, 0x49,0x89,0x6b,0x10, 0x49,0x89,0x73,0x18, 0x49,0x89,0x7b,0x20,
    0x41,0x54, 0x41,0x56, 0x41,0x57, 0x48,0x83,0xec,0x30, 0x84,0xd2, 0xb8,0x50,0x00,0x00,0x00,
    0x41,0xbe,0xa0,0x00,0x00,0x00, 0x49,0x8b,0xf9, 0x44,0x0f,0x44 };
#define STOLEN 15
#define EXPECTED_RVA 0x73b1750u

enum { START_GAME = 1, START_HOOKED = 2, START_OTHER = 0 };
/* the function's start as it is now: the game's own bytes, a 15-byte "jmp [rip+0]; <address>; nop" hook such as
   RoGText's (or another copy of this DLL), or something else */
static int start_kind(const uint8_t *p, uint64_t *target) {
    for (int i = STOLEN; i < 48; i++) if (p[i] != SIG[i]) return START_OTHER;
    int game = 1;
    for (int i = 0; i < STOLEN; i++) if (p[i] != SIG[i]) { game = 0; break; }
    if (game) return START_GAME;
    if (p[0] == 0xFF && p[1] == 0x25 && p[2] == 0 && p[3] == 0 && p[4] == 0 && p[5] == 0 && p[14] == 0x90) {
        uint64_t t = 0; for (int i = 0; i < 8; i++) t |= (uint64_t)p[6 + i] << (8 * i);
        if (t) { if (target) *target = t; return START_HOOKED; }
    }
    return START_OTHER;
}

/* bytes 15..47 are never touched by a 15-byte hook: they identify the function whatever its start holds now */
static int rest_matches(const uint8_t *p) { for (int i = STOLEN; i < 48; i++) if (p[i] != SIG[i]) return 0; return 1; }

static uint8_t *find_function(const uint8_t *base) {
    const uint8_t *nt = base + *(const int32_t *)(base + 0x3c);
    if (*(const uint32_t *)nt != 0x4550) return 0;
    uint32_t imageSize = *(const uint32_t *)(nt + 24 + 56);
    if (EXPECTED_RVA + 48 < imageSize && rest_matches(base + EXPECTED_RVA)) return (uint8_t *)base + EXPECTED_RVA;
    /* another game build: search the executable sections for the function (untouched or hooked) */
    uint16_t nsec = *(const uint16_t *)(nt + 6), optsz = *(const uint16_t *)(nt + 20);
    const uint8_t *sec = nt + 24 + optsz; const uint8_t *hit = 0; int count = 0;
    for (int s = 0; s < nsec; s++, sec += 40) {
        uint32_t vsize = *(const uint32_t *)(sec + 8), va = *(const uint32_t *)(sec + 12), ch = *(const uint32_t *)(sec + 36);
        if (!(ch & 0x20000000) || vsize < 48) continue;                 /* executable only */
        if (sec[0] == '.' && sec[1] == 'd' && sec[2] == 'i') continue;    /* not the protection layer's section */
        const uint8_t *p = base + va, *end = base + va + vsize - 48;
        for (; p < end; p++)
            if ((p[0] == 0x4c || p[0] == 0xFF) && p[15] == 0x49 && p[16] == 0x89 && start_kind(p, 0) != START_OTHER) { hit = p; if (++count > 1) break; }
        if (count > 1) break;
    }
    if (count != 1) return 0;
    log_time(); log_str("lookup function found by search at RVA "); log_hex((uint64_t)(hit - base)); log_str("\r\n");
    return (uint8_t *)hit;
}

static int write_code(uint8_t *at, const uint8_t *bytes, int n) {
    DWORD old = 0;
    if (!VirtualProtect(at, (size_t)n, 0x40 /*PAGE_EXECUTE_READWRITE*/, &old)) return 0;
    memcpy(at, bytes, (size_t)n);
    VirtualProtect(at, (size_t)n, old, &old);
    FlushInstructionCache(GetCurrentProcess(), at, (size_t)n);
    return 1;
}

static void jmp_patch(uint8_t *patch, uint64_t to) {       /* jmp qword [rip+0] ; <to> ; nop */
    patch[0] = 0xFF; patch[1] = 0x25; patch[2] = patch[3] = patch[4] = patch[5] = 0;
    for (int i = 0; i < 8; i++) patch[6 + i] = (uint8_t)(to >> (8 * i));
    patch[14] = 0x90;
}

/* hook the untouched function: its first 15 bytes go to a trampoline that jumps back to fn + 15 */
static int hook_fresh(uint8_t *fn) {
    uint8_t *tr = VirtualAlloc(0, 64, 0x3000, 0x40 /*PAGE_EXECUTE_READWRITE*/);
    if (!tr) { LOG("cannot allocate the trampoline - nothing was changed\r\n"); return 0; }
    memcpy(tr, fn, STOLEN);
    jmp_patch(tr + STOLEN, (uint64_t)(uintptr_t)(fn + STOLEN));
    FlushInstructionCache(GetCurrentProcess(), tr, 64);
    g_next = (LookupFn)(void *)tr;
    uint8_t patch[STOLEN]; jmp_patch(patch, (uint64_t)(uintptr_t)&hook_lookup);
    if (!write_code(fn, patch, STOLEN)) { LOG("cannot unprotect the lookup function - nothing was changed\r\n"); g_next = 0; return 0; }
    LOG("text hook installed on the game's lookup function\r\n");
    return 1;
}

/* the function already jumps to another hook (RoGText's): call that one for keys that are not ours */
static int hook_chain(uint8_t *fn, uint64_t other) {
    g_next = (LookupFn)(uintptr_t)other;
    uint8_t patch[STOLEN]; jmp_patch(patch, (uint64_t)(uintptr_t)&hook_lookup);
    if (!write_code(fn, patch, STOLEN)) { LOG("cannot unprotect the lookup function - nothing was changed\r\n"); g_next = 0; return 0; }
    log_time(); log_str("text hook installed in front of the hook that was already there (at "); log_hex(other);
    log_str(") - keys that are not ours go on to it\r\n");
    return 1;
}

/* ------------------------------------------------------------------------------------------------ deciding when */
#define WAIT_ALONE_MS   8000      /* no RoGText.dll in the game: hook after this long */
#define WAIT_ROGTEXT_MS 60000     /* RoGText.dll loaded but its hook not seen yet: wait at most this long */
static int g_started = 0, g_state = 0;           /* state: 0 deciding, 1 hooked, 2 gave up */
static uint64_t g_t0 = 0;
static int g_saidWait = 0;
static uint8_t *g_fn = 0;

static int other_text_dll_loaded(void) {
    static const WCH rog[] = { 'R','o','G','T','e','x','t','.','d','l','l',0 };
    return GetModuleHandleW(rog) != 0;
}

/* one decision step; returns g_state */
static int step(void) {
    if (g_state || !g_started) return g_state;
    if (g_n == 0) { LOG("no texts - the hook is not installed\r\n"); g_state = 2; return g_state; }
    uint64_t now = GetTickCount64();
    if (!g_t0) g_t0 = now;
    if (!g_fn) {
        const uint8_t *base = GetModuleHandleW(0);
        g_fn = find_function(base);
        if (!g_fn) { LOG("the game's text lookup function was not found - this game build is not supported; nothing was changed\r\n"); g_state = 2; return g_state; }
        log_time(); log_str("lookup function at "); log_hex((uint64_t)(uintptr_t)g_fn); log_str("\r\n");
    }
    uint64_t other = 0;
    int kind = start_kind(g_fn, &other);
    if (kind == START_HOOKED) { g_state = hook_chain(g_fn, other) ? 1 : 2; return g_state; }
    if (kind == START_OTHER) {
        LOG("the start of the lookup function has been changed by something this DLL does not know - nothing was changed\r\n");
        g_state = 2; return g_state;
    }
    /* untouched: let RoGText.dll go first if it is in the game, since it can only hook the untouched function */
    int rog = other_text_dll_loaded();
    uint64_t wait = rog ? WAIT_ROGTEXT_MS : WAIT_ALONE_MS;
    if (now - g_t0 < wait) {
        if (!g_saidWait) {
            g_saidWait = 1;
            if (rog) LOG("RoGText.dll is in the game - waiting for it to hook the lookup function first\r\n");
            else LOG("waiting a few seconds in case another text mod hooks the lookup function first\r\n");
        }
        return g_state;
    }
    if (rog) LOG("RoGText.dll is in the game but did not hook the lookup function - hooking it directly\r\n");
    g_state = hook_fresh(g_fn) ? 1 : 2;
    return g_state;
}

/* ------------------------------------------------------------------------------------------------ Lua entry points */
/* package.loadlib("<dir>\\SkinSlotText.dll", "luaopen_skinslottext")() : reads the texts (no Lua values returned) */
EXPORT int luaopen_skinslottext(void *L) {
    (void)L;
    if (g_started) return 0;
    g_started = 1;
    HANDLE me = 0;
    GetModuleHandleExW(4 | 2 /*FROM_ADDRESS | UNCHANGED_REFCOUNT*/, (const void *)&luaopen_skinslottext, &me);
    DWORD n = GetModuleFileNameW(me, g_dir, 512);
    while (n > 0 && g_dir[n - 1] != '\\' && g_dir[n - 1] != '/') n--;
    g_dir[n] = 0;
    log_open();
    LOG("SkinSlotText " VERSION " starting\r\n");
    load_texts();
    return 0;
}

/* package.loadlib(..., "skinslottext_tick")() : called about once a second on the game thread until it has decided */
EXPORT int skinslottext_tick(void *L) {
    (void)L;
    step();
    return 0;
}

#ifndef SST_TEST
BOOL WINAPI DllMain(HANDLE h, DWORD reason, void *r) { (void)h; (void)reason; (void)r; return 1; }
#endif
