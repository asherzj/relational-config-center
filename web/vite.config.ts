import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "RCC_");
  const adminTarget = env.RCC_ADMIN_URL || "http://127.0.0.1:8080";
  const adminToken = env.RCC_ADMIN_TOKEN;

  return {
    plugins: [react()],
    server: {
      host: "127.0.0.1",
      port: 5173,
      proxy: {
        "/api": {
          target: adminTarget,
          changeOrigin: true,
          configure(proxy) {
            proxy.on("proxyReq", (proxyRequest) => {
              if (adminToken) {
                proxyRequest.setHeader("Authorization", `Bearer ${adminToken}`);
              }
            });
          },
        },
      },
    },
    test: {
      environment: "jsdom",
      setupFiles: "./src/test/setup.ts",
      css: true,
    },
  };
});
