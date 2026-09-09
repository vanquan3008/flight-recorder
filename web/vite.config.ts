import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: fileURLToPath(new URL("../backend/internal/webui/dist", import.meta.url)),
    emptyOutDir: true,
    assetsDir: "assets",
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
      "/health": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    exclude: ["e2e/**", "node_modules/**"],
  },
});
