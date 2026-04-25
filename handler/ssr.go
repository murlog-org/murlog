package handler

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/i18n"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/media"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

// htmlPolicy is the shared HTML sanitization policy.
// SPA 側の DOMPurify 許可リストと同等のホワイトリスト。
// 共有 HTML サニタイズポリシー。
var htmlPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowStandardURLs()
	p.AllowElements("p", "br", "span", "em", "strong", "b", "i", "u",
		"ul", "ol", "li", "blockquote", "pre", "code", "a")
	p.AllowAttrs("href", "rel", "class").OnElements("a")
	p.AllowAttrs("class").OnElements("span")
	p.RequireNoFollowOnLinks(false)
	return p
}()

// SanitizeContentHTML sanitizes untrusted HTML content (posts, bios).
// Used at Inbox receive time (store) and SSR output time (defense in depth).
// 信頼できない HTML コンテンツ (投稿, bio) をサニタイズする。
// Inbox 受信時 (保存) と SSR 出力時 (多層防御) の両方で使用。
func SanitizeContentHTML(s string) string {
	return htmlPolicy.Sanitize(s)
}

// sanitizeHTML wraps SanitizeContentHTML for template.HTML output.
// template.HTML 出力用の SanitizeContentHTML ラッパー。
func sanitizeHTML(s string) template.HTML {
	return template.HTML(SanitizeContentHTML(s))
}

// profileData is the template data for the SSR profile page.
// SSR プロフィールページのテンプレートデータ。
// autoLinkValue wraps a URL string in an <a> tag, or escapes plain text.
// URL 文字列を <a> タグでラップするか、プレーンテキストをエスケープする。
func autoLinkValue(s string) template.HTML {
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		escaped := template.HTMLEscapeString(s)
		return template.HTML(`<a href="` + escaped + `" rel="nofollow noopener" target="_blank">` + escaped + `</a>`)
	}
	return template.HTML(template.HTMLEscapeString(s))
}

// toSSRFields converts persona custom fields to SSR template fields with auto-linked values.
// ペルソナのカスタムフィールドを自動リンク付き SSR テンプレートフィールドに変換する。
func toSSRFields(fields []murlog.CustomField) []ssrField {
	if len(fields) == 0 {
		return nil
	}
	out := make([]ssrField, len(fields))
	for i, f := range fields {
		out[i] = ssrField{Name: f.Name, Value: autoLinkValue(f.Value)}
	}
	return out
}

// ssrField is a custom field with auto-linked value for SSR templates.
// SSR テンプレート用の自動リンク付きカスタムフィールド。
type ssrField struct {
	Name  string
	Value template.HTML
}

type profileData struct {
	T             func(string) string
	Lang          string
	BaseURL       string
	Domain        string
	Username      string
	DisplayName   string
	Summary       template.HTML
	AvatarURL     string
	HeaderURL     string
	ProfileURL    string
	Fields        []ssrField
	Title         string
	Description   string
	RobotsContent string // meta robots content / meta robots の content 属性
	ThemeCSS      string // theme CSS path / テーマ CSS パス
	Posts         []postData
	SSRData       template.JS // JSON for <script id="ssr-data">
}

// postData is a single post in the SSR template.
// SSR テンプレート内の 1 投稿。
type postData struct {
	ID        string
	Content   template.HTML
	CreatedAt time.Time
	Permalink string
}

// toPostDataList converts enriched postJSON to SSR postData, resolving reblog wrappers.
// enriched postJSON を SSR postData に変換し、リブログ wrapper を解決する。
func toPostDataList(enriched []postJSON, profileURL string) []postData {
	items := make([]postData, len(enriched))
	for i, pj := range enriched {
		content := pj.Content
		if pj.Reblog != nil {
			content = pj.Reblog.Content
		}
		t, _ := time.Parse(time.RFC3339, pj.CreatedAt)
		items[i] = postData{
			ID:        pj.ID,
			Content:   sanitizeHTML(content),
			CreatedAt: t,
			Permalink: profileURL + "/posts/" + pj.ID,
		}
	}
	return items
}

