import { test, expect } from "@playwright/test";
import { setupServer, rpcLogin, rpcCall, createTestImageBuffer } from "./helpers";

test.describe.serial("Media upload & management", () => {
  let cookie = "";

  test("setup server", async ({ request }) => {
    await setupServer(request);
    cookie = await rpcLogin(request);
  });

  test("upload media via REST endpoint", async ({ request }) => {
    const png = createTestImageBuffer();

    const res = await request.post("/api/mur/v1/media", {
      headers: { Cookie: cookie },
      multipart: {
        file: { name: "test.png", mimeType: "image/png", buffer: png },
        alt: "test image",
      },
    });

    expect(res.status()).toBe(201);
    const body = await res.json();
    expect(body.id).toBeTruthy();
    expect(body.url).toBeTruthy();
    expect(body.path).toContain("attachments/");
    expect(body.mime_type).toBe("image/png");
    expect(body.alt).toBe("test image");

    // Cleanup.
    await rpcCall(request, cookie, "media.delete", { id: body.id });
  });

  test("create post with attachment", async ({ request }) => {
    const png = createTestImageBuffer();

    // Upload media.
    // メディアをアップロード。
    const uploadRes = await request.post("/api/mur/v1/media", {
      headers: { Cookie: cookie },
      multipart: {
        file: { name: "attach.png", mimeType: "image/png", buffer: png },
        alt: "attachment",
      },
    });
    const media = await uploadRes.json();

    // Create post with attachment.
    // 添付ファイル付きで投稿を作成。
    const postRes = await rpcCall(request, cookie, "posts.create", {
      content: "<p>Post with image</p>",
      attachments: [media.id],
    });
    expect(postRes.result).toBeTruthy();
    expect(postRes.result.attachments).toHaveLength(1);
    expect(postRes.result.attachments[0].id).toBe(media.id);
    expect(postRes.result.attachments[0].alt).toBe("attachment");

    // Cleanup.
    await rpcCall(request, cookie, "posts.delete", { id: postRes.result.id });
  });

  test("delete media", async ({ request }) => {
    const png = createTestImageBuffer();

    const uploadRes = await request.post("/api/mur/v1/media", {
      headers: { Cookie: cookie },
      multipart: {
        file: { name: "delete-me.png", mimeType: "image/png", buffer: png },
      },
    });
    const media = await uploadRes.json();
    expect(media.id).toBeTruthy();

    // Delete the media.
    // メディアを削除。
    const deleteRes = await rpcCall(request, cookie, "media.delete", { id: media.id });
    expect(deleteRes.result?.status).toBe("ok");
  });
});
