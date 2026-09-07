import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "RCC_");
  const adminTarget = env.RCC_ADMIN_URL || "http://127.0.0.1:8080";
  if (env.RCC_ADMIN_TOKEN || process.env.RCC_ADMIN_TOKEN) {
    throw new Error("RCC_ADMIN_TOKEN has been removed; use Local Account sessions through the same-origin proxy.");
  }

  return {
    plugins: [react(), tailwindcss()],
    resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
    build: {
      rolldownOptions: {
        output: {
          codeSplitting: {
            groups: [{
              name: (id) => {
                if (!id.includes("node_modules")) return null;
                return /\/node_modules\/(react|react-dom|scheduler)\//.test(id) ? "react" : "vendor";
              },
            }],
          },
        },
      },
    },
    server: {
      host: "127.0.0.1",
      port: 5173,
      proxy: {
        "/api": {
          target: adminTarget,
          changeOrigin: true,
        },
      },
    },
    test: {
      environment: "jsdom",
      setupFiles: "./src/test/setup.ts",
      css: true,
      // Full-page user-event flows mount many Radix controls under parallel CI workers.
      testTimeout: 10_000,
    },
  };
});
