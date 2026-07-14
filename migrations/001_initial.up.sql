CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS catalog;
CREATE SCHEMA IF NOT EXISTS reservation;
CREATE SCHEMA IF NOT EXISTS checkout;
CREATE SCHEMA IF NOT EXISTS messaging;
CREATE SCHEMA IF NOT EXISTS notification;

CREATE TABLE identity.users (
    id              text PRIMARY KEY,
    email           text NOT NULL,
    password_hash   text NOT NULL,
    display_name    text NOT NULL,
    role            text NOT NULL CHECK (role IN ('buyer', 'organizer', 'admin')),
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_normalized CHECK (email = lower(btrim(email)))
);
CREATE UNIQUE INDEX users_email_unique_idx ON identity.users (email);

CREATE TABLE catalog.venues (
    id                  text PRIMARY KEY,
    owner_id            text NOT NULL REFERENCES identity.users(id),
    name                text NOT NULL,
    country_code        char(2) NOT NULL,
    city                text NOT NULL,
    address             text NOT NULL,
    declared_capacity   integer NOT NULL CHECK (declared_capacity > 0),
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (name, city, address)
);
CREATE INDEX venues_location_idx ON catalog.venues (country_code, city);

CREATE TABLE catalog.halls (
    id                  text PRIMARY KEY,
    venue_id            text NOT NULL REFERENCES catalog.venues(id) ON DELETE CASCADE,
    name                text NOT NULL,
    declared_capacity   integer NOT NULL CHECK (declared_capacity > 0),
    UNIQUE (venue_id, name)
);

CREATE TABLE catalog.sections (
    id          text PRIMARY KEY,
    hall_id     text NOT NULL REFERENCES catalog.halls(id) ON DELETE CASCADE,
    name        text NOT NULL,
    sort_order  integer NOT NULL CHECK (sort_order >= 0),
    UNIQUE (hall_id, name),
    UNIQUE (hall_id, sort_order)
);

CREATE TABLE catalog.seat_rows (
    id          text PRIMARY KEY,
    section_id  text NOT NULL REFERENCES catalog.sections(id) ON DELETE CASCADE,
    label       text NOT NULL,
    sort_order  integer NOT NULL CHECK (sort_order >= 0),
    UNIQUE (section_id, label),
    UNIQUE (section_id, sort_order)
);

CREATE TABLE catalog.seats (
    id          text PRIMARY KEY,
    row_id      text NOT NULL REFERENCES catalog.seat_rows(id) ON DELETE CASCADE,
    number      text NOT NULL,
    sort_order  integer NOT NULL CHECK (sort_order >= 0),
    accessible  boolean NOT NULL DEFAULT false,
    UNIQUE (row_id, number),
    UNIQUE (row_id, sort_order)
);
CREATE INDEX seats_row_idx ON catalog.seats (row_id, sort_order);

CREATE TABLE catalog.events (
    id              text PRIMARY KEY,
    organizer_id    text NOT NULL REFERENCES identity.users(id),
    title           text NOT NULL,
    description     text NOT NULL,
    genre           text NOT NULL,
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'cancelled', 'archived')),
    published_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    search_document tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(description, '') || ' ' || coalesce(genre, ''))
    ) STORED,
    CONSTRAINT published_event_has_timestamp CHECK ((status = 'published') = (published_at IS NOT NULL) OR status IN ('cancelled', 'archived'))
);
CREATE INDEX events_search_idx ON catalog.events USING gin (search_document);
CREATE INDEX events_discovery_idx ON catalog.events (status, genre, published_at DESC, id);

CREATE TABLE catalog.tags (
    id      text PRIMARY KEY,
    name    text NOT NULL CHECK (name = lower(btrim(name))),
    UNIQUE (name)
);

