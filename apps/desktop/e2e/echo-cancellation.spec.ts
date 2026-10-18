import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

test("echo cancellation and auto-switch remain independently configurable and persistent", async ({ launchApp }) => {
  test.skip(process.platform !== "darwin", "Native echo cancellation currently supports macOS");
  let { app, page } = await launchApp();
  const openInput = async () => {
    await page.goto("/");
    await page.setViewportSize({ width: 1000, height: 760 });
    await page.getByRole("tablist").getByRole("tab", { name: /Input|输入/, exact: true }).click();
  };
  await openInput();
  let echo = page.getByTestId("echo-cancellation-field").getByRole("switch");
  let auto = page.getByTestId("auto-switch-input-field").getByRole("switch");
  await expect(echo).toBeChecked();
  await expect(auto).toBeEnabled();
  await expect(auto).toBeChecked();
  await auto.click();
  await expect.poll(async () => {
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    return snapshot.settings.recording.echoCancellation && !snapshot.settings.recording.autoSwitchDevice && !snapshot.status.autoSwitchEnabled;
  }).toBe(true);
  await app.close();

  ({ app, page } = await launchApp());
  await openInput();
  echo = page.getByTestId("echo-cancellation-field").getByRole("switch");
  auto = page.getByTestId("auto-switch-input-field").getByRole("switch");
  await expect(echo).toBeChecked();
  await expect(auto).not.toBeChecked();
  await expect(auto).toBeEnabled();
  await echo.click();
  await expect(auto).not.toBeChecked();
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).settings.recording.echoCancellation).toBe(false);
  await echo.click();
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).settings.recording.echoCancellation).toBe(true);
  await expect(auto).not.toBeChecked();
  await auto.click();
  await expect.poll(async () => {
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    return snapshot.settings.recording.echoCancellation && snapshot.settings.recording.autoSwitchDevice && snapshot.status.autoSwitchEnabled;
  }).toBe(true);
  await page.setViewportSize({ width: 380, height: 640 });
  await expect(auto).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await app.close();
});
