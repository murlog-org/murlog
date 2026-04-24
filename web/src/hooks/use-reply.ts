// useReply — shared reply state management for my pages.
// my ページ共通の返信状態管理フック。

import { useCallback, useState } from "preact/hooks";
import type { Post } from "../components/post-card";

export function useReply() {
	const [replyTo, setReplyTo] = useState<Post | null>(null);

	const handleReply = useCallback((post: Post) => {
		setReplyTo(post);
		window.scrollTo({ top: 0, behavior: "smooth" });
	}, []);

	const clearReply = useCallback(() => {
		setReplyTo(null);
	}, []);

	return { replyTo, handleReply, clearReply };
}
