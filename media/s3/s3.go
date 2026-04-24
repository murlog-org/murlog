package s3

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/murlog-org/murlog/media"
)

// Options holds configuration for the S3-compatible media store.
// S3 互換メディアストアの設定。
type Options struct {
	Bucket    string // バケット名 / Bucket name
	Region    string // リージョン / Region (e.g. "auto" for R2)
	Endpoint  string // カスタムエンドポイント / Custom endpoint (e.g. "https://xxx.r2.cloudflarestorage.com")
	AccessKey string // アクセスキー / Access key
	SecretKey string // シークレットキー / Secret key
	PublicURL string // 公開 URL ベース / Public URL base (e.g. "https://pub-xxx.r2.dev")
}

type s3Store struct {
	bucket    string
	region    string
	endpoint  string
	accessKey string
	secretKey string
	publicURL string
	client    *http.Client
}

// New creates an S3-compatible media store.
// S3 互換メディアストアを生成する。
func New(opts Options) media.Store {
	endpoint := strings.TrimRight(opts.Endpoint, "/")
	publicURL := strings.TrimRight(opts.PublicURL, "/")
	return &s3Store{
		bucket:    opts.Bucket,
		region:    opts.Region,
		endpoint:  endpoint,
		accessKey: opts.AccessKey,
		secretKey: opts.SecretKey,
		publicURL: publicURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				// httpoxy 対策: CGI 環境での HTTP_PROXY 環境変数を無視。
				// httpoxy mitigation: ignore HTTP_PROXY env in CGI.
				Proxy: func(*http.Request) (*url.URL, error) { return nil, nil },
			},
		},
	}
}

// objectURL returns the S3 API URL for an object.
// オブジェクトの S3 API URL を返す。
func (s *s3Store) objectURL(name string) string {
	return s.endpoint + "/" + s.bucket + "/" + name
}

// Save uploads a file to S3.
// ファイルを S3 にアップロードする。
func (s *s3Store) Save(name string, r io.Reader) error {
	body, err := readBody(r)
	if err != nil {
		return fmt.Errorf("s3: read body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, s.objectURL(name), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("s3: create request: %w", err)
	}

	// Detect content type from extension. / 拡張子から Content-Type を推定。
	req.Header.Set("Content-Type", detectContentType(name))
	signWithBody(req, s.accessKey, s.secretKey, s.region, "s3", body)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("s3: put object: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("s3: put object: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// Open downloads a file from S3.
// S3 からファイルをダウンロードする。
func (s *s3Store) Open(name string) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, s.objectURL(name), nil)
	if err != nil {
		return nil, fmt.Errorf("s3: create request: %w", err)
	}

	signBodyless(req, s.accessKey, s.secretKey, s.region, "s3")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3: get object: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("s3: get object: status %d", resp.StatusCode)
	}
	// M14: Close を resp.Body に委譲し、TCP コネクションリークを防止。
	// M14: Delegate Close to resp.Body to prevent TCP connection leaks.
	return &limitedReadCloser{
		Reader: io.LimitReader(resp.Body, maxUploadSize),
		Closer: resp.Body,
	}, nil
}

// limitedReadCloser wraps a LimitReader with the original body's Close.
// LimitReader を元のボディの Close でラップする。
type limitedReadCloser struct {
	io.Reader
	io.Closer
}

// Delete removes a file from S3.
// S3 からファイルを削除する。
func (s *s3Store) Delete(name string) error {
	req, err := http.NewRequest(http.MethodDelete, s.objectURL(name), nil)
	if err != nil {
		return fmt.Errorf("s3: create request: %w", err)
	}

	signBodyless(req, s.accessKey, s.secretKey, s.region, "s3")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("s3: delete object: %w", err)
	}
	defer resp.Body.Close()

	// S3 returns 204 No Content on successful delete.
	// S3 は削除成功時に 204 を返す。
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("s3: delete object: status %d", resp.StatusCode)
	}
	return nil
}

// URL returns the public URL for a media file.
// メディアファイルの公開 URL を返す。
func (s *s3Store) URL(name string) string {
	return s.publicURL + "/" + name
}

// detectContentType returns the MIME type based on file extension.
// ファイル拡張子から MIME タイプを返す。
func detectContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".gif"):
		return "image/gif"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
