// PostCard — reusable post card component for SPA pages.
// SPA ページ用の再利用可能な投稿カードコンポーネント。

import { formatTime } from "../lib/format";
import { t } from "../lib/i18n";
import { sanitize } from "../lib/sanitize";
import { DropdownMenu } from "./dropdown";
import { Icon } from "./icons";
import { openLightbox } from "./lightbox";
import { LinkPreview } from "./link-preview";

// acctFromURI converts "https://host/users/name" to "@name@host".
// Actor URI を @user@host 形式に変換する。
export function acctFromURI(uri: string): string {
	try {
		const u = new URL(uri);
		const name = u.pathname.split("/").pop() || "";
		return `@${name}@${u.host}`;
	} catch {
		return uri;
	}
}

type Attachment = {
	id: string;
	url: string;
	mime_type: string;
	alt: string;
	width: number;
	height: number;
};

export type Post = {
	id: string;
	persona_id: string;
	content: string;
	summary?: string;
	sensitive?: boolean;
	visibility: string;
	origin: string;
	actor_uri: string;
	actor_acct?: string;
	actor_name?: string;
	actor_avatar_url?: string;
	reblogged_by_uri?: string;
	reblogged_by_acct?: string;
	reblogged_by_name?: string;
	reblogged_by_avatar_url?: string;
	in_reply_to_uri?: string;
	in_reply_to_post?: Post;
	attachments?: Attachment[];
	pinned?: boolean;
	url?: string;
	favourites_count: number;
	reblogs_count: number;
	favourited: boolean;
	reblogged: boolean;
	reblog?: Post;
	created_at: string;
	updated_at: string;
};

type PostCardActions = {
	onFavourite: (post: Post) => void;
	onReblog: (post: Post) => void;
	onReply?: (post: Post) => void;
	onPin?: (post: Post) => void;
	onDelete?: (id: string) => void;
	onEditStart?: (post: Post) => void;
};

type Props = {
	post: Post;
	actions: PostCardActions;
	expanded: boolean;
	onToggleExpand: (id: string) => void;
	// Edit state (only for owner posts). / 編集状態（オーナー投稿のみ）。
	editing?: boolean;
	editContent?: string;
	onEditChange?: (content: string) => void;
	onEditSave?: (id: string) => void;
	onEditCancel?: () => void;
	// Link prefix for user profile links (default: "/my/users").
	// ユーザープロフィールリンクのプレフィックス（デフォルト: "/my/users"）。
	profileLinkPrefix?: string;
	// Compact mode for thread context (no avatar, no action bar, smaller text).
	// スレッドコンテキスト用コンパクトモード（アバター・アクションバーなし、小さめテキスト）。
	compact?: boolean;
};