// postPageData is the template data for the SSR post permalink page.
// SSR 投稿パーマリンクページのテンプレートデータ。
type postPageData struct {
	T             func(string) string
	Lang          string
	BaseURL       string
	Domain        string
	Username      string
	DisplayName   string
	AvatarURL     string
	ProfileURL    string
	PostID        string
	Content       template.HTML
	Attachments   []attachmentJSON
	CreatedAt     time.Time
	Permalink     string
	Title         string
	Description   string
	RobotsContent string
	ThemeCSS      string // theme CSS path / テーマ CSS パス
	OGImage       string // first attachment URL for og:image / og:image 用の最初の添付 URL
	SSRData       template.JS
}

// homePersonaData is a persona with its posts for the home page SSR.
// ホームページ SSR 用のペルソナ+投稿データ。
type homePersonaData struct {
	Username    string
	DisplayName string
	Summary     template.HTML
	AvatarURL   string
	HeaderURL   string
	ProfileURL  string
	Fields      []ssrField
	Primary     bool
	Posts       []postData
}

// homeData is the template data for the SSR home page.
// SSR ホームページのテンプレートデータ。
type homeData struct {
	T             func(string) string
	Lang          string
	BaseURL       string
	Domain        string
	Title         string
	RobotsContent string
	ThemeCSS      string // theme CSS path / テーマ CSS パス
	Personas      []homePersonaData
	SSRData       template.JS
}

// robotsMetaContent builds the meta robots content attribute from settings.
// 設定から meta robots の content 属性を組み立てる。
func (h *Handler) robotsMetaContent(ctx context.Context) string {
	noIndex, _ := h.store.GetSetting(ctx, SettingRobotsNoIndex)
	noAI, _ := h.store.GetSetting(ctx, SettingRobotsNoAI)

	parts := []string{}
	if noIndex == "true" {
		parts = append(parts, "noindex", "nofollow")
	}
	if noAI == "true" {
		parts = append(parts, "noai", "noimageai")
	}
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += ", " + p
	}
	return result
}

// loadSSRTemplates loads SSR templates from embedded files.
// 埋め込みファイルから SSR テンプレートを読み込む。
func (h *Handler) loadSSRTemplates() {
	load := func(name string) *template.Template {
		data, err := templatesFS.ReadFile("templates/" + name)
		if err != nil {
			log.Printf("ssr: %s not loaded: %v", name, err)
			return nil
		}
		tmpl, err := template.New(name).Parse(string(data))
		if err != nil {
			log.Printf("ssr: %s parse error: %v", name, err)
			return nil
		}
		return tmpl
	}
	h.profileTmpl = load("profile.tmpl")
	h.postTmpl = load("post.tmpl")
	h.homeTmpl = load("home.tmpl")
	h.tagTmpl = load("tag.tmpl")
}

