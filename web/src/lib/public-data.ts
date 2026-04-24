// Shared helpers for building Handlebars template data on public pages (home, profile, tag).
// 公開ページ (ホーム・プロフィール・タグ) の Handlebars テンプレートデータ構築共通ヘルパー。

type R = Record<string, unknown>;

// PersonaContext holds the resolved display fields for a local persona.
// ローカルペルソナの表示用フィールド。
export type PersonaContext = {
  displayName: string;
  avatarURL: string;
  profileURL: string;
  handle: string;
};

// buildPersonaContext extracts display fields from a persona record.
// ペルソナレコードから表示用フィールドを抽出する。
export function buildPersonaContext(persona: R, domain: string, baseURL: string): PersonaContext {
  return {
    displayName: (persona.display_name as string) || (persona.username as string) || "",
    avatarURL: (persona.avatar_url as string) || "",
    profileURL: `${baseURL}/users/${persona.username}`,
    handle: `@${persona.username}@${domain}`,
  };
}

// buildPersonaTemplateData builds the full template data for a persona section.
// Combines raw API fields, resolved context, and mapped posts.
// ペルソナセクションのテンプレートデータをまとめて構築する。
// API の生フィールド、解決済みコンテキスト、マッピング済み投稿を結合。
export function buildPersonaTemplateData(persona: R, ctx: PersonaContext, domain: string, posts: R[]): R {
  return {
    ...persona,
    ...ctx,
    domain,
    headerURL: (persona.header_url as string) || "",
    postCount: persona.post_count ?? 0,
    followingCount: persona.following_count ?? 0,
    followersCount: persona.followers_count ?? 0,
    posts: mapPostsForTemplate(posts, ctx),
  };
}

// mapPostsForTemplate maps posts to Handlebars template data.
// Unwraps reblog wrappers and resolves author info (local persona or remote actor).
// 投稿を Handlebars テンプレートデータにマッピングする。
// リブログ wrapper を展開し、著者情報を解決する (ローカルペルソナ or リモート Actor)。
export function mapPostsForTemplate(posts: R[], ctx: PersonaContext): R[] {
  return posts.map((p) => {
    const up = unwrapReblog(p, ctx.displayName);
    const useActorInfo = !!up.actor_uri;
    return {
      ...up,
      permalink: `${ctx.profileURL}/posts/${up.id}`,
      createdAt: up.created_at,
      avatarURL: useActorInfo ? (up.actor_avatar_url || ctx.avatarURL) : ctx.avatarURL,
      displayName: useActorInfo ? ((up.actor_name as string) || (up.actor_acct as string) || ctx.displayName) : ctx.displayName,
      handle: useActorInfo ? ((up.actor_acct as string) || ctx.handle) : ctx.handle,
      profileURL: useActorInfo ? (up.actor_uri || ctx.profileURL) : ctx.profileURL,
    };
  });
}

// unwrapReblog extracts the original post from a reblog wrapper.
// Returns the post unchanged if it's not a wrapper.
// リブログ wrapper から元投稿を取り出す。wrapper でなければそのまま返す。
function unwrapReblog(p: R, localDisplayName: string): R {
  const reblog = p.reblog as R | undefined;
  if (!reblog) return p;
  return {
    ...reblog,
    // Preserve the wrapper's id and created_at for cursor pagination.
    // カーソルページネーション用に wrapper の id と created_at を保持。
    id: p.id,
    created_at: p.created_at,
    reblogLabel: localDisplayName,
  };
}
