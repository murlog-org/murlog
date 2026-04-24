// Link preview card — batches OGP requests via JSON-RPC batch.
// リンクプレビューカード — JSON-RPC バッチで OGP リクエストをまとめる。

import { useEffect, useState } from "preact/hooks";
import { callBatch } from "../lib/api";

type OGP = {
	url: string;
	title?: string;
	description?: string;
	image?: string;
	site_name?: string;
};

// extractFirstURL extracts the first external https URL from HTML content.
// HTML コンテンツから最初の外部 https URL を抽出する。
export function extractFirstURL(html: string): string | null {
	const origin = location.origin;

	// Try <a href="..."> first (linked content). / まずリンク済みコンテンツを探す。
	const hrefRe = /href="(https:\/\/[^"]+)"/g;
	let m: RegExpExecArray | null;
	while ((m = hrefRe.exec(html))) {
		if (!m[1].startsWith(`${origin}/`)) return m[1];
	}

	// Fallback: bare URL in plain text (old posts without auto-linking).
	// フォールバック: 自動リンク化前の古い投稿のプレーンテキスト URL。
	const bareRe = /(https:\/\/[^\s<>"]+)/g;
	while ((m = bareRe.exec(html))) {
		if (!m[1].startsWith(`${origin}/`)) return m[1];
	}

	return null;
}

// In-memory cache to avoid re-fetching. / 再取得を避けるインメモリキャッシュ。
const cache = new Map<string, OGP | null>();

// Batch queue — collects URLs and flushes in a single callBatch.
// バッチキュー — URL を収集して1回の callBatch でまとめて取得。
type Waiter = (ogp: OGP | null) => void;
let pending = new Map<string, Waiter[]>();
let flushTimer: ReturnType<typeof setTimeout> | null = null;

function enqueue(url: string, cb: Waiter): void {
	if (!pending.has(url)) pending.set(url, []);
	pending.get(url)!.push(cb);

	if (!flushTimer) {
		flushTimer = setTimeout(flush, 50);
	}
}

async function flush(): Promise<void> {
	flushTimer = null;
	const batch = pending;
	pending = new Map();

	const urls = Array.from(batch.keys());
	if (urls.length === 0) return;

	const requests = urls.map((url) => ({
		method: "links.preview",
		params: { url },
	}));
	const results = await callBatch(requests);

	for (let i = 0; i < urls.length; i++) {
		const url = urls[i];
		const res = results[i];
		const ogp = res?.result as OGP | undefined;
		const value = ogp && (ogp.title || ogp.description) ? ogp : null;
		cache.set(url, value);
		for (const cb of batch.get(url) || []) cb(value);
	}
}

export function LinkPreview({ content }: { content: string }) {
	const [ogp, setOgp] = useState<OGP | null>(null);

	const url = extractFirstURL(content);

	useEffect(() => {
		if (!url) return;

		if (cache.has(url)) {
			setOgp(cache.get(url) || null);
			return;
		}

		enqueue(url, setOgp);
	}, [url]);

	if (!ogp) return null;

	return (
		<a
			href={ogp.url}
			class="link-preview"
			target="_blank"
			rel="noopener noreferrer"
		>
			{ogp.image && (
				<img class="link-preview-image" src={ogp.image} alt="" loading="lazy" />
			)}
			<div class="link-preview-body">
				{ogp.title && <div class="link-preview-title">{ogp.title}</div>}
				{ogp.description && (
					<div class="link-preview-desc">{ogp.description}</div>
				)}
				{ogp.site_name && <div class="link-preview-site">{ogp.site_name}</div>}
			</div>
		</a>
	);
}