// renderHome renders the SSR home page with all personas and their posts.
// 全ペルソナと各投稿を含むホームページをレンダリングする。
func (h *Handler) renderHome(w http.ResponseWriter, r *http.Request) {
	if h.homeTmpl == nil {
		h.serveSPA(w, r)
		return
	}

	ctx := r.Context()
	lang := i18n.DetectLang(r)
	base := h.baseURL(r)
	domain := h.domain(r)

	personas, err := h.store.ListPersonas(ctx)
	if err != nil || len(personas) == 0 {
		h.serveSPA(w, r)
		return
	}

	var homePersonas []homePersonaData
	type ssrPersonaJSON struct {
		accountJSON
		Posts []postJSON `json:"posts"`
	}
	var ssrPersonas []ssrPersonaJSON

	for _, p := range personas {
		profileURL := base + "/users/" + p.Username
		enriched, _ := h.listPublicPosts(ctx, p.ID, id.Nil, 20)
		postItems := toPostDataList(enriched, profileURL)

		displayName := p.DisplayName
		if displayName == "" {
			displayName = p.Username
		}

		homePersonas = append(homePersonas, homePersonaData{
			Username:    p.Username,
			DisplayName: displayName,
			Summary:     sanitizeHTML(h.formatBio(ctx, p.Summary)),
			AvatarURL:   h.resolveMediaURL(base, p.AvatarPath),
			HeaderURL:   h.resolveMediaURL(base, p.HeaderPath),
			ProfileURL:  profileURL,
			Fields:      toSSRFields(p.Fields()),
			Primary:     p.Primary,
			Posts:       postItems,
		})

		ssrPersonas = append(ssrPersonas, ssrPersonaJSON{
			accountJSON: h.toAccountFromPersona(r.Context(), base, p),
			Posts:       enriched,
		})
	}

	ssrJSON := buildSSRJSON(map[string]any{
		"personas": ssrPersonas,
	})

	data := &homeData{
		T:             func(key string) string { return i18n.T(lang, key) },
		Lang:          lang,
		BaseURL:       base,
		Domain:        domain,
		Title:         "murlog - " + domain,
		RobotsContent: h.robotsMetaContent(ctx),
		ThemeCSS:      "/themes/default/style.css",
		Personas:      homePersonas,
		SSRData:       ssrJSON,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.homeTmpl.Execute(w, data); err != nil {
		log.Printf("ssr: render home: %v", err)
	}
}

// renderProfile renders the SSR profile page with recent public posts.
// SSR プロフィールページを最新の公開投稿と共にレンダリングする。
func (h *Handler) renderProfile(w http.ResponseWriter, r *http.Request, persona *murlog.Persona) {
	if h.profileTmpl == nil {
		h.serveSPA(w, r)
		return
	}

	ctx := r.Context()
	lang := i18n.DetectLang(r)
	base := h.baseURL(r)
	profileURL := base + "/users/" + persona.Username

	enriched, _ := h.listPublicPosts(ctx, persona.ID, id.Nil, 20)
	postItems := toPostDataList(enriched, profileURL)

	displayName := persona.DisplayName
	if displayName == "" {
		displayName = persona.Username
	}

	description := stripHTML(persona.Summary)
	if description == "" && len(enriched) > 0 {
		description = truncate(stripHTML(enriched[0].Content), 200)
	}

	// Build SSR data JSON (same shape as API responses).
	// SSR データ JSON を構築（API レスポンスと同じ形式）。
	ssrJSON := buildSSRJSON(map[string]any{
		"persona": h.toAccountFromPersona(r.Context(), base, persona),
		"posts":   enriched,
	})

	domain := h.domain(r)
	data := &profileData{
		T:             func(key string) string { return i18n.T(lang, key) },
		Lang:          lang,
		BaseURL:       base,
		Domain:        domain,
		Username:      persona.Username,
		DisplayName:   displayName,
		Summary:       sanitizeHTML(h.formatBio(r.Context(), persona.Summary)),
		AvatarURL:     h.resolveMediaURL(base, persona.AvatarPath),
		HeaderURL:     h.resolveMediaURL(base, persona.HeaderPath),
		ProfileURL:    profileURL,
		Fields:        toSSRFields(persona.Fields()),
		Title:         "murlog - " + displayName + " " + persona.Username + "@" + domain,
		Description:   description,
		RobotsContent: h.robotsMetaContent(ctx),
		ThemeCSS:      "/themes/default/style.css",
		Posts:         postItems,
		SSRData:       ssrJSON,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.profileTmpl.Execute(w, data); err != nil {
		log.Printf("ssr: render profile: %v", err)
	}
}

// handlePost serves the post permalink page with Content Negotiation.
// Content Negotiation 付きで投稿パーマリンクページを返す。
func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	postID := r.PathValue("id")

	persona, err := h.store.GetPersonaByUsername(r.Context(), username)
	if err != nil || persona == nil {
		renderNotFound(w)
		return
	}

	pid, err := id.Parse(postID)
	if err != nil {
		renderNotFound(w)
		return
	}

	post, err := h.store.GetPost(r.Context(), pid)
	if err != nil || post == nil {
		renderNotFound(w)
		return
	}

	if post.PersonaID != persona.ID || post.Origin != "local" || post.Visibility != murlog.VisibilityPublic {
		renderNotFound(w)
		return
	}

	// Vary: Accept ensures CDN/proxies cache HTML and JSON-LD separately.
	// Vary: Accept で CDN/プロキシが HTML と JSON-LD を別々にキャッシュする。
	w.Header().Set("Vary", "Accept")

	if isActivityPubRequest(r) {
		h.renderPostActivityPub(w, r, persona, post)
		return
	}

	h.renderPostHTML(w, r, persona, post)
}

// renderPostHTML renders the SSR post permalink page.
// SSR 投稿パーマリンクページをレンダリングする。
func (h *Handler) renderPostHTML(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, post *murlog.Post) {
	if h.postTmpl == nil {
		h.serveSPA(w, r)
		return
	}

	lang := i18n.DetectLang(r)
	base := h.baseURL(r)
	profileURL := base + "/users/" + persona.Username
	permalink := profileURL + "/posts/" + post.ID.String()

	displayName := persona.DisplayName
	if displayName == "" {
		displayName = persona.Username
	}

	description := truncate(stripHTML(post.Content), 200)

	// Load attachments. / 添付ファイルを読み込む。
	atts, _ := h.store.ListAttachmentsByPost(r.Context(), post.ID)
	var ssrAtts []attachmentJSON
	var ogImage string
	for _, a := range atts {
		u := h.attachmentURL(base, a)
		ssrAtts = append(ssrAtts, attachmentJSON{URL: u, Alt: a.Alt, MimeType: a.MimeType})
		if ogImage == "" {
			ogImage = u
		}
	}

	ssrJSON := buildSSRJSON(map[string]any{
		"persona": h.toAccountFromPersona(r.Context(), base, persona),
		"post":    h.enrichPostJSON(r.Context(), post),
	})

	domain := h.domain(r)
	data := &postPageData{
		T:           func(key string) string { return i18n.T(lang, key) },
		Lang:        lang,
		BaseURL:     base,
		Domain:      domain,
		Username:    persona.Username,
		DisplayName: displayName,
		AvatarURL:   h.resolveMediaURL(base, persona.AvatarPath),
		ProfileURL:  profileURL,
		PostID:      post.ID.String(),
		Content:     sanitizeHTML(post.Content),
		Attachments: ssrAtts,
		CreatedAt:   post.CreatedAt,
		Permalink:   permalink,
		Title:       fmt.Sprintf("murlog - %s %s@%s %s", displayName, persona.Username, domain, i18n.T(lang, "public.post_title_suffix")),
		Description:   description,
		RobotsContent: h.robotsMetaContent(r.Context()),
		ThemeCSS:      "/themes/default/style.css",
		OGImage:       ogImage,
		SSRData:       ssrJSON,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.postTmpl.Execute(w, data); err != nil {
		log.Printf("ssr: render post: %v", err)
	}
}

// renderPostActivityPub returns a Note JSON-LD for the post.
// 投稿の Note JSON-LD を返す。
func (h *Handler) renderPostActivityPub(w http.ResponseWriter, r *http.Request, persona *murlog.Persona, post *murlog.Post) {
	base := h.baseURL(r)
	actorURL := base + "/users/" + persona.Username

	note := h.buildLocalNote(r.Context(), base, actorURL, post)
	note.Context = "https://www.w3.org/ns/activitystreams"

	w.Header().Set("Content-Type", "application/activity+json; charset=utf-8")
	json.NewEncoder(w).Encode(note)
}


// renderNotFound renders a minimal 404 HTML page.
// 最小限の 404 HTML ページをレンダリングする。
func renderNotFound(w http.ResponseWriter) {
	renderErrorPage(w, http.StatusNotFound, "Not Found")
}

// renderInternalError renders a minimal 500 HTML page.
// 最小限の 500 HTML ページをレンダリングする。
func renderInternalError(w http.ResponseWriter) {
	renderErrorPage(w, http.StatusInternalServerError, "Internal Server Error")
}

// renderErrorPage renders a minimal error HTML page with the given status code.
// 指定されたステータスコードで最小限のエラー HTML ページをレンダリングする。
func renderErrorPage(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%d %s</title>
<style>body{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;color:#333;background:#fff}
@media(prefers-color-scheme:dark){body{color:#ccc;background:#1a1a1a}}
.c{text-align:center}h1{font-size:48px;margin:0;opacity:.3}p{margin:8px 0 0;opacity:.6}</style>
</head><body><div class="c"><h1>%d</h1><p>%s</p></div></body></html>`, code, message, code, message)
}

// --- SSR data helpers ---

// buildSSRJSON marshals data to template.JS for safe embedding in <script>.
// <script> 内に安全に埋め込むため template.JS に変換する。
func buildSSRJSON(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("ssr: marshal ssr-data: %v", err)
		return template.JS("{}")
	}
	return template.JS(b)
}

// --- helpers ---

var reHTMLTag = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes HTML tags for plain text (OGP descriptions, etc).
// プレーンテキスト用に HTML タグを除去する。
func stripHTML(s string) string {
	return strings.TrimSpace(reHTMLTag.ReplaceAllString(s, ""))
}

// truncate truncates a string to n runes, adding "..." if truncated.
// 文字列を n ルーンで切り詰め、切り詰めた場合は "..." を付加する。
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "..."
}

// resolveMediaURL builds an absolute media URL from a relative path.
// 相対パスから絶対メディア URL を組み立てる。
func (h *Handler) resolveMediaURL(base, path string) string {
	return media.ResolveURL(h.media, base, path)
}

// tagPageData is the template data for the SSR tag page.
// SSR タグページのテンプレートデータ。
type tagPageData struct {
	T             func(string) string
	Lang          string
	BaseURL       string
	Domain        string
	Tag           string
	Title         string
	RobotsContent string
	ThemeCSS      string
	Posts         []postData
	SSRData       template.JS
}

// renderTagPage renders the SSR tag page with local public posts.
// ローカル公開投稿でタグページの SSR をレンダリングする。
func (h *Handler) renderTagPage(w http.ResponseWriter, r *http.Request, tag string) {
	if h.tagTmpl == nil {
		h.serveSPA(w, r)
		return
	}

	ctx := r.Context()
	lang := i18n.DetectLang(r)
	base := h.baseURL(r)

	posts, _ := h.store.ListPostsByHashtag(ctx, tag, id.Nil, 20, true)
	enriched := h.enrichPostJSONList(ctx, posts)

	// Build profileURL from primary persona for post permalinks.
	// 投稿パーマリンク用にプライマリペルソナから profileURL を構築。
	profileURL := base
	if persona, _ := h.primaryPersona(ctx); persona != nil {
		profileURL = base + "/users/" + persona.Username
	}
	postItems := toPostDataList(enriched, profileURL)

	ssrJSON := buildSSRJSON(map[string]any{
		"tag":   tag,
		"posts": enriched,
	})

	domain := h.domain(r)
	data := &tagPageData{
		T:             func(key string) string { return i18n.T(lang, key) },
		Lang:          lang,
		BaseURL:       base,
		Domain:        domain,
		Tag:           tag,
		Title:         "#" + tag + " - murlog",
		RobotsContent: h.robotsMetaContent(ctx),
		ThemeCSS:      "/themes/default/style.css",
		Posts:         postItems,
		SSRData:       ssrJSON,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tagTmpl.Execute(w, data); err != nil {
		log.Printf("ssr: render tag: %v", err)
	}
}
