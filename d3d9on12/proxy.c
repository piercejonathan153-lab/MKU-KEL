/* MKUKE d3d9.dll proxy: runs the game on Microsoft's D3D9On12 layer (real Direct3D 12).
   Build: see build.sh. 32-bit, no C runtime. */
typedef unsigned int UINT; typedef int BOOL; typedef long HRESULT; typedef unsigned long DWORD;
typedef void *HMODULE; typedef void *PVOID; typedef const char *LPCSTR;
#define WINAPI __stdcall
__declspec(dllimport) HMODULE WINAPI LoadLibraryA(LPCSTR);
__declspec(dllimport) void *WINAPI GetProcAddress(HMODULE, LPCSTR);
__declspec(dllimport) UINT WINAPI GetSystemDirectoryA(char *, UINT);
__declspec(dllimport) DWORD WINAPI GetEnvironmentVariableA(LPCSTR, char *, DWORD);
__declspec(dllimport) void *WINAPI CreateFileA(LPCSTR, DWORD, DWORD, void *, DWORD, DWORD, void *);
__declspec(dllimport) BOOL WINAPI WriteFile(void *, const void *, DWORD, DWORD *, void *);
__declspec(dllimport) BOOL WINAPI CloseHandle(void *);
__declspec(dllimport) DWORD WINAPI GetModuleFileNameA(HMODULE, char *, DWORD);
__declspec(dllimport) BOOL WINAPI VirtualProtect(void *, unsigned long, DWORD, DWORD *);
__declspec(dllimport) void *WINAPI CreateThread(void *, unsigned long, DWORD (WINAPI *)(void *), void *, DWORD, DWORD *);
__declspec(dllimport) void WINAPI Sleep(DWORD);
__declspec(dllimport) BOOL WINAPI IsWindowVisible(void *);
__declspec(dllimport) BOOL WINAPI IsIconic(void *);
__declspec(dllimport) BOOL WINAPI GetWindowRect(void *, long *);
__declspec(dllimport) long WINAPI GetWindowLongA(void *, int);
__declspec(dllimport) int __cdecl wsprintfA(char *, LPCSTR, ...);

typedef struct { BOOL Enable9On12; void *pD3D12Device; void *ppD3D12Queues[2]; UINT NumQueues; UINT NodeMask; } D3D9ON12_ARGS;

static HMODULE real;
static int vsync = -1; /* -1 = leave to game */
static int flipmode;
static int lockfix = 1;

static void logline(const char *s) {
    char path[300]; DWORD n = GetModuleFileNameA(0, path, 260), w; int i;
    for (i = (int)n; i > 0 && path[i - 1] != '\\'; i--) ;
    const char *f = "d3d9on12_proxy.log"; int j = 0; while (f[j]) { path[i + j] = f[j]; j++; } path[i + j] = 0;
    void *h = CreateFileA(path, 4 /*FILE_APPEND_DATA*/, 1, 0, 4 /*OPEN_ALWAYS*/, 0x80, 0);
    if (h == (void *)-1) return;
    n = 0; while (s[n]) n++;
    WriteFile(h, s, n, &w, 0); WriteFile(h, "\r\n", 2, &w, 0); CloseHandle(h);
}

static void load(void) {
    if (real) return;
    char p[300]; UINT n = GetSystemDirectoryA(p, 260);
    const char *d = "\\d3d9.dll"; int i = 0; while (d[i]) { p[n + i] = d[i]; i++; } p[n + i] = 0;
    real = LoadLibraryA(p);
    char e[8]; DWORD k = GetEnvironmentVariableA("MKUKE_VSYNC", e, 8);
    if (k) vsync = (e[0] == '1');
    if (GetEnvironmentVariableA("MKUKE_NOLOCKFIX", e, 8)) lockfix = 0;
    char buf[400]; wsprintfA(buf, "loaded %s -> %p, vsync=%d", p, real, vsync); logline(buf);
}

