package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KLX1899/KiTicket/internal/platform/auth"
	"github.com/KLX1899/KiTicket/internal/reservation/domain"
)

func TestAcquireHighContentionAllowsOneOwner(t *testing.T) {
	store := newMemoryLockStore()
	service, err := New(store, nil, &sequenceIDs{})
	if err != nil {
		t.Fatal(err)
	}

	const contenders = 256
	start := make(chan struct{})
	results := make(chan error, contenders)
	var group sync.WaitGroup
	for contender := 0; contender < contenders; contender++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			owner := fmt.Sprintf("buyer-%d", index)
			_, acquireErr := service.Acquire(context.Background(), auth.Principal{UserID: owner, Role: "buyer"}, domain.AcquireRequest{
				ScheduleID: "schedule-1", OwnerID: owner, SeatIDs: []string{"seat-2", "seat-1"},
				IdempotencyKey: fmt.Sprintf("request-%d", index), TTL: time.Minute,
			})
			results <- acquireErr
		}(contender)
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		default:
			var conflict *domain.SeatConflictError
			if !errors.As(result, &conflict) {
				t.Fatalf("unexpected result: %v", result)
			}
			conflicts++
		}
	}
	if successes != 1 || conflicts != contenders-1 {
		t.Fatalf("successes=%d conflicts=%d, want 1/%d", successes, conflicts, contenders-1)
	}
	if len(store.locks) != 2 {
		t.Fatalf("partial multi-seat acquisition detected: %d locks", len(store.locks))
	}
}

func TestAcquireIsIdempotentAndRejectsKeyReuse(t *testing.T) {
	store := newMemoryLockStore()
	service, err := New(store, nil, &sequenceIDs{})
	if err != nil {
		t.Fatal(err)
	}
	principal := auth.Principal{UserID: "buyer-1", Role: "buyer"}
	request := domain.AcquireRequest{ScheduleID: "schedule-1", OwnerID: "buyer-1", SeatIDs: []string{"seat-1"}, IdempotencyKey: "request-1", TTL: time.Minute}
	first, err := service.Acquire(context.Background(), principal, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Acquire(context.Background(), principal, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ReservationID != second.ReservationID || !second.Replayed {
		t.Fatalf("unexpected replay: first=%+v second=%+v", first, second)
	}
	request.SeatIDs = []string{"seat-2"}
	if _, err := service.Acquire(context.Background(), principal, request); !errors.Is(err, domain.ErrIdempotencyMismatch) {
		t.Fatalf("expected idempotency mismatch, got %v", err)
	}
}

func TestReleaseRejectsStaleFence(t *testing.T) {
	store := newMemoryLockStore()
	service, err := New(store, nil, &sequenceIDs{})
	if err != nil {
		t.Fatal(err)
	}
	principal := auth.Principal{UserID: "buyer-1", Role: "buyer"}
	lock, err := service.Acquire(context.Background(), principal, domain.AcquireRequest{
		ScheduleID: "schedule-1", OwnerID: "buyer-1", SeatIDs: []string{"seat-1"}, IdempotencyKey: "request-1", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = service.Release(context.Background(), principal, domain.ReleaseRequest{
		ReservationID: lock.ReservationID, ScheduleID: lock.ScheduleID, OwnerID: lock.OwnerID,
		SeatIDs: lock.SeatIDs, Fence: lock.Fence + 1,
	})
	if !errors.Is(err, domain.ErrLockLost) {
		t.Fatalf("expected stale release rejection, got %v", err)
	}
	if len(store.locks) != 1 {
		t.Fatal("stale release removed the active lock")
	}
}

type sequenceIDs struct{ value atomic.Uint64 }

func (s *sequenceIDs) New() (string, error) { return fmt.Sprintf("id-%d", s.value.Add(1)), nil }

type memoryLockStore struct {
	mu    sync.Mutex
	locks map[string]domain.Lock
	idem  map[string]idemResult
	fence int64
}

type idemResult struct {
	hash string
	lock domain.Lock
}

func newMemoryLockStore() *memoryLockStore {
	return &memoryLockStore{locks: make(map[string]domain.Lock), idem: make(map[string]idemResult)}
}

func (m *memoryLockStore) Acquire(_ context.Context, command LockCommand) (domain.Lock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idemKey := command.ScheduleID + "\x00" + command.OwnerID + "\x00" + command.IdempotencyKey
	if saved, ok := m.idem[idemKey]; ok {
		if saved.hash != command.RequestHash {
			return domain.Lock{}, domain.ErrIdempotencyMismatch
		}
		result := saved.lock
		result.Replayed = true
		return result, nil
	}
	for _, seat := range command.SeatIDs {
		if _, exists := m.locks[command.ScheduleID+"\x00"+seat]; exists {
			return domain.Lock{}, &domain.SeatConflictError{SeatID: seat}
		}
	}
	m.fence++
	lock := domain.Lock{
		ReservationID: command.ReservationID, ScheduleID: command.ScheduleID, OwnerID: command.OwnerID,
		SeatIDs: append([]string(nil), command.SeatIDs...), Fence: m.fence,
		ExpiresAt: time.Now().Add(time.Duration(command.TTLMillis) * time.Millisecond),
	}
	for _, seat := range command.SeatIDs {
		m.locks[command.ScheduleID+"\x00"+seat] = lock
	}
	m.idem[idemKey] = idemResult{hash: command.RequestHash, lock: lock}
	return lock, nil
}

func (m *memoryLockStore) Validate(_ context.Context, request domain.ReleaseRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.validate(request)
}

func (m *memoryLockStore) Release(_ context.Context, request domain.ReleaseRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.validate(request); err != nil {
		return err
	}
	for _, seat := range request.SeatIDs {
		delete(m.locks, request.ScheduleID+"\x00"+seat)
	}
	return nil
}

func (m *memoryLockStore) validate(request domain.ReleaseRequest) error {
	for _, seat := range request.SeatIDs {
		lock, ok := m.locks[request.ScheduleID+"\x00"+seat]
		if !ok || lock.ReservationID != request.ReservationID || lock.OwnerID != request.OwnerID || lock.Fence != request.Fence {
			return domain.ErrLockLost
		}
	}
	return nil
}
