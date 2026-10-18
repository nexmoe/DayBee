import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";
import { randomBytes } from "node:crypto";
import { createServer } from "node:net";
import { resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";

const productName: string = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")).productName;

export async function launchNative(storeDir: string) {
  const port = await new Promise<number>((resolve, reject) => {
    const server = createServer(); server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") throw new Error("Missing test port");
      server.close(() => resolve(address.port));
    });
  });
  const token = randomBytes(32).toString("hex");
  const target = process.platform === "win32" ? "windows-amd64" : `darwin-${process.arch === "x64" ? "amd64" : "arm64"}`;
  const executable = process.env.DAYBEE_TEST_EXECUTABLE || resolve("dist", target,
    ...(process.platform === "darwin" ? [`${productName}.app`, "Contents", "MacOS", productName] : [`${productName}.exe`]));
  const child = spawn(executable, [], {
    env: { ...process.env, DAYBEE_E2E_TEST: "1", DAYBEE_TEST_BACKGROUND: "1", DAYBEE_STORE_DIR: storeDir, DAYBEE_TEST_TOKEN: token, DAYBEE_TEST_PORT: String(port) }, stdio: "pipe"
  });
  let logs = Buffer.alloc(0);
  const captureLog = (data: Buffer) => { logs = Buffer.concat([logs, data]).subarray(-64 * 1024); };
  child.stdout?.on("data", captureLog);
  child.stderr?.on("data", captureLog);
  let spawnError: Error | undefined;
  child.once("error", error => { spawnError = error; });
  const close = async () => {
    if (child.exitCode !== null || child.signalCode !== null || spawnError) return;
    child.kill("SIGTERM");
    await Promise.race([new Promise(resolve => child.once("exit", resolve)), delay(5000).then(() => child.kill("SIGKILL"))]);
  };
  const request = async (path: string, body?: unknown) => {
    const response = await fetch(`http://127.0.0.1:${port}${path}`, {
      method: "POST", headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(15000)
    });
    if (!response.ok) throw new Error(await response.text()); return response.json();
  };
  const deadline = Date.now() + 30000;
  try { while (true) {
    if (spawnError) throw spawnError;
    try {
      await request("/snapshot");
      const ready = await request("/eval", { code: "Boolean(window.mygo && document.querySelector('[data-window=compact]'))" });
      if (ready) {
        const window = await request("/window");
        if (window.visible || window.focused) throw new Error("E2E backend must remain hidden and unfocused");
        break;
      }
      if (Date.now() > deadline) throw new Error("Native page did not become ready");
      await delay(100);
    }
    catch (error) { if (Date.now() > deadline || child.exitCode !== null) throw new Error(`${error}\n${logs}`); await delay(100); }
  } } catch (error) { await close(); throw error; }
  return {
    call: (method: string, args: unknown[]) => request("/eval", { code: `window.mygo.call(${JSON.stringify(method)}, ...${JSON.stringify(args)})` }),
    snapshot: () => request("/snapshot"),
    completeSpeech: (text: string) => request("/speech", { text }),
    setFixture: (snapshot: unknown) => request("/fixture", snapshot),
    close
  };
}
