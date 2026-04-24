import { test, expect } from "@playwright/test";
import { setupServer, login, rpcLogin, rpcCall } from "./helpers";

test.describe.serial("SPA — My page", () => {
  test("setup server", async ({ request }) => {
    await setupServer(request);
    // Clean up any leftover posts from previous test runs.
    // 前回テストの残留投稿をクリーンアップ。
    const cookie = await rpcLogin(request);
    const list = await rpcCall(request, cookie, "timeline.home", { limit: 100 });
    for (const p of list.result || []) {
      await rpcCall(request, cookie, "posts.delete", { id: p.id });
    }
  });

  test("login → create post → edit → delete", async ({ page }) => {
    await login(page);

    // Create post
    await page.fill("textarea", "Hello from Playwright!");
    await page.locator('.compose button[type="submit"]').click();
    await expect(page.locator(".post-content").first()).toContainText(
      "Hello from Playwright!"
    );

    // Edit post — open dot-menu, click edit
    const postCard = page.locator(".card").nth(1);
    await postCard.locator(".dot-menu-btn").click();
    await postCard.getByRole("link", { name: /edit|編集/i }).click();
    const editTextarea = postCard.locator("textarea");
    await expect(editTextarea).toBeVisible();
    await editTextarea.fill("Edited by Playwright!");
    await postCard.getByRole("button", { name: /save|保存/i }).first().click();
    await expect(page.locator(".post-content").first()).toContainText(
      "Edited by Playwright!"
    );

    // Delete post — open dot-menu, click delete
    await postCard.locator(".dot-menu-btn").click();
    await postCard.getByRole("link", { name: /delete|削除/i }).click();
    await expect(page.locator(".post-content")).toHaveCount(0);
  });

  test("blocks page — block and unblock actor + domain", async ({ page }) => {
    await login(page);

    await page.evaluate(() => { window.location.href = "/my/blocks"; });
    await page.waitForURL("**/my/blocks");
    await expect(page.locator('input[placeholder*="user@"]')).toBeVisible({ timeout: 10000 });

    // Block an actor
    const actorInput = page.locator('input[placeholder*="user@"]');
    await actorInput.fill("https://spam.example/users/bad");
    await page.locator("form").first().locator('button[type="submit"]').click();
    await expect(page.locator(".handle").filter({ hasText: "spam.example/users/bad" })).toBeVisible();

    // Unblock
    await page.locator(".card").filter({ hasText: "spam.example/users/bad" }).locator(".btn-outline").click();
    await expect(page.locator(".handle").filter({ hasText: "spam.example/users/bad" })).not.toBeVisible();

    // Switch to domain tab
    await page.locator(".tab").filter({ hasText: /domain|ドメイン/i }).click();
    await expect(page.locator('input[placeholder="spam.example.com"]')).toBeVisible();

    // Block a domain
    const domainInput = page.locator('input[placeholder="spam.example.com"]');
    await domainInput.fill("evil.example");
    await page.locator("form").first().locator('button[type="submit"]').click();
    await expect(page.locator(".post-author-name").filter({ hasText: "evil.example" })).toBeVisible();

    // Unblock domain
    await page.locator(".card").filter({ hasText: "evil.example" }).locator(".btn-outline").click();
    await expect(page.locator(".post-author-name").filter({ hasText: "evil.example" })).not.toBeVisible();
  });

  test("like → unlike → reblog → unreblog", async ({ page }) => {
    await login(page);

    await page.fill("textarea", "Interaction test post");
    await page.locator('.compose button[type="submit"]').click();
    await expect(page.locator(".post-content").first()).toContainText("Interaction test post");

    const postCard = page.locator(".card").nth(1);
    const stats = postCard.locator(".post-stats .post-stat");
    const likeBtn = stats.nth(2);
    const reblogBtn = stats.nth(1);

    // Like
    await expect(likeBtn).not.toHaveClass(/\bactive\b/);
    await likeBtn.click();
    await expect(likeBtn).toHaveClass(/\bactive\b/);

    // Unlike
    await likeBtn.click();
    await expect(likeBtn).not.toHaveClass(/\bactive\b/);

    // Reblog
    await expect(reblogBtn).not.toHaveClass(/\bactive\b/);
    await reblogBtn.click();
    await expect(reblogBtn).toHaveClass(/\bactive\b/);

    // Unreblog
    await reblogBtn.click();
    await expect(reblogBtn).not.toHaveClass(/\bactive\b/);

    // Cleanup
    await postCard.locator(".dot-menu-btn").click();
    await postCard.getByRole("link", { name: /delete|削除/i }).click();
    await expect(page.locator(".post-content")).toHaveCount(0);
  });

  test("settings page — edit display_name", async ({ page }) => {
    await login(page);

    await page.evaluate(() => { window.location.href = "/my/settings"; });
    await page.waitForURL("**/my/settings");
    await expect(page.locator('input[type="text"]').first()).toBeVisible({ timeout: 10000 });

    // Change display_name
    const nameInput = page.locator('input[type="text"]').first();
    await nameInput.fill("Alice Updated");
    await page.locator('button[type="submit"]').first().click();

    // Verify saved message appears
    await expect(page.getByText(/saved|保存しました/i).first()).toBeVisible({ timeout: 5000 });

    // Restore original name
    await nameInput.fill("Alice");
    await page.locator('button[type="submit"]').first().click();
    await expect(page.getByText(/saved|保存しました/i).first()).toBeVisible({ timeout: 5000 });
  });

  test("follow page — tab switch", async ({ page }) => {
    await login(page);

    await page.evaluate(() => { window.location.href = "/my/follow"; });
    await page.waitForURL("**/my/follow");
    await expect(page.locator(".tab").first()).toBeVisible({ timeout: 10000 });

    // Following tab should be active by default
    await expect(page.locator(".tab.active")).toContainText(/following|フォロー/i);

    // Switch to followers tab
    await page.locator(".tab").filter({ hasText: /followers|フォロワー/i }).click();
    await expect(page.locator(".tab.active")).toContainText(/followers|フォロワー/i);
  });

  test("notifications page — loads", async ({ page }) => {
    await login(page);

    await page.evaluate(() => { window.location.href = "/my/notifications"; });
    await page.waitForURL("**/my/notifications");
    // Page should load without error — check for empty state or notification list.
    // エラーなく読み込まれること。
    await expect(page.locator(".screen")).toBeVisible({ timeout: 10000 });
  });

  test("public home — renders profile", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator(".profile-body")).toBeVisible({ timeout: 10000 });
  });

  test("remote profile — /users/@user@host shows remote profile", async ({ page }) => {
    await login(page);
    await page.goto("/users/@bob@remote.example");
    // Remote actor lookup may fail (no real server), so check for acct display or error state.
    // リモート Actor の解決は失敗しうるので、acct 表示またはエラー状態を確認。
    await expect(page.getByText("@bob@remote.example")).toBeVisible({ timeout: 10000 });
  });

  test("local profile — /users/alice shows local profile", async ({ page }) => {
    await page.goto("/users/alice");
    await expect(page.locator(".handle")).toBeVisible();
    await expect(page.locator(".handle")).not.toContainText("@bob@");
  });

  test("logout redirects to login", async ({ page }) => {
    await login(page);

    // Logout via header dot-menu
    await page.locator(".header .dot-menu-btn").click();
    await page.getByRole("link", { name: /logout|ログアウト/i }).click();
    await expect(page.locator("h1")).toContainText("murlog");
  });
});
