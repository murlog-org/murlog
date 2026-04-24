// ComposeBox — reusable post composition form with attachments, CW, visibility, and reply-to.
// 投稿フォームの共通コンポーネント（添付・CW・公開範囲・返信先対応）。

import { useRef, useState } from "preact/hooks";
import type { AttachmentMeta } from "../lib/api";
import { call, uploadMedia } from "../lib/api";
import { t } from "../lib/i18n";
import { convertIfNeeded } from "../lib/image";
import { Icon } from "./icons";
import type { Post } from "./post-card";

type Props = {
	// Called after a post is successfully created. / 投稿成功後に呼ばれる。
	onPosted: () => void;
	// Pre-set reply target. / 事前設定された返信先。
	replyTo?: Post | null;
	// Called when reply target is cleared. / 返信先がクリアされた時に呼ばれる。
	onClearReply?: () => void;
};

export function ComposeBox({ onPosted, replyTo, onClearReply }: Props) {
	const [content, setContent] = useState("");
	const [visibility, setVisibility] = useState("public");
	const [posting, setPosting] = useState(false);
	const [error, setError] = useState("");
	const [attachments, setAttachments] = useState<AttachmentMeta[]>([]);
	const [uploading, setUploading] = useState(false);
	const [showCW, setShowCW] = useState(false);
	const [summary, setSummary] = useState("");
	const fileRef = useRef<HTMLInputElement>(null);

	const handleFileSelect = async (e: Event) => {
		const input = e.target as HTMLInputElement;
		const files = input.files;
		if (!files || files.length === 0) return;
		if (attachments.length + files.length > 4) {
			setError(t("my.compose.max_attachments") || "Maximum 4 attachments");
			return;
		}
		setUploading(true);
		setError("");
		for (const raw of Array.from(files)) {
			let file: File;
			try {
				file = await convertIfNeeded(raw);
			} catch {
				setError(
					t("my.compose.heic_unsupported") ||
						"HEIC is not supported in this browser.",
				);
				break;
			}
			const { result, error: err } = await uploadMedia(file);
			if (err) {
				setError(err);
				break;
			}
			if (result) setAttachments((prev) => [...prev, result]);
		}
		setUploading(false);
		if (fileRef.current) fileRef.current.value = "";
	};

	const removeAttachment = (id: string) => {
		setAttachments((prev) => prev.filter((a) => a.id !== id));
		call("media.delete", { id });
	};

	const handlePost = async (e: Event) => {
		e.preventDefault();
		if (!content.trim() && attachments.length === 0) return;
		setPosting(true);
		setError("");
		const params: Record<string, unknown> = { content, visibility };
		if (summary.trim()) {
			params.summary = summary.trim();
			params.sensitive = true;
		}
		if (attachments.length > 0)
			params.attachments = attachments.map((a) => a.id);
		if (replyTo) params.in_reply_to = replyTo.id;
		const { error: err } = await call("posts.create", params);
		setPosting(false);
		if (err) {
			setError(err.message);
			return;
		}
		setContent("");
		setSummary("");
		setShowCW(false);
		setAttachments([]);
		onPosted();
	};

	return (
		<div class="card compose">
			{error && (
				<p class="meta" style={{ color: "var(--danger)", marginBottom: 8 }}>
					{error}
				</p>
			)}
			<form onSubmit={handlePost}>
				{replyTo && (
					<div class="reply-context">
						<span class="meta">
							{t("my.compose.reply_to")}{" "}
							{replyTo.actor_name || replyTo.actor_uri || t("my.compose.self")}
						</span>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							onClick={onClearReply}
							aria-label="Cancel reply"
						>
							×
						</button>
					</div>
				)}
				{showCW && (
					<input
						type="text"
						value={summary}
						onInput={(e) => setSummary((e.target as HTMLInputElement).value)}
						placeholder={t("my.compose.cw_placeholder")}
						class="cw-input"
					/>
				)}
				<textarea
					value={content}
					onInput={(e) => setContent((e.target as HTMLTextAreaElement).value)}
					placeholder={
						replyTo
							? t("my.compose.reply_placeholder") || "Write a reply..."
							: t("my.compose.placeholder")
					}
				/>
				{attachments.length > 0 && (
					<div class="attachment-previews">
						{attachments.map((a) => (
							<div class="attachment-preview" key={a.id}>
								<img src={a.url} alt={a.alt} />
								<button
									type="button"
									class="attachment-remove"
									onClick={() => removeAttachment(a.id)}
									aria-label="Remove"
								>
									×
								</button>
							</div>
						))}
					</div>
				)}
				<div class="compose-footer">
					<div class="compose-actions">
						<input
							ref={fileRef}
							type="file"
							accept="image/jpeg,image/png,image/gif,image/webp,image/heic,image/heif"
							multiple
							style={{ display: "none" }}
							onChange={handleFileSelect}
						/>
						<button
							type="button"
							class={`btn btn-sm ${showCW ? "btn-active" : "btn-outline"}`}
							onClick={() => {
								setShowCW(!showCW);
								if (showCW) setSummary("");
							}}
						>
							{t("my.compose.cw")}
						</button>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							onClick={() => fileRef.current?.click()}
							disabled={uploading || attachments.length >= 4}
						>
							{uploading ? "..." : <Icon name="attach" size={14} />}
						</button>
						<select
							value={visibility}
							onChange={(e) =>
								setVisibility((e.target as HTMLSelectElement).value)
							}
							class="btn btn-outline btn-sm"
						>
							<option value="public">{t("my.visibility.public")}</option>
							<option value="unlisted">{t("my.visibility.unlisted")}</option>
							<option value="followers">{t("my.visibility.followers")}</option>
						</select>
						<span class={`char-count${content.length > 3000 ? " over" : ""}`}>
							{content.length}/3000
						</span>
					</div>
					<button
						class="btn btn-primary btn-sm"
						type="submit"
						disabled={
							posting ||
							content.length > 3000 ||
							(!content.trim() && attachments.length === 0)
						}
					>
						{posting
							? "..."
							: replyTo
								? t("my.compose.reply") || "Reply"
								: t("my.compose.post")}
					</button>
				</div>
			</form>
		</div>
	);
}
