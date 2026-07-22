BEGIN;

-- Password login for these actors is intentionally not seeded. Use the registration API;
-- the organizer/admin rows exist only to make the catalog demo deterministic.
INSERT INTO identity.users (id, email, password_hash, display_name, role) VALUES
    ('organizer_demo', 'organizer@kiticket.local', '!registration-required!', 'Demo Organizer', 'organizer'),
    ('admin_demo', 'admin@kiticket.local', '!registration-required!', 'Demo Administrator', 'admin'),
    ('buyer_demo', 'buyer@kiticket.local', '!registration-required!', 'Demo Buyer', 'buyer')
ON CONFLICT DO NOTHING;

INSERT INTO catalog.venues (id, owner_id, name, country_code, city, address, declared_capacity) VALUES
    ('venue_azadi', 'organizer_demo', 'Azadi Arts Center', 'IR', 'Tehran', 'Azadi Square', 8)
ON CONFLICT DO NOTHING;
INSERT INTO catalog.halls (id, venue_id, name, declared_capacity) VALUES
    ('hall_main', 'venue_azadi', 'Main Hall', 8)
ON CONFLICT DO NOTHING;
INSERT INTO catalog.sections (id, hall_id, name, sort_order) VALUES
    ('section_orchestra', 'hall_main', 'Orchestra', 1)
ON CONFLICT DO NOTHING;
INSERT INTO catalog.seat_rows (id, section_id, label, sort_order) VALUES
    ('row_a', 'section_orchestra', 'A', 1),
    ('row_b', 'section_orchestra', 'B', 2)
ON CONFLICT DO NOTHING;
INSERT INTO catalog.seats (id, row_id, number, sort_order, accessible) VALUES
    ('seat_a1', 'row_a', '1', 1, true),
    ('seat_a2', 'row_a', '2', 2, false),
    ('seat_a3', 'row_a', '3', 3, false),
    ('seat_a4', 'row_a', '4', 4, true),
    ('seat_b1', 'row_b', '1', 1, true),
    ('seat_b2', 'row_b', '2', 2, false),
    ('seat_b3', 'row_b', '3', 3, false),
    ('seat_b4', 'row_b', '4', 4, true)
ON CONFLICT DO NOTHING;

INSERT INTO catalog.events (id, organizer_id, title, description, genre, status, published_at) VALUES
    ('event_jazz', 'organizer_demo', 'Tehran Night Jazz', 'An evening of contemporary jazz.', 'music', 'published', now())
ON CONFLICT DO NOTHING;
INSERT INTO catalog.tags (id, name) VALUES ('tag_live', 'live'), ('tag_jazz', 'jazz') ON CONFLICT DO NOTHING;
INSERT INTO catalog.event_tags (event_id, tag_id) VALUES
    ('event_jazz', 'tag_live'), ('event_jazz', 'tag_jazz')
ON CONFLICT DO NOTHING;
INSERT INTO catalog.event_schedules (id, event_id, hall_id, starts_at, ends_at) VALUES
    ('schedule_jazz_1', 'event_jazz', 'hall_main', '2030-06-15T16:00:00Z', '2030-06-15T19:00:00Z')
ON CONFLICT DO NOTHING;
INSERT INTO catalog.pricing_categories (id, event_id, name, price_minor, currency) VALUES
    ('price_standard', 'event_jazz', 'Standard', 2500000, 'IRR')
ON CONFLICT DO NOTHING;
INSERT INTO catalog.schedule_seat_prices (schedule_id, seat_id, pricing_category_id, price_minor, currency)
SELECT 'schedule_jazz_1', id, 'price_standard', 2500000, 'IRR'
FROM catalog.seats
ON CONFLICT DO NOTHING;

COMMIT;
