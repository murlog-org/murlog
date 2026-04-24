// SPA i18n — loads translations from /locales/{lang}.json.
// SPA 多言語対応 — /locales/{lang}.json から翻訳を読み込む。

let translations: Record<string, string> = {};
let loaded = false;
let loadPromise: Promise<void> | null = null;

// load fetches the locale file matching the page language.
// ページ言語に合致するロケールファイルを取得する。
export function load(): Promise<void> {
	if (loaded) return Promise.resolve();
	if (loadPromise) return loadPromise;
	loadPromise = (async () => {
		const lang = document.documentElement.lang || "en";
		try {
			const qs =
				typeof __BUILD_HASH__ !== "undefined" ? `?v=${__BUILD_HASH__}` : "";
			const res = await fetch(`/locales/${lang}.json${qs}`);
			if (res.ok) {
				translations = await res.json();
			}
		} catch {
			// Fall through — keys returned as-is.
		}
		loaded = true;
	})();
	return loadPromise;
}

// t returns the translated string for the given key.
// 指定キーの翻訳文字列を返す。
export function t(key: string): string {
	return translations[key] ?? key;
}
