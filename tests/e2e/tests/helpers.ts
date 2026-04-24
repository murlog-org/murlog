// Shared e2e test helpers.
// e2e テスト共通ヘルパー。

import { expect, type APIRequestContext, type Page } from "@playwright/test";
import * as OTPAuth from "otpauth";

// setupServer runs the 2-step setup if not already done.
// 未セットアップなら 2 ステップのセットアップを実行する。
export async function setupServer(request: APIRequestContext) {
  const step1 = await request.get("/admin/setup/server", { maxRedirects: 0 });
  if (step1.status() === 200) {
    const csrf1 = step1.headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((h) => h.value)
      .find((c) => c.startsWith("_csrf="))?.split("=")[1]?.split(";")[0] || "";
    await request.post("/admin/setup/server", {
      form: { data_dir: "./data", media_path: "./media", _csrf: csrf1 },
      maxRedirects: 0,
    });
  }
  const step2 = await request.get("/admin/setup", { maxRedirects: 0 });
  if (step2.status() === 200) {
    const csrf2 = step2.headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((h) => h.value)
      .find((c) => c.startsWith("_csrf="))?.split("=")[1]?.split(";")[0] || "";
    await request.post("/admin/setup", {
      form: {
        domain: "localhost",
        username: "alice",
        display_name: "Alice",
        password: "Test1234!",
        password_confirm: "Test1234!",
        _csrf: csrf2,
      },
      maxRedirects: 0,
    });
  }
}

// login navigates to the login page, enters the password, and waits for the timeline.
// ログインページに遷移し、パスワードを入力してタイムライン表示を待つ。
export async function login(page: Page) {
  await page.goto("/my/login");
  await page.fill('input[type="password"]', "Test1234!");
  await page.click('button[type="submit"]');
  await expect(page.locator(".tab").first()).toBeVisible();
}

// rpcLogin logs in via RPC and returns the session cookie string.
// RPC でログインしてセッション Cookie 文字列を返す。
export async function rpcLogin(request: APIRequestContext): Promise<string> {
  const loginRes = await request.post("/api/mur/v1/rpc", {
    data: { jsonrpc: "2.0", id: 1, method: "auth.login", params: { password: "Test1234!" } },
  });
  const cookies = loginRes.headersArray()
    .filter((h) => h.name.toLowerCase() === "set-cookie")
    .map((h) => h.value);
  return cookies.map((c) => c.split(";")[0]).join("; ");
}

// Generic JSON-RPC 2.0 helper.
// 汎用 JSON-RPC 2.0 ヘルパー。
export async function rpcCall(
  request: APIRequestContext,
  cookie: string,
  method: string,
  params: Record<string, unknown> = {},
): Promise<{ result?: any; error?: any }> {
  const res = await request.post("/api/mur/v1/rpc", {
    headers: { Cookie: cookie },
    data: { jsonrpc: "2.0", id: 1, method, params },
  });
  return res.json();
}

// Login with a custom password (for password-change tests).
// カスタムパスワードでログイン (パスワード変更テスト用)。
export async function loginWithPassword(page: Page, password: string) {
  await page.goto("/my/login");
  await page.fill('input[type="password"]', password);
  await page.click('button[type="submit"]');
  await expect(page.locator(".tab").first()).toBeVisible();
}

// Login with password + TOTP code.
// パスワード + TOTP コードでログイン。
export async function loginWithTOTP(page: Page, password: string, totpCode: string) {
  await page.goto("/my/login");
  await page.fill('input[type="password"]', password);
  await page.locator('input[inputmode="numeric"]').fill(totpCode);
  await page.click('button[type="submit"]');
  await expect(page.locator(".tab").first()).toBeVisible();
}

// Generate a 6-digit TOTP code from a base32 secret.
// base32 秘密鍵から 6 桁 TOTP コードを生成する。
export function generateTOTPCode(secret: string): string {
  const totp = new OTPAuth.TOTP({
    secret: OTPAuth.Secret.fromBase32(secret),
    digits: 6,
    period: 30,
    algorithm: "SHA1",
  });
  return totp.generate();
}

// Minimal valid 1x1 red pixel PNG (for media upload tests).
// 最小の 1x1 赤ピクセル PNG (メディアアップロードテスト用)。
export function createTestImageBuffer(): Buffer {
  return Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg==",
    "base64",
  );
}
