import { resolve } from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig(({ command }) => {
  const devUrl = process.env.PORTLESS_URL ? new URL(process.env.PORTLESS_URL) : undefined;
  if (command === "serve" && (!devUrl || !process.env.PORT) && process.env.DAYBEE_E2E_TEST !== "1") {
    throw new Error("Development requires portless. Run bun run dev:web or bun run dev.");
  }

  return {
    root: "src/renderer",
    base: "./",
    plugins: [react(), tailwindcss(), {
      name: "portless-hmr-csp",
      apply: "serve",
      transformIndexHtml(html) {
        // WebSocket origins need an explicit CSP entry in some WebViews.
        return devUrl
          ? html.replace("connect-src 'self'", `connect-src 'self' ${devUrl.origin.replace(/^http/, "ws")}`)
          : html;
      }
    }],
    resolve: {
      alias: {
        "@": resolve("src/renderer/src"),
        "@daybee/shared": resolve("../../packages/shared/src/index.ts")
      }
    },
    build: { outDir: "../../out/renderer", emptyOutDir: true },
    server: {
      host: "127.0.0.1",
      port: process.env.PORT ? Number(process.env.PORT) : undefined,
      strictPort: true,
      allowedHosts: devUrl ? [devUrl.hostname] : undefined,
      origin: devUrl?.origin,
      hmr: devUrl ? {
        protocol: devUrl.protocol === "https:" ? "wss" : "ws",
        host: devUrl.hostname,
        clientPort: Number(devUrl.port || (devUrl.protocol === "https:" ? 443 : 80))
      } : undefined
    }
  };
});
