import { expect, test } from "./fixtures";

test("shows scoped MCP client configuration in the compact General tab", async ({ launchApp, storeDir }) => {
  const { app, page } = await launchApp();
  await page.setViewportSize({ width: 380, height: 480 });
  await page.getByRole("tab", { name: /General|通用/, exact: true }).click();
  const mcp = page.getByTestId("mcp-settings");
  const copy = mcp.getByRole("button", { name: /Copy MCP configuration|复制 MCP 配置/ });
  await expect(copy).toBeEnabled();
  await mcp.getByText(/View configuration|查看连接配置/, { exact: true }).click();
  const field = mcp.getByRole("textbox", { name: /MCP client configuration|MCP 客户端配置/ });
  const value = JSON.parse(await field.inputValue());
  expect(value.mcpServers.daybee.args).toEqual(["--mcp"]);
  expect(value.mcpServers.daybee.command).toBeTruthy();
  expect(value.mcpServers.daybee.env).toEqual({ DAYBEE_STORE_DIR: storeDir });
  expect(await mcp.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
  // Verify the copy action without replacing the user's system clipboard.
  await page.evaluate(() => {
    const bridge = window.mygo!;
    const call = bridge.call.bind(bridge);
    bridge.call = ((method: string, ...args: unknown[]) => method === "DesktopService.CopyMCPConfig"
      ? call("DesktopService.GetMCPConfig") : call(method, ...args)) as typeof bridge.call;
  });
  await copy.click();
  await expect(mcp.getByRole("button", { name: /Configuration copied|已复制连接配置/ })).toBeVisible();
  for (const tab of await page.getByRole("tab").all()) await expect(tab).toBeInViewport();
  await app.close();
});
