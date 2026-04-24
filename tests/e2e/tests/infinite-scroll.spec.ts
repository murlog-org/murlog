import { test, expect } from "@playwright/test";
import { setupServer, login, rpcLogin } from "./helpers";

test.describe.serial("Infinite scroll", () => {
  let cookie = "";

  test("setup and create 25 posts", async ({ request }) => {
    await setupServer(request);
    cookie = await rpcLogin(request);

    // Create 25 posts via RPC (page size is 20, so we need > 20).
    // RPC で 25 件投稿作成 (ページサイズ 20 なので 20 件超必要)。
    for (let i = 1; i <= 25; i++) {
      await request.post("/api/mur/v1/rpc", {
        headers: { Cookie: cookie },
        data: {
          jsonrpc: "2.0", id: i + 1,
          method: "posts.create",
          params: { content: `<p>Post number ${i}</p>` },
        },
      });
    }
  });

  test("my page — loads more posts on scroll", async ({ page }) => {
    await login(page);

    // Initial load should show 20 posts (default page size).
    // 初回ロードで 20 件表示されること。
    const posts = page.locator(".post-content");
    await expect(posts).toHaveCount(20, { timeout: 10000 });

    // Scroll to bottom — sentinel triggers next page load.
    // 下にスクロール — センチネルが次ページの読み込みをトリガー。
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await expect(posts).toHaveCount(25, { timeout: 10000 });
  });

  test("cleanup — delete all posts", async ({ request }) => {
    const listRes = await request.post("/api/mur/v1/rpc", {
      headers: { Cookie: cookie },
      data: { jsonrpc: "2.0", id: 1, method: "timeline.home", params: { limit: 100 } },
    });
    const body = await listRes.json();
    const posts = body.result || [];
    for (const p of posts) {
      await request.post("/api/mur/v1/rpc", {
        headers: { Cookie: cookie },
        data: { jsonrpc: "2.0", id: 1, method: "posts.delete", params: { id: p.id } },
      });
    }
  });
});
