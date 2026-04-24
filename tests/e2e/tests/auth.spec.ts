import { test, expect } from "@playwright/test";
import {
  setupServer,
  login,
  loginWithPassword,
  loginWithTOTP,
  rpcCall,
  rpcLogin,
  generateTOTPCode,
} from "./helpers";

test.describe.serial("Authentication & Security", () => {
  test("setup server", async ({ request }) => {
    await setupServer(request);
  });

  test("login failure — wrong password shows error", async ({ page }) => {
    await page.goto("/my/login");
    await page.fill('input[type="password"]', "wrongpassword");
    await page.click('button[type="submit"]');

    // Error message should appear.
    // エラーメッセージが表示されること。
    await expect(page.locator(".error")).toBeVisible({ timeout: 5000 });

    // Should still be on login page.
    // ログインページのままであること。
    expect(page.url()).toContain("/my/login");
  });

  test("change password via RPC and verify login", async ({ page, request }) => {
    // Change password via RPC.
    // RPC でパスワードを変更。
    const cookie = await rpcLogin(request);
    const changeRes = await rpcCall(request, cookie, "auth.change_password", {
      current_password: "Test1234!",
      new_password: "NewPass123!",
    });
    expect(changeRes.result?.status).toBe("ok");

    try {
      // Verify login with new password.
      // 新パスワードでログインできることを確認。
      await loginWithPassword(page, "NewPass123!");

      // Verify old password fails.
      // 旧パスワードではログインできないことを確認。
      await page.locator(".header .dot-menu-btn").click();
      await page.getByRole("link", { name: /logout|ログアウト/i }).click();
      await page.waitForURL("**/my/login");
      await page.fill('input[type="password"]', "Test1234!");
      await page.click('button[type="submit"]');
      await expect(page.locator(".error")).toBeVisible({ timeout: 5000 });
    } finally {
      // Always restore original password.
      // 必ず元のパスワードに復元する。
      const res = await request.post("/api/mur/v1/rpc", {
        data: {
          jsonrpc: "2.0", id: 1,
          method: "auth.login",
          params: { password: "NewPass123!" },
        },
      });
      const cookies = res.headersArray()
        .filter((h) => h.name.toLowerCase() === "set-cookie")
        .map((h) => h.value.split(";")[0])
        .join("; ");
      await rpcCall(request, cookies, "auth.change_password", {
        current_password: "NewPass123!",
        new_password: "Test1234!",
      });
    }
  });

  test("TOTP full cycle — setup, verify, login, disable", async ({ page, request }) => {
    let totpSecret = "";

    try {
      await login(page);

      await page.evaluate(() => { window.location.href = "/my/settings"; });
      await page.waitForURL("**/my/settings");

      // Locate TOTP section.
      // TOTP セクションを特定。
      const totpSection = page.locator(".card-section").filter({
        hasText: /two-factor|二要素|2fa|totp/i,
      });
      await expect(totpSection).toBeVisible({ timeout: 10000 });

      // Click setup button.
      // Setup ボタンをクリック。
      await totpSection.locator(".btn-outline").first().click();

      // Wait for QR code canvas and secret to appear.
      // QR コード canvas と秘密鍵の表示を待つ。
      await expect(totpSection.locator("canvas")).toBeVisible({ timeout: 5000 });

      // Extract base32 secret from the meta element.
      // meta 要素から base32 秘密鍵を抽出。
      const secretEl = totpSection.locator(".meta").filter({ hasText: /^[A-Z2-7]{16,}$/ });
      totpSecret = (await secretEl.textContent())!.trim();
      expect(totpSecret).toMatch(/^[A-Z2-7]+$/);

      // Generate code and verify — do this immediately to stay within the 30s window.
      // コード生成と検証 — 30 秒窓内に収めるため即座に実行。
      const code = generateTOTPCode(totpSecret);
      await totpSection.locator('input[inputmode="numeric"]').fill(code);
      await totpSection.locator('button[type="submit"]').click();

      // TOTP should now be enabled.
      // TOTP が有効になったこと。
      await expect(
        totpSection.getByText(/enabled|有効/i),
      ).toBeVisible({ timeout: 5000 });

      // Logout and re-login with TOTP.
      // ログアウトして TOTP 付きでログインし直す。
      await page.locator(".header .dot-menu-btn").click();
      await page.getByRole("link", { name: /logout|ログアウト/i }).click();
      await page.waitForURL("**/my/login");

      // TOTP input should be visible on login page.
      // ログインページに TOTP 入力が表示されること。
      await expect(page.locator('input[inputmode="numeric"]')).toBeVisible({ timeout: 5000 });

      const freshCode = generateTOTPCode(totpSecret);
      await loginWithTOTP(page, "Test1234!", freshCode);

      // Disable TOTP from settings.
      // Settings から TOTP を無効化。
      await page.evaluate(() => { window.location.href = "/my/settings"; });
      await page.waitForURL("**/my/settings");

      const totpSection2 = page.locator(".card-section").filter({
        hasText: /two-factor|二要素|2fa|totp/i,
      });
      await expect(totpSection2).toBeVisible({ timeout: 10000 });
      await totpSection2.locator(".btn-outline").first().click();

      // Confirm TOTP is disabled.
      // TOTP が無効化されたことを確認。
      await expect(
        totpSection2.getByText(/disabled|無効/i),
      ).toBeVisible({ timeout: 5000 });

      // Verify login works without TOTP.
      // TOTP なしでログインできることを確認。
      await page.locator(".header .dot-menu-btn").click();
      await page.getByRole("link", { name: /logout|ログアウト/i }).click();
      await login(page);
    } finally {
      // Always disable TOTP via RPC if it was enabled.
      // TOTP が有効化されていた場合、RPC で確実に無効化する。
      if (totpSecret) {
        const code = generateTOTPCode(totpSecret);
        const loginRes = await request.post("/api/mur/v1/rpc", {
          data: {
            jsonrpc: "2.0", id: 1,
            method: "auth.login",
            params: { password: "Test1234!", totp_code: code },
          },
        });
        const cookies = loginRes.headersArray()
          .filter((h) => h.name.toLowerCase() === "set-cookie")
          .map((h) => h.value.split(";")[0])
          .join("; ");
        if (cookies) {
          await rpcCall(request, cookies, "totp.disable", {});
        }
      }
    }
  });
});