/* ---- force VSync by patching CreateDevice / CreateDeviceEx in the IDirect3D9(Ex) vtable ---- */
typedef struct { UINT bw, bh, fmt, cnt, ms, msq, swap; void *hwnd; BOOL windowed, autodepth; UINT dfmt, flags, refresh, interval; } PP;
typedef HRESULT (WINAPI *CD_t)(void *, UINT, UINT, void *, DWORD, PP *, void **);
typedef HRESULT (WINAPI *CDX_t)(void *, UINT, UINT, void *, DWORD, PP *, void *, void **);
static CD_t origCD; static CDX_t origCDX;
static void fixpp(PP *pp) {
    if (!pp) return;
    if (vsync >= 0) pp->interval = vsync ? 1 : 0x80000000u;
    /* D3D9On12's copy/discard present path shows nothing on screen: use the modern flip model instead */
    if (pp->windowed && (pp->swap == 1 || pp->swap == 3) && pp->ms == 0) {
        pp->swap = 5; /* D3DSWAPEFFECT_FLIPEX */
        flipmode = 1;
        if (pp->cnt < 2) pp->cnt = 2;
    }
    char b[300]; wsprintfA(b, "pp: %ux%u fmt=%u cnt=%u ms=%u swap=%u hwnd=%p windowed=%d autodepth=%d dfmt=%u flags=0x%x refresh=%u interval=0x%x",
        pp->bw, pp->bh, pp->fmt, pp->cnt, pp->ms, pp->swap, pp->hwnd, pp->windowed, pp->autodepth, pp->dfmt, pp->flags, pp->refresh, pp->interval);
    logline(b);
}
/* log the first presents of the device */
typedef HRESULT (WINAPI *PR_t)(void *, void *, void *, void *, void *);
typedef HRESULT (WINAPI *PRX_t)(void *, void *, void *, void *, void *, DWORD);
static PR_t origPR; static PRX_t origPRX; static volatile int npr, nfail, nreset; static void *gwnd;
static DWORD WINAPI watch(void *p) {
    int i;
    for (i = 0; i < 30; i++) {
        Sleep(2000);
        long r[4] = {0, 0, 0, 0}; if (gwnd) GetWindowRect(gwnd, r);
        char b[200]; wsprintfA(b, "t=%ds presents=%d resets=%d visible=%d iconic=%d style=0x%08lx exstyle=0x%08lx rect=%ld,%ld,%ld,%ld",
            (i + 1) * 2, npr, nreset, gwnd ? IsWindowVisible(gwnd) : -1, gwnd ? IsIconic(gwnd) : -1,
            gwnd ? GetWindowLongA(gwnd, -16) : 0, gwnd ? GetWindowLongA(gwnd, -20) : 0, r[0], r[1], r[2], r[3]);
        logline(b);
    }
    return 0;
}
typedef HRESULT (WINAPI *RS_t)(void *, PP *);
static RS_t origRS;
static HRESULT WINAPI hookRS(void *d, PP *pp) { nreset++; fixpp(pp); HRESULT r = origRS(d, pp); char b[80]; wsprintfA(b, "Reset hr=0x%08lx", r); logline(b); return r; }
static void logpr(const char *w, HRESULT r) {
    npr++;
    if (r < 0 && nfail < 20) { nfail++; char b[100]; wsprintfA(b, "%s #%d FAILED hr=0x%08lx", w, npr, r); logline(b); }
    else if (npr <= 3 || npr == 600) { char b[100]; wsprintfA(b, "%s #%d hr=0x%08lx", w, npr, r); logline(b); }
}

