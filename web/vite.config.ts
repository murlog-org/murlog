import { defineConfig } from "vite";
import preact from "@preact/preset-vite";
import { readFileSync } from "fs";

const version = readFileSync("../version.txt", "utf-8").trim();

export default defineConfig({
  plugins: [preact()],
  server: {
    port: 5173,
    proxy: {
      // API requests → Go server
      "/api": "http://localhost:8080",
      // ActivityPub / WebFinger
      "/users": "http://localhost:8080",
      "/.well-known": "http://localhost:8080",
      // OAuth
      "/oauth": "http://localhost:8080",
      // Admin SSR (setup, reset)
      "/admin": "http://localhost:8080",
      // Media files
      "/media": "http://localhost:8080",
    },
  },
  define: {
    __BUILD_HASH__: JSON.stringify(Date.now().toString(36)),
    __APP_VERSION__: JSON.stringify(version),
  },
  build: {
    outDir: "dist",
    manifest: true,
    rollupOptions: {
      output: {
        // Fixed file names — cache busting via ?v= query param instead of hash in filename.
        // 固定ファイル名 — キャッシュバスティングはファイル名ハッシュではなく ?v= クエリで行う。
        entryFileNames: "assets/index.js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/[name][extname]",
      },
    },
  },
});
