// Package worker processes background jobs from the queue.
// キューからバックグラウンドジョブを処理するパッケージ。
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/activitypub"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/internal/mediautil"
	"github.com/murlog-org/murlog/media"
	"github.com/murlog-org/murlog/hashtag"
	"github.com/murlog-org/murlog/mention"
	"github.com/murlog-org/murlog/queue"
	"github.com/murlog-org/murlog/store"
)

const (
	// PollInterval is the time between queue polls when idle.
	// アイドル時のキューポーリング間隔。
	PollInterval = 5 * time.Second

	// DefaultMinConcurrency is the default baseline for adaptive concurrency.
	// アダプティブ並列度のデフォルト下限。
	DefaultMinConcurrency = 8

	// DefaultMaxConcurrency is the default upper limit for adaptive concurrency.
	// アダプティブ並列度のデフォルト上限。
	DefaultMaxConcurrency = 32

	// JobTimeout is the maximum duration for a single job execution.
	// 1ジョブの最大実行時間。
	JobTimeout = 60 * time.Second

	// DefaultJobRetentionDays is the default number of days to keep completed jobs.
	// 完了ジョブのデフォルト保持日数。
	DefaultJobRetentionDays = 7
)

// Worker processes jobs from the queue.
// キューからジョブを取得して処理するワーカー。
type Worker struct {
	queue            queue.Queue
	store            store.Store
	media            media.Store
	minConcurrency   int
	maxConcurrency   int
	jobRetentionDays int
}

// New creates a new Worker. minC/maxC/retentionDays of 0 use defaults.
// ワーカーを生成する。minC/maxC/retentionDays が 0 ならデフォルト値を使用。
func New(q queue.Queue, s store.Store, m media.Store, minC, maxC, retentionDays int) *Worker {
	if minC <= 0 {
		minC = DefaultMinConcurrency
	}
	if maxC <= 0 {
		maxC = DefaultMaxConcurrency
	}
	if maxC < minC {
		maxC = minC
	}
	if retentionDays <= 0 {
		retentionDays = DefaultJobRetentionDays
	}
	return &Worker{queue: q, store: s, media: m, minConcurrency: minC, maxConcurrency: maxC, jobRetentionDays: retentionDays}
}

// Run starts the worker loop with adaptive concurrent job processing.
// Blocks until ctx is cancelled. Concurrency scales with queue pressure.
// アダプティブ並列ジョブ処理でワーカーループを開始する。
// ctx がキャンセルされるまでブロック。キュー圧に応じて並列度をスケール。
// concurrencyReevalInterval is the number of jobs between concurrency re-evaluations.
// 並列度を再評価するジョブ間隔。
const concurrencyReevalInterval = 10

func (w *Worker) Run(ctx context.Context) {
	concurrency := w.minConcurrency
	sem := make(chan struct{}, concurrency)
	log.Printf("worker: started (concurrency=%d)", concurrency)
	var wg sync.WaitGroup
	sinceLastEval := 0

	for {
		job, err := w.queue.Claim(ctx)
		if err != nil {
			log.Printf("worker: claim error: %v", err)
		}

		if job != nil {
			// Re-evaluate concurrency every N jobs (catches fan-out bursts).
			// N件ごとに並列度を再評価（fan-out バーストを検知）。
			sinceLastEval++
			if sinceLastEval >= concurrencyReevalInterval {
				sinceLastEval = 0
				newC := w.decideConcurrency(ctx)
				if newC != concurrency {
					// H3: 全 goroutine 完了後にセマフォを差し替える。
					// H3: Wait for all goroutines to finish before replacing semaphore.
					wg.Wait()
					log.Printf("worker: concurrency %d → %d", concurrency, newC)
					concurrency = newC
					sem = make(chan struct{}, concurrency)
				}
			}

			sem <- struct{}{} // acquire semaphore slot
			wg.Add(1)
			go func(j *murlog.QueueJob) {
				defer wg.Done()
				defer func() { <-sem }()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("worker: panic processing job %s (%s): %v", j.ID, j.Type, r)
						// M12: 親 ctx がキャンセル済みでも DB 書き込みできるよう Background を使用。
						// M12: Use Background context to ensure DB write succeeds even if parent ctx is cancelled.
						bgCtx, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer bgCancel()
						w.queue.Fail(bgCtx, j.ID, time.Now().Add(30*time.Second), fmt.Sprintf("panic: %v", r))
					}
				}()
				w.process(ctx, j)
			}(job)
			continue // check for more jobs immediately
		}

		// No jobs — cleanup, re-evaluate concurrency, and wait before polling again.
		// ジョブなし — クリーンアップ、並列度再評価、ポーリング待ち。
		sinceLastEval = 0
		w.cleanup(ctx)

		newC := w.decideConcurrency(ctx)
		if newC != concurrency {
			// H3: 全 goroutine 完了後にセマフォを差し替える。
			// H3: Wait for all goroutines to finish before replacing semaphore.
			wg.Wait()
			log.Printf("worker: concurrency %d → %d", concurrency, newC)
			concurrency = newC
			sem = make(chan struct{}, concurrency)
		}

		select {
		case <-ctx.Done():
			wg.Wait()
			log.Println("worker: stopped")
			return
		case <-time.After(PollInterval):
		}
	}
}

// RunOnce claims and processes a single job, then returns.
// Returns true if a job was processed, false if the queue was empty.
// ジョブを1件だけ取得・処理して戻る。処理したら true、空なら false。
func (w *Worker) RunOnce(ctx context.Context) bool {
	job, err := w.queue.Claim(ctx)
	if err != nil {
		log.Printf("worker: claim error: %v", err)
		return false
	}
	if job == nil {
		return false
	}
	w.process(ctx, job)
	return true
}

