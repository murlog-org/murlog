// Public home page — renders all personas using Handlebars theme.
// 公開ホームページ — Handlebars テーマで全ペルソナをレンダリング。

import { useEffect, useRef } from "preact/hooks";
import { loadTheme, renderTemplate, getSSRData, activatePublic } from "../lib/theme";
import { call } from "../lib/api";
import { buildPersonaContext, buildPersonaTemplateData } from "../lib/public-data";

type Props = {
  path?: string;
};

type PersonaWithPosts = Record<string, unknown> & {
  posts: Record<string, unknown>[];
};

type SSRHomeData = {
  personas: PersonaWithPosts[];
};

export function PublicHome(_props: Props) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    (async () => {
      // 1. Get data: SSR prefetch or API fallback.
      let data = getSSRData<SSRHomeData>();
      if (!data) {
        const res = await call<Record<string, unknown>[]>("personas.list");
        if (res.result) {
          // Fetch posts for each persona.
          const personas: PersonaWithPosts[] = [];
          for (const p of res.result) {
            const postsRes = await call<Record<string, unknown>[]>("posts.list", {
              persona_id: p.id,
              public_only: true,
            });
            personas.push({ ...p, posts: postsRes.result ?? [] });
          }
          data = { personas };
        }
      }
      if (!data) return;

      // 2. Load theme.
      const theme = await loadTheme("default");

      // 3. Build template context.
      const domain = location.hostname;
      const baseURL = location.origin;

      const templateData = {
        domain,
        personas: data.personas.map((p) => {
          const ctx = buildPersonaContext(p, domain, baseURL);
          return buildPersonaTemplateData(p, ctx, domain, p.posts || []);
        }),
      };

      // 4. Render and swap.
      const html = renderTemplate(theme, "home", templateData);
      if (ref.current && html) {
        activatePublic();
        ref.current.innerHTML = html;
      }
    })();
  }, []);

  return <div ref={ref} />;
}
