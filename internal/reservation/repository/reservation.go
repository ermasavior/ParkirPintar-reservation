package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"
	"parkir-pintar/services/reservation/pkg/logger"

	"github.com/jackc/pgx/v5"
)

const spotLockTTL = 30 * time.Second
const spotLockKeyPrefix = "lock:spot:"

// GetByIdempotencyKey returns an existing reservation by idempotency key
func (r *ReservationRepository) GetByIdempotencyKey(ctx context.Context, key string) (*model.Reservation, *apperror.AppError) {
	query := `SELECT r.id, r.spot_id, r.status, r.qr_code_url,
	           s.spot_code, s.floor_number
	           FROM reservations r
	           JOIN spots s ON s.id = r.spot_id
	           WHERE r.idempotency_key = $1`

	var res model.Reservation
	err := r.db.QueryRow(ctx, query, key).Scan(
		&res.ID,
		&res.SpotID,
		&res.Status,
		&res.QRCodeURL,
		&res.SpotCode,
		&res.FloorNumber,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error(ctx, "GetByIdempotencyKey failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to query reservation by idempotency key")
	}
	return &res, nil
}

// GetByID returns a reservation by its UUID, joining spot for code and floor
func (r *ReservationRepository) GetByID(ctx context.Context, id string) (*model.Reservation, *apperror.AppError) {
	query := `SELECT r.id, r.idempotency_key, r.driver_id, r.spot_id, r.vehicle_type,
	           r.assignment_mode, r.status, r.confirmed_at, r.expires_at, r.created_at,
	           s.spot_code, s.floor_number
	           FROM reservations r
	           JOIN spots s ON s.id = r.spot_id
	           WHERE r.id = $1`

	var res model.Reservation
	err := r.db.QueryRow(ctx, query, id).Scan(
		&res.ID,
		&res.IdempotencyKey,
		&res.DriverID,
		&res.SpotID,
		&res.VehicleType,
		&res.AssignmentMode,
		&res.Status,
		&res.ConfirmedAt,
		&res.ExpiresAt,
		&res.CreatedAt,
		&res.SpotCode,
		&res.FloorNumber,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.New("not_found", "reservation not found")
		}
		logger.Error(ctx, "GetByID failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to query reservation")
	}
	return &res, nil
}

// HasActiveReservation checks if a driver already has a PENDING_PAYMENT or CONFIRMED reservation
func (r *ReservationRepository) HasActiveReservation(ctx context.Context, driverID string) (bool, *apperror.AppError) {
	query := `SELECT COUNT(*) FROM reservations WHERE driver_id = $1 AND status = ANY($2)`

	var count int
	err := r.db.QueryRow(ctx, query, driverID, []int{
		int(model.ReservationStatusPendingPayment),
		int(model.ReservationStatusConfirmed),
	}).Scan(&count)
	if err != nil {
		logger.Error(ctx, "HasActiveReservation failed", slog.String("error", err.Error()))
		return false, apperror.New("db_error", "failed to check active reservation")
	}
	return count > 0, nil
}

// GetAvailableSpot returns any available spot matching the vehicle type with FOR UPDATE SKIP LOCKED
func (r *ReservationRepository) GetAvailableSpot(ctx context.Context, vehicleType model.VehicleType) (*model.Spot, *apperror.AppError) {
	query := `SELECT id, floor_number, spot_code, vehicle_type, status
	           FROM spots WHERE status = $1 AND vehicle_type = $2
	           LIMIT 1 FOR UPDATE SKIP LOCKED`

	var spot model.Spot
	err := r.db.QueryRow(ctx, query, model.SpotStatusAvailable, vehicleType).Scan(
		&spot.ID,
		&spot.FloorNumber,
		&spot.SpotCode,
		&spot.VehicleType,
		&spot.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.New("no_spots_available", "no available spots for the requested vehicle type")
		}
		logger.Error(ctx, "GetAvailableSpot failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to query available spot")
	}
	return &spot, nil
}

