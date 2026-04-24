// Internal post permalink page — view any post stored locally with thread context and reactions.
// 内部投稿パーマリンクページ — スレッドコンテキスト+リアクション付きでローカル保存済みの投稿を表示。

import { useCallback, useEffect, useState } from "preact/hooks";
import { ComposeBox } from "../components/compose-box";
import { ErrorRetry } from "../components/error-retry";
import { Loading } from "../components/loading";
import type { Post } from "../components/post-card";
import { PostCard } from "../components/post-card";
import { useAsyncLoad } from "../hooks/use-async-load";
import { call, callOrThrow } from "../lib/api";
import { load as loadI18n } from "../lib/i18n";

type Props = {
	path?: string;
	id?: string;
};

type ThreadData = {
	ancestors: Post[];
	post: Post;
	descendants: Post[];
};

export function MyPost({ id: postId }: Props) {
	const [thread, setThread] = useState<ThreadData | null>(null);
	const [replyTo, setReplyTo] = useState<Post | null>(null);
	const [expandedPosts, setExpandedPosts] = useState<Set<string>>(new Set());
	const loader = useAsyncLoad();

	const loadPost = useCallback(() => {
		if (!postId) return;
		loader.run(async () => {
			await loadI18n();
			const t = await callOrThrow<ThreadData>("posts.get_thread", {
				id: postId,
			});
			setThread(t);
		});
	}, [postId, loader.run]);

	useEffect(() => {
		loadPost();
	}, [loadPost]);

	const updatePost = (id: string, updater: (p: Post) => Post) => {
		setThread((prev) => {
			if (!prev) return prev;
			if (prev.post.id === id) return { ...prev, post: updater(prev.post) };
			return {
				...prev,
				ancestors: prev.ancestors.map((p) => (p.id === id ? updater(p) : p)),
				descendants: prev.descendants.map((p) =>
					p.id === id ? updater(p) : p,
				),
			};
		});
	};

	const handleFavourite = async (p: Post) => {
		const method = p.favourited ? "favourites.delete" : "favourites.create";
		const { error } = await call(method, { post_id: p.id });
		if (error) return;
		updatePost(p.id, (prev) => ({
			...prev,
			favourited: !prev.favourited,
			favourites_count: prev.favourites_count + (prev.favourited ? -1 : 1),
		}));
	};

	const handleReblog = async (p: Post) => {
		const method = p.reblogged ? "reblogs.delete" : "reblogs.create";
		const { error } = await call(method, { post_id: p.id });
		if (error) return;
		updatePost(p.id, (prev) => ({
			...prev,
			reblogged: !prev.reblogged,
			reblogs_count: prev.reblogs_count + (prev.reblogged ? -1 : 1),
		}));
	};

	const handleReply = (p: Post) => {
		setReplyTo(p);
		window.scrollTo({ top: 0, behavior: "smooth" });
	};

	const actions = {
		onFavourite: handleFavourite,
		onReblog: handleReblog,
		onReply: handleReply,
	};

	const toggleExpand = (id: string) =>
		setExpandedPosts((prev) => {
			const next = new Set(prev);
			if (next.has(id)) next.delete(id);
			else next.add(id);
			return next;
		});

	if (!loader.ready) return <Loading />;

	return (
		<div class="screen">
			{loader.error && <ErrorRetry onRetry={loader.retry} />}

			{/* Compose (shown when replying) / 返信時に表示される投稿フォーム */}
			{replyTo && (
				<ComposeBox
					replyTo={replyTo}
					onClearReply={() => setReplyTo(null)}
					onPosted={() => {
						setReplyTo(null);
						loadPost();
					}}
				/>
			)}

			{thread && (
				<>
					{/* Ancestors / 祖先 */}
					{thread.ancestors.map((p) => (
						<a key={p.id} href={`/my/posts/${p.id}`} class="thread-link">
							<PostCard
								post={p}
								actions={actions}
								expanded={expandedPosts.has(p.id)}
								onToggleExpand={toggleExpand}
								compact
							/>
						</a>
					))}

					{/* Main post / メイン投稿 */}
					<PostCard
						post={thread.post}
						actions={actions}
						expanded={expandedPosts.has(thread.post.id)}
						onToggleExpand={toggleExpand}
					/>

					{/* Descendants / 子孫 */}
					{thread.descendants.map((p) => (
						<a key={p.id} href={`/my/posts/${p.id}`} class="thread-link">
							<PostCard
								post={p}
								actions={actions}
								expanded={expandedPosts.has(p.id)}
								onToggleExpand={toggleExpand}
								compact
							/>
						</a>
					))}
				</>
			)}
		</div>
	);
}
