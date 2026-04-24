// Public post permalink page — renders single post with thread context.
// 公開投稿パーマリンクページ — スレッドコンテキスト付きで投稿をレンダリング。

import { useState, useEffect, useRef } from "preact/hooks";
import { loadTheme, renderTemplate, getSSRData, activatePublic } from "../lib/theme";
import { call } from "../lib/api";
import { formatTime } from "../lib/format";
import { sanitize } from "../lib/sanitize";

type Props = {
  path?: string;
  username?: string;
  id?: string;
};

type PostData = {
  id: string;
  content: string;
  actor_uri?: string;
  actor_name?: string;
  origin: string;
  created_at: string;
  [key: string]: unknown;
};

type ThreadData = {
  ancestors: PostData[];
  post: PostData;
  descendants: PostData[];
};

type SSRPostData = {
  persona: Record<string, unknown>;
  post: Record<string, unknown>;
};

function ThreadPost({ post, highlight }: { post: PostData; highlight?: boolean }) {
  return (
    <div class={`card ${highlight ? "thread-target" : "thread-post"}`}>
      {post.actor_uri && (
        <div class="meta" style={{ marginBottom: 4 }}>{post.actor_name || post.actor_uri}</div>
      )}
      <div
        class="post-content"
        dangerouslySetInnerHTML={{ __html: sanitize(post.content) }}
      />
      <span class="meta">{formatTime(post.created_at)}</span>
    </div>
  );
}

export function PublicPost({ username, id: postId }: Props) {
  const mainRef = useRef<HTMLDivElement>(null);
  const [thread, setThread] = useState<ThreadData | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!username || !postId) return;

    (async () => {
      // Fetch thread data. / スレッドデータを取得。
      const threadRes = await call<ThreadData>("posts.get_thread", { id: postId });
      if (threadRes.result) {
        setThread(threadRes.result);
      }

      // Render main post with theme. / メイン投稿をテーマでレンダリング。
      let data = getSSRData<SSRPostData>();
      if (!data) {
        const pRes = await call<Record<string, unknown>>("personas.get", { username });
        const postRes = await call<Record<string, unknown>>("posts.get", { id: postId });
        if (pRes.result && postRes.result) {
          data = { persona: pRes.result, post: postRes.result };
        }
      }
      if (!data) {
        setLoaded(true);
        return;
      }

      const theme = await loadTheme("default");

      const persona = data.persona as Record<string, string>;
      const post = data.post as Record<string, unknown>;
      const domain = location.hostname;
      const baseURL = location.origin;
      const profileURL = `${baseURL}/users/${persona.username}`;

      const templateData = {
        ...post,
        username: persona.username,
        displayName: persona.display_name || persona.username,
        domain,
        avatarURL: (persona.avatar_url as string) || "",
        handle: `@${persona.username}@${domain}`,
        profileURL,
        permalink: `${profileURL}/posts/${post.id}`,
        createdAt: post.created_at,
      };

      const html = renderTemplate(theme, "post", templateData);
      if (mainRef.current && html) {
        activatePublic();
        mainRef.current.innerHTML = html;
      }
      setLoaded(true);
    })();
  }, [username, postId]);

  if (!loaded && !thread) return null;

  return (
    <div class="screen">
      {/* Ancestors / 祖先 */}
      {thread && thread.ancestors && thread.ancestors.length > 0 && (
        <div class="thread-ancestors">
          {thread.ancestors.map((p) => (
            <a key={p.id} href={`/users/${username}/posts/${p.id}`} class="thread-link">
              <ThreadPost post={p} />
            </a>
          ))}
        </div>
      )}

      {/* Main post (theme-rendered) / メイン投稿（テーマ描画） */}
      <div ref={mainRef} class="thread-target" />

      {/* Descendants / 子孫 */}
      {thread && thread.descendants && thread.descendants.length > 0 && (
        <div class="thread-descendants">
          {thread.descendants.map((p) => (
            <a key={p.id} href={`/users/${username}/posts/${p.id}`} class="thread-link">
              <ThreadPost post={p} />
            </a>
          ))}
        </div>
      )}
    </div>
  );
}
