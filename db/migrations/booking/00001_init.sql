-- +goose Up
CREATE TYPE booking_status AS ENUM ('CONFIRMED', 'CANCELLED');

CREATE TABLE bookings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id varchar(64) NOT NULL,
    flight_id uuid NOT NULL,
    passenger_name varchar(200) NOT NULL,
    passenger_email varchar(254) NOT NULL,
    seat_count integer NOT NULL,
    total_price_minor bigint NOT NULL,
    currency varchar(3) NOT NULL DEFAULT 'RUB',
    status booking_status NOT NULL DEFAULT 'CONFIRMED',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    cancelled_at timestamptz,
    CONSTRAINT bookings_user_id_not_empty CHECK (user_id <> ''),
    CONSTRAINT bookings_passenger_name_not_empty CHECK (passenger_name <> ''),
    CONSTRAINT bookings_passenger_email_not_empty CHECK (passenger_email <> ''),
    CONSTRAINT bookings_seat_count_positive CHECK (seat_count > 0),
    CONSTRAINT bookings_total_price_positive CHECK (total_price_minor > 0),
    CONSTRAINT bookings_currency_iso CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX bookings_user_idx ON bookings (user_id);
CREATE INDEX bookings_flight_idx ON bookings (flight_id);

-- +goose Down
DROP TABLE bookings;
DROP TYPE booking_status;
