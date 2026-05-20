package repository

import (
	"context"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ DB = (*pgxpool.Pool)(nil)

type Reservation interface {
	// GetByIdempotencyKey returns an existing reservation by idempotency key (for duplicate detection)
	GetByIdempotencyKey(ctx context.Context, key string) (*model.Reservation, *apperror.AppError)

	// GetByID returns a reservation by its UUID
	GetByID(ctx context.Context, id string) (*model.Reservation, *apperror.AppError)

	// HasActiveReservation checks if a driver already has a PENDING_PAYMENT or CONFIRMED reservation
	HasActiveReservation(ctx context.Context, driverID string) (bool, *apperror.AppError)

	// GetAvailableSpot returns any available spot matching the vehicle type (SYSTEM_ASSIGNED)
	GetAvailableSpot(ctx context.Context, vehicleType model.VehicleType) (*model.Spot, *apperror.AppError)

	// GetSpotByID returns a spot by its UUID with a FOR UPDATE lock
	GetSpotByID(ctx context.Context, spotID string) (*model.Spot, *apperror.AppError)

	// CreateReservationAndLockSpot atomically inserts a reservation and sets spot status to LOCKED
	CreateReservationAndLockSpot(ctx context.Context, reservation *model.Reservation) (*model.Reservation, *apperror.AppError)

	// AcquireSpotLock acquires a Redis distributed lock on a spot (TTL: 30s)
	AcquireSpotLock(ctx context.Context, spotID string, driverID string) (bool, *apperror.AppError)

	// ReleaseSpotLock releases the Redis distributed lock on a spot
	ReleaseSpotLock(ctx context.Context, spotID string) *apperror.AppError

	// ConfirmReservation sets reservation status=CONFIRMED, confirmed_at=now(), expires_at=now()+1h
	ConfirmReservation(ctx context.Context, reservationID string) *apperror.AppError

	// CancelReservationAndReleaseSpot atomically sets reservation=CANCELLED and spot=AVAILABLE
	CancelReservationAndReleaseSpot(ctx context.Context, reservationID, spotID string) *apperror.AppError

	// GetExpiredReservations returns CONFIRMED reservations whose expires_at < now()
	GetExpiredReservations(ctx context.Context) ([]model.Reservation, *apperror.AppError)

	// ExpireReservationAndReleaseSpot atomically sets reservation=EXPIRED and spot=AVAILABLE
	ExpireReservationAndReleaseSpot(ctx context.Context, reservationID, spotID string) *apperror.AppError
}

type ReservationRepository struct {
	db    DB
	redis *redis.Client
}

func NewReservation(db DB, redis *redis.Client) Reservation {
	return &ReservationRepository{
		db:    db,
		redis: redis,
	}
}
