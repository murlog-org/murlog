// HTML sanitization for untrusted content (remote posts, actor summaries).
// 信頼できないコンテンツ（リモート投稿、Actor サマリー）の HTML サニタイズ。

import DOMPurify from "dompurify";

// sanitize cleans HTML, allowing only safe tags and attributes.
// 安全なタグと属性のみ許可して HTML をクリーンにする。
export function sanitize(html: string): string {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      "p", "br", "a", "span", "em", "strong", "b", "i", "u",
      "ul", "ol", "li", "blockquote", "pre", "code",
    ],
    ALLOWED_ATTR: ["href", "rel", "class"],
  });
}
