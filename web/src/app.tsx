import { useEffect, useState } from "preact/hooks";
import Router, { route as navigate } from "preact-router";
import { AboutDialog } from "./components/about-dialog";
import { DropdownMenu } from "./components/dropdown";
import { Lightbox, openLightbox } from "./components/lightbox";
import { Logo } from "./components/logo";
import { call, isUnauthorized } from "./lib/api";
import {
	isLoggedIn as isGlobalLoggedIn,
	setLoggedIn as setGlobalLoggedIn,
} from "./lib/auth";
import { load as loadI18n, t } from "./lib/i18n";
import { BlocksPage } from "./pages/blocks";
import { DomainFailuresPage } from "./pages/domain-failures";
import { FollowPage } from "./pages/follow";
import { Login } from "./pages/login";
import { My } from "./pages/my";
import { MyFollowList } from "./pages/my-follow-list";
import { MyPost } from "./pages/my-post";
import { MyProfile } from "./pages/my-profile";
import { MyTagPage } from "./pages/my-tag";
import { NotificationsPage } from "./pages/notifications";
import { PublicFollowList } from "./pages/public-follow-list";
import { PublicHome } from "./pages/public-home";
import { PublicPost } from "./pages/public-post";
import { PublicProfile } from "./pages/public-profile";
import { QueuePage } from "./pages/queue";
import { SettingsPage } from "./pages/settings";
import { TagPage } from "./pages/tag";

// Pages where the header should be hidden.
// ヘッダーを非表示にするページ。
const hideHeader = new Set(["/my/login"]);

// Check if path is a public page (themed, no app header).
// 公開ページかどうか判定（テーマ適用、アプリヘッダー非表示）。
function isPublicPage(path: string): boolean {
	return (
		path === "/" || path.startsWith("/users/") || path.startsWith("/tags/")
	);
}

// NotFound is the fallback page for unmatched routes.
// 一致しないルートのフォールバックページ。
function NotFound({ default: _ }: { default?: boolean }) {
	return (
		<div class="screen" style={{ textAlign: "center", padding: "64px 16px" }}>
			<h1 style={{ fontSize: 48, margin: 0, opacity: 0.3 }}>404</h1>
			<p style={{ opacity: 0.6 }}>Not Found</p>
		</div>
	);
}

// HeaderMenu renders the ... dropdown in the app header.
// ヘッダーの ... ドロップダウンメニュー。
function HeaderMenu({ onAbout }: { onAbout: () => void }) {
	const handleLogout = async () => {
		await call("auth.logout");
		navigate("/my/login");
	};

	return (
		<DropdownMenu iconSize={18}>
			<a href="/my/">{t("my.home") || "Timeline"}</a>
			<a href="/my/notifications">{t("my.notifications") || "Notifications"}</a>
			<a href="/my/follow">{t("my.follow") || "Follow"}</a>
			<a href="/my/blocks">{t("my.blocks") || "Blocks"}</a>
			<a href="/my/settings">{t("my.settings") || "Settings"}</a>
			<a href="/">{t("my.public_page") || "Public Page"}</a>
			<a
				href="#"
				onClick={(e) => {
					e.preventDefault();
					handleLogout();
				}}
			>
				{t("my.logout") || "Logout"}
			</a>
			<a
				href="#"
				onClick={(e) => {
					e.preventDefault();
					onAbout();
				}}
			>
				{t("my.about") || "About"}
			</a>
		</DropdownMenu>
	);
}

// PublicMenu renders the ... dropdown for public pages.
// 公開ページの ... ドロップダウンメニュー。
function PublicMenu({ onAbout }: { onAbout: () => void }) {
	return (
		<DropdownMenu iconSize={18}>
			<a href="/my/login">{t("public.login") || "Login"}</a>
			<a
				href="#"
				onClick={(e) => {
					e.preventDefault();
					onAbout();
				}}
			>
				{t("my.about") || "About"}
			</a>
		</DropdownMenu>
	);
}

// ProfileRouter dispatches to local or remote profile based on username format.
// Logged-in users viewing remote profiles are redirected to /my/users/.
// username の形式でローカル/リモートプロフィールを振り分ける。
// ログイン済みユーザーのリモートプロフィール表示は /my/users/ にリダイレクト。
function ProfileRouter({
	username,
	path,
}: {
	username?: string;
	path?: string;
}) {
	if (username?.startsWith("@") && username.includes("@", 1)) {
		// @user@host → redirect to /my/users/ if logged in, else 404.
		// ログイン済みなら /my/users/ にリダイレクト、未ログインなら 404。
		if (isGlobalLoggedIn()) {
			navigate(`/my/users/${username}`);
			return null;
		}
		return <NotFound default />;
	}
	return <PublicProfile username={username} path={path} />;
}

