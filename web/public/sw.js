// Minimal Service Worker — required for PWA install prompt.
// Intentionally empty: no offline caching or background sync.
// 最小限の Service Worker — PWA インストールプロンプトに必要。
// オフラインキャッシュ・バックグラウンド同期は対象外。
self.addEventListener("fetch", () => {});
