package redislock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/KLX1899/KiTicket/internal/reservation/application"
	"github.com/KLX1899/KiTicket/internal/reservation/domain"
)

func TestStoreHighContentionRealRedis(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	client, err := NewClient(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping Redis: %v", err)
	}
	prefix := fmt.Sprintf("kiticket:test:%d", time.Now().UnixNano())
	store, err := New(client, prefix, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		iterator := client.Scan(context.Background(), 0, prefix+":*", 100).Iterator()
		for iterator.Next(context.Background()) {
			_ = client.Del(context.Background(), iterator.Val()).Err()
		}
	})

	const contenders = 200
	start := make(chan struct{})
	results := make(chan error, contenders)
	var group sync.WaitGroup
	for index := 0; index < contenders; index++ {
		group.Add(1)
		go func(number int) {
			defer group.Done()
			<-start
			_, acquireErr := store.Acquire(ctx, application.LockCommand{
				ReservationID: fmt.Sprintf("reservation-%d", number), ScheduleID: "schedule-1", OwnerID: fmt.Sprintf("buyer-%d", number),
				SeatIDs: []string{"seat-1", "seat-2"}, IdempotencyKey: fmt.Sprintf("request-%d", number),
				RequestHash: fmt.Sprintf("hash-%d", number), TTLMillis: time.Minute.Milliseconds(),
			})
			results <- acquireErr
		}(index)
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result == nil {
			successes++
			continue
		}
		var conflict *domain.SeatConflictError
		if !errors.As(result, &conflict) {
			t.Fatalf("unexpected result: %v", result)
		}
		if conflict.SeatID != "seat-1" {
			t.Fatalf("non-deterministic conflict: %s", conflict.SeatID)
		}
	}
	if successes != 1 {
		t.Fatalf("got %d successful acquisitions, want exactly 1", successes)
	}
}

func TestStoreIdempotencyAndStaleReleaseRealRedis(t *testing.T) {
	rawURL := os.Getenv("TEST_REDIS_URL")
	if rawURL == "" {
		t.Skip("TEST_REDIS_URL is not set")
	}
	client, err := NewClient(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store, err := New(client, fmt.Sprintf("kiticket:test:%d", time.Now().UnixNano()), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	command := application.LockCommand{
		ReservationID: "reservation-1", ScheduleID: "schedule-1", OwnerID: "buyer-1", SeatIDs: []string{"seat-1"},
		IdempotencyKey: "request-1", RequestHash: "hash-1", TTLMillis: time.Minute.Milliseconds(),
	}
	first, err := store.Acquire(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	command.ReservationID = "reservation-2"
	second, err := store.Acquire(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if first.ReservationID != second.ReservationID || !second.Replayed {
		t.Fatalf("unexpected replay: first=%+v second=%+v", first, second)
	}
	stale := domain.ReleaseRequest{ReservationID: first.ReservationID, ScheduleID: first.ScheduleID, OwnerID: first.OwnerID, SeatIDs: first.SeatIDs, Fence: first.Fence + 1}
	if err := store.Release(ctx, stale); !errors.Is(err, domain.ErrLockLost) {
		t.Fatalf("expected stale release rejection, got %v", err)
	}
	valid := stale
	valid.Fence = first.Fence
	if err := store.Validate(ctx, valid); err != nil {
		t.Fatalf("stale release removed active lock: %v", err)
	}
}
