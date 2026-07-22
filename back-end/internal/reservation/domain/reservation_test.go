package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAcquireRequestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		request   AcquireRequest
		wantSeats []string
		wantErr   bool
	}{
		{
			name:      "normalizes deterministic order",
			request:   AcquireRequest{ScheduleID: "schedule-1", OwnerID: "buyer-1", SeatIDs: []string{"seat-2", "seat-1"}, IdempotencyKey: "request-123", TTL: time.Minute},
			wantSeats: []string{"seat-1", "seat-2"},
		},
		{
			name:    "rejects duplicate seat",
			request: AcquireRequest{ScheduleID: "schedule-1", OwnerID: "buyer-1", SeatIDs: []string{"seat-1", "seat-1"}, IdempotencyKey: "request-123", TTL: time.Minute},
			wantErr: true,
		},
		{
			name:    "rejects short TTL",
			request: AcquireRequest{ScheduleID: "schedule-1", OwnerID: "buyer-1", SeatIDs: []string{"seat-1"}, IdempotencyKey: "request-123", TTL: MinimumTTL - time.Millisecond},
			wantErr: true,
		},
		{
			name:    "rejects unsafe identifier",
			request: AcquireRequest{ScheduleID: "schedule:{1}", OwnerID: "buyer-1", SeatIDs: []string{"seat-1"}, IdempotencyKey: "request-123", TTL: time.Minute},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			seats, err := test.request.Validate()
			if test.wantErr {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("expected invalid request, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(seats, test.wantSeats) {
				t.Fatalf("got seats %v, want %v", seats, test.wantSeats)
			}
		})
	}
}
