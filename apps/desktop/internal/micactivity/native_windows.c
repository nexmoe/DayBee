#define COBJMACROS
#include <windows.h>
#include <mmdeviceapi.h>
#include <audiopolicy.h>
#include <shellapi.h>
#include <stdlib.h>
#include <wchar.h>

DWORD *daybeeCapturePIDs(UINT *count, HRESULT *status) {
    *count = 0;
    *status = CoInitializeEx(NULL, COINIT_MULTITHREADED);
    if (FAILED(*status)) return NULL;
    IMMDeviceEnumerator *devices = NULL;
    IMMDeviceCollection *endpoints = NULL;
    DWORD *pids = NULL;
    *status = CoCreateInstance(&CLSID_MMDeviceEnumerator, NULL, CLSCTX_ALL,
        &IID_IMMDeviceEnumerator, (void **)&devices);
    if (FAILED(*status)) goto done;
    *status = IMMDeviceEnumerator_EnumAudioEndpoints(devices, eCapture, DEVICE_STATE_ACTIVE, &endpoints);
    if (FAILED(*status)) goto done;
    UINT endpointCount = 0;
    *status = IMMDeviceCollection_GetCount(endpoints, &endpointCount);
    if (FAILED(*status)) goto done;
    for (UINT i = 0; i < endpointCount; i++) {
        IMMDevice *endpoint = NULL;
        IAudioSessionManager2 *manager = NULL;
        IAudioSessionEnumerator *sessions = NULL;
        HRESULT hr = IMMDeviceCollection_Item(endpoints, i, &endpoint);
        if (SUCCEEDED(hr)) hr = IMMDevice_Activate(endpoint, &IID_IAudioSessionManager2,
            CLSCTX_ALL, NULL, (void **)&manager);
        if (SUCCEEDED(hr)) hr = IAudioSessionManager2_GetSessionEnumerator(manager, &sessions);
        int sessionCount = 0;
        if (SUCCEEDED(hr)) hr = IAudioSessionEnumerator_GetCount(sessions, &sessionCount);
        for (int j = 0; SUCCEEDED(hr) && j < sessionCount; j++) {
            IAudioSessionControl *session = NULL;
            IAudioSessionControl2 *details = NULL;
            AudioSessionState state;
            DWORD pid = 0;
            hr = IAudioSessionEnumerator_GetSession(sessions, j, &session);
            if (SUCCEEDED(hr)) {
                hr = IAudioSessionControl_GetState(session, &state);
                if (SUCCEEDED(hr) && state == AudioSessionStateActive &&
                    SUCCEEDED(IAudioSessionControl_QueryInterface(session, &IID_IAudioSessionControl2, (void **)&details)) &&
                    SUCCEEDED(IAudioSessionControl2_GetProcessId(details, &pid)) && pid != 0) {
                    DWORD *next = realloc(pids, (*count + 1) * sizeof(DWORD));
                    if (!next) { hr = E_OUTOFMEMORY; }
                    else { pids = next; pids[(*count)++] = pid; }
                }
                if (details) IAudioSessionControl2_Release(details);
                IAudioSessionControl_Release(session);
            }
        }
        if (sessions) IAudioSessionEnumerator_Release(sessions);
        if (manager) IAudioSessionManager2_Release(manager);
        if (endpoint) IMMDevice_Release(endpoint);
        // An incomplete scan must not end sessions on an unavailable device.
        if (FAILED(hr)) { *status = hr; goto done; }
    }
done:
    if (endpoints) IMMDeviceCollection_Release(endpoints);
    if (devices) IMMDeviceEnumerator_Release(devices);
    CoUninitialize();
    return pids;
}

int daybeeProcessPath(DWORD pid, WCHAR *path, DWORD size) {
    HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if (!process) return 0;
    BOOL success = QueryFullProcessImageNameW(process, 0, path, &size);
    CloseHandle(process);
    return success;
}

void daybeeApplicationInfo(const WCHAR *path, WCHAR *name, DWORD nameSize, unsigned char *pixels) {
    DWORD ignored = 0;
    DWORD size = GetFileVersionInfoSizeW(path, &ignored);
    void *version = size ? malloc(size) : NULL;
    if (version && GetFileVersionInfoW(path, 0, size, version)) {
        struct { WORD language; WORD codepage; } *translation = NULL;
        UINT length = 0;
        if (VerQueryValueW(version, L"\\VarFileInfo\\Translation", (void **)&translation, &length) && length >= 4) {
            WCHAR key[128];
            swprintf(key, 128, L"\\StringFileInfo\\%04x%04x\\FileDescription", translation->language, translation->codepage);
            WCHAR *description = NULL;
            if (VerQueryValueW(version, key, (void **)&description, &length) && length > 1)
                lstrcpynW(name, description, nameSize);
        }
    }
    free(version);
    SHFILEINFOW info = {0};
    if (!SHGetFileInfoW(path, 0, &info, sizeof(info), SHGFI_ICON | SHGFI_LARGEICON)) return;
    BITMAPINFO format = {0};
    format.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    format.bmiHeader.biWidth = 32;
    format.bmiHeader.biHeight = -32;
    format.bmiHeader.biPlanes = 1;
    format.bmiHeader.biBitCount = 32;
    format.bmiHeader.biCompression = BI_RGB;
    void *bits = NULL;
    HDC dc = CreateCompatibleDC(NULL);
    HBITMAP bitmap = CreateDIBSection(dc, &format, DIB_RGB_COLORS, &bits, NULL, 0);
    if (bitmap && bits) {
        HGDIOBJ previous = SelectObject(dc, bitmap);
        ZeroMemory(bits, 32 * 32 * 4);
        if (DrawIconEx(dc, 0, 0, info.hIcon, 32, 32, 0, NULL, DI_NORMAL)) {
            CopyMemory(pixels, bits, 32 * 32 * 4);
            BOOL hasAlpha = FALSE;
            for (int i = 3; i < 32 * 32 * 4; i += 4) hasAlpha |= pixels[i] != 0;
            // Legacy icons carry transparency in an AND mask instead of alpha.
            if (!hasAlpha && DrawIconEx(dc, 0, 0, info.hIcon, 32, 32, 0, NULL, DI_MASK)) {
                unsigned char *mask = bits;
                for (int i = 0; i < 32 * 32 * 4; i += 4) pixels[i + 3] = mask[i] ? 0 : 255;
            }
        }
        SelectObject(dc, previous);
    }
    if (bitmap) DeleteObject(bitmap);
    if (dc) DeleteDC(dc);
    DestroyIcon(info.hIcon);
}