export function PostCard({
	post,
	actions,
	expanded,
	onToggleExpand,
	editing,
	editContent,
	onEditChange,
	onEditSave,
	onEditCancel,
	profileLinkPrefix = "/my/users",
	compact = false,
}: Props) {
	const profileHref = (acctOrURI: string, actorURI?: string) => {
		if (actorURI) {
			const acct = acctOrURI || acctFromURI(actorURI);
			return `${profileLinkPrefix}/${acct}`;
		}
		// Local post: extract username from post URL (/users/{username}/posts/{id}).
		// ローカル投稿: URL からユーザー名を抽出。
		if (post.url) {
			const m = post.url.match(/\/users\/([^/]+)/);
			if (m) return `${profileLinkPrefix}/${m[1]}`;
		}
		return "/";
	};

	const renderAttachments = (attachments?: Attachment[]) => {
		if (!attachments || attachments.length === 0) return null;
		return (
			<div class={`post-media post-media-${Math.min(attachments.length, 4)}`}>
				{attachments.map((att) => (
					<a
						key={att.id}
						href={att.url}
						onClick={(e) => {
							e.preventDefault();
							const img = (e.currentTarget as HTMLElement).querySelector("img");
							if (img) openLightbox(img);
						}}
					>
						<img src={att.url} alt={att.alt} loading="lazy" />
					</a>
				))}
			</div>
		);
	};

	// Rewrite local hashtag links to /my/tags/ when inside /my/ pages.
	// /my/ ページ内ではローカルハッシュタグリンクを /my/tags/ に書き換え。
	const prepareContent = (html: string): string => {
		let s = sanitize(html);
		if (profileLinkPrefix === "/my/users") {
			const origin = location.origin;
			s = s.replace(
				new RegExp(`href="${origin}/tags/`, "g"),
				`href="${origin}/my/tags/`,
			);
			s = s.replace(/href="\/tags\//g, 'href="/my/tags/');
		}
		return s;
	};

	const renderContent = () => {
		if (displayPost.summary || displayPost.sensitive) {
			const cwText =
				displayPost.summary || t("my.post.sensitive") || "Sensitive content";
			return (
				<>
					<div class="cw-summary">
						<span>{cwText}</span>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							onClick={() => onToggleExpand(post.id)}
						>
							{expanded ? t("my.post.show_less") : t("my.post.show_more")}
						</button>
					</div>
					{expanded && (
						<>
							<div
								class="post-content"
								dangerouslySetInnerHTML={{
									__html: prepareContent(displayPost.content),
								}}
							/>
							{displayPost.attachments && displayPost.attachments.length > 0 ? (
								renderAttachments(displayPost.attachments)
							) : (
								<LinkPreview content={displayPost.content} />
							)}
						</>
					)}
				</>
			);
		}
		return (
			<>
				<div
					class="post-content"
					dangerouslySetInnerHTML={{
						__html: prepareContent(displayPost.content),
					}}
				/>
				{displayPost.attachments && displayPost.attachments.length > 0 ? (
					renderAttachments(displayPost.attachments)
				) : (
					<LinkPreview content={displayPost.content} />
				)}
			</>
		);
	};

	// Reblog wrapper: unwrap and display the original post, passing reblogged=true.
	// リブログ wrapper: 元投稿を展開して表示し、reblogged=true を引き継ぐ。
	const displayPost = post.reblog || post;
	const isReblogWrapper = !!post.reblog;

	if (editing && onEditChange && onEditSave && onEditCancel) {
		return (
			<div class="card">
				<textarea
					class="compose"
					value={editContent}
					onInput={(e) => onEditChange((e.target as HTMLTextAreaElement).value)}
					style={{
						width: "100%",
						padding: "8px 10px",
						border: "1px solid var(--border)",
						borderRadius: "var(--radius)",
						fontSize: "14px",
						fontFamily: "inherit",
						background: "var(--paper)",
						color: "var(--ink)",
						height: "80px",
						resize: "vertical",
					}}
				/>
				<div style={{ marginTop: 8, display: "flex", gap: 6 }}>
					<button
						type="button"
						class="btn btn-primary btn-sm"
						onClick={() => onEditSave(post.id)}
					>
						{t("my.post.save")}
					</button>
					<button
						type="button"
						class="btn btn-outline btn-sm"
						onClick={onEditCancel}
					>
						{t("my.post.cancel")}
					</button>
				</div>
			</div>
		);
	}

	if (compact) {
		return (
			<div class="card post-compact">
				<div class="post-compact-header">
					<span class="post-compact-author">
						{displayPost.actor_name ||
							displayPost.actor_acct?.split("@")[1] ||
							displayPost.actor_uri?.split("/").pop() ||
							"?"}
					</span>
					<time class="meta">{formatTime(post.created_at)}</time>
				</div>
				{renderContent()}
			</div>
		);
	}

	return (
		<div class="card">
			{post.pinned && (
				<div class="post-label">
					<Icon name="pin" size={14} /> {t("my.post.pinned")}
				</div>
			)}
			{isReblogWrapper && (
				<div class="post-label">
					<Icon name="reblog" size={14} />{" "}
					{post.actor_name || t("my.post.you") || "you"}{" "}
					{t("my.post.reblogged")}
				</div>
			)}
			{!isReblogWrapper && post.reblogged_by_uri && (
				<div class="post-label">
					<Icon name="reblog" size={14} />{" "}
					<a
						href={profileHref(
							post.reblogged_by_acct || acctFromURI(post.reblogged_by_uri),
							post.reblogged_by_uri,
						)}
					>
						{post.reblogged_by_name ||
							post.reblogged_by_acct?.split("@")[1] ||
							post.reblogged_by_uri.split("/").pop()}
					</a>{" "}
					{t("my.post.reblogged")}
				</div>
			)}
			{displayPost.in_reply_to_uri && (
				<div class="post-label">
					<Icon name="reply" size={14} /> {t("my.post.replied_to")}{" "}
					{displayPost.in_reply_to_post ? (
						<a
							href={
								displayPost.in_reply_to_post.url || displayPost.in_reply_to_uri
							}
						>
							{displayPost.in_reply_to_post.actor_name ||
								displayPost.in_reply_to_post.actor_acct ||
								(displayPost.in_reply_to_post.actor_uri
									? acctFromURI(displayPost.in_reply_to_post.actor_uri)
									: t("my.post.reply_to_self"))}
						</a>
					) : (
						<a href={displayPost.in_reply_to_uri}>
							{acctFromURI(displayPost.in_reply_to_uri)}
						</a>
					)}
				</div>
			)}

			{/* Post header / 投稿ヘッダー */}
			<div class="post-header">
				<div class="post-author">
					{(() => {
						const href = profileHref(
							displayPost.actor_acct || "",
							displayPost.actor_uri,
						);
						return displayPost.actor_avatar_url ? (
							<a href={href}>
								<img
									class="post-avatar"
									src={displayPost.actor_avatar_url}
									alt=""
									width="40"
									height="40"
								/>
							</a>
						) : (
							<a href={href}>
								<div
									class="post-avatar"
									style={{
										width: 40,
										height: 40,
										borderRadius: "50%",
										background: "var(--secondary)",
									}}
								/>
							</a>
						);
					})()}
					<div>
						{displayPost.actor_uri ? (
							<>
								<a
									href={profileHref(
										displayPost.actor_acct ||
											acctFromURI(displayPost.actor_uri),
										displayPost.actor_uri,
									)}
									class="post-author-name"
								>
									{displayPost.actor_name ||
										displayPost.actor_acct?.split("@")[1] ||
										displayPost.actor_uri.split("/").pop()}
								</a>
								<a
									href={profileHref(
										displayPost.actor_acct ||
											acctFromURI(displayPost.actor_uri),
										displayPost.actor_uri,
									)}
									class="handle"
								>
									{displayPost.actor_acct || acctFromURI(displayPost.actor_uri)}
								</a>
							</>
						) : (
							<a href={profileHref("", undefined)} class="post-author-name">
								{displayPost.actor_name || "you"}
							</a>
						)}
					</div>
				</div>
				{displayPost.url ? (
					<a href={displayPost.url}>
						<time>{formatTime(post.created_at)}</time>
					</a>
				) : (
					<time>{formatTime(post.created_at)}</time>
				)}
			</div>

			{renderContent()}

			{/* Stats / 統計 — actions target the original post for reblog wrappers */}
			{/* アクションはリブログ wrapper の場合、元投稿に対して実行 */}
			<div class="post-stats">
				{actions.onReply && (
					<button
						type="button"
						class="post-stat"
						onClick={() => actions.onReply!(displayPost)}
						aria-label="Reply"
					>
						<Icon name="reply" />
					</button>
				)}
				<button
					type="button"
					class={`post-stat${displayPost.reblogged ? " active" : ""}`}
					onClick={() => actions.onReblog(displayPost)}
					aria-label="Reblog"
				>
					<Icon name="reblog" />
					{displayPost.reblogs_count > 0 && ` ${displayPost.reblogs_count}`}
				</button>
				<button
					type="button"
					class={`post-stat${displayPost.favourited ? " active" : ""}`}
					onClick={() => actions.onFavourite(displayPost)}
					aria-label="Favourite"
				>
					<Icon name="heart" />
					{displayPost.favourites_count > 0 &&
						` ${displayPost.favourites_count}`}
				</button>
				{!isReblogWrapper && (
					<DropdownMenu class="post-stat post-stat-action dot-menu">
						{displayPost.url && (
							<a
								href="#"
								onClick={(e) => {
									e.preventDefault();
									const url = displayPost.url!.startsWith("http")
										? displayPost.url!
										: location.origin + displayPost.url!;
									navigator.clipboard.writeText(url);
								}}
							>
								{t("my.post.copy_link") || "Copy link"}
							</a>
						)}
						{post.origin === "local" && actions.onPin && (
							<>
								<a
									href="#"
									onClick={(e) => {
										e.preventDefault();
										actions.onPin!(post);
									}}
								>
									{post.pinned ? t("my.post.unpin") : t("my.post.pin")}
								</a>
								{actions.onEditStart && (
									<a
										href="#"
										onClick={(e) => {
											e.preventDefault();
											actions.onEditStart!(post);
										}}
									>
										{t("my.post.edit")}
									</a>
								)}
								{actions.onDelete && (
									<a
										href="#"
										class="dot-menu-danger"
										onClick={(e) => {
											e.preventDefault();
											actions.onDelete!(post.id);
										}}
									>
										{t("my.post.delete")}
									</a>
								)}
							</>
						)}
					</DropdownMenu>
				)}
			</div>
		</div>
	);
}
