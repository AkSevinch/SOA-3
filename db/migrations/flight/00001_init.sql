-- +goose Up
CREATE TYPE flight_status AS ENUM ('SCHEDULED', 'DEPARTED', 'CANCELLED', 'COMPLETED');
CREATE TYPE reservation_status AS ENUM ('ACTIVE', 'RELEASED', 'EXPIRED');

CREATE TABLE flights (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    flight_number varchar(10) NOT NULL,
    airline_code varchar(3) NOT NULL,
    origin varchar(3) NOT NULL,
    destination varchar(3) NOT NULL,
    departure_time timestamptz NOT NULL,
    arrival_time timestamptz NOT NULL,
    total_seats integer NOT NULL,
    available_seats integer NOT NULL,
    price_minor bigint NOT NULL,
    currency varchar(3) NOT NULL DEFAULT 'RUB',
    status flight_status NOT NULL DEFAULT 'SCHEDULED',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT flights_total_seats_positive CHECK (total_seats > 0),
    CONSTRAINT flights_available_seats_valid CHECK (available_seats >= 0 AND available_seats <= total_seats),
    CONSTRAINT flights_price_positive CHECK (price_minor > 0),
    CONSTRAINT flights_time_order CHECK (arrival_time > departure_time),
    CONSTRAINT flights_route_diff CHECK (origin <> destination),
    CONSTRAINT flights_origin_iata CHECK (origin ~ '^[A-Z]{3}$'),
    CONSTRAINT flights_destination_iata CHECK (destination ~ '^[A-Z]{3}$'),
    CONSTRAINT flights_currency_iso CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT flights_airline_code_format CHECK (airline_code ~ '^[A-Z0-9]{2,3}$'),
    CONSTRAINT flights_flight_number_not_empty CHECK (flight_number <> '')
);

CREATE UNIQUE INDEX flights_number_date_uidx
    ON flights (flight_number, ((departure_time AT TIME ZONE 'UTC')::date));
CREATE INDEX flights_search_idx ON flights (origin, destination, departure_time);
CREATE INDEX flights_status_idx ON flights (status);

CREATE TABLE seat_reservations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    flight_id uuid NOT NULL REFERENCES flights(id) ON DELETE RESTRICT,
    booking_id uuid NOT NULL,
    seat_count integer NOT NULL,
    status reservation_status NOT NULL DEFAULT 'ACTIVE',
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT seat_reservations_seat_count_positive CHECK (seat_count > 0)
);

CREATE UNIQUE INDEX seat_reservations_booking_uidx ON seat_reservations (booking_id);
CREATE INDEX seat_reservations_flight_status_idx ON seat_reservations (flight_id, status);

-- +goose Down
DROP TABLE seat_reservations;
DROP TABLE flights;
DROP TYPE reservation_status;
DROP TYPE flight_status;
