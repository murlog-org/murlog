// Theme loader — fetch, cache, and render Handlebars templates.
// テーマローダー — Handlebars テンプレートの取得・キャッシュ・レンダリング。

import Handlebars from "handlebars";
import { load as loadI18n, t as i18nT } from "./i18n";
import { formatTime } from "./format";
import { callBatch } from "./api";

// --- Types ---

export type ThemeConfig = {
  name: string;
  version: string;
};

type ThemeCache = {
  config: ThemeConfig;
  basePath: string;
  templates: Record<string, HandlebarsTemplateDelegate>;
};

// --- Cache ---

const cache: Record<string, ThemeCache> = {};

// --- Handlebars helpers ---

Handlebars.registerHelper("isoDate", (date: string) => {
  return new Date(date).toISOString();
});

Handlebars.registerHelper("formatDate", (date: string) => {
  return formatTime(date);
});

// autoLinkText wraps a URL string in an <a> tag, or escapes plain text.
// URL 文字列を <a> タグでラップするか、プレーンテキストをエスケープする。
export function autoLinkText(text: string): string {
  if (/^https?:\/\/\S+$/.test(text)) {
    const escaped = Handlebars.Utils.escapeExpression(text);
    return `<a href="${escaped}" rel="nofollow noopener" target="_blank">${escaped}</a>`;
  }
  return Handlebars.Utils.escapeExpression(text);
}

Handlebars.registerHelper("autoLink", (text: string) => {
  return new Handlebars.SafeString(autoLinkText(text));
});

// Handlebars i18n helper — delegates to shared i18n module.
// Handlebars i18n ヘルパー — 共通 i18n モジュールに委譲。
Handlebars.registerHelper("t", (key: string) => {
  return i18nT(key);
});

// mediaGridClass returns "post-media post-media-N" for attachment grids.
// 添付画像グリッド用に "post-media post-media-N" を返す。
Handlebars.registerHelper("mediaGridClass", (attachments: unknown[]) => {
  return `post-media post-media-${Math.min(attachments?.length ?? 0, 4)}`;
});

// --- Public API ---

// loadTheme fetches theme config + templates and caches them.
// テーマ設定+テンプレートを取得・キャッシュする。
export async function loadTheme(name: string): Promise<ThemeCache> {
  if (cache[name]) return cache[name];

  const basePath = `/themes/${name}`;
  const qs = typeof __BUILD_HASH__ !== "undefined" ? `?v=${__BUILD_HASH__}` : "";

  // Fetch translations + theme files in parallel.
  // 翻訳 + テーマファイルを並行で取得。
  const [, configRes, postArticleRes, homeRes, profileRes, postRes, postCardRes, tagRes] = await Promise.all([
    loadI18n(),
    fetch(`${basePath}/theme.json${qs}`),
    fetch(`${basePath}/templates/post-article.hbs${qs}`),
    fetch(`${basePath}/templates/home.hbs${qs}`),
    fetch(`${basePath}/templates/profile.hbs${qs}`),
    fetch(`${basePath}/templates/post.hbs${qs}`),
    fetch(`${basePath}/templates/post-card.hbs${qs}`),
    fetch(`${basePath}/templates/tag.hbs${qs}`),
  ] as const);

  const config: ThemeConfig = await configRes.json();
  const postArticleSrc = await postArticleRes.text();
  const homeSrc = await homeRes.text();
  const profileSrc = await profileRes.text();
  const postSrc = await postRes.text();
  const postCardSrc = await postCardRes.text();
  const tagSrc = await tagRes.text();

  // Register partials before compiling templates.
  // テンプレートコンパイル前にパーシャルを登録。
  Handlebars.registerPartial("post-article", postArticleSrc);

  // Compile templates.
  const templates: Record<string, HandlebarsTemplateDelegate> = {
    home: Handlebars.compile(homeSrc),
    profile: Handlebars.compile(profileSrc),
    post: Handlebars.compile(postSrc),
    "post-card": Handlebars.compile(postCardSrc),
    tag: Handlebars.compile(tagSrc),
  };

  // Load theme CSS and wait for it before returning.
  // テーマ CSS をロードし、読み込み完了を待ってから返す。
  const linkId = `theme-css-${name}`;
  if (!document.getElementById(linkId)) {
    await new Promise<void>((resolve) => {
      const link = document.createElement("link");
      link.id = linkId;
      link.rel = "stylesheet";
      link.href = `${basePath}/style.css${qs}`;
      link.onload = () => resolve();
      link.onerror = () => resolve();
      document.head.appendChild(link);
    });
  }

  const entry: ThemeCache = { config, basePath, templates };
  cache[name] = entry;
  return entry;
}

// renderTemplate renders a cached template with the given data.
// `themeBase` (e.g. "/themes/default") is auto-injected for asset references.
// キャッシュ済みテンプレートを指定データでレンダリングする。
// テーマアセット参照用の `themeBase` を自動注入する。
export function renderTemplate(
  theme: ThemeCache,
  templateName: string,
  data: Record<string, unknown>
): string {
  const tmpl = theme.templates[templateName];
  if (!tmpl) return "";
  return tmpl({ ...data, themeBase: theme.basePath });
}

