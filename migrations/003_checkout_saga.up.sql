ALTER TABLE checkout.orders
    ADD COLUMN reservation_fence bigint NOT NULL DEFAULT 1 CHECK (reservation_fence > 0),
    ADD COLUMN booking_id text REFERENCES reservation.bookings(id);

ALTER TABLE checkout.tickets
    ADD COLUMN qr_payload text NOT NULL;

CREATE INDEX orders_recovery_idx ON checkout.orders (state, updated_at, id)
WHERE state NOT IN ('completed', 'cancelled', 'failed', 'refunded');