CREATE TABLE catalog.event_tags (
    event_id text NOT NULL REFERENCES catalog.events(id) ON DELETE CASCADE,
    tag_id   text NOT NULL REFERENCES catalog.tags(id) ON DELETE CASCADE,
    PRIMARY KEY (event_id, tag_id)
);
CREATE INDEX event_tags_tag_idx ON catalog.event_tags (tag_id, event_id);

CREATE TABLE catalog.event_schedules (
    id          text PRIMARY KEY,
    event_id    text NOT NULL REFERENCES catalog.events(id) ON DELETE CASCADE,
    hall_id     text NOT NULL REFERENCES catalog.halls(id),
    starts_at   timestamptz NOT NULL,
    ends_at     timestamptz NOT NULL,
    status      text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'cancelled', 'completed')),
    version     bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CHECK (ends_at > starts_at),
    EXCLUDE USING gist (
        hall_id WITH =,
        tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status = 'scheduled')
);
CREATE INDEX schedules_event_date_idx ON catalog.event_schedules (event_id, starts_at, id);
CREATE INDEX schedules_upcoming_idx ON catalog.event_schedules (starts_at, id) WHERE status = 'scheduled';

CREATE TABLE catalog.pricing_categories (
    id          text PRIMARY KEY,
    event_id    text NOT NULL REFERENCES catalog.events(id) ON DELETE CASCADE,
    name        text NOT NULL,
    price_minor bigint NOT NULL CHECK (price_minor >= 0),
    currency    char(3) NOT NULL CHECK (currency = upper(currency)),
    UNIQUE (event_id, name)
);

CREATE TABLE catalog.schedule_seat_prices (
    schedule_id        text NOT NULL REFERENCES catalog.event_schedules(id) ON DELETE CASCADE,
    seat_id            text NOT NULL REFERENCES catalog.seats(id),
    pricing_category_id text NOT NULL REFERENCES catalog.pricing_categories(id),
    price_minor        bigint NOT NULL CHECK (price_minor >= 0),
    currency           char(3) NOT NULL CHECK (currency = upper(currency)),
    PRIMARY KEY (schedule_id, seat_id)
);
CREATE INDEX schedule_seat_price_filter_idx ON catalog.schedule_seat_prices (schedule_id, price_minor, seat_id);

CREATE TABLE reservation.bookings (
    id              text PRIMARY KEY,
    reservation_id  text NOT NULL UNIQUE,
    schedule_id     text NOT NULL REFERENCES catalog.event_schedules(id),
    buyer_id        text NOT NULL REFERENCES identity.users(id),
    status          text NOT NULL CHECK (status IN ('confirmed', 'cancelled')),
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bookings_buyer_idx ON reservation.bookings (buyer_id, created_at DESC, id);

CREATE TABLE reservation.booked_seats (
    booking_id  text NOT NULL REFERENCES reservation.bookings(id) ON DELETE CASCADE,
    schedule_id text NOT NULL REFERENCES catalog.event_schedules(id),
    seat_id     text NOT NULL REFERENCES catalog.seats(id),
    PRIMARY KEY (booking_id, seat_id),
    UNIQUE (schedule_id, seat_id)
);
CREATE INDEX booked_seats_schedule_idx ON reservation.booked_seats (schedule_id, seat_id);

CREATE TABLE checkout.orders (
    id                  text PRIMARY KEY,
    buyer_id            text NOT NULL REFERENCES identity.users(id),
    reservation_id      text NOT NULL UNIQUE,
    schedule_id         text NOT NULL REFERENCES catalog.event_schedules(id),
    state               text NOT NULL CHECK (state IN (
        'pending', 'payment_pending', 'payment_uncertain', 'paid',
        'booking_confirmed', 'ticket_issued', 'completed', 'cancelled',
        'failed', 'refund_pending', 'refunded'
    )),
    amount_minor        bigint NOT NULL CHECK (amount_minor >= 0),
    currency            char(3) NOT NULL CHECK (currency = upper(currency)),
    state_version       bigint NOT NULL DEFAULT 1 CHECK (state_version > 0),
    failure_code        text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX orders_buyer_idx ON checkout.orders (buyer_id, created_at DESC, id);

CREATE TABLE checkout.order_seats (
    order_id    text NOT NULL REFERENCES checkout.orders(id) ON DELETE CASCADE,
    seat_id     text NOT NULL REFERENCES catalog.seats(id),
    price_minor bigint NOT NULL CHECK (price_minor >= 0),
    PRIMARY KEY (order_id, seat_id)
);

CREATE TABLE checkout.payments (
    id                  text PRIMARY KEY,
    order_id            text NOT NULL REFERENCES checkout.orders(id),
    provider            text NOT NULL,
    provider_reference  text NOT NULL,
    status              text NOT NULL CHECK (status IN ('initiated', 'confirmed', 'failed', 'uncertain', 'refunded')),
    amount_minor        bigint NOT NULL CHECK (amount_minor >= 0),
    currency            char(3) NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_reference)
);

CREATE TABLE checkout.idempotency_keys (
    scope           text NOT NULL,
    key_hash        text NOT NULL,
    request_hash    text NOT NULL,
    resource_id     text,
    response_status integer,
    response_body   jsonb,
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, key_hash)
);
CREATE INDEX checkout_idempotency_expiry_idx ON checkout.idempotency_keys (expires_at);

