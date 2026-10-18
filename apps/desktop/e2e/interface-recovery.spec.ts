import type { DesktopSnapshot } from "@daybee/shared";
import type { Page, Route } from "@playwright/test";
import { expect, test } from "./fixtures";

const mainWindow = /\/src\/components\/main-window\.tsx/;

async function holdMainWindow(page: Page, body: string) {
  let release = () => {};
  const gate = new Promise<void>(resolve => { release = resolve; });
  const handler = async (route: Route) => {
    await gate;
    await route.fulfill({ status: 200, contentType: "application/javascript", body });
  };
  await page.route(mainWindow, handler);
  return {
    release,
    async stop() { await page.unroute(mainWindow, handler); }
  };
}

test("shows a visible recovery affordance after a synthetic render failure", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  const logged: string[] = [];
  page.on("console", message => { if (message.type() === "error") logged.push(message.text()); });
  await page.addInitScript(() => {
    const desktop = window as Window & { __desktopCalls?: string[] };
    const bridge = window.mygo;
    if (!bridge) return;
    const call = bridge.call.bind(bridge);
    desktop.__desktopCalls = [];
    bridge.call = ((method: string, ...args: unknown[]) => {
      desktop.__desktopCalls?.push(method);
      return call(method, ...args);
    }) as typeof bridge.call;
  });
  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  snapshot.settings.desktop.language = "zh-CN";
  await app.setFixture(snapshot);
  const held = await holdMainWindow(page, "export function MainWindow(){ throw new Error(\"synthetic render failure\"); }\n");
  await page.goto("/?window=main");
  await expect(page.getByRole("status")).toHaveText("正在加载界面");
  held.release();
  await expect(page.getByRole("heading", { name: "界面加载失败" })).toBeVisible();
  await expect(page.getByRole("button", { name: "刷新界面" })).toBeVisible();
  await expect.poll(() => logged.some(line => line.includes("synthetic render failure") && line.includes("MainWindow"))).toBe(true);
  const calls = await page.evaluate(() => (window as Window & { __desktopCalls?: string[] }).__desktopCalls ?? []);
  expect(calls.some(call => /Recording/.test(call))).toBe(false);
  await held.stop();
  await page.getByRole("button", { name: "刷新界面" }).click();
  await expect(page.locator("[data-window=main]")).toBeVisible();
  await expect(page.getByRole("heading", { name: "界面加载失败" })).toHaveCount(0);

  snapshot.settings.desktop.language = "en-US";
  await app.setFixture(snapshot);
  const again = await holdMainWindow(page, "export function MainWindow(){ throw new Error(\"synthetic render failure\"); }\n");
  await page.reload();
  await expect(page.getByRole("status")).toHaveText("Loading the interface");
  again.release();
  await expect(page.getByRole("heading", { name: "The interface failed to load" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Reload the interface" })).toBeVisible();
  await again.stop();
  await page.getByRole("button", { name: "Reload the interface" }).click();
  await expect(page.locator("[data-window=main]")).toBeVisible();
  await expect(page.getByRole("heading", { name: "The interface failed to load" })).toHaveCount(0);

  const nullLogs = logged.length;
  const thrownNull = await holdMainWindow(page, "export function MainWindow(){ throw null; }\n");
  await page.reload();
  await expect(page.getByRole("status")).toHaveText("Loading the interface");
  thrownNull.release();
  await expect(page.getByRole("heading", { name: "The interface failed to load" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Reload the interface" })).toBeVisible();
  await expect.poll(() => logged.slice(nullLogs).some(line => line.includes("null") && line.includes("MainWindow"))).toBe(true);
  const nullCalls = await page.evaluate(() => (window as Window & { __desktopCalls?: string[] }).__desktopCalls ?? []);
  expect(nullCalls.some(call => /Recording/.test(call))).toBe(false);
  await thrownNull.stop();
  await page.getByRole("button", { name: "Reload the interface" }).click();
  await expect(page.locator("[data-window=main]")).toBeVisible();
  await expect(page.getByRole("heading", { name: "The interface failed to load" })).toHaveCount(0);
  await app.close();
});
