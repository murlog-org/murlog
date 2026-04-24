// Public profile page — renders persona + posts using Handlebars theme.
// 公開プロフィールページ — Handlebars テーマでペルソナ+投稿をレンダリング。

import { useEffect } from "preact/hooks";
import { loadTheme, getSSRData } from "../lib/theme";
import { call } from "../lib/api";
import { Loading } from "../components/loading";
import { buildPersonaContext, buildPersonaTemplateData, mapPostsForTemplate } from "../lib/public-data";
import { usePublicTimeline, PAGE_SIZE } from "../hooks/use-public-timeline";

type Props = {
  path?: string;
  username?: string;
};

type SSRProfileData = {
  persona: Record<string, unknown>;
  posts: Record<string, unknown>[];
};

export function PublicProfile({ username }: Props) {
  const timeline = usePublicTimeline({
    templateName: "profile",
    fetchMore: async (cursor) => {
      const res = await call<Record<string, unknown>[]>("posts.list", {
        username,
        cursor,
        limit: PAGE_SIZE,
        public_only: true,
      });
      return res.result ?? [];
    },
    buildAppendData: (posts, ctx) => ({
      posts: mapPostsForTemplate(posts, ctx),
    }),
  });

  useEffect(() => {
    if (!username) return;

    (async () => {
      // 1. Get data: SSR prefetch or API fallback.
      let data = getSSRData<SSRProfileData>();
      if (!data) {
        const pRes = await call<Record<string, unknown>>("personas.get", { username });
        const postsRes = await call<Record<string, unknown>[]>("posts.list", { username, limit: PAGE_SIZE, public_only: true });
        if (pRes.result) {
          data = { persona: pRes.result, posts: postsRes.result ?? [] };
        }
      }
      if (!data) return;

      // 2. Load theme.
      const theme = await loadTheme("default");
      const persona = data.persona;
      const ctx = buildPersonaContext(persona, location.hostname, location.origin);

      // 3. Wire up infinite scroll.
      timeline.init({ theme, ctx, posts: data.posts || [] });

      // 4. Render.
      timeline.renderPage(theme, buildPersonaTemplateData(persona, ctx, location.hostname, data.posts || []));
    })();
  }, [username]);

  return (
    <>
      <div ref={timeline.ref} />
      {timeline.loading && <Loading />}
      <div ref={timeline.sentinelRef} />
    </>
  );
}
