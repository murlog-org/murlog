package activitypub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// WebFingerResponse is the JSON Resource Descriptor (JRD) from a WebFinger endpoint.
type WebFingerResponse struct {
	Subject string          `json:"subject"`
	Links   []WebFingerLink `json:"links"`
}

// WebFingerLink is a single link in a WebFinger response.
type WebFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href,omitempty"`
}

// LookupWebFinger resolves an acct URI (user@domain) to an Actor URI via WebFinger.
// acct URI (user@domain) を WebFinger で Actor URI に解決する。
func LookupWebFinger(acct string) (string, error) {
	// Parse "user@domain" or "acct:user@domain".
	acct = strings.TrimPrefix(acct, "acct:")
	acct = strings.TrimPrefix(acct, "@")
	parts := strings.SplitN(acct, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("webfinger: invalid acct %q", acct)
	}
	domain := parts[1]

	wfURL := fmt.Sprintf("https://%s/.well-known/webfinger?resource=%s",
		domain, url.QueryEscape("acct:"+acct))

	req, err := http.NewRequest("GET", wfURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/jrd+json, application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("webfinger: request %s: %w", domain, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("webfinger: %s returned %d", domain, resp.StatusCode)
	}

	// Limit response body to 256 KB.
	// レスポンスボディを 256 KB に制限。
	var jrd WebFingerResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&jrd); err != nil {
		return "", fmt.Errorf("webfinger: decode %s: %w", domain, err)
	}

	for _, link := range jrd.Links {
		if link.Rel == "self" && (link.Type == "application/activity+json" || link.Type == "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\"") {
			if link.Href != "" {
				return link.Href, nil
			}
		}
	}

	return "", fmt.Errorf("webfinger: no ActivityPub actor link for %s", acct)
}
