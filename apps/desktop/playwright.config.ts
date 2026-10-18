import { defineConfig } from "@playwright/test";

export default defineConfig({
  fullyParallel: false,
  reporter: "list",
  testDir: "./e2e",
  timeout: 30_000,
  use: {
    baseURL: "http://127.0.0.1:41739",
    trace: "on-first-retry"
  },
  workers: 1,
  webServer: {
    command: "bunx --no-install vite --port 41739",
    env: { DAYBEE_E2E_TEST: "1", PORT: "41739", PORTLESS_URL: "" },
    url: "http://127.0.0.1:41739",
    reuseExistingServer: false
  }
});
