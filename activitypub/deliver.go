package activitypub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Deliver sends an activity to a remote inbox.
// Activity をリモートの Inbox に送信する。
//
// keyID: the signing key identifier (e.g. "https://example.com/users/alice#main-key")
// privateKeyPEM: RSA private key in PEM format for signing
// inboxURL: the remote actor's inbox URL
// activity: the activity to send
func Deliver(keyID, privateKeyPEM, inboxURL string, activity interface{}) error {
	body, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("activitypub: marshal activity: %w", err)
	}

	req, err := http.NewRequest("POST", inboxURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("activitypub: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/activity+json")
	req.Header.Set("User-Agent", UserAgent)

	// Sign the request. / リクエストに署名する。
	if err := SignRequest(req, keyID, privateKeyPEM, body); err != nil {
		return fmt.Errorf("activitypub: sign request: %w", err)
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("activitypub: deliver to %s: %w", inboxURL, err)
	}
	defer resp.Body.Close()

	// 2xx is success. / 2xx なら成功。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Include response body prefix for debugging.
		// デバッグ用にレスポンスボディ先頭を含める。
		peek, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("activitypub: deliver to %s: status %d: %s", inboxURL, resp.StatusCode, string(peek))
	}

	return nil
}