// RunBatch processes pending jobs concurrently up to the given limit and timeout.
// Concurrency scales adaptively based on queue pressure.
// Returns the number of jobs processed.
// 件数制限・時間制限付きでジョブを並列処理する。
// キュー圧に応じて並列度をアダプティブにスケール。処理した件数を返す。
func (w *Worker) RunBatch(ctx context.Context, limit int, timeout time.Duration) int {
	// Recover stale running jobs (stuck due to CGI timeout or crash).
	// CGI タイムアウトやクラッシュで running のまま放置されたジョブを回復。
	w.queue.RecoverStale(ctx, 5*time.Minute)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	concurrency := w.decideConcurrency(ctx)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var processed atomic.Int32

	for int(processed.Load()) < limit {
		if ctx.Err() != nil {
			break
		}
		job, _ := w.queue.Claim(ctx)
		if job == nil {
			break
		}

		processed.Add(1)
		sem <- struct{}{} // acquire semaphore slot
		wg.Add(1)
		go func(j *murlog.QueueJob) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("worker: panic processing job %s (%s): %v", j.ID, j.Type, r)
					// M12: 親 ctx がキャンセル済みでも DB 書き込みできるよう Background を使用。
					// M12: Use Background context to ensure DB write succeeds even if parent ctx is cancelled.
					bgCtx, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer bgCancel()
					w.queue.Fail(bgCtx, j.ID, time.Now().Add(30*time.Second), fmt.Sprintf("panic: %v", r))
				}
			}()
			w.process(ctx, j)
		}(job)
	}
	wg.Wait()
	w.cleanup(ctx)
	return int(processed.Load())
}

// cleanup deletes old completed jobs, expired sessions, and old read notifications.
// 古い完了ジョブ、期限切れセッション、古い既読通知を削除する。
func (w *Worker) cleanup(ctx context.Context) {
	w.queue.Cleanup(ctx, time.Now().Add(-time.Duration(w.jobRetentionDays)*24*time.Hour))
	w.store.DeleteExpiredSessions(ctx)
	w.store.CleanupNotifications(ctx, time.Now().Add(-90*24*time.Hour))
}

// decideConcurrency returns the appropriate concurrency level based on queue pressure.
// キュー圧に応じた並列度を返す。
func (w *Worker) decideConcurrency(ctx context.Context) int {
	stats, err := w.queue.Stats(ctx)
	if err != nil {
		return w.minConcurrency
	}
	mid := (w.minConcurrency + w.maxConcurrency) / 2
	switch {
	case stats.Pending >= 500:
		return w.maxConcurrency
	case stats.Pending >= 100:
		return mid
	default:
		return w.minConcurrency
	}
}

func (w *Worker) process(ctx context.Context, job *murlog.QueueJob) {
	// Per-job timeout to prevent a single slow delivery from blocking indefinitely.
	// 1ジョブごとのタイムアウトで、遅い配送による無期限ブロックを防ぐ。
	ctx, cancel := context.WithTimeout(ctx, JobTimeout)
	defer cancel()

	var err error
	switch job.Type {
	case murlog.JobAcceptFollow:
		err = w.handleAcceptFollow(ctx, job)
	case murlog.JobRejectFollow:
		err = w.handleRejectFollow(ctx, job)
	case murlog.JobDeliverPost:
		err = w.handleDeliverPost(ctx, job)
	case murlog.JobDeliverNote:
		err = w.handleDeliverNote(ctx, job)
	case murlog.JobUpdatePost:
		err = w.handleUpdatePost(ctx, job)
	case murlog.JobDeliverUpdateNote:
		err = w.handleDeliverUpdateNote(ctx, job)
	case murlog.JobSendFollow:
		err = w.handleSendFollow(ctx, job)
	case murlog.JobUpdateActor:
		err = w.handleUpdateActor(ctx, job)
	case murlog.JobDeliverUpdate:
		err = w.handleDeliverUpdate(ctx, job)
	case murlog.JobDeliverDelete:
		err = w.handleDeliverDelete(ctx, job)
	case murlog.JobDeliverDeleteNote:
		err = w.handleDeliverDeleteNote(ctx, job)
	case murlog.JobSendUndoFollow:
		err = w.handleSendUndoFollow(ctx, job)
	case murlog.JobSendLike:
		err = w.handleSendLike(ctx, job)
	case murlog.JobSendUndoLike:
		err = w.handleSendUndoLike(ctx, job)
	case murlog.JobSendAnnounce:
		err = w.handleSendAnnounce(ctx, job)
	case murlog.JobSendUndoAnnounce:
		err = w.handleSendUndoAnnounce(ctx, job)
	case murlog.JobDeliverAnnounce:
		err = w.handleDeliverAnnounce(ctx, job)
	case murlog.JobSendBlock:
		err = w.handleSendBlock(ctx, job)
	case murlog.JobSendUndoBlock:
		err = w.handleSendUndoBlock(ctx, job)
	case murlog.JobFetchRemoteActor:
		err = w.handleFetchRemoteActor(ctx, job)
	default:
		log.Printf("worker: unknown job type %q (id=%s)", job.Type, job.ID)
		// Complete unknown jobs to avoid infinite retries.
		w.queue.Complete(ctx, job.ID)
		return
	}

	if err != nil {
		log.Printf("worker: job %s (%s) failed (attempt %d): %v", job.ID, job.Type, job.Attempts, err)
		// Track domain-level failures for circuit breaker.
		// サーキットブレーカー用にドメイン別の失敗を記録。
		if domain := domainFromPayload(job.Payload); domain != "" {
			w.store.IncrementDomainFailure(ctx, domain, err.Error())
		}
		if job.Attempts >= murlog.MaxJobAttempts {
			log.Printf("worker: job %s (%s) max attempts reached, giving up", job.ID, job.Type)
			w.queue.Dead(ctx, job.ID, fmt.Sprintf("max attempts (%d) reached: %v", murlog.MaxJobAttempts, err))
			return
		}
		// Exponential backoff: 30s, 2m, 8m, 32m
		delay := time.Duration(30<<(job.Attempts-1)) * time.Second
		w.queue.Fail(ctx, job.ID, time.Now().Add(delay), err.Error())
		return
	}

	// Reset domain failure counter on success.
	// 成功時にドメイン失敗カウンターをリセット。
	if domain := domainFromPayload(job.Payload); domain != "" {
		w.store.ResetDomainFailure(ctx, domain)
	}

	w.queue.Complete(ctx, job.ID)
}

