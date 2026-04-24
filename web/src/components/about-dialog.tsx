// AboutDialog — modal showing version, license, and links.
// バージョン・ライセンス・リンクを表示するモーダル。

import { Logo } from "./logo";

type Props = {
	onClose: () => void;
};

export function AboutDialog({ onClose }: Props) {
	const version =
		typeof __APP_VERSION__ !== "undefined" ? __APP_VERSION__ : "dev";

	return (
		<div
			class="overlay"
			onClick={(e) => {
				if (e.target === e.currentTarget) onClose();
			}}
		>
			<div class="dialog">
				<div class="dialog-header">
					<h2 class="dialog-title">About murlog</h2>
					<button
						type="button"
						class="dialog-close"
						onClick={onClose}
						aria-label="Close"
					>
						&times;
					</button>
				</div>
				<div class="dialog-body" style={{ textAlign: "center" }}>
					<div
						style={{
							margin: "8px 0 16px",
							display: "flex",
							justifyContent: "center",
						}}
					>
						<Logo size={48} />
					</div>
					<p style={{ fontSize: "18px", fontWeight: 600, margin: "0 0 4px" }}>
						murlog
					</p>
					<p class="meta" style={{ margin: "0 0 16px" }}>
						v{version}
					</p>
					<p style={{ fontSize: "14px", lineHeight: 1.6, margin: "0 0 16px" }}>
						A single-user ActivityPub server.
						<br />
						CGI-compatible, Go + SQLite.
					</p>
					<div
						style={{
							display: "flex",
							flexDirection: "column",
							gap: 8,
							alignItems: "center",
							fontSize: "14px",
						}}
					>
						<a
							href="https://murlog.org"
							target="_blank"
							rel="noopener noreferrer"
						>
							murlog.org
						</a>
						<a
							href="https://github.com/murlog-org/murlog"
							target="_blank"
							rel="noopener noreferrer"
						>
							GitHub
						</a>
					</div>
					<p class="meta" style={{ margin: "16px 0 0", fontSize: "12px" }}>
						AGPL-3.0 License
					</p>
				</div>
			</div>
		</div>
	);
}
