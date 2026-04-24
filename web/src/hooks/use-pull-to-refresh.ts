import { useEffect, useRef, useState, useCallback } from "preact/hooks";

const THRESHOLD = 60; // px to trigger refresh / リフレッシュ発火の閾値

/**
 * Pull-to-refresh hook for PWA.
 * PWA 用の上スクロールリフレッシュ hook。
 *
 * Listens on document for touch events; triggers when user pulls down
 * from scroll position 0.
 * document の touch イベントを監視し、スクロール位置 0 から
 * 下に引っ張った時にリフレッシュを発火する。
 */
export function usePullToRefresh(onRefresh: () => Promise<void>) {
  const [refreshing, setRefreshing] = useState(false);
  const [pullDistance, setPullDistance] = useState(0);
  const startY = useRef(0);
  const pulling = useRef(false);
  const refreshingRef = useRef(false);

  const handleRefresh = useCallback(async () => {
    refreshingRef.current = true;
    setRefreshing(true);
    setPullDistance(0);
    try {
      await onRefresh();
    } finally {
      refreshingRef.current = false;
      setRefreshing(false);
    }
  }, [onRefresh]);

  useEffect(() => {
    const onTouchStart = (e: TouchEvent) => {
      if (window.scrollY <= 0 && !refreshingRef.current) {
        startY.current = e.touches[0].clientY;
        pulling.current = true;
      }
    };

    const onTouchMove = (e: TouchEvent) => {
      if (!pulling.current) return;
      const dy = e.touches[0].clientY - startY.current;
      if (dy > 0 && window.scrollY <= 0) {
        setPullDistance(Math.min(dy * 0.4, THRESHOLD * 1.5));
      } else {
        pulling.current = false;
        setPullDistance(0);
      }
    };

    const onTouchEnd = () => {
      if (!pulling.current) return;
      pulling.current = false;
      // Read current pullDistance via DOM to avoid stale closure.
      // stale closure 回避のため現在値を取得。
      setPullDistance((current) => {
        if (current >= THRESHOLD) {
          handleRefresh();
        }
        return 0;
      });
    };

    document.addEventListener("touchstart", onTouchStart, { passive: true });
    document.addEventListener("touchmove", onTouchMove, { passive: true });
    document.addEventListener("touchend", onTouchEnd);

    return () => {
      document.removeEventListener("touchstart", onTouchStart);
      document.removeEventListener("touchmove", onTouchMove);
      document.removeEventListener("touchend", onTouchEnd);
    };
  }, [handleRefresh]);

  return { refreshing, pullDistance };
}
