import { defineConfig } from "@playwright/test";

const baseURL = process.env.BASE_URL || "http://localhost:8080";
const isExternal = !!process.env.BASE_URL;

export default defineConfig({
  globalSetup: isExternal ? undefined : "./global-setup.ts",
  testDir: "./tests",
  timeout: isExternal ? 60_000 : 30_000,
  retries: isExternal ? 1 : 0,
  workers: 1,
  use: {
    baseURL,
  },
  // CGI モードはプロセス起動が遅いため expect タイムアウトを延長。
  // Extend expect timeout for CGI mode (slow process startup).
  expect: {
    timeout: isExternal ? 10_000 : 5_000,
  },
  // Build and start murlog server before tests (skip if external).
  // テスト前に murlog をビルド・起動する (外部サーバーならスキップ)。
  webServer: isExternal
    ? undefined
    : {
        command: "cd ../.. && make serve",
        url: `${baseURL}/admin/setup`,
        reuseExistingServer: false,
        timeout: 15_000,
      },
});
