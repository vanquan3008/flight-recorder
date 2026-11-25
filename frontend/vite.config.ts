import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
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
    pool: "vmThreads",
    exclude: ["e2e/**", "node_modules/**"],
    setupFiles: ["./src/testing/setup.ts"],
    // The first render of App loads lazy routes; on a loaded machine that can pass vitest's default 5s.
    testTimeout: 25_000,
  },
});
