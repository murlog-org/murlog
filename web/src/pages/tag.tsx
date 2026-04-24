// Tag page — displays posts with a specific hashtag using Handlebars theme.
// タグページ — Handlebars テーマで特定のハッシュタグを持つ投稿を表示する。

import { useEffect } from "preact/hooks";
import { Loading } from "../components/loading";
import { PAGE_SIZE, usePublicTimeline } from "../hooks/use-public-timeline";
import { call } from "../lib/api";
import { buildPersonaContext, mapPostsForTemplate } from "../lib/public-data";
import { getSSRData, loadTheme } from "../lib/theme";

type Props = {
	path?: string;
	tag?: string;
};

type SSRTagData = {
	tag: string;
	posts: Record<string, unknown>[];
};

export function TagPage({ tag }: Props) {
	const timeline = usePublicTimeline({
		templateName: "tag",
		fetchMore: async (cursor) => {
			const res = await call<Record<string, unknown>[]>("posts.list_by_tag", {
				tag,
				cursor,
				limit: PAGE_SIZE,
			});
			return res.result ?? [];
		},
		buildAppendData: (posts, ctx) => ({
			posts: mapPostsForTemplate(posts, ctx),
		}),
	});

	useEffect(() => {
		if (!tag) return;

		(async () => {
			// 1. Get data: SSR prefetch or API fallback.
			let posts: Record<string, unknown>[] = [];
			const ssrData = getSSRData<SSRTagData>();
			if (ssrData?.posts) {
				posts = ssrData.posts;
			}

			// Fetch persona + theme (+ posts if no SSR data) in parallel.
			// ペルソナ + テーマ (+ SSR データなければ投稿) を並行取得。
			const fetches: [Promise<unknown>, Promise<unknown>, Promise<unknown>?] = [
				call<Record<string, unknown>[]>("personas.list"),
				loadTheme("default"),
			];
			if (posts.length === 0) {
				fetches.push(
					call<Record<string, unknown>[]>("posts.list_by_tag", {
						tag,
						limit: PAGE_SIZE,
					}),
				);
			}
			const results = await Promise.all(fetches);
			const personasRes = results[0] as { result?: Record<string, unknown>[] };
			const theme = results[1] as Awaited<ReturnType<typeof loadTheme>>;
			if (posts.length === 0 && results[2]) {
				posts =
					(results[2] as { result?: Record<string, unknown>[] }).result ?? [];
			}

			let ctx = buildPersonaContext({}, location.hostname, location.origin);
			if (personasRes.result && personasRes.result.length > 0) {
				const persona =
					personasRes.result.find((p) => p.primary) || personasRes.result[0];
				ctx = buildPersonaContext(persona, location.hostname, location.origin);
			}

			timeline.init({ theme, ctx, posts });

			timeline.renderPage(theme, {
				tag,
				posts: mapPostsForTemplate(posts, ctx),
			});
		})();
	}, [tag]);

	return (
		<>
			<div ref={timeline.ref} />
			{timeline.loading && <Loading />}
			<div ref={timeline.sentinelRef} />
		</>
	);
}
