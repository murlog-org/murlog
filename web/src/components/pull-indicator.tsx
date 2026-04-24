/** Pull-to-refresh visual indicator.
 *  上スクロールリフレッシュのインジケーター。 */
export function PullIndicator({ distance, refreshing }: { distance: number; refreshing: boolean }) {
  if (!refreshing && distance <= 0) return null;
  return (
    <div style={{
      textAlign: "center",
      padding: refreshing ? "12px 0" : `${Math.max(distance * 0.3, 0)}px 0`,
      opacity: refreshing ? 1 : Math.min(distance / 60, 1),
      transition: refreshing ? "none" : "padding 0.1s",
      color: "var(--text-muted, #888)",
      fontSize: 13,
    }}>
      {refreshing ? "↻" : "↓"}
    </div>
  );
}
