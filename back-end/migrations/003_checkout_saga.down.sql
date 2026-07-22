DROP INDEX IF EXISTS checkout.orders_recovery_idx;
ALTER TABLE checkout.tickets DROP COLUMN IF EXISTS qr_payload;
ALTER TABLE checkout.orders DROP COLUMN IF EXISTS booking_id, DROP COLUMN IF EXISTS reservation_fence;
