// ErrorRetry — displays an error message with an optional retry button.
// エラーメッセージとリトライボタンを表示するコンポーネント。

import { t } from "../lib/i18n";

type Props = {
	message?: string;
	onRetry?: () => void;
};

export function ErrorRetry({ message, onRetry }: Props) {
	return (
		<div class="card" style={{ textAlign: "center", padding: 16 }}>
			<p class="meta" style={{ color: "var(--danger)" }}>
				{message || t("my.error.load_failed") || "Failed to load."}
			</p>
			{onRetry && (
				<button
					type="button"
					class="btn btn-outline btn-sm"
					style={{ marginTop: 8 }}
					onClick={onRetry}
				>
					{t("my.error.retry") || "Retry"}
				</button>
			)}
		</div>
	);
}
