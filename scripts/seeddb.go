// seeddb generates a SQLite DB with large dataset for performance testing.
// パフォーマンステスト用の大量データ入り SQLite DB を生成する。
//
// Usage:
//   go run scripts/seeddb.go [-posts 5000] [-followers 500] [-favs 2000] [-o seed.db]
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/murlog-org/murlog"
	"github.com/murlog-org/murlog/id"
	"github.com/murlog-org/murlog/store"
	_ "github.com/murlog-org/murlog/store/sqlite"
)

func main() {
	posts := flag.Int("posts", 5000, "number of posts")
	followers := flag.Int("followers", 500, "number of followers")
	favs := flag.Int("favs", 2000, "number of favourites")
	reblogs := flag.Int("reblogs", 1000, "number of reblogs")
	domain := flag.String("domain", "localhost", "server domain (e.g. murlog.example.com)")
	protocol := flag.String("protocol", "https", "server protocol (http or https)")
	username := flag.String("username", "alice", "persona username")
	output := flag.String("o", "seed.db", "output DB path")
	flag.Parse()

	os.Remove(*output)

	s, err := store.Open("sqlite", *output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store.Open: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Migrate: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()

	// Create persona. / ペルソナを作成。
	persona := &murlog.Persona{
		ID:            id.New(),
		Username:      *username,
		DisplayName:   *username,
		Summary:       "<p>Performance test persona</p>",
		PublicKeyPEM:  "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
		PrivateKeyPEM: "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
		Primary:       true,
		FieldsJSON:    `[{"name":"Website","value":"https://example.com"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.CreatePersona(ctx, persona); err != nil {
		fmt.Fprintf(os.Stderr, "CreatePersona: %v\n", err)
		os.Exit(1)
	}

	// Settings. / 設定。
	s.SetSetting(ctx, "setup_complete", "true")
	s.SetSetting(ctx, "domain", *domain)
	s.SetSetting(ctx, "protocol", *protocol)

	// Posts. / 投稿。
	fmt.Printf("Creating %d posts...\n", *posts)
	postIDs := make([]id.ID, *posts)
	for i := 0; i < *posts; i++ {
		pid := id.New()
		postIDs[i] = pid
		s.CreatePostBulk(ctx, &murlog.Post{
			ID: pid, PersonaID: persona.ID,
			Content:    fmt.Sprintf("<p>Post number %d. This is some sample content for performance testing.</p>", i+1),
			Visibility: murlog.VisibilityPublic, Origin: "local",
			CreatedAt: now.Add(-time.Duration(*posts-i) * time.Minute), UpdatedAt: now,
		})
		if (i+1)%1000 == 0 {
			fmt.Printf("  %d/%d posts\n", i+1, *posts)
		}
	}

	// Followers. / フォロワー。
	fmt.Printf("Creating %d followers...\n", *followers)
	for i := 0; i < *followers; i++ {
		s.CreateFollowerBulk(ctx, &murlog.Follower{
			ID: id.New(), PersonaID: persona.ID,
			ActorURI: fmt.Sprintf("https://remote%d.example/users/follower%d", i%20, i),
			Approved: true, CreatedAt: now,
		})
	}

	// Favourites. / いいね。
	fmt.Printf("Creating %d favourites...\n", *favs)
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < *favs; i++ {
		s.CreateFavourite(ctx, &murlog.Favourite{
			ID: id.New(), PostID: postIDs[rng.Intn(*posts)],
			ActorURI: fmt.Sprintf("https://remote.example/users/faver%d", i), CreatedAt: now,
		})
	}

	// Reblogs. / リブログ。
	fmt.Printf("Creating %d reblogs...\n", *reblogs)
	for i := 0; i < *reblogs; i++ {
		s.CreateReblog(ctx, &murlog.Reblog{
			ID: id.New(), PostID: postIDs[rng.Intn(*posts)],
			ActorURI: fmt.Sprintf("https://remote.example/users/reblogger%d", i), CreatedAt: now,
		})
	}

	// Refresh cached counters after bulk insert. / バルク挿入後にカウンターを一括更新。
	s.RefreshAllCounters(ctx)

	fi, _ := os.Stat(*output)
	fmt.Printf("\nDone! %s (%.1f MB)\n", *output, float64(fi.Size())/1024/1024)
	fmt.Printf("  Posts: %d, Followers: %d, Favourites: %d, Reblogs: %d\n",
		*posts, *followers, *favs, *reblogs)
}
