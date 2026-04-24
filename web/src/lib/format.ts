// Shared formatting utilities.
// 共通フォーマットユーティリティ。

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

// formatTime returns a locale-aware relative time string.
// ロケール対応の相対時間文字列を返す。
export function formatTime(iso: string): string {
	const d = new Date(iso);
	const diff = (d.getTime() - Date.now()) / 1000;
	const absDiff = Math.abs(diff);
	if (absDiff < 60) return rtf.format(Math.round(diff), "second");
	if (absDiff < 3600) return rtf.format(Math.round(diff / 60), "minute");
	if (absDiff < 86400) return rtf.format(Math.round(diff / 3600), "hour");
	if (absDiff < 2592000) return rtf.format(Math.round(diff / 86400), "day");
	return d.toLocaleDateString();
}