// GetSpotByID returns a spot by its UUID with FOR UPDATE lock
func (r *ReservationRepository) GetSpotByID(ctx context.Context, spotID string) (*model.Spot, *apperror.AppError) {
	query := `SELECT id, floor_number, spot_code, vehicle_type, status
	           FROM spots WHERE id = $1 FOR UPDATE`

	var spot model.Spot
	err := r.db.QueryRow(ctx, query, spotID).Scan(
		&spot.ID,
		&spot.FloorNumber,
		&spot.SpotCode,
		&spot.VehicleType,
		&spot.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.New("not_found", "spot not found")
		}
		logger.Error(ctx, "GetSpotByID failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to query spot")
	}
	return &spot, nil
}

// CreateReservationAndLockSpot atomically inserts a reservation and sets spot status to LOCKED
func (r *ReservationRepository) CreateReservationAndLockSpot(ctx context.Context, reservation *model.Reservation) (*model.Reservation, *apperror.AppError) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		logger.Error(ctx, "CreateReservationAndLockSpot: begin tx failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to begin transaction")
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	insertQuery := `INSERT INTO reservations
	  (idempotency_key, driver_id, spot_id, vehicle_type, assignment_mode, status, qr_code_url, created_at)
	  VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	  RETURNING id, created_at`

	err = tx.QueryRow(ctx, insertQuery,
		reservation.IdempotencyKey,
		reservation.DriverID,
		reservation.SpotID,
		reservation.VehicleType,
		reservation.AssignmentMode,
		model.ReservationStatusPendingPayment,
		reservation.QRCodeURL,
	).Scan(&reservation.ID, &reservation.CreatedAt)
	if err != nil {
		logger.Error(ctx, "CreateReservationAndLockSpot: insert reservation failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to insert reservation")
	}

	_, err = tx.Exec(ctx,
		`UPDATE spots SET status = $1 WHERE id = $2`,
		model.SpotStatusLocked, reservation.SpotID,
	)
	if err != nil {
		logger.Error(ctx, "CreateReservationAndLockSpot: update spot failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to lock spot")
	}

	if err = tx.Commit(ctx); err != nil {
		logger.Error(ctx, "CreateReservationAndLockSpot: commit failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to commit transaction")
	}

	reservation.Status = model.ReservationStatusPendingPayment
	return reservation, nil
}

// AcquireSpotLock acquires a Redis distributed lock on a spot (TTL: 30s)
func (r *ReservationRepository) AcquireSpotLock(ctx context.Context, spotID string, driverID string) (bool, *apperror.AppError) {
	key := fmt.Sprintf("%s%s", spotLockKeyPrefix, spotID)
	ok, err := r.redis.SetNX(ctx, key, driverID, spotLockTTL).Result()
	if err != nil {
		logger.Error(ctx, "AcquireSpotLock failed", slog.String("spot_id", spotID), slog.String("error", err.Error()))
		return false, apperror.New("redis_error", "failed to acquire spot lock")
	}
	return ok, nil
}

// ReleaseSpotLock releases the Redis distributed lock on a spot
func (r *ReservationRepository) ReleaseSpotLock(ctx context.Context, spotID string) *apperror.AppError {
	key := fmt.Sprintf("%s%s", spotLockKeyPrefix, spotID)
	if err := r.redis.Del(ctx, key).Err(); err != nil {
		logger.Error(ctx, "ReleaseSpotLock failed", slog.String("spot_id", spotID), slog.String("error", err.Error()))
		return apperror.New("redis_error", "failed to release spot lock")
	}
	return nil
}