static HRESULT WINAPI hookPR(void *d, void *a, void *b, void *c, void *e) {
    if (npr < 3) { char x[160]; wsprintfA(x, "Present args src=%p dst=%p wnd=%p dirty=%p", a, b, c, e); logline(x); }
    if (flipmode) { a = b = c = e = 0; } /* flip model only allows a plain full-window present */
    HRESULT r = origPR(d, a, b, c, e); logpr("Present", r); return r;
}
static HRESULT WINAPI hookPRX(void *d, void *a, void *b, void *c, void *e, DWORD f) { if (flipmode) { a = b = c = e = 0; } HRESULT r = origPRX(d, a, b, c, e, f); logpr("PresentEx", r); return r; }
/* log every distinct texture format/usage/pool the game creates */
typedef HRESULT (WINAPI *CT_t)(void *, UINT, UINT, UINT, DWORD, UINT, UINT, void **, void *);
static CT_t origCT; static unsigned seen[64]; static int nseen;
/* texture LockRect: log flag usage and (experiment) drop DISCARD/NOOVERWRITE so partial updates keep their content */
typedef HRESULT (WINAPI *LR_t)(void *, UINT, void *, const void *, DWORD);
static LR_t origLR; static unsigned lrseen[16]; static int nlr;
static HRESULT WINAPI hookLR(void *t, UINT lv, void *lr, const void *rc, DWORD fl) {
    DWORD nf = lockfix ? (fl & ~(0x2000u /*DISCARD*/ | 0x1000u /*NOOVERWRITE*/)) : fl;
    HRESULT r = origLR(t, lv, lr, rc, nf);
    unsigned key = fl | (rc ? 0x80000000u : 0) | (r < 0 ? 0x40000000u : 0);
    int i; for (i = 0; i < nlr && lrseen[i] != key; i++) ;
    if (i == nlr && nlr < 16) { lrseen[nlr++] = key; char b[120]; wsprintfA(b, "LockRect flags=0x%x rect=%d -> 0x%x hr=0x%08lx", fl, rc != 0, nf, r); logline(b); }
    return r;
}
static HRESULT WINAPI hookCT(void *d, UINT w, UINT h, UINT lv, DWORD usage, UINT fmt, UINT pool, void **tex, void *sh) {
    HRESULT r = origCT(d, w, h, lv, usage, fmt, pool, tex, sh);
    if (r >= 0 && tex && *tex) {
        void **vt = *(void ***)*tex; DWORD old;
        if (vt[19] != (void *)hookLR && VirtualProtect(&vt[19], sizeof(void *), 0x40, &old)) { origLR = (LR_t)vt[19]; vt[19] = (void *)hookLR; VirtualProtect(&vt[19], sizeof(void *), old, &old); }
    }
    unsigned key = (fmt & 0xFFFFFF) ^ (usage << 24) ^ (pool << 30) ^ ((r < 0) << 31);
    int i; for (i = 0; i < nseen && seen[i] != key; i++) ;
    if (i == nseen && nseen < 64) {
        seen[nseen++] = key;
        char b[200]; char f[5] = {0};
        if (fmt > 0xFFFF) { f[0] = (char)fmt; f[1] = (char)(fmt >> 8); f[2] = (char)(fmt >> 16); f[3] = (char)(fmt >> 24); }
        wsprintfA(b, "CreateTexture fmt=%u%s%s%s usage=0x%x pool=%u size=%ux%u levels=%u hr=0x%08lx", fmt, f[0] ? " (" : "", f, f[0] ? ")" : "", usage, pool, w, h, lv, r);
        logline(b);
    }
    return r;
}
static void hookdev(void *dev, int ex) {
    if (!dev) return;
    void **vt = *(void ***)dev; DWORD old;
    if (VirtualProtect(vt, 122 * sizeof(void *), 0x40, &old)) {
        if (vt[17] != (void *)hookPR) { origPR = (PR_t)vt[17]; vt[17] = (void *)hookPR; }
        if (vt[16] != (void *)hookRS) { origRS = (RS_t)vt[16]; vt[16] = (void *)hookRS; }
        if (vt[23] != (void *)hookCT) { origCT = (CT_t)vt[23]; vt[23] = (void *)hookCT; }
        if (ex && vt[121] != (void *)hookPRX) { origPRX = (PRX_t)vt[121]; vt[121] = (void *)hookPRX; }
        VirtualProtect(vt, 122 * sizeof(void *), old, &old);
    }
}
static HRESULT WINAPI hookCD(void *s, UINT a, UINT t, void *w, DWORD f, PP *pp, void **dev) {
    fixpp(pp); HRESULT r = origCD(s, a, t, w, f, pp, dev);
    char b[100]; wsprintfA(b, "CreateDevice adapter=%u flags=0x%x hr=0x%08lx", a, f, r); logline(b);
    if (r >= 0) { hookdev(*dev, 0); if (!gwnd) { gwnd = pp->hwnd ? pp->hwnd : w; DWORD id; CreateThread(0, 0, watch, 0, 0, &id); } } return r;
}
static HRESULT WINAPI hookCDX(void *s, UINT a, UINT t, void *w, DWORD f, PP *pp, void *fm, void **dev) {
    fixpp(pp); HRESULT r = origCDX(s, a, t, w, f, pp, fm, dev);
    char b[100]; wsprintfA(b, "CreateDeviceEx adapter=%u flags=0x%x fullscreenmode=%p hr=0x%08lx", a, f, fm, r); logline(b);
    if (r >= 0) { hookdev(*dev, 1); if (!gwnd) { gwnd = pp->hwnd ? pp->hwnd : w; DWORD id; CreateThread(0, 0, watch, 0, 0, &id); } } return r;
}
static void hook(void *d3d, int ex) {
    if (!d3d) return;
    void **vt = *(void ***)d3d; DWORD old;
    if (VirtualProtect(vt, 21 * sizeof(void *), 0x40, &old)) {
        if (vt[16] != (void *)hookCD) { origCD = (CD_t)vt[16]; vt[16] = (void *)hookCD; }
        if (ex && vt[20] != (void *)hookCDX) { origCDX = (CDX_t)vt[20]; vt[20] = (void *)hookCDX; }
        VirtualProtect(vt, 21 * sizeof(void *), old, &old);
    }
}

