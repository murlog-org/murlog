import { test, expect } from "@playwright/test";
import { setupServer, login, rpcLogin, rpcCall } from "./helpers";

test.describe.serial("Notifications", () => {
  let cookie = "";

  test("setup server", async ({ request }) => {
    await setupServer(request);
    cookie = await rpcLogin(request);
  });

  test("notifications.list returns empty", async ({ request }) => {
    const res = await rpcCall(request, cookie, "notifications.list", {});
    expect(res.result).toEqual([]);
  });

  test("notifications.read_all succeeds on empty", async ({ request }) => {
    const res = await rpcCall(request, cookie, "notifications.read_all", {});
    expect(res.result?.status).toBe("ok");
  });

  test("notifications page — empty state", async ({ page }) => {
    await login(page);

    await page.evaluate(() => { window.location.href = "/my/notifications"; });
    await page.waitForURL("**/my/notifications");

    // Page should load without error.
    // エラーなく読み込まれること。
    await expect(page.locator(".screen")).toBeVisible({ timeout: 10000 });

    // No notification cards.
    // 通知カードが表示されないこと。
    await expect(page.locator(".card.notif")).toHaveCount(0);
  });

  test("notifications.poll returns empty", async ({ request }) => {
    const res = await rpcCall(request, cookie, "notifications.poll", {
      since: "00000000-0000-0000-0000-000000000000",
    });
    expect(res.result).toEqual([]);
  });
});
