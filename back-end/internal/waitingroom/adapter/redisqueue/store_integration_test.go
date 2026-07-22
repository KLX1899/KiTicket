package redisqueue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/KLX1899/KiTicket/internal/reservation/adapter/redislock"
)

func TestFairAdmissionAndReplayProtectionRealRedis(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	client, err := redislock.NewClient(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store, err := New(client)
	if err != nil {
		t.Fatal(err)
	}
	eventID := fmt.Sprintf("event-%d", time.Now().UnixNano())
	keys := store.keys(eventID)
	t.Cleanup(func() { _ = client.Del(context.Background(), keys[:]...).Err() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for index := 1; index <= 3; index++ {
		joined, err := store.Join(ctx, eventID, fmt.Sprintf("user-%d", index), fmt.Sprintf("token-%d", index), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if joined.Sequence != int64(index) {
			t.Fatalf("sequence=%d, want %d", joined.Sequence, index)
		}
		status, err := store.Status(ctx, eventID, fmt.Sprintf("user-%d", index), fmt.Sprintf("token-%d", index))
		if err != nil || status.Position != int64(index) {
			t.Fatalf("position=%d err=%v, want %d", status.Position, err, index)
		}
	}
	count, err := store.Admit(ctx, eventID, 2, 5*time.Minute)
	if err != nil || count != 2 {
		t.Fatalf("admitted=%d err=%v, want 2", count, err)
	}
	for index := 1; index <= 2; index++ {
		status, err := store.Status(ctx, eventID, fmt.Sprintf("user-%d", index), fmt.Sprintf("token-%d", index))
		if err != nil || status.State != "admitted" {
			t.Fatalf("user %d status=%+v err=%v", index, status, err)
		}
	}
	third, err := store.Status(ctx, eventID, "user-3", "token-3")
	if err != nil || third.State != "queued" || third.Position != 1 {
		t.Fatalf("third user status=%+v err=%v", third, err)
	}
	replayed, err := store.Consume(ctx, eventID, "token-1", "command-a")
	if err != nil || replayed {
		t.Fatalf("first consume replayed=%v err=%v", replayed, err)
	}
	replayed, err = store.Consume(ctx, eventID, "token-1", "command-a")
	if err != nil || !replayed {
		t.Fatalf("idempotent consume replayed=%v err=%v", replayed, err)
	}
	if _, err := store.Consume(ctx, eventID, "token-1", "command-b"); !errors.Is(err, ErrAdmissionReplay) {
		t.Fatalf("expected cross-command replay rejection, got %v", err)
	}
}