// getSSRData reads prefetched JSON from <script id="ssr-data">.
// Removes the element after reading to avoid stale data on SPA navigation.
// <script id="ssr-data"> からプリフェッチ済み JSON を取得し、読後に削除する。
export function getSSRData<T>(): T | null {
  const el = document.getElementById("ssr-data");
  if (!el?.textContent) return null;
  try {
    const data = JSON.parse(el.textContent) as T;
    el.remove();
    return data;
  } catch {
    return null;
  }
}

// activatePublic switches body class to "public" and removes SSR content.
// Called by public page components after theme rendering completes.
// body クラスを "public" に切り替え、SSR コンテンツを削除する。
// テーマ描画完了後に公開ページコンポーネントから呼ぶ。
export function activatePublic(): void {
  document.body.classList.remove("ssr", "spa", "public");
  document.body.classList.add("public");
  const app = document.getElementById("app");
  if (!app) return;
  for (const el of Array.from(app.querySelectorAll(".ssr-content"))) {
    el.remove();
  }
  injectLinkPreviews(app);
}

// injectLinkPreviews finds links in post-content and appends OGP preview cards.
// post-content 内のリンクを検出して OGP プレビューカードを挿入する。
const ogpCache = new Map<string, { title?: string; description?: string; image?: string; site_name?: string } | null>();

function injectLinkPreviews(root: HTMLElement): void {
  const posts = root.querySelectorAll(".post-content");
  const origin = location.origin;
  const toFetch: { postEl: HTMLElement; url: string }[] = [];

  for (const post of Array.from(posts)) {
    if (post.nextElementSibling?.classList.contains("post-media")) continue;

    // Find first external link, or fallback to bare URL in text.
    // 最初の外部リンクを検索、なければテキスト内の生 URL にフォールバック。
    const links = post.querySelectorAll("a[href^='https://']");
    let url = "";
    for (const l of Array.from(links) as HTMLAnchorElement[]) {
      if (!l.href.startsWith(origin + "/")) { url = l.href; break; }
    }
    if (!url) {
      const bareMatch = (post.textContent || "").match(/https:\/\/[^\s]+/);
      if (bareMatch && !bareMatch[0].startsWith(origin + "/")) url = bareMatch[0];
    }
    if (!url) continue;

    if (ogpCache.has(url)) {
      const cached = ogpCache.get(url);
      if (cached) insertPreviewCard(post as HTMLElement, url, cached);
      continue;
    }

    toFetch.push({ postEl: post as HTMLElement, url });
  }

  if (toFetch.length === 0) return;

  // Batch fetch all uncached URLs in a single RPC call.
  // キャッシュにない URL を1回のバッチ RPC でまとめて取得。
  const uniqueURLs = [...new Set(toFetch.map((f) => f.url))];
  const requests = uniqueURLs.map((url) => ({ method: "links.preview", params: { url } }));
  callBatch(requests).then((results) => {
    const urlMap = new Map<string, { title?: string; description?: string; image?: string; site_name?: string }>();
    for (let i = 0; i < uniqueURLs.length; i++) {
      const res = results[i];
      const ogp = res?.result as { title?: string; description?: string; image?: string; site_name?: string } | undefined;
      if (ogp && (ogp.title || ogp.description)) {
        ogpCache.set(uniqueURLs[i], ogp);
        urlMap.set(uniqueURLs[i], ogp);
      } else {
        ogpCache.set(uniqueURLs[i], null);
      }
    }
    for (const { postEl, url } of toFetch) {
      const ogp = urlMap.get(url);
      if (ogp) insertPreviewCard(postEl, url, ogp);
    }
  });
}

function insertPreviewCard(postEl: HTMLElement, url: string, ogp: { title?: string; description?: string; image?: string; site_name?: string }): void {
  const card = document.createElement("a");
  card.href = url;
  card.className = "link-preview";
  card.target = "_blank";
  card.rel = "noopener noreferrer";

  let html = "";
  if (ogp.image) {
    html += `<img class="link-preview-image" src="${escapeAttr(ogp.image)}" alt="" loading="lazy" />`;
  }
  html += `<div class="link-preview-body">`;
  if (ogp.title) html += `<div class="link-preview-title">${escapeHTML(ogp.title)}</div>`;
  if (ogp.description) html += `<div class="link-preview-desc">${escapeHTML(ogp.description)}</div>`;
  if (ogp.site_name) html += `<div class="link-preview-site">${escapeHTML(ogp.site_name)}</div>`;
  html += `</div>`;

  card.innerHTML = html;
  postEl.insertAdjacentElement("afterend", card);
}

function escapeHTML(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function escapeAttr(s: string): string {
  return escapeHTML(s).replace(/"/g, "&quot;");
}
