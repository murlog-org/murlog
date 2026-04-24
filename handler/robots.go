package handler

import (
	"fmt"
	"net/http"
)

// Known AI crawler User-Agents to block when robots_noai is enabled.
// robots_noai 有効時にブロックする AI クローラーの User-Agent 一覧。
var aiCrawlers = []string{
	"GPTBot",
	"ChatGPT-User",
	"Google-Extended",
	"CCBot",
	"anthropic-ai",
	"ClaudeBot",
	"Claude-Web",
	"Bytespider",
	"Diffbot",
	"FacebookBot",
	"PerplexityBot",
	"Applebot-Extended",
	"cohere-ai",
}

// handleRobotsTxt serves a dynamic robots.txt based on settings.
// 設定に基づいて動的に robots.txt を生成する。
func (h *Handler) handleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	noIndex, _ := h.store.GetSetting(ctx, SettingRobotsNoIndex)
	noAI, _ := h.store.GetSetting(ctx, SettingRobotsNoAI)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Always block admin and API paths.
	// 管理・API パスは常にブロック。
	fmt.Fprintln(w, "User-agent: *")
	if noIndex == "true" {
		fmt.Fprintln(w, "Disallow: /")
	} else {
		fmt.Fprintln(w, "Disallow: /admin/")
		fmt.Fprintln(w, "Disallow: /api/")
		fmt.Fprintln(w, "Disallow: /my/")
	}
	fmt.Fprintln(w)

	// Block AI crawlers if enabled.
	// AI クローラーブロックが有効な場合。
	if noAI == "true" {
		for _, bot := range aiCrawlers {
			fmt.Fprintf(w, "User-agent: %s\n", bot)
			fmt.Fprintln(w, "Disallow: /")
			fmt.Fprintln(w)
		}
	}
}