typedef void *(WINAPI *C9On12_t)(UINT, D3D9ON12_ARGS *, UINT);
typedef HRESULT (WINAPI *C9On12Ex_t)(UINT, D3D9ON12_ARGS *, UINT, void **);
typedef void *(WINAPI *C9_t)(UINT);
typedef HRESULT (WINAPI *C9Ex_t)(UINT, void **);

void *WINAPI Direct3DCreate9(UINT sdk) {
    load();
    D3D9ON12_ARGS a = {1, 0, {0, 0}, 0, 0};
    C9On12_t f = (C9On12_t)GetProcAddress(real, "Direct3DCreate9On12");
    void *r = f ? f(sdk, &a, 1) : 0;
    if (!r) { logline("Direct3DCreate9On12 failed, using normal D3D9"); r = ((C9_t)GetProcAddress(real, "Direct3DCreate9"))(sdk); }
    else logline("Direct3DCreate9 -> D3D9On12 (Direct3D 12)");
    hook(r, 0); return r;
}
HRESULT WINAPI Direct3DCreate9Ex(UINT sdk, void **out) {
    load();
    D3D9ON12_ARGS a = {1, 0, {0, 0}, 0, 0};
    C9On12Ex_t f = (C9On12Ex_t)GetProcAddress(real, "Direct3DCreate9On12Ex");
    HRESULT r = f ? f(sdk, &a, 1, out) : -1;
    if (r < 0) { logline("Direct3DCreate9On12Ex failed, using normal D3D9Ex"); r = ((C9Ex_t)GetProcAddress(real, "Direct3DCreate9Ex"))(sdk, out); }
    else logline("Direct3DCreate9Ex -> D3D9On12 (Direct3D 12)");
    if (r >= 0) hook(*out, 1); return r;
}

/* ---- plain forwarders for everything else d3d9.dll exports ---- */
#define FWD(name) static void *p_##name; \
    __declspec(dllexport) __declspec(naked) void name(void) { __asm { jmp dword ptr [p_##name] } }
#define FWDLIST X(D3DPERF_BeginEvent) X(D3DPERF_EndEvent) X(D3DPERF_GetStatus) X(D3DPERF_QueryRepeatFrame) \
    X(D3DPERF_SetMarker) X(D3DPERF_SetOptions) X(D3DPERF_SetRegion) X(DebugSetLevel) X(DebugSetMute) \
    X(Direct3DShaderValidatorCreate9) X(PSGPError) X(PSGPSampleTexture) X(Direct3D9EnableMaximizedWindowedModeShim) \
    X(Direct3DCreate9On12) X(Direct3DCreate9On12Ex)
#define X(n) FWD(n)
FWDLIST
#undef X

BOOL WINAPI DllMain(HMODULE h, DWORD reason, void *r) {
    if (reason == 1) {
        load();
#define X(n) p_##n = GetProcAddress(real, #n);
        FWDLIST
#undef X
    }
    return 1;
}
