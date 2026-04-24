// Shared type definitions for the SPA.
// SPA 共通の型定義。

// Account is the unified API representation of a user account (local persona or remote actor).
// ユーザーアカウント (ローカルペルソナまたはリモート Actor) の統一 API 表現。
export type Account = {
	id?: string;
	uri?: string;
	acct: string;
	username: string;
	display_name: string;
	summary: string;
	summary_html?: string;
	fields?: { name: string; value: string }[];
	avatar_url: string;
	header_url?: string;
	featured_url?: string;
	primary?: boolean;
	locked?: boolean;
	show_follows?: boolean;
	discoverable?: boolean;
	post_count: number;
	following_count: number;
	followers_count: number;
	created_at?: string;
	updated_at?: string;
};
