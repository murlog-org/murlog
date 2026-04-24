// useInfiniteScroll — shared IntersectionObserver hook for infinite scroll pagination.
// 無限スクロールページネーション用の共通 IntersectionObserver フック。

import { useEffect, useRef } from "preact/hooks";

type Options = {
	hasMore: boolean;
	loading: boolean;
	onLoadMore: () => void;
	// Re-setup trigger — pass a ready/loaded flag so the observer is created
	// after the sentinel DOM element appears (e.g. after initial data load).
	// 再セットアップトリガー — sentinel DOM が現れた後に observer を作成するため
	// ready/loaded フラグを渡す。
	ready?: boolean;
};

export function useInfiniteScroll({
	hasMore,
	loading,
	onLoadMore,
	ready = true,
}: Options) {
	const sentinelRef = useRef<HTMLDivElement>(null);
	const hasMoreRef = useRef(hasMore);
	const loadingRef = useRef(loading);
	hasMoreRef.current = hasMore;
	loadingRef.current = loading;

	useEffect(() => {
		const el = sentinelRef.current;
		if (!el) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (
					entries[0].isIntersecting &&
					hasMoreRef.current &&
					!loadingRef.current
				) {
					onLoadMore();
				}
			},
			{ rootMargin: "200px" },
		);
		observer.observe(el);
		return () => observer.disconnect();
	}, [onLoadMore, ready]);

	return sentinelRef;
}