// accept_follow payload
type acceptFollowPayload struct {
	PersonaID  string `json:"persona_id"`
	ActivityID string `json:"activity_id"`
	ActorURI   string `json:"actor_uri"`
}

// parsePayload decodes a job's JSON payload into the given type.
// ジョブの JSON ペイロードを指定の型にデコードする。
func parsePayload[T any](job *murlog.QueueJob) (T, error) {
	var v T
	if err := json.Unmarshal([]byte(job.Payload), &v); err != nil {
		return v, fmt.Errorf("parse payload: %w", err)
	}
	return v, nil
}

func (w *Worker) handleAcceptFollow(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[acceptFollowPayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	// Resolve remote actor to get inbox URL.
	// リモート Actor を取得して Inbox URL を得る。
	actor, err := w.resolveActor(ctx, p.ActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor: %w", err)
	}

	actorURI, keyID := w.actorContext(ctx, persona)

	accept := activitypub.NewActivity(fmt.Sprintf("%s#accepts/%s", actorURI, job.ID), "Accept", actorURI, p.ActivityID)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, accept)
}

func (w *Worker) handleRejectFollow(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[acceptFollowPayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	// Resolve remote actor to get inbox URL.
	// リモート Actor を取得して Inbox URL を得る。
	actor, err := w.resolveActor(ctx, p.ActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor: %w", err)
	}

	actorURI, keyID := w.actorContext(ctx, persona)

	reject := activitypub.NewActivity(fmt.Sprintf("%s#rejects/%s", actorURI, job.ID), "Reject", actorURI, p.ActivityID)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, reject)
}

// deliver_post payload — fan-out job.
// フォロワー一覧を取得し、各フォロワーに deliver_note ジョブを enqueue する。
type deliverPostPayload struct {
	PostID string `json:"post_id"`
}

func (w *Worker) handleDeliverPost(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[deliverPostPayload](job)
	if err != nil {
		return err
	}

	postID, err := id.Parse(p.PostID)
	if err != nil {
		return fmt.Errorf("parse post_id: %w", err)
	}

	post, err := w.store.GetPost(ctx, postID)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}

	// Track actor URIs already queued for delivery to avoid duplicates.
	// 重複配送を防ぐため、既に enqueue 済みの Actor URI を追跡。
	delivered := make(map[string]bool)

	// Render text content to HTML before mention processing.
	// メンション処理前にテキストコンテンツを HTML に変換。
	if post.ContentType == murlog.ContentTypeText {
		base := w.baseURL(ctx)
		post.Content = renderTextToHTML(post.Content, base)
		if len(post.ContentMap) > 0 {
			for lang, text := range post.ContentMap {
				post.ContentMap[lang] = renderTextToHTML(text, base)
			}
		}
	}

	// Resolve mentions: parse @user@domain, lookup actors, replace with HTML links.
	// メンション解決: @user@domain をパースし、Actor を検索、HTML リンクに変換。
	mentionAccts := mention.ParseMentions(post.Content)
	// Limit mention resolution to prevent resource exhaustion from crafted posts.
	// 細工された投稿からのリソース枯渇を防ぐためメンション解決数を制限。
	const maxMentionResolve = 20
	if len(mentionAccts) > maxMentionResolve {
		mentionAccts = mentionAccts[:maxMentionResolve]
	}
	if len(mentionAccts) > 0 {
		base := w.baseURL(ctx)
		localHost := ""
		if u, err := url.Parse(base); err == nil {
			localHost = u.Host
		}

		resolved := make(map[string]mention.Resolved)
		for _, acct := range mentionAccts {
			// Skip self-mentions. / 自己メンションをスキップ。
			parts := strings.SplitN(acct, "@", 2)
			if len(parts) == 2 && parts[1] == localHost {
				continue
			}

			// Try local DB cache first, then WebFinger + fetch.
			// まずローカル DB キャッシュを試み、なければ WebFinger + fetch。
			actor, err := w.resolveActorByAcct(ctx, acct)
			if err != nil {
				log.Printf("worker: resolve mention %s: %v", acct, err)
				continue
			}

			profileURL := actor.URI
			if actor.Acct != "" {
				// Build a human-readable profile URL.
				// 人間が読めるプロフィール URL を組み立てる。
				if u, err := url.Parse(actor.URI); err == nil {
					profileURL = u.Scheme + "://" + u.Host + "/@" + actor.Username
				}
			}
			resolved[acct] = mention.Resolved{
				Acct:       acct,
				ActorURI:   actor.URI,
				ProfileURL: profileURL,
			}
		}

		if len(resolved) > 0 {
			// Replace @user@domain with HTML links in content.
			// コンテンツ中の @user@domain を HTML リンクに置換。
			post.Content = mention.ReplaceWithHTML(post.Content, resolved)

			// Also update contentMap if present.
			// contentMap があればそちらも更新。
			if len(post.ContentMap) > 0 {
				for lang, text := range post.ContentMap {
					post.ContentMap[lang] = mention.ReplaceWithHTML(text, resolved)
				}
			}

			// Store resolved mentions on the post.
			// 解決済みメンションを投稿に保存。
			mentions := make([]murlog.Mention, 0, len(resolved))
			for _, rm := range resolved {
				mentions = append(mentions, murlog.Mention{Acct: rm.Acct, Href: rm.ActorURI})
			}
			post.SetMentions(mentions)

			// Enqueue delivery to each mentioned actor.
			// メンション先の各 Actor に配送ジョブを enqueue。
			for _, rm := range resolved {
				delivered[rm.ActorURI] = true
				w.queue.Enqueue(ctx, murlog.NewJob(murlog.JobDeliverNote, map[string]string{
						"post_id":   p.PostID,
						"actor_uri": rm.ActorURI,
					}))
			}

			// Save only mentions_json to DB (don't overwrite plain text content with rendered HTML).
			// DB には mentions_json だけ保存 (レンダリング済み HTML でプレーンテキストを上書きしない)。
			w.store.UpdatePostMentions(ctx, post.ID, post.MentionsJSON)
		}
	}

	// If this is a reply to a remote post, also deliver to the reply target's actor.
	// リモート投稿へのリプライなら、リプライ先の Actor にも配送。
	if post.InReplyToURI != "" {
		if parent, err := w.store.GetPostByURI(ctx, post.InReplyToURI); err == nil && parent.Origin == "remote" && parent.ActorURI != "" {
			if !delivered[parent.ActorURI] {
				delivered[parent.ActorURI] = true
				w.queue.Enqueue(ctx, murlog.NewJob(murlog.JobDeliverNote, map[string]string{
						"post_id":   p.PostID,
						"actor_uri": parent.ActorURI,
					}))
			}
		}
	}

	return w.fanoutToFollowers(ctx, post.PersonaID, func(f *murlog.Follower) *murlog.QueueJob {
		if delivered[f.ActorURI] {
			return nil
		}
		return murlog.NewJob(murlog.JobDeliverNote, map[string]string{
				"post_id":   p.PostID,
				"actor_uri": f.ActorURI,
			})
	})
}