CREATE TABLE checkout.order_transitions (
    order_id        text NOT NULL REFERENCES checkout.orders(id) ON DELETE CASCADE,
    version         bigint NOT NULL,
    from_state      text,
    to_state        text NOT NULL,
    reason          text,
    correlation_id  text NOT NULL,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (order_id, version)
);

CREATE TABLE checkout.tickets (
    id              text PRIMARY KEY,
    booking_id      text NOT NULL REFERENCES reservation.bookings(id),
    order_id        text NOT NULL REFERENCES checkout.orders(id),
    seat_id         text NOT NULL REFERENCES catalog.seats(id),
    token_digest    bytea NOT NULL UNIQUE,
    qr_signature    bytea NOT NULL,
    issued_at       timestamptz NOT NULL DEFAULT now(),
    revoked_at      timestamptz,
    revocation_reason text,
    UNIQUE (booking_id, seat_id),
    UNIQUE (order_id, seat_id)
);

CREATE TABLE messaging.outbox (
    id              text PRIMARY KEY,
    owner_context   text NOT NULL,
    aggregate_type  text NOT NULL,
    aggregate_id    text NOT NULL,
    event_type      text NOT NULL,
    event_version   integer NOT NULL CHECK (event_version > 0),
    correlation_id  text NOT NULL,
    causation_id    text,
    payload         jsonb NOT NULL,
    occurred_at     timestamptz NOT NULL,
    available_at    timestamptz NOT NULL DEFAULT now(),
    attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    published_at    timestamptz,
    last_error      text
);
CREATE INDEX outbox_dispatch_idx ON messaging.outbox (available_at, occurred_at, id) WHERE published_at IS NULL;

CREATE TABLE messaging.inbox (
    consumer_name   text NOT NULL,
    event_id        text NOT NULL,
    event_type      text NOT NULL,
    aggregate_id    text NOT NULL,
    occurred_at     timestamptz NOT NULL,
    processed_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_name, event_id)
);

CREATE TABLE notification.deliveries (
    id              text PRIMARY KEY,
    event_id        text NOT NULL,
    channel         text NOT NULL CHECK (channel IN ('email', 'sms')),
    recipient_ref   text NOT NULL,
    template        text NOT NULL,
    state           text NOT NULL CHECK (state IN ('pending', 'sent', 'retrying', 'dead_letter')),
    attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, channel, recipient_ref, template)
);
CREATE INDEX notification_retry_idx ON notification.deliveries (next_attempt_at, id) WHERE state IN ('pending', 'retrying');