// ConfirmReservation sets reservation status=CONFIRMED, confirmed_at=now(), expires_at=now()+1h
func (r *ReservationRepository) ConfirmReservation(ctx context.Context, reservationID string) *apperror.AppError {
	_, err := r.db.Exec(ctx,
		`UPDATE reservations
		 SET status = $1, confirmed_at = NOW(), expires_at = NOW() + INTERVAL '1 hour'
		 WHERE id = $2`,
		model.ReservationStatusConfirmed, reservationID,
	)
	if err != nil {
		logger.Error(ctx, "ConfirmReservation failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to confirm reservation")
	}
	return nil
}

// CancelReservationAndReleaseSpot atomically sets reservation=CANCELLED and spot=AVAILABLE
func (r *ReservationRepository) CancelReservationAndReleaseSpot(ctx context.Context, reservationID, spotID string) *apperror.AppError {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		logger.Error(ctx, "CancelReservationAndReleaseSpot: begin tx failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to begin transaction")
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	_, err = tx.Exec(ctx,
		`UPDATE reservations SET status = $1 WHERE id = $2`,
		model.ReservationStatusCancelled, reservationID,
	)
	if err != nil {
		logger.Error(ctx, "CancelReservationAndReleaseSpot: cancel reservation failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to cancel reservation")
	}

	_, err = tx.Exec(ctx,
		`UPDATE spots SET status = $1 WHERE id = $2`,
		model.SpotStatusAvailable, spotID,
	)
	if err != nil {
		logger.Error(ctx, "CancelReservationAndReleaseSpot: release spot failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to release spot")
	}

	if err = tx.Commit(ctx); err != nil {
		logger.Error(ctx, "CancelReservationAndReleaseSpot: commit failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to commit transaction")
	}

	return nil
}

// GetExpiredReservations returns CONFIRMED reservations whose expires_at < now()
func (r *ReservationRepository) GetExpiredReservations(ctx context.Context) ([]model.Reservation, *apperror.AppError) {
	query := `SELECT id, driver_id, spot_id FROM reservations
	           WHERE status = $1 AND expires_at < NOW()`

	rows, err := r.db.Query(ctx, query, model.ReservationStatusConfirmed)
	if err != nil {
		logger.Error(ctx, "GetExpiredReservations failed", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to query expired reservations")
	}
	defer rows.Close()

	var reservations []model.Reservation
	for rows.Next() {
		var res model.Reservation
		if err := rows.Scan(&res.ID, &res.DriverID, &res.SpotID); err != nil {
			logger.Error(ctx, "GetExpiredReservations scan failed", slog.String("error", err.Error()))
			return nil, apperror.New("db_error", "failed to scan expired reservation")
		}
		reservations = append(reservations, res)
	}
	if err := rows.Err(); err != nil {
		logger.Error(ctx, "GetExpiredReservations rows error", slog.String("error", err.Error()))
		return nil, apperror.New("db_error", "failed to iterate expired reservations")
	}

	return reservations, nil
}

// ExpireReservationAndReleaseSpot atomically sets reservation=EXPIRED and spot=AVAILABLE
func (r *ReservationRepository) ExpireReservationAndReleaseSpot(ctx context.Context, reservationID, spotID string) *apperror.AppError {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		logger.Error(ctx, "ExpireReservationAndReleaseSpot: begin tx failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to begin transaction")
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	_, err = tx.Exec(ctx,
		`UPDATE reservations SET status = $1 WHERE id = $2`,
		model.ReservationStatusExpired, reservationID,
	)
	if err != nil {
		logger.Error(ctx, "ExpireReservationAndReleaseSpot: expire reservation failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to expire reservation")
	}

	_, err = tx.Exec(ctx,
		`UPDATE spots SET status = $1 WHERE id = $2`,
		model.SpotStatusAvailable, spotID,
	)
	if err != nil {
		logger.Error(ctx, "ExpireReservationAndReleaseSpot: release spot failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to release spot")
	}

	if err = tx.Commit(ctx); err != nil {
		logger.Error(ctx, "ExpireReservationAndReleaseSpot: commit failed", slog.String("error", err.Error()))
		return apperror.New("db_error", "failed to commit transaction")
	}

	return nil
}
