// Shared hook for public timeline pages with infinite scroll + theme rendering.
// 無限スクロール + テーマ描画付き公開タイムラインページの共通フック。

import { useRef, useState, useCallback } from "preact/hooks";
import { loadTheme, renderTemplate, activatePublic } from "../lib/theme";
import { type PersonaContext, mapPostsForTemplate } from "../lib/public-data";
import { useInfiniteScroll } from "./use-infinite-scroll";

type R = Record<string, unknown>;
type ThemeCache = Awaited<ReturnType<typeof loadTheme>>;

const PAGE_SIZE = 20;

type TimelineConfig = {
  // Full-page template name (e.g. "profile", "tag").
  // ページ全体のテンプレート名。
  templateName: string;
  // Template name for appending more posts (default: "post-card").
  // 追加投稿用テンプレート名 (デフォルト: "post-card")。
  appendTemplateName?: string;
  // Fetch the next page of posts given a cursor. Return the raw post records.
  // カーソルを受け取り次ページの投稿を取得する関数。
  fetchMore: (cursor: string) => Promise<R[]>;
  // Build the template data for appending more posts.
  // 追加投稿のテンプレートデータを構築する関数。
  buildAppendData: (posts: R[], ctx: PersonaContext) => R;
  // Extract HTML to append from the rendered template (default: use as-is).
  // 描画済みテンプレートから追記する HTML を抽出する関数 (デフォルト: そのまま使用)。
  extractAppendHTML?: (rendered: string) => string;
};

// InitResult is returned by the page's initial load to wire up the hook.
// ページ初期ロードからフックに渡す初期化結果。
export type InitResult = {
  theme: ThemeCache;
  ctx: PersonaContext;
  posts: R[];
};

export function usePublicTimeline(config: TimelineConfig) {
  const ref = useRef<HTMLDivElement>(null);
  const postsRef = useRef<HTMLDivElement>(null);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const lastPostIdRef = useRef<string | null>(null);
  const themeRef = useRef<ThemeCache | null>(null);
  const ctxRef = useRef<PersonaContext | null>(null);

  // Called by the page after initial data is fetched and rendered.
  // ページが初期データ取得・描画後に呼ぶ。
  const init = useCallback((result: InitResult) => {
    themeRef.current = result.theme;
    ctxRef.current = result.ctx;
    const posts = result.posts;
    setHasMore(posts.length >= PAGE_SIZE);
    if (posts.length > 0) {
      lastPostIdRef.current = posts[posts.length - 1].id as string;
    }
  }, []);

  // Render the full-page template and swap into the DOM.
  // ページ全体テンプレートを描画して DOM に差し替え。
  const renderPage = useCallback((theme: ThemeCache, templateData: R) => {
    const html = renderTemplate(theme, config.templateName, templateData);
    if (ref.current && html) {
      ref.current.innerHTML = html;
      activatePublic();
      postsRef.current = ref.current.querySelector(".posts") as HTMLDivElement;
    }
  }, [config.templateName]);

  // Load more posts on scroll.
  // スクロール時に投稿を追加読み込み。
  const loadMore = useCallback(async () => {
    if (!themeRef.current || !ctxRef.current || !lastPostIdRef.current) return;
    setLoading(true);
    try {
      const posts = await config.fetchMore(lastPostIdRef.current);
      setHasMore(posts.length >= PAGE_SIZE);
      if (posts.length > 0) {
        lastPostIdRef.current = posts[posts.length - 1].id as string;
        const data = config.buildAppendData(posts, ctxRef.current);
        if (data && postsRef.current) {
          const appendTmpl = config.appendTemplateName || "post-card";
          let html = renderTemplate(themeRef.current, appendTmpl, data);
          if (config.extractAppendHTML) {
            html = config.extractAppendHTML(html);
          }
          postsRef.current.insertAdjacentHTML("beforeend", html);
        }
      }
    } finally {
      setLoading(false);
    }
  }, [config]);

  const sentinelRef = useInfiniteScroll({
    hasMore,
    loading,
    onLoadMore: loadMore,
  });

  return { ref, sentinelRef, loading, hasMore, init, renderPage, mapPostsForTemplate, PAGE_SIZE };
}

export { PAGE_SIZE };
