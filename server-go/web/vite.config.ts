import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: Object.fromEntries(
      ["/api", "/health", "/workers", "/worker", "/releases"].map(p => [
        p,
        {
          target: "http://127.0.0.1:3000",
          changeOrigin: true,
          configure: (proxy: any) => {
            proxy.on("error", (err: any) => {
              // eslint-disable-next-line no-console
              console.error("[vite proxy error]", err.code, err.message);
            });
          },
        },
      ]),
    ),

    // Switch back to remote when needed:
    // proxy: { "/api": "https://gitai.tongbaninfo.com", ... }
  },
  build: {
    outDir: "dist",
    sourcemap: true,
  },
});