// resolveActorByAcct resolves a remote actor by acct (user@domain).
// First checks the local DB cache, then falls back to WebFinger + fetch.
// acct (user@domain) でリモート Actor を解決する。まずローカル DB キャッシュを確認し、なければ WebFinger + fetch。
func (w *Worker) resolveActorByAcct(ctx context.Context, acct string) (*murlog.RemoteActor, error) {
	// Check DB cache by acct. / acct で DB キャッシュを確認。
	cached, err := w.store.GetRemoteActorByAcct(ctx, acct)
	if err == nil && cached.Inbox != "" && time.Since(cached.FetchedAt) < 24*time.Hour {
		return cached, nil
	}

	// WebFinger to get actor URI. / WebFinger で Actor URI を取得。
	actorURI, err := activitypub.LookupWebFinger(acct)
	if err != nil {
		return nil, fmt.Errorf("webfinger %s: %w", acct, err)
	}

	// Fetch and cache the actor (resolveActor handles caching).
	// Actor を取得してキャッシュ (resolveActor がキャッシュを処理)。
	return w.resolveActor(ctx, actorURI)
}

// deliver_note payload — single delivery job.
// 1フォロワーに対して Note (Create Activity) を配送する。
type deliverNotePayload struct {
	PostID   string `json:"post_id"`
	ActorURI string `json:"actor_uri"`
}

func (w *Worker) handleDeliverNote(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[deliverNotePayload](job)
	if err != nil {
		return err
	}

	postID, err := id.Parse(p.PostID)
	if err != nil {
		return fmt.Errorf("parse post_id: %w", err)
	}

	post, err := w.store.GetPost(ctx, postID)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, post.PersonaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.ActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.ActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"
	postURI := base + "/users/" + persona.Username + "/posts/" + post.ID.String()

	// Build Note object — set to/cc based on visibility.
	// 公開範囲に応じて to/cc を設定。
	var to, cc []string
	switch post.Visibility {
	case murlog.VisibilityFollowers:
		to = []string{actorURI + "/followers"}
	case murlog.VisibilityUnlisted:
		to = []string{actorURI + "/followers"}
		cc = []string{"https://www.w3.org/ns/activitystreams#Public"}
	default: // Public
		to = []string{"https://www.w3.org/ns/activitystreams#Public"}
		cc = []string{actorURI + "/followers"}
	}


	note := activitypub.Note{
		Context:      "https://www.w3.org/ns/activitystreams",
		ID:           postURI,
		Type:         "Note",
		AttributedTo: actorURI,
		InReplyTo:    post.InReplyToURI,
		Content:      post.Content,
		ContentMap:   post.ContentMap,
		Summary:      post.Summary,
		Sensitive:    post.Sensitive,
		Published:    post.CreatedAt.UTC().Format(time.RFC3339),
		To:           to,
		CC:           cc,
	}

	// Add mention tags and cc entries from resolved mentions.
	// 解決済みメンションから tag と cc を追加。
	mentions := post.Mentions()
	for _, m := range mentions {
		note.Tag = append(note.Tag, activitypub.NoteTag{
			Type: "Mention",
			Href: m.Href,
			Name: "@" + m.Acct,
		})
		note.CC = append(note.CC, m.Href)
	}

	// Add hashtag tags from resolved hashtags.
	// 解決済みハッシュタグから tag を追加。
	for _, tag := range post.Hashtags() {
		note.Tag = append(note.Tag, activitypub.NoteTag{
			Type: "Hashtag",
			Href: base + "/tags/" + tag,
			Name: "#" + tag,
		})
	}

	// Attach media if present. / メディアがあれば添付。
	atts, _ := w.store.ListAttachmentsByPost(ctx, postID)
	for _, a := range atts {
		note.Attachment = append(note.Attachment, activitypub.NoteAttachment{
			Type:      "Document",
			MediaType: a.MimeType,
			URL:       w.resolveMediaURL(base, a.FilePath),
			Name:      a.Alt,
			Width:     a.Width,
			Height:    a.Height,
		})
	}

	create := activitypub.NewActivity(postURI+"/activity", "Create", actorURI, note)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, create)
}

// update_post payload — fan-out job.
// フォロワー一覧を取得し、各フォロワーに deliver_update_note ジョブを enqueue する。
type updatePostPayload struct {
	PostID string `json:"post_id"`
}

