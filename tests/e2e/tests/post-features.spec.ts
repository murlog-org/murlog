import { test, expect } from "@playwright/test";
import { setupServer, login, rpcLogin, rpcCall } from "./helpers";

test.describe.serial("Post features — reply, thread, pin, CW, visibility", () => {
  let cookie = "";

  test("setup server", async ({ request }) => {
    await setupServer(request);
    cookie = await rpcLogin(request);
    // Clean up any leftover posts from previous test runs.
    // 前回テストの残留投稿をクリーンアップ。
    const list = await rpcCall(request, cookie, "timeline.home", { limit: 100 });
    for (const p of list.result || []) {
      await rpcCall(request, cookie, "posts.delete", { id: p.id });
    }
  });

  test("reply sets in_reply_to_uri and UI shows reply context", async ({ page, request }) => {
    // Create parent and reply via RPC.
    // RPC で親投稿と返信を作成。
    const parentRes = await rpcCall(request, cookie, "posts.create", {
      content: "<p>Parent post for thread</p>",
    });
    const parentId = parentRes.result.id;

    const replyRes = await rpcCall(request, cookie, "posts.create", {
      content: "<p>Reply to parent</p>",
      in_reply_to: parentId,
    });
    const replyId = replyRes.result.id;

    // in_reply_to_uri should be set (points to parent post URI).
    // in_reply_to_uri が設定されていること (親投稿の URI を指す)。
    expect(replyRes.result.in_reply_to_uri).toBeTruthy();
    expect(replyRes.result.in_reply_to_uri).toContain(parentId);

    // Verify thread traversal — ancestors should include the parent.
    // スレッド辿りの検証 — ancestors に親投稿が含まれること。
    const threadRes = await rpcCall(request, cookie, "posts.get_thread", { id: replyId });
    expect(threadRes.result.ancestors.length).toBeGreaterThanOrEqual(1);
    expect(threadRes.result.ancestors[0].content).toContain("Parent post for thread");
    expect(threadRes.result.post.content).toContain("Reply to parent");

    // Verify in_reply_to_post is enriched in timeline.
    // タイムラインで in_reply_to_post が展開されていること。
    const listRes = await rpcCall(request, cookie, "timeline.home", { limit: 5 });
    const reply = (listRes.result || []).find((p: any) => p.id === replyId);
    expect(reply).toBeTruthy();
    expect(reply.in_reply_to_post).toBeTruthy();
    expect(reply.in_reply_to_post.content).toContain("Parent post for thread");

    // Verify reply button shows reply-context in UI.
    // UI で返信ボタンが reply-context を表示することを確認。
    await login(page);
    const postCard = page.locator(".card").nth(1);
    const replyBtn = postCard.locator(".post-stat").first();
    await replyBtn.click();
    await expect(page.locator(".reply-context")).toBeVisible();

    // Cancel reply.
    // 返信をキャンセル。
    await page.locator(".reply-context button").click();
    await expect(page.locator(".reply-context")).not.toBeVisible();

    // Cleanup.
    await rpcCall(request, cookie, "posts.delete", { id: replyId });
    await rpcCall(request, cookie, "posts.delete", { id: parentId });
  });

  test("pin and unpin post", async ({ page, request }) => {
    await login(page);

    // Create a post.
    await page.fill("textarea", "Pin me please");
    await page.locator('.compose button[type="submit"]').click();
    await expect(page.locator(".post-content").first()).toContainText("Pin me please");

    // Open dot-menu and pin.
    // dot-menu を開いてピン留め。
    const postCard = page.locator(".card").nth(1);
    await postCard.locator(".dot-menu-btn").click();
    await postCard.getByRole("link", { name: /^pin$|^ピン留め$/i }).click();

    // Verify via RPC.
    // RPC でピン留め状態を確認。
    const listRes = await rpcCall(request, cookie, "timeline.home", { limit: 5 });
    const post = (listRes.result || []).find((p: any) => p.content?.includes("Pin me please"));
    expect(post).toBeTruthy();
    expect(post.pinned).toBe(true);

    // Unpin via dot-menu.
    // dot-menu でピン解除。
    await postCard.locator(".dot-menu-btn").click();
    await postCard.getByRole("link", { name: /unpin|ピン留め解除/i }).click();

    // Verify unpin via RPC.
    const listRes2 = await rpcCall(request, cookie, "timeline.home", { limit: 5 });
    const post2 = (listRes2.result || []).find((p: any) => p.content?.includes("Pin me please"));
    expect(post2.pinned).toBeFalsy();

    // Cleanup.
    await rpcCall(request, cookie, "posts.delete", { id: post2.id });
  });

  test("content warning — create and toggle", async ({ page, request }) => {
    await login(page);

    // Toggle CW mode.
    // CW モードをトグル。
    await page.locator(".compose-actions").getByText("CW").click();

    // CW input should appear.
    // CW 入力欄が表示されること。
    await expect(page.locator(".cw-input")).toBeVisible();

    await page.locator(".cw-input").fill("Spoiler warning");
    await page.fill("textarea", "Hidden content behind CW");
    await page.locator('.compose button[type="submit"]').click();

    // CW summary should be visible, content should be folded.
    // CW サマリが表示され、コンテンツが折りたたまれること。
    await expect(page.locator(".cw-summary").first()).toContainText("Spoiler warning");

    // Click "Show more" to reveal content.
    // "Show more" をクリックしてコンテンツを表示。
    await page.locator(".cw-summary .btn").first().click();
    await expect(page.locator(".post-content").first()).toContainText("Hidden content behind CW");

    // Click "Show less" to fold again.
    // "Show less" をクリックして再び折りたたむ。
    await page.locator(".cw-summary .btn").first().click();
    await expect(page.locator(".post-content").first()).not.toBeVisible();

    // Cleanup.
    const listRes = await rpcCall(request, cookie, "timeline.home", { limit: 5 });
    for (const p of listRes.result || []) {
      await rpcCall(request, cookie, "posts.delete", { id: p.id });
    }
  });

  test("visibility levels via compose select", async ({ page, request }) => {
    await login(page);

    // Select "unlisted" visibility and post.
    // "unlisted" 公開範囲を選択して投稿。
    await page.locator(".compose-actions select").selectOption("unlisted");
    await page.fill("textarea", "Unlisted post");
    await page.locator('.compose button[type="submit"]').click();
    await expect(page.locator(".post-content").first()).toContainText("Unlisted post");

    // Verify via RPC.
    const listRes = await rpcCall(request, cookie, "timeline.home", { limit: 5 });
    const unlisted = (listRes.result || []).find((p: any) => p.content?.includes("Unlisted post"));
    expect(unlisted).toBeTruthy();
    expect(unlisted.visibility).toBe("unlisted");

    // Select "followers" visibility and post.
    // "followers" 公開範囲を選択して投稿。
    await page.locator(".compose-actions select").selectOption("followers");
    await page.fill("textarea", "Followers-only post");
    await page.locator('.compose button[type="submit"]').click();
    await expect(page.locator(".post-content").first()).toContainText("Followers-only post");

    const listRes2 = await rpcCall(request, cookie, "timeline.home", { limit: 5 });
    const followersOnly = (listRes2.result || []).find((p: any) =>
      p.content?.includes("Followers-only post"),
    );
    expect(followersOnly).toBeTruthy();
    expect(followersOnly.visibility).toBe("followers");

    // Cleanup.
    for (const p of listRes2.result || []) {
      await rpcCall(request, cookie, "posts.delete", { id: p.id });
    }
  });
});
