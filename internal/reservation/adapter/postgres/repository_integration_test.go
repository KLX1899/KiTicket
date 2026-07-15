package postgres

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
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFinalizeHighContentionCreatesAtMostOneBooking(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repository, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("contention_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM reservation.bookings WHERE id LIKE $1`, prefix+"%")
	})

	const contenders = 64
	start := make(chan struct{})
	results := make(chan error, contenders)
	var group sync.WaitGroup
	for index := 0; index < contenders; index++ {
		group.Add(1)
		go func(number int) {
			defer group.Done()
			<-start
			_, finalizeErr := repository.Finalize(ctx, application.Booking{
				ID:            fmt.Sprintf("%s_booking_%d", prefix, number),
				ReservationID: fmt.Sprintf("%s_reservation_%d", prefix, number),
				ScheduleID:    "schedule_jazz_1", OwnerID: "buyer_demo", SeatIDs: []string{"seat_a1"},
			})
			results <- finalizeErr
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	successes := 0
	alreadyBooked := 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, domain.ErrAlreadyBooked):
			alreadyBooked++
		default:
			t.Fatalf("unexpected finalization result: %v", result)
		}
	}
	if successes != 1 || alreadyBooked != contenders-1 {
		t.Fatalf("successes=%d already_booked=%d, want 1/%d", successes, alreadyBooked, contenders-1)
	}
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM reservation.booked_seats
		WHERE schedule_id = 'schedule_jazz_1' AND seat_id = 'seat_a1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("durable invariant violated: got %d bookings for one schedule/seat", count)
	}
}
