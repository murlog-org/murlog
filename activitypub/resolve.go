package activitypub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// UserAgent is the User-Agent string used for outgoing requests.
// 送信リクエストで使う User-Agent 文字列。
const UserAgent = "murlog/0.1"

// HTTPClient is the shared HTTP client for outgoing federation requests.
// SSRF 対策として、プライベート IP への接続をブロックする。
// テスト時に差し替え可能。
// 連合リクエスト用の共有 HTTP クライアント。
var HTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy:       func(*http.Request) (*url.URL, error) { return nil, nil },
		DialContext: SSRFSafeDialer,
	},
}

// SSRFSafeDialer wraps the default dialer and rejects connections to private/loopback IPs.
// デフォルト Dialer をラップし、プライベート/ループバック IP への接続を拒否する。
func SSRFSafeDialer(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ssrf: invalid address %q: %w", addr, err)
	}

	// Resolve DNS first, then check all returned IPs.
	// まず DNS 解決し、返された全 IP をチェック。
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("ssrf: resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("ssrf: no DNS records for %q", host)
	}

	for _, ip := range ips {
		if isPrivateIP(ip.IP) {
			return nil, fmt.Errorf("ssrf: blocked request to private IP %s (host: %s)", ip.IP, host)
		}
	}

	// Connect using the resolved IPs via the standard dialer.
	// 標準 Dialer で解決済み IP に接続。
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// isPrivateIP returns true if the IP is loopback, private, or link-local.
// ループバック、プライベート、リンクローカルの IP なら true を返す。
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// FetchActorSigned fetches a remote Actor by URI with HTTP Signature.
// Always use signed requests — GoToSocial requires signatures on all GET requests.
// URI でリモート Actor を署名付きで取得する。
// 常に署名付きリクエストを使用する（GTS は全 GET に署名を要求するため）。
func FetchActorSigned(uri, keyID, privateKeyPEM string) (*Actor, error) {
	return fetchActor(uri, keyID, privateKeyPEM)
}

func fetchActor(uri, keyID, privateKeyPEM string) (*Actor, error) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", UserAgent)

	// Sign the request if credentials are provided.
	if keyID != "" && privateKeyPEM != "" {
		if err := SignRequest(req, keyID, privateKeyPEM, nil); err != nil {
			return nil, fmt.Errorf("activitypub: sign fetch request: %w", err)
		}
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("activitypub: fetch actor %s: %w", uri, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("activitypub: fetch actor %s: status %d", uri, resp.StatusCode)
	}

	// Limit response body to 1 MB to prevent OOM from malicious servers.
	// 悪意あるサーバーからの OOM を防ぐためレスポンスボディを 1 MB に制限。
	var actor Actor
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&actor); err != nil {
		return nil, fmt.Errorf("activitypub: decode actor %s: %w", uri, err)
	}
	return &actor, nil
}

// FetchNoteSigned fetches a remote Note by URI with HTTP Signature.
// URI でリモート Note を署名付きで取得する。
func FetchNoteSigned(uri, keyID, privateKeyPEM string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", UserAgent)

	// Sign the request if credentials are provided.
	// 認証情報があればリクエストに署名。
	if keyID != "" && privateKeyPEM != "" {
		if err := SignRequest(req, keyID, privateKeyPEM, nil); err != nil {
			return nil, fmt.Errorf("activitypub: sign fetch request: %w", err)
		}
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("activitypub: fetch note %s: %w", uri, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("activitypub: fetch note %s: status %d", uri, resp.StatusCode)
	}

	// Limit response body to 1 MB to prevent OOM from malicious servers.
	// 悪意あるサーバーからの OOM を防ぐためレスポンスボディを 1 MB に制限。
	var obj map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&obj); err != nil {
		return nil, fmt.Errorf("activitypub: decode note %s: %w", uri, err)
	}

	// Validate that the object is a Note.
	// オブジェクトが Note であることを検証。
	objType, _ := obj["type"].(string)
	if objType != "Note" {
		return nil, fmt.Errorf("activitypub: fetch note %s: unexpected type %q", uri, objType)
	}

	return obj, nil
}

// CollectionPage holds items from a single page of a remote collection, plus the next page URL.
// リモートコレクションの1ページ分のアイテムと次ページ URL を保持する。
type CollectionPage struct {
	Items      []map[string]interface{}
	Next       string // 次ページ URL (空なら最終ページ) / next page URL (empty if last page)
	TotalItems int    // コレクション全体の件数 (-1 なら不明) / total items in collection (-1 if unknown)
}

// FetchCollectionSigned fetches a single page of a remote OrderedCollection with HTTP Signature.
// Returns items from the first (or specified) page and the "next" URL for pagination.
// リモート OrderedCollection の1ページを署名付きで取得する。
// 最初の (または指定された) ページのアイテムと次ページ URL を返す。
func FetchCollectionSigned(uri, keyID, privateKeyPEM string, limit int) (*CollectionPage, error) {
	obj, err := fetchJSONSigned(uri, keyID, privateKeyPEM)
	if err != nil {
		return nil, err
	}

	// Extract totalItems from the collection root (before following "first").
	// "first" を辿る前にコレクションルートから totalItems を取得。
	totalItems := -1
	if v, ok := obj["totalItems"].(float64); ok {
		totalItems = int(v)
	}

	// Follow "first" page only for collection roots (not page responses).
	// コレクションルートの場合のみ "first" ページを辿る (ページレスポンスでは辿らない)。
	objType, _ := obj["type"].(string)
	if objType == "OrderedCollection" || objType == "Collection" {
		if first, ok := obj["first"].(string); ok && first != "" {
			obj, err = fetchJSONSigned(first, keyID, privateKeyPEM)
			if err != nil {
				return nil, err
			}
		} else if firstObj, ok := obj["first"].(map[string]interface{}); ok {
			obj = firstObj
		}
	}

	// Extract orderedItems or items.
	// orderedItems または items を抽出。
	var rawItems []interface{}
	if items, ok := obj["orderedItems"].([]interface{}); ok {
		rawItems = items
	} else if items, ok := obj["items"].([]interface{}); ok {
		rawItems = items
	}

	if limit > 0 && len(rawItems) > limit {
		rawItems = rawItems[:limit]
	}

	var result []map[string]interface{}
	for _, item := range rawItems {
		switch v := item.(type) {
		case map[string]interface{}:
			result = append(result, v)
		case string:
			result = append(result, map[string]interface{}{"id": v})
		}
	}

	next, _ := obj["next"].(string)
	return &CollectionPage{Items: result, Next: next, TotalItems: totalItems}, nil
}

// fetchJSONSigned fetches a JSON document with HTTP Signature.
// 署名付きで JSON ドキュメントを取得する。
func fetchJSONSigned(uri, keyID, privateKeyPEM string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/activity+json, application/ld+json")
	req.Header.Set("User-Agent", UserAgent)

	if keyID != "" && privateKeyPEM != "" {
		if err := SignRequest(req, keyID, privateKeyPEM, nil); err != nil {
			return nil, fmt.Errorf("activitypub: sign request: %w", err)
		}
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("activitypub: fetch %s: %w", uri, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("activitypub: fetch %s: status %d", uri, resp.StatusCode)
	}

	var obj map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&obj); err != nil {
		return nil, fmt.Errorf("activitypub: decode %s: %w", uri, err)
	}
	return obj, nil
}
