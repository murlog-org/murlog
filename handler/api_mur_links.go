package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/murlog-org/murlog/activitypub"
	"golang.org/x/net/html"
)

type linkPreviewParams struct {
	URL string `json:"url"`
}

type linkPreviewJSON struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
}

// linkPreviewClient is a dedicated HTTP client for OGP fetching with short timeout.
// SSRF 対策 + httpoxy 対策として Proxy 無効化 + プライベート IP ブロック。
// OGP 取得専用の短タイムアウト HTTP クライアント。
var linkPreviewClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		Proxy:       func(*http.Request) (*url.URL, error) { return nil, nil },
		DialContext: activitypub.SSRFSafeDialer,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

func (h *Handler) rpcLinksPreview(ctx context.Context, params json.RawMessage) (any, *rpcErr) {
	req, rErr := parseParams[linkPreviewParams](params)
	if rErr != nil {
		return nil, rErr
	}
	if req.URL == "" {
		return nil, newRPCErr(codeInvalidParams, "url is required")
	}
	if !strings.HasPrefix(req.URL, "https://") && !strings.HasPrefix(req.URL, "http://") {
		return nil, newRPCErr(codeInvalidParams, "url must be http or https")
	}

	preview, err := fetchOGP(ctx, req.URL)
	if err != nil {
		return nil, newRPCErr(codeInternalError, "fetch failed")
	}
	return preview, nil
}

// fetchOGP fetches a URL and extracts Open Graph meta tags.
// URL を取得して Open Graph メタタグを抽出する。
func fetchOGP(ctx context.Context, url string) (*linkPreviewJSON, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "murlog/1.0 (link preview)")
	httpReq.Header.Set("Accept", "text/html")

	resp, err := linkPreviewClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	// Limit read to 256KB — only need <head> section.
	// <head> セクションのみ必要なので 256KB に制限。
	body := io.LimitReader(resp.Body, 256*1024)

	preview := &linkPreviewJSON{URL: url}
	tokenizer := html.NewTokenizer(body)
	inHead := false

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			return preview, nil
		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := tokenizer.TagName()
			tag := string(tn)

			if tag == "head" {
				inHead = true
				continue
			}
			if tag == "body" {
				return preview, nil
			}

			if inHead && tag == "meta" && hasAttr {
				var property, name, content string
				for {
					key, val, more := tokenizer.TagAttr()
					k := string(key)
					v := string(val)
					switch k {
					case "property":
						property = v
					case "name":
						name = v
					case "content":
						content = v
					}
					if !more {
						break
					}
				}
				switch property {
				case "og:title":
					preview.Title = content
				case "og:description":
					preview.Description = content
				case "og:image":
					preview.Image = content
				case "og:site_name":
					preview.SiteName = content
				}
				// Fallback to <meta name="description"> if no og:description.
				// og:description がなければ <meta name="description"> にフォールバック。
				if name == "description" && preview.Description == "" {
					preview.Description = content
				}
			}

			// Fallback: <title> tag. / フォールバック: <title> タグ。
			if inHead && tag == "title" && preview.Title == "" {
				tokenizer.Next()
				preview.Title = strings.TrimSpace(tokenizer.Token().Data)
			}
		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			if string(tn) == "head" {
				return preview, nil
			}
		}
	}
}