func (w *Worker) handleUpdatePost(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[updatePostPayload](job)
	if err != nil {
		return err
	}

	postID, err := id.Parse(p.PostID)
	if err != nil {
		return fmt.Errorf("parse post_id: %w", err)
	}

	post, err := w.store.GetPost(ctx, postID)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}

	// Track actor URIs already queued for delivery to avoid duplicates.
	// 重複配送を防ぐため、既に enqueue 済みの Actor URI を追跡。
	delivered := make(map[string]bool)

	// Deliver to mentioned actors. / メンション先の Actor に配送。
	for _, m := range post.Mentions() {
		delivered[m.Href] = true
		w.queue.Enqueue(ctx, murlog.NewJob(murlog.JobDeliverUpdateNote, map[string]string{
				"post_id":   p.PostID,
				"actor_uri": m.Href,
			}))
	}

	// If this is a reply to a remote post, also deliver to the reply target's actor.
	// リモート投稿へのリプライなら、リプライ先の Actor にも配送。
	if post.InReplyToURI != "" {
		if parent, err := w.store.GetPostByURI(ctx, post.InReplyToURI); err == nil && parent.Origin == "remote" && parent.ActorURI != "" {
			if !delivered[parent.ActorURI] {
				delivered[parent.ActorURI] = true
				w.queue.Enqueue(ctx, murlog.NewJob(murlog.JobDeliverUpdateNote, map[string]string{
						"post_id":   p.PostID,
						"actor_uri": parent.ActorURI,
					}))
			}
		}
	}

	return w.fanoutToFollowers(ctx, post.PersonaID, func(f *murlog.Follower) *murlog.QueueJob {
		if delivered[f.ActorURI] {
			return nil
		}
		return murlog.NewJob(murlog.JobDeliverUpdateNote, map[string]string{
				"post_id":   p.PostID,
				"actor_uri": f.ActorURI,
			})
	})
}

// deliver_update_note payload — single delivery job.
// 1フォロワーに対して Update Note を配送する。
type deliverUpdateNotePayload struct {
	PostID   string `json:"post_id"`
	ActorURI string `json:"actor_uri"`
}

