package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuyerPurchaseWorkflow(t *testing.T) {
	identityURL := os.Getenv("E2E_IDENTITY_URL")
	catalogURL := os.Getenv("E2E_CATALOG_URL")
	reservationURL := os.Getenv("E2E_RESERVATION_URL")
	checkoutURL := os.Getenv("E2E_CHECKOUT_URL")
	waitingRoomURL := os.Getenv("E2E_WAITING_ROOM_URL")
	if baseURL := os.Getenv("E2E_BASE_URL"); baseURL != "" {
		identityURL, catalogURL, reservationURL, checkoutURL = baseURL, baseURL, baseURL, baseURL
		waitingRoomURL = baseURL
	}
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if identityURL == "" || catalogURL == "" || reservationURL == "" || checkoutURL == "" || waitingRoomURL == "" || databaseURL == "" {
		t.Skip("E2E service URLs and TEST_POSTGRES_URL are required")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	suffix := fmt.Sprint(time.Now().UnixNano())
	email := "buyer-" + suffix + "@example.test"

	var session struct {
		AccessToken string `json:"access_token"`
		User        struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	requestJSON(t, client, http.MethodPost, identityURL+"/v1/auth/register", "", "", map[string]any{
		"email": email, "display_name": "E2E Buyer", "password": "correct-horse-battery-staple",
	}, http.StatusCreated, &session)
	if session.AccessToken == "" || session.User.ID == "" {
		t.Fatal("registration did not return an access token and user")
	}

	var joined struct {
		QueueToken string `json:"queue_token"`
	}
	requestJSON(t, client, http.MethodPost, waitingRoomURL+"/v1/waiting-room/event_jazz/join", session.AccessToken, "", nil, http.StatusCreated, &joined)
	if joined.QueueToken == "" {
		t.Fatal("waiting-room join did not return a queue token")
	}
	admissionToken := ""
	deadline := time.Now().Add(5 * time.Second)
	for admissionToken == "" && time.Now().Before(deadline) {
		var status struct {
			State          string `json:"state"`
			AdmissionToken string `json:"admission_token"`
		}
		requestJSON(t, client, http.MethodPost, waitingRoomURL+"/v1/waiting-room/status", session.AccessToken, "", map[string]any{"queue_token": joined.QueueToken}, http.StatusOK, &status)
		admissionToken = status.AdmissionToken
		if admissionToken == "" {
			timer := time.NewTimer(50 * time.Millisecond)
			<-timer.C
		}
	}
	if admissionToken == "" {
		t.Fatal("waiting-room admission did not complete before deadline")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var orderID, bookingID string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if orderID == "" {
			_ = pool.QueryRow(cleanupCtx, `SELECT id, coalesce(booking_id, '') FROM checkout.orders WHERE buyer_id = $1 ORDER BY created_at DESC LIMIT 1`, session.User.ID).Scan(&orderID, &bookingID)
		}
		if orderID != "" {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM notification.deliveries WHERE event_id IN (SELECT id FROM messaging.outbox WHERE aggregate_id = $1)`, orderID)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM messaging.inbox WHERE event_id IN (SELECT id FROM messaging.outbox WHERE aggregate_id = $1)`, orderID)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM messaging.outbox WHERE aggregate_id = $1`, orderID)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM checkout.tickets WHERE order_id = $1`, orderID)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM checkout.payments WHERE order_id = $1`, orderID)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM checkout.idempotency_keys WHERE resource_id = $1`, orderID)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM checkout.orders WHERE id = $1`, orderID)
		}
		if bookingID != "" {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM reservation.bookings WHERE id = $1`, bookingID)
		}
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM identity.users WHERE id = $1`, session.User.ID)
	})

	var discovery struct {
		Items []struct {
			ScheduleID        string `json:"schedule_id"`
			AvailableSeats    int    `json:"available_seats"`
			AvailabilityScope string `json:"availability_scope"`
		} `json:"items"`
	}
	requestJSON(t, client, http.MethodGet, catalogURL+"/v1/events?q=Jazz&available_only=true", "", "", nil, http.StatusOK, &discovery)
	if len(discovery.Items) == 0 || discovery.Items[0].ScheduleID != "schedule_jazz_1" || discovery.Items[0].AvailableSeats < 1 {
		t.Fatalf("seed event was not discoverable: %+v", discovery)
	}

	var lock struct {
		ReservationID string   `json:"reservation_id"`
		ScheduleID    string   `json:"schedule_id"`
		SeatIDs       []string `json:"seat_ids"`
		Fence         int64    `json:"fence"`
	}
	requestJSON(t, client, http.MethodPost, reservationURL+"/v1/reservation-locks", session.AccessToken, "reserve-"+suffix, map[string]any{
		"event_id": "event_jazz", "schedule_id": "schedule_jazz_1", "seat_ids": []string{"seat_b4"}, "ttl_seconds": 300,
	}, http.StatusCreated, &lock, map[string]string{"X-Admission-Token": admissionToken})
	if lock.ReservationID == "" || lock.Fence <= 0 {
		t.Fatalf("invalid reservation lock: %+v", lock)
	}

	checkoutBody := map[string]any{
		"reservation_id": lock.ReservationID, "reservation_fence": lock.Fence,
		"schedule_id": lock.ScheduleID, "seat_ids": lock.SeatIDs,
	}
	var purchase struct {
		Order struct {
			ID        string `json:"id"`
			State     string `json:"state"`
			BookingID string `json:"booking_id"`
		} `json:"order"`
		Tickets []struct {
			ID        string `json:"id"`
			QRPayload string `json:"qr_payload"`
		} `json:"tickets"`
	}
	idempotencyKey := "checkout-" + suffix
	requestJSON(t, client, http.MethodPost, checkoutURL+"/v1/checkouts", session.AccessToken, idempotencyKey, checkoutBody, http.StatusCreated, &purchase)
	orderID = purchase.Order.ID
	bookingID = purchase.Order.BookingID
	if purchase.Order.State != "completed" || bookingID == "" || len(purchase.Tickets) != 1 || purchase.Tickets[0].QRPayload == "" {
		t.Fatalf("purchase did not complete: %+v", purchase)
	}

	var replay struct {
		Order   struct{ ID, State string } `json:"order"`
		Tickets []struct {
			QRPayload string `json:"qr_payload"`
		} `json:"tickets"`
	}
	requestJSON(t, client, http.MethodPost, checkoutURL+"/v1/checkouts", session.AccessToken, idempotencyKey, checkoutBody, http.StatusCreated, &replay)
	if replay.Order.ID != orderID || replay.Order.State != "completed" || len(replay.Tickets) != 1 {
		t.Fatalf("checkout replay was not idempotent: %+v", replay)
	}

	var verification struct {
		Valid    bool   `json:"valid"`
		TicketID string `json:"ticket_id"`
	}
	requestJSON(t, client, http.MethodPost, checkoutURL+"/v1/tickets/verify", "", "", map[string]any{
		"qr_payload": purchase.Tickets[0].QRPayload,
	}, http.StatusOK, &verification)
	if !verification.Valid || verification.TicketID != purchase.Tickets[0].ID {
		t.Fatalf("ticket verification failed: %+v", verification)
	}

	// Prove the transactional outbox was broker-confirmed, consumed exactly once,
	// and handed to the asynchronous development provider before test cleanup.
	notificationDeadline := time.Now().Add(5 * time.Second)
	for {
		var sent int
		err := pool.QueryRow(ctx, `
			SELECT count(*) FROM notification.deliveries d
			JOIN messaging.outbox o ON o.id = d.event_id
			WHERE o.aggregate_id = $1 AND o.event_type = 'ticket.issued' AND o.published_at IS NOT NULL AND d.state = 'sent'`, orderID).Scan(&sent)
		if err != nil {
			t.Fatal(err)
		}
		if sent == 1 {
			break
		}
		if time.Now().After(notificationDeadline) {
			t.Fatal("asynchronous ticket notification was not delivered before deadline")
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-timer.C:
		}
	}
}

func requestJSON(t *testing.T, client *http.Client, method, endpoint, token, idempotencyKey string, body any, wantStatus int, target any, extraHeaders ...map[string]string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "e2e-"+fmt.Sprint(time.Now().UnixNano()))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	for _, headers := range extraHeaders {
		for key, value := range headers {
			request.Header.Set(key, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s returned %d, want %d: %s", method, endpoint, response.StatusCode, wantStatus, responseBody)
	}
	if target != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, target); err != nil {
			t.Fatalf("decode %s %s: %v; body=%s", method, endpoint, err, responseBody)
		}
	}
}
