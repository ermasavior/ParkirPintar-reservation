# reservation-service

[![Golang CI/CD](https://github.com/ermasavior/parkirpintar-reservation/actions/workflows/cicd.yml/badge.svg)](https://github.com/ermasavior/parkirpintar-reservation/actions/workflows/cicd.yml)

Handles spot reservation, distributed locking, booking-fee payment initiation, and reservation expiry.

## Responsibilities

- `CreateReservation` — acquires a Redis lock on the spot, writes the reservation (`PENDING_PAYMENT`), calls Payment Service for a QRIS booking-fee code
- `GetReservation` — returns current reservation state
- Consumes `payment.booking.done` from NATS → transitions reservation to `CONFIRMED` (success) or `CANCELLED` (failure), sets `expires_at = confirmed_at + 1h`
- DB polling scheduler (every 30s) — atomically expires `CONFIRMED` reservations past `expires_at` and releases spots back to `AVAILABLE`

## gRPC API

```
service ReservationService {
  rpc CreateReservation (CreateReservationRequest) returns (CreateReservationResponse);
  rpc GetReservation    (GetReservationRequest)    returns (GetReservationResponse);
}
```

Proto: [`proto/reservation/v1/reservation.proto`](proto/reservation/v1/reservation.proto)

## Dependencies

| Dependency | Purpose |
|---|---|
| PostgreSQL | Reservation + spot state |
| Redis | Short-lived spot lock (`SET NX`, TTL 30s) |
| NATS JetStream | Consume `payment.booking.done` |
| Payment Service (gRPC) | Create booking-fee QRIS payment |

## Configuration

```bash
cp .env.example .env
```

Key variables: `POSTGRES_DSN`, `REDIS_URL`, `NATS_URL`, `PAYMENT_SERVICE_URL`

## Development

```bash
make run              # run locally
make build            # compile binary → bin/reservation
make test             # all tests
make test-unit        # unit tests only
make unit-test-coverage
make proto            # regenerate gRPC code from .proto
make mock             # regenerate mocks
```
