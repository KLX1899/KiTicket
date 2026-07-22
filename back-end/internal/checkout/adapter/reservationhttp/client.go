// Package reservationhttp calls the reservation service's idempotent internal API.
package reservationhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/KLX1899/KiTicket/internal/checkout/application"
)

type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

func New(baseURL string, secret []byte, client *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || len(secret) < 32 || client == nil {
		return nil, errors.New("reservation client requires URL, internal secret, and HTTP client")
	}
	return &Client{baseURL: baseURL, secret: string(secret), http: client}, nil
}

func (c *Client) Finalize(ctx context.Context, reservation application.Reservation) (application.Booking, error) {
	var booking application.Booking
	if err := c.call(ctx, reservation, "finalize", &booking); err != nil {
		return application.Booking{}, err
	}
	if booking.ID == "" {
		return application.Booking{}, errors.New("reservation service returned an empty booking ID")
	}
	return booking, nil
}

func (c *Client) Release(ctx context.Context, reservation application.Reservation) error {
	return c.call(ctx, reservation, "release", nil)
}

func (c *Client) call(ctx context.Context, reservation application.Reservation, action string, target any) error {
	body, err := json.Marshal(map[string]any{
		"buyer_id": reservation.BuyerID, "schedule_id": reservation.ScheduleID,
		"seat_ids": reservation.SeatIDs, "fence": reservation.Fence,
	})
	if err != nil {
		return fmt.Errorf("encode reservation %s: %w", action, err)
	}
	endpoint := c.baseURL + "/internal/v1/reservation-locks/" + url.PathEscape(reservation.ID) + "/" + action
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create reservation %s request: %w", action, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Secret", c.secret)
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call reservation %s: %w", action, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusUnprocessableEntity {
			return fmt.Errorf("%w: reservation %s returned HTTP %d: %s", application.ErrReservationRejected, action, response.StatusCode, strings.TrimSpace(string(limited)))
		}
		return fmt.Errorf("reservation %s returned HTTP %d: %s", action, response.StatusCode, strings.TrimSpace(string(limited)))
	}
	if target != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
			return fmt.Errorf("decode reservation %s response: %w", action, err)
		}
	}
	return nil
}
