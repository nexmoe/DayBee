import { defineConfig } from "vitest/config";
import { resolve } from "node:path";

export default defineConfig({
  resolve: { alias: { "@": resolve("src/renderer/src") } },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"]
  }
});