func (w *Worker) handleDeliverUpdateNote(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[deliverUpdateNotePayload](job)
	if err != nil {
		return err
	}

	postID, err := id.Parse(p.PostID)
	if err != nil {
		return fmt.Errorf("parse post_id: %w", err)
	}

	post, err := w.store.GetPost(ctx, postID)
	if err != nil {
		return fmt.Errorf("get post: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, post.PersonaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.ActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.ActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"
	postURI := base + "/users/" + persona.Username + "/posts/" + post.ID.String()

	// Build Note object — set to/cc based on visibility.
	// 公開範囲に応じて to/cc を設定。
	var to, cc []string
	switch post.Visibility {
	case murlog.VisibilityFollowers:
		to = []string{actorURI + "/followers"}
	case murlog.VisibilityUnlisted:
		to = []string{actorURI + "/followers"}
		cc = []string{"https://www.w3.org/ns/activitystreams#Public"}
	default: // Public
		to = []string{"https://www.w3.org/ns/activitystreams#Public"}
		cc = []string{actorURI + "/followers"}
	}

	// Render text content to HTML for AP delivery.
	// AP 配送用にテキストコンテンツを HTML に変換。
	noteContent := post.Content
	noteContentMap := post.ContentMap
	if post.ContentType == murlog.ContentTypeText {
		noteContent = renderTextToHTML(post.Content, w.baseURL(ctx))
		if len(post.ContentMap) > 0 {
			noteContentMap = make(map[string]string, len(post.ContentMap))
			for lang, text := range post.ContentMap {
				noteContentMap[lang] = renderTextToHTML(text, w.baseURL(ctx))
			}
		}
	}

	note := activitypub.Note{
		Context:      "https://www.w3.org/ns/activitystreams",
		ID:           postURI,
		Type:         "Note",
		AttributedTo: actorURI,
		InReplyTo:    post.InReplyToURI,
		Content:      noteContent,
		ContentMap:   noteContentMap,
		Summary:      post.Summary,
		Sensitive:    post.Sensitive,
		Published:    post.CreatedAt.UTC().Format(time.RFC3339),
		Updated:      post.UpdatedAt.UTC().Format(time.RFC3339),
		To:           to,
		CC:           cc,
	}

	// Add mention tags and cc entries from resolved mentions.
	// 解決済みメンションから tag と cc を追加。
	for _, m := range post.Mentions() {
		note.Tag = append(note.Tag, activitypub.NoteTag{
			Type: "Mention",
			Href: m.Href,
			Name: "@" + m.Acct,
		})
		note.CC = append(note.CC, m.Href)
	}

	// Add hashtag tags from resolved hashtags.
	// 解決済みハッシュタグから tag を追加。
	for _, tag := range post.Hashtags() {
		note.Tag = append(note.Tag, activitypub.NoteTag{
			Type: "Hashtag",
			Href: base + "/tags/" + tag,
			Name: "#" + tag,
		})
	}

	// Attach media if present. / メディアがあれば添付。
	atts, _ := w.store.ListAttachmentsByPost(ctx, postID)
	for _, a := range atts {
		note.Attachment = append(note.Attachment, activitypub.NoteAttachment{
			Type:      "Document",
			MediaType: a.MimeType,
			URL:       w.resolveMediaURL(base, a.FilePath),
			Name:      a.Alt,
			Width:     a.Width,
			Height:    a.Height,
		})
	}

	update := activitypub.NewActivity(postURI+"/update", "Update", actorURI, note)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, update)
}

// send_follow payload
type sendFollowPayload struct {
	FollowID string `json:"follow_id"`
}

func (w *Worker) handleSendFollow(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendFollowPayload](job)
	if err != nil {
		return err
	}

	followID, err := id.Parse(p.FollowID)
	if err != nil {
		return fmt.Errorf("parse follow_id: %w", err)
	}

	follow, err := w.store.GetFollow(ctx, followID)
	if err != nil {
		return fmt.Errorf("get follow: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, follow.PersonaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, follow.TargetURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", follow.TargetURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	followActivity := activitypub.NewActivity(actorURI+"#follows/"+follow.ID.String(), "Follow", actorURI, follow.TargetURI)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, followActivity)
}

// actorContext returns the actorURI and keyID for a persona.
// ペルソナの actorURI と keyID を返す。
func (w *Worker) actorContext(ctx context.Context, persona *murlog.Persona) (actorURI, keyID string) {
	actorURI = w.baseURL(ctx) + "/users/" + persona.Username
	keyID = actorURI + "#main-key"
	return
}

// baseURL returns the base URL (e.g. "https://example.com") from DB settings.
func (w *Worker) baseURL(ctx context.Context) string {
	protocol := "https"
	if p, _ := w.store.GetSetting(ctx, "protocol"); p == "http" {
		protocol = "http"
	}
	domain, _ := w.store.GetSetting(ctx, "domain")
	return protocol + "://" + domain
}

// resolveMediaURL returns the absolute URL for a media file.
// メディアファイルの絶対 URL を返す。
func (w *Worker) resolveMediaURL(base, path string) string {
	return media.ResolveURL(w.media, base, path)
}

// fanoutPageSize is the number of followers to process per page in fan-out jobs.
// ファンアウトジョブで1ページあたりに処理するフォロワー数。
const fanoutPageSize = 100

// fanoutToFollowers creates jobs for each follower and enqueues them in batches.
// makeJob returns nil to skip a follower (e.g. already delivered).
// 各フォロワーのジョブを生成し、バッチで一括 enqueue する。
// makeJob が nil を返すとそのフォロワーをスキップする。
func (w *Worker) fanoutToFollowers(ctx context.Context, personaID id.ID, makeJob func(f *murlog.Follower) *murlog.QueueJob) error {
	// Cache dead domain checks per fan-out to avoid repeated DB queries.
	// fan-out ごとに dead ドメインチェックをキャッシュし、DB クエリの繰り返しを避ける。
	deadCache := make(map[string]bool)
	isDead := func(domain string) bool {
		if v, ok := deadCache[domain]; ok {
			return v
		}
		dead, _ := w.store.IsDomainDead(ctx, domain)
		deadCache[domain] = dead
		return dead
	}

	cursor := id.Nil
	for {
		page, err := w.store.ListFollowersPaged(ctx, personaID, cursor, fanoutPageSize)
		if err != nil {
			return fmt.Errorf("list followers: %w", err)
		}
		var batch []*murlog.QueueJob
		for _, f := range page {
			// Skip dead domains (circuit breaker).
			// dead ドメインをスキップ（サーキットブレーカー）。
			if domain := domainFromActorURI(f.ActorURI); domain != "" && isDead(domain) {
				continue
			}
			if job := makeJob(f); job != nil {
				batch = append(batch, job)
			}
		}
		if len(batch) > 0 {
			if err := w.queue.EnqueueBatch(ctx, batch); err != nil {
				return fmt.Errorf("enqueue batch: %w", err)
			}
		}
		if len(page) < fanoutPageSize {
			break
		}
		cursor = page[len(page)-1].ID
	}
	return nil
}

// domainFromPayload extracts the target domain from a job payload.
// Checks actor_uri, target_actor_uri, target_uri fields in order.
// ジョブ payload から宛先ドメインを抽出する。
func domainFromPayload(payload string) string {
	var p map[string]string
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return ""
	}
	uri := p["actor_uri"]
	if uri == "" {
		uri = p["target_actor_uri"]
	}
	if uri == "" {
		uri = p["target_uri"]
	}
	if uri == "" {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// domainFromActorURI extracts the hostname from an actor URI.
// Actor URI からホスト名を抽出する。
func domainFromActorURI(actorURI string) string {
	u, err := url.Parse(actorURI)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// resolveActor fetches a remote actor, using the DB cache when fresh (24h).
// If force is true, bypasses the cache TTL check.
// リモート Actor を取得する。DB キャッシュが新鮮なら使う。force=true でキャッシュをバイパス。
func (w *Worker) resolveActor(ctx context.Context, uri string, force ...bool) (*murlog.RemoteActor, error) {
	// Check cache (fresh within 24h), unless force fetch.
	if len(force) == 0 || !force[0] {
		cached, err := w.store.GetRemoteActor(ctx, uri)
		if err == nil && cached.Inbox != "" && time.Since(cached.FetchedAt) < 24*time.Hour {
			return cached, nil
		}
	}

	// Get primary persona for signing.
	personas, _ := w.store.ListPersonas(ctx)
	if len(personas) == 0 {
		return nil, fmt.Errorf("no persona available for signing")
	}
	base := w.baseURL(ctx)
	keyID := base + "/users/" + personas[0].Username + "#main-key"
	actor, err := activitypub.FetchActorSigned(uri, keyID, personas[0].PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	// Build acct from actor URI domain + preferredUsername.
	// Actor URI のドメインと preferredUsername から acct を組み立てる。
	acct := actorAcct(actor.PreferredUsername, uri)

	// Cache it.
	ra := &murlog.RemoteActor{
		URI:          actor.ID,
		Username:     actor.PreferredUsername,
		DisplayName:  actor.Name,
		Summary:      actor.Summary,
		Inbox:        actor.Inbox,
		AvatarURL:    mediautil.ResolveActorIcon(actor),
		HeaderURL:    mediautil.ResolveActorHeader(actor),
		FieldsJSON:   murlog.MustJSON(activitypub.ResolveActorFields(actor)),
		Acct:         acct,
		FetchedAt:    time.Now(),
	}
	w.store.UpsertRemoteActor(ctx, ra)
	return ra, nil
}

// actorAcct builds "user@domain" from a username and actor URI.
// username と Actor URI から "user@domain" を組み立てる。
func actorAcct(username, uri string) string {
	if username == "" {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil || u.Host == "" {
		return ""
	}
	return username + "@" + u.Host
}

// update_actor payload — fan-out job.
// フォロワー一覧を取得し、各フォロワーに deliver_update ジョブを enqueue する。
type updateActorPayload struct {
	PersonaID string `json:"persona_id"`
}

func (w *Worker) handleUpdateActor(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[updateActorPayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	return w.fanoutToFollowers(ctx, personaID, func(f *murlog.Follower) *murlog.QueueJob {
		return murlog.NewJob(murlog.JobDeliverUpdate, map[string]string{
				"persona_id": p.PersonaID,
				"actor_uri":  f.ActorURI,
			})
	})
}

// deliver_update payload — single delivery job.
// 1フォロワーに対して Update Actor を配送する。
type deliverUpdatePayload struct {
	PersonaID string `json:"persona_id"`
	ActorURI  string `json:"actor_uri"`
}

func (w *Worker) handleDeliverUpdate(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[deliverUpdatePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.ActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.ActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	actorObj := activitypub.BuildLocalActor(persona, base, func(path string) string {
		return w.resolveMediaURL(base, path)
	})

	update := activitypub.NewActivity(actorURI + "#updates/" + job.ID.String(), "Update", actorURI, actorObj)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, update)
}

// deliver_delete payload — fan-out job for post deletion.
// 投稿削除のファンアウトジョブ。フォロワーごとに deliver_delete_note をキューに追加。
type deliverDeletePayload struct {
	PersonaID string `json:"persona_id"`
	PostID    string `json:"post_id"`
}

func (w *Worker) handleDeliverDelete(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[deliverDeletePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	return w.fanoutToFollowers(ctx, personaID, func(f *murlog.Follower) *murlog.QueueJob {
		return murlog.NewJob(murlog.JobDeliverDeleteNote, map[string]string{
				"persona_id": p.PersonaID,
				"post_id":    p.PostID,
				"actor_uri":  f.ActorURI,
			})
	})
}

// deliver_delete_note payload — single Delete Activity delivery.
// 1フォロワーに対して Delete Activity を配送する。
type deliverDeleteNotePayload struct {
	PersonaID string `json:"persona_id"`
	PostID    string `json:"post_id"`
	ActorURI  string `json:"actor_uri"`
}

func (w *Worker) handleDeliverDeleteNote(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[deliverDeleteNotePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.ActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.ActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"
	postURI := base + "/users/" + persona.Username + "/posts/" + p.PostID

	delete := activitypub.NewActivity(postURI + "#delete", "Delete", actorURI, postURI)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, delete)
}

// send_undo_follow payload — send Undo Follow to remote actor.
// リモート Actor に Undo Follow を送信する。
type sendUndoFollowPayload struct {
	PersonaID string `json:"persona_id"`
	FollowID  string `json:"follow_id"`
	TargetURI string `json:"target_uri"`
}

func (w *Worker) handleSendUndoFollow(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendUndoFollowPayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.TargetURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.TargetURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	undo := activitypub.NewUndoActivity(actorURI+"#undo-follow/"+p.FollowID, actorURI, "Follow", actorURI+"#follows/"+p.FollowID, p.TargetURI)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, undo)
}

// send_like payload — send Like to remote actor.
// リモート Actor に Like を送信する。
type sendLikePayload struct {
	PersonaID      string `json:"persona_id"`
	PostURI        string `json:"post_uri"`
	TargetActorURI string `json:"target_actor_uri"`
}

func (w *Worker) handleSendLike(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendLikePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.TargetActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.TargetActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	like := activitypub.NewActivity(actorURI+"#likes/"+job.ID.String(), "Like", actorURI, p.PostURI)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, like)
}

// send_undo_like payload — send Undo Like to remote actor.
// リモート Actor に Undo Like を送信する。
type sendUndoLikePayload struct {
	PersonaID      string `json:"persona_id"`
	PostURI        string `json:"post_uri"`
	TargetActorURI string `json:"target_actor_uri"`
}

func (w *Worker) handleSendUndoLike(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendUndoLikePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.TargetActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.TargetActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	undo := activitypub.NewUndoActivity(actorURI+"#undo-like/"+job.ID.String(), actorURI, "Like", actorURI+"#likes/"+job.ID.String(), p.PostURI)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, undo)
}

// send_announce payload — send Announce to all followers + post author.
// 全フォロワーと投稿者に Announce を送信する。
type sendAnnouncePayload struct {
	PersonaID      string `json:"persona_id"`
	PostURI        string `json:"post_uri"`
	TargetActorURI string `json:"target_actor_uri"`
}

func (w *Worker) handleSendAnnounce(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendAnnouncePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	// Track actor URIs already queued to avoid duplicates.
	// 重複配送を防ぐため、既に enqueue 済みの Actor URI を追跡。
	delivered := make(map[string]bool)

	// Enqueue delivery to post author. / 投稿者への配送をキューに追加。
	if p.TargetActorURI != "" {
		delivered[p.TargetActorURI] = true
		w.queue.Enqueue(ctx, murlog.NewJob(murlog.JobDeliverAnnounce, map[string]string{
				"persona_id": p.PersonaID,
				"post_uri":   p.PostURI,
				"actor_uri":  p.TargetActorURI,
				"activity":   "Announce",
			}))
	}

	// Enqueue delivery to each follower. / 各フォロワーへの配送をキューに追加。
	return w.fanoutToFollowers(ctx, personaID, func(f *murlog.Follower) *murlog.QueueJob {
		if delivered[f.ActorURI] {
			return nil
		}
		delivered[f.ActorURI] = true
		return murlog.NewJob(murlog.JobDeliverAnnounce, map[string]string{
				"persona_id": p.PersonaID,
				"post_uri":   p.PostURI,
				"actor_uri":  f.ActorURI,
				"activity":   "Announce",
			})
	})
}

// deliver_announce payload — single Announce/Undo Announce delivery.
// 1アクターに対して Announce または Undo Announce を配送する。
type deliverAnnouncePayload struct {
	PersonaID string `json:"persona_id"`
	PostURI   string `json:"post_uri"`
	ActorURI  string `json:"actor_uri"`
	Activity  string `json:"activity"` // "Announce" or "Undo"
}

func (w *Worker) handleDeliverAnnounce(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[deliverAnnouncePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.ActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.ActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	var activity interface{}
	if p.Activity == "Undo" {
		activity = activitypub.NewUndoActivity(actorURI+"#undo-announce/"+job.ID.String(), actorURI, "Announce", actorURI+"#announces/"+job.ID.String(), p.PostURI)
	} else {
		activity = activitypub.NewActivity(actorURI+"#announces/"+job.ID.String(), "Announce", actorURI, p.PostURI)
	}

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, activity)
}

// send_undo_announce payload — send Undo Announce to all followers + post author.
// 全フォロワーと投稿者に Undo Announce を送信する。
type sendUndoAnnouncePayload struct {
	PersonaID      string `json:"persona_id"`
	PostURI        string `json:"post_uri"`
	TargetActorURI string `json:"target_actor_uri"`
}

func (w *Worker) handleSendUndoAnnounce(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendUndoAnnouncePayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	delivered := make(map[string]bool)

	if p.TargetActorURI != "" {
		delivered[p.TargetActorURI] = true
		w.queue.Enqueue(ctx, murlog.NewJob(murlog.JobDeliverAnnounce, map[string]string{
				"persona_id": p.PersonaID,
				"post_uri":   p.PostURI,
				"actor_uri":  p.TargetActorURI,
				"activity":   "Undo",
			}))
	}

	return w.fanoutToFollowers(ctx, personaID, func(f *murlog.Follower) *murlog.QueueJob {
		if delivered[f.ActorURI] {
			return nil
		}
		delivered[f.ActorURI] = true
		return murlog.NewJob(murlog.JobDeliverAnnounce, map[string]string{
				"persona_id": p.PersonaID,
				"post_uri":   p.PostURI,
				"actor_uri":  f.ActorURI,
				"activity":   "Undo",
			})
	})
}

// send_block payload — send Block to remote actor.
// リモート Actor に Block を送信する。
type sendBlockPayload struct {
	PersonaID      string `json:"persona_id"`
	TargetActorURI string `json:"target_actor_uri"`
}

func (w *Worker) handleSendBlock(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendBlockPayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.TargetActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.TargetActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	block := activitypub.NewActivity(actorURI + "#blocks/" + job.ID.String(), "Block", actorURI, p.TargetActorURI)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, block)
}

// send_undo_block payload — send Undo Block to remote actor.
// リモート Actor に Undo Block を送信する。
type sendUndoBlockPayload struct {
	PersonaID      string `json:"persona_id"`
	TargetActorURI string `json:"target_actor_uri"`
}

func (w *Worker) handleSendUndoBlock(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[sendUndoBlockPayload](job)
	if err != nil {
		return err
	}

	personaID, err := id.Parse(p.PersonaID)
	if err != nil {
		return fmt.Errorf("parse persona_id: %w", err)
	}

	persona, err := w.store.GetPersona(ctx, personaID)
	if err != nil {
		return fmt.Errorf("get persona: %w", err)
	}

	actor, err := w.resolveActor(ctx, p.TargetActorURI)
	if err != nil {
		return fmt.Errorf("resolve actor %s: %w", p.TargetActorURI, err)
	}

	base := w.baseURL(ctx)
	actorURI := base + "/users/" + persona.Username
	keyID := actorURI + "#main-key"

	undo := activitypub.NewUndoActivity(actorURI+"#undo-block/"+job.ID.String(), actorURI, "Block", actorURI+"#blocks/"+job.ID.String(), p.TargetActorURI)

	return activitypub.Deliver(keyID, persona.PrivateKeyPEM, actor.Inbox, undo)
}

// fetch_remote_actor payload — fetch and cache a remote actor by URI.
// リモート Actor を URI でフェッチしてキャッシュする。
type fetchRemoteActorPayload struct {
	ActorURI string `json:"actor_uri"`
}

func (w *Worker) handleFetchRemoteActor(ctx context.Context, job *murlog.QueueJob) error {
	p, err := parsePayload[fetchRemoteActorPayload](job)
	if err != nil {
		return err
	}
	if p.ActorURI == "" {
		return nil
	}

	// Skip if already cached with avatar (avoid redundant fetches).
	// アバター付きでキャッシュ済みなら冗長なフェッチをスキップ。
	if cached, err := w.store.GetRemoteActor(ctx, p.ActorURI); err == nil && cached.AvatarURL != "" {
		return nil
	}

	// Force fetch (bypass 24h cache). / 強制フェッチ (24hキャッシュをバイパス)。
	_, err = w.resolveActor(ctx, p.ActorURI, true)
	return err
}

// renderTextToHTML converts plain text to HTML for AP delivery.
// Same logic as handler.formatPostContent but avoids circular import.
// プレーンテキストを AP 配送用 HTML に変換する。
var urlReWorker = regexp.MustCompile(`https?://[^\s<>"]+`)

func renderTextToHTML(text string, baseURL string) string {
	escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(text)
	linked := urlReWorker.ReplaceAllStringFunc(escaped, func(rawURL string) string {
		trimmed := strings.TrimRight(rawURL, ".,;:!?)")
		suffix := rawURL[len(trimmed):]
		return `<a href="` + trimmed + `" rel="nofollow noopener" target="_blank">` + trimmed + `</a>` + suffix
	})
	withTags := hashtag.ReplaceWithHTML(linked, baseURL)
	return "<p>" + strings.ReplaceAll(withTags, "\n", "<br>") + "</p>"
}
