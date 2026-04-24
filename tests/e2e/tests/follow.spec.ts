import { test, expect } from "@playwright/test";
import { setupServer, login, rpcLogin, rpcCall } from "./helpers";

test.describe.serial("Follow page — UI interactions", () => {
  let cookie = "";

  test("setup server", async ({ request }) => {
    await setupServer(request);
    cookie = await rpcLogin(request);
  });

  test("search — nonexistent user shows error", async ({ page }) => {
    await login(page);

    await page.evaluate(() => { window.location.href = "/my/follow"; });
    await page.waitForURL("**/my/follow");
    await expect(page.locator('input[type="text"]').first()).toBeVisible({ timeout: 10000 });

    // Search for a nonexistent user.
    // 存在しないユーザーを検索。
    await page.locator('input[type="text"]').first().fill("nobody@nonexistent.example");
    await page.locator('button[type="submit"]').first().click();

    // Error should appear (lookup fails since no real remote server).
    // エラーが表示されること (リモートサーバーがないので lookup 失敗)。
    // The lookup may take time (WebFinger request to nonexistent host).
    // Lookup は時間がかかる場合がある (存在しないホストへの WebFinger リクエスト)。
    await expect(page.locator('p.meta[style*="danger"]')).toBeVisible({
      timeout: 20000,
    });

    // No lookup result card.
    // lookup 結果カードが表示されないこと。
    await expect(page.locator(".lookup-result")).not.toBeVisible();
  });

  test("tabs and empty states", async ({ page }) => {
    await login(page);

    await page.evaluate(() => { window.location.href = "/my/follow"; });
    await page.waitForURL("**/my/follow");
    await expect(page.locator(".tab").first()).toBeVisible({ timeout: 10000 });

    // Following tab should be active by default.
    // Following タブがデフォルトでアクティブ。
    await expect(page.locator(".tab.active")).toContainText(/following|フォロー/i);

    // No user cards in empty state.
    // 空状態では user-card がないこと。
    await expect(page.locator(".user-card")).toHaveCount(0);

    // Switch to followers tab.
    // フォロワータブに切り替え。
    await page.locator(".tab").filter({ hasText: /followers|フォロワー/i }).click();
    await expect(page.locator(".tab.active")).toContainText(/followers|フォロワー/i);
    await expect(page.locator(".user-card")).toHaveCount(0);
  });

  test("follows and followers list via RPC — empty", async ({ request }) => {
    // follows.list should return empty array.
    // follows.list が空配列を返すこと。
    const followsRes = await rpcCall(request, cookie, "follows.list", {});
    expect(followsRes.result).toEqual([]);

    // followers.list should return empty array.
    // followers.list が空配列を返すこと。
    const followersRes = await rpcCall(request, cookie, "followers.list", {});
    expect(followersRes.result).toEqual([]);

    // followers.pending should return empty array.
    // followers.pending が空配列を返すこと。
    const pendingRes = await rpcCall(request, cookie, "followers.pending", {});
    expect(pendingRes.result).toEqual([]);
  });
});
