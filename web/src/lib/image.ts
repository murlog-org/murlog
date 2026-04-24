// Image conversion utilities for media upload.
// メディアアップロード用の画像変換ユーティリティ。

// HEIC/HEIF MIME types. / HEIC/HEIF の MIME タイプ。
const heicTypes = new Set(["image/heic", "image/heif"]);

// convertIfNeeded converts HEIC/HEIF files to JPEG using the browser's native
// image decoder (Canvas API). Other formats pass through unchanged.
// ブラウザのネイティブ画像デコーダ (Canvas API) で HEIC/HEIF を JPEG に変換する。
// 他のフォーマットはそのまま返す。
export async function convertIfNeeded(file: File): Promise<File> {
  if (!heicTypes.has(file.type)) {
    return file;
  }

  // Use createImageBitmap to decode via browser/OS native codec.
  // createImageBitmap でブラウザ/OS のネイティブコーデック経由でデコードする。
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    throw new Error("HEIC decoding not supported in this browser");
  }

  const canvas = document.createElement("canvas");
  canvas.width = bitmap.width;
  canvas.height = bitmap.height;
  const ctx = canvas.getContext("2d")!;
  ctx.drawImage(bitmap, 0, 0);
  bitmap.close();

  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("canvas export failed"))),
      "image/jpeg",
      0.92
    );
  });

  const name = file.name.replace(/\.hei[cf]$/i, ".jpg");
  return new File([blob], name, { type: "image/jpeg" });
}