// FollowListRouter redirects remote follow lists to /my/users/ when logged in.
// ログイン済みのリモートフォロー一覧を /my/users/ にリダイレクト。
function FollowListRouter({
	username,
	type,
}: {
	username?: string;
	path?: string;
	type: "following" | "followers";
}) {
	if (username?.startsWith("@") && username.includes("@", 1)) {
		if (isGlobalLoggedIn()) {
			navigate(`/my/users/${username}/${type}`);
			return null;
		}
		return <NotFound default />;
	}
	return <PublicFollowList username={username} type={type} />;
}

export function App() {
	const [path, setPath] = useState(location.pathname);
	const [loggedIn, setLoggedIn] = useState(false);
	const [showAbout, setShowAbout] = useState(false);

	// Set body class for CSS scoping.
	// SPA pages switch immediately. Public pages are switched by the page component
	// after theme rendering completes (to avoid FOUC).
	// CSS スコープ用の body クラス設定。
	// SPA ページは即切り替え。公開ページはテーマ描画完了後にコンポーネント側で切り替え。
	useEffect(() => {
		if (!isPublicPage(path)) {
			document.body.classList.remove("ssr", "spa", "public");
			document.body.classList.add("spa");
		}
	}, [path]);

	// Global click handler for post attachment images (theme-rendered pages).
	// テーマ描画ページの投稿添付画像用グローバルクリックハンドラ。
	useEffect(() => {
		const handler = (e: MouseEvent) => {
			const link = (e.target as HTMLElement).closest(".post-media a");
			if (!link) return;
			const img = link.querySelector("img") as HTMLImageElement | null;
			if (!img) return;
			e.preventDefault();
			openLightbox(img);
		};
		document.addEventListener("click", handler);
		return () => document.removeEventListener("click", handler);
	}, []);

	// Load i18n and check login state once on mount.
	// マウント時に i18n を読み込み、ログイン状態を確認。
	const [i18nReady, setI18nReady] = useState(false);
	useEffect(() => {
		(async () => {
			await loadI18n();
			setI18nReady(true);
			const res = await call("timeline.home", { limit: 1 });
			if (!isUnauthorized(res.error)) {
				setLoggedIn(true);
				setGlobalLoggedIn(true);
			}
		})();
	}, []);

	// i18nReady triggers re-render so t() returns translated strings.
	// i18nReady の変更で再レンダリングし t() が翻訳済み文字列を返す。
	const showAppHeader = loggedIn || !isPublicPage(path);
	void i18nReady; // re-render trigger

	return (
		<>
			{!hideHeader.has(path) && (
				<header class="header">
					<a
						href={showAppHeader ? "/my/" : "/"}
						class="header-brand"
						onClick={(e) => {
							e.preventDefault();
							navigate(showAppHeader ? "/my/" : "/");
						}}
					>
						<Logo size={20} />
						<span>murlog</span>
					</a>
					{showAppHeader ? (
						<HeaderMenu onAbout={() => setShowAbout(true)} />
					) : (
						<PublicMenu onAbout={() => setShowAbout(true)} />
					)}
				</header>
			)}
			<Router onChange={(e) => setPath(e.url)}>
				<TagPage path="/tags/:tag" />
				<PublicPost path="/users/:username/posts/:id" />
				<FollowListRouter path="/users/:username/following" type="following" />
				<FollowListRouter path="/users/:username/followers" type="followers" />
				<ProfileRouter path="/users/:username" />
				<PublicHome path="/" />
				<Login path="/my/login" />
				<FollowPage path="/my/follow" />
				<NotificationsPage path="/my/notifications" />
				<SettingsPage path="/my/settings" />
				<BlocksPage path="/my/blocks" />
				<QueuePage path="/my/queue" />
				<DomainFailuresPage path="/my/domain-failures" />
				<MyPost path="/my/posts/:id" />
				<MyTagPage path="/my/tags/:tag" />
				<MyFollowList path="/my/users/:username/following" type="following" />
				<MyFollowList path="/my/users/:username/followers" type="followers" />
				<MyProfile path="/my/users/:username" />
				<My path="/my/" />
				<NotFound default />
			</Router>
			<Lightbox />
			{showAbout && <AboutDialog onClose={() => setShowAbout(false)} />}
		</>
	);
}
