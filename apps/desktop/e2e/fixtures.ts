import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { test as base, expect, type Page } from "@playwright/test";
import { launchNative } from "./native-driver";
import { DEFAULT_SETTINGS, type DesktopSnapshot } from "@daybee/shared";

type AppContext = { app: {
  close(): Promise<void>;
  completeSpeech(text: string): Promise<unknown>;
  refreshHistory(): Promise<unknown>;
  setFixture(snapshot: DesktopSnapshot): Promise<unknown>;
}; page: Page };

export const test = base.extend<{
  launchApp: (options?: { voiceprintSetup?: "pending" | "skipped" | "ready"; platformASRDefault?: boolean }) => Promise<AppContext>;
  storeDir: string;
}>({
  storeDir: async ({}, use) => {
    const directory = mkdtempSync(`${tmpdir()}/daybee-mygo-e2e-`);
    try { await use(directory); } finally { rmSync(directory, { force: true, recursive: true }); }
  },
  launchApp: async ({ browser, storeDir }, use) => {
    const cleanup: (() => Promise<void>)[] = [];
    try {
      await use(async (options) => {
        const settingsPath = join(storeDir, "desktop-settings.json");
        if (!existsSync(settingsPath)) {
          const settings = structuredClone(DEFAULT_SETTINGS);
          settings.desktop.voiceprintSetup = options?.voiceprintSetup ?? "skipped";
          settings.desktop.launchAtLogin = false;
          settings.recording.outputDir = join(storeDir, "recordings");
          if (options?.platformASRDefault) {
            delete (settings.recording as Partial<typeof settings.recording>).asrModel;
          }
          writeFileSync(settingsPath, JSON.stringify({ settings, nativeDevices: true }));
        }
        const native = await launchNative(storeDir);
        const page = await browser.newPage({ viewport: { width: 420, height: 620 } });
        let closed = false;
        const close = async () => { if (closed) return; closed = true; try { await page.close(); } finally { await native.close(); } };
        cleanup.push(close);
        await page.exposeFunction("__nativeCall", native.call);
        await page.exposeFunction("__nativeSnapshot", native.snapshot);
        await page.addInitScript(({ platform }) => {
          const testWindow = window as unknown as {
            __nativeCall(method: string, args: unknown[]): Promise<unknown>;
            __nativeSnapshot(): Promise<unknown>;
          };
          const listeners = new Map<string, Set<(payload: unknown) => void>>();
          window.mygo = {
            platform, windowId: 1, version: "0.3.4",
            call: (method: string, ...args: unknown[]) => testWindow.__nativeCall(method, args),
            on: (name: string, listener: (payload: unknown) => void) => {
              if (!listeners.has(name)) listeners.set(name, new Set());
              listeners.get(name)!.add(listener);
              return () => { listeners.get(name)?.delete(listener); };
            }
          } as NonNullable<Window["mygo"]>;
          setInterval(() => { void testWindow.__nativeSnapshot().then(snapshot => {
            listeners.get("desktop:snapshot")?.forEach(listener => listener(snapshot));
          }); }, 100);
        }, { platform: process.platform === "win32" ? "windows" : "darwin" });
        await page.goto("/");
        await expect(page.getByRole("button", { name: /Start recording|开始录音|Stop|停止/ })).toBeVisible();
        return { app: {
          close, completeSpeech: native.completeSpeech, setFixture: native.setFixture,
          refreshHistory: async () => {
            const snapshot = await native.snapshot() as DesktopSnapshot;
            // The mock batch operation refreshes disk fixtures through the normal
            // service path without waiting for the five-minute fallback scan.
            return native.call("DesktopService.RunTranscribe", [snapshot.settings.recording.outputDir]);
          }
        }, page };
      });
    } finally { for (const close of cleanup) await close(); }
  }
});
export { expect };
