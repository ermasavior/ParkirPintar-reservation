package usecase

import (
	"context"
	"log/slog"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"
	"parkir-pintar/services/reservation/pkg/logger"
)

// CreateReservation handles the full reservation creation flow:
// 1. Idempotency check
// 2. Validate driver has no active reservation
// 3. Acquire Redis lock on spot
// 4. Verify spot is AVAILABLE in DB (FOR UPDATE)
// 5. INSERT reservation + UPDATE spot=LOCKED in a single transaction
// 6. Release Redis lock
// 7. Call Payment Service to create QRIS payment (stub — to be wired)
// 8. Return response with qr_code_url
func (u *ReservationUsecase) CreateReservation(ctx context.Context, req model.CreateReservationRequest) (*model.CreateReservationResponse, *apperror.AppError) {
	// Step 1: Idempotency check
	if resp, appErr := u.checkIdempotency(ctx, req.IdempotencyKey); appErr != nil || resp != nil {
		return resp, appErr
	}

	// Step 2: Validate driver has no active reservation
	hasActive, appErr := u.repo.HasActiveReservation(ctx, req.DriverID)
	if appErr != nil {
		return nil, appErr
	}
	if hasActive {
		return nil, apperror.New("conflict", "driver already has an active reservation")
	}

	// Step 3 & 4: Resolve spot and acquire Redis lock
	var spot *model.Spot
	if req.Mode == model.AssignmentModeUserSelected {
		spot, appErr = u.resolveUserSelectedSpot(ctx, req)
	} else {
		spot, appErr = u.resolveSystemAssignedSpot(ctx, req)
	}
	if appErr != nil {
		return nil, appErr
	}

	// Step 5: INSERT reservation + UPDATE spot=LOCKED in a single DB transaction
	// Step 7: Call Payment Service to create QRIS booking fee payment
	// TODO: wire up Payment Service client (gRPC)
	qrCodeURL := "https://payment-gateway.example.com/qris/stub"

	reservation := &model.Reservation{
		IdempotencyKey: req.IdempotencyKey,
		DriverID:       req.DriverID,
		SpotID:         spot.ID,
		VehicleType:    req.VehicleType,
		AssignmentMode: req.Mode,
		QRCodeURL:      qrCodeURL,
	}

	created, appErr := u.repo.CreateReservationAndLockSpot(ctx, reservation)
	if appErr != nil {
		_ = u.repo.ReleaseSpotLock(ctx, spot.ID)
		return nil, appErr
	}

	// Step 6: Release Redis lock — spot is now protected by DB status=LOCKED
	if releaseErr := u.repo.ReleaseSpotLock(ctx, spot.ID); releaseErr != nil {
		// Non-fatal: lock will expire via TTL. Log and continue.
		logger.Warn(ctx, "CreateReservation: failed to release spot lock after DB commit",
			slog.String("spot_id", spot.ID),
			slog.String("error", releaseErr.Error()),
		)
	}

	logger.Info(ctx, "CreateReservation: payment service call stubbed",
		slog.String("reservation_id", created.ID),
		slog.String("spot_id", spot.ID),
	)

	return &model.CreateReservationResponse{
		ReservationID: created.ID,
		SpotID:        spot.ID,
		SpotCode:      spot.SpotCode,
		FloorNumber:   spot.FloorNumber,
		Status:        model.ReservationStatusPendingPayment,
		QRCodeURL:     qrCodeURL,
	}, nil
}

// checkIdempotency returns a cached response if the idempotency key was already processed.
// Returns (nil, nil) when no duplicate is found and processing should continue.
func (u *ReservationUsecase) checkIdempotency(ctx context.Context, idempotencyKey string) (*model.CreateReservationResponse, *apperror.AppError) {
	existing, appErr := u.repo.GetByIdempotencyKey(ctx, idempotencyKey)
	if appErr != nil {
		return nil, appErr
	}
	if existing == nil {
		return nil, nil
	}

	logger.Info(ctx, "CreateReservation: duplicate request, returning cached response",
		slog.String("idempotency_key", idempotencyKey),
		slog.String("reservation_id", existing.ID),
	)
	return &model.CreateReservationResponse{
		ReservationID: existing.ID,
		SpotID:        existing.SpotID,
		SpotCode:      existing.SpotCode,
		FloorNumber:   existing.FloorNumber,
		Status:        existing.Status,
		QRCodeURL:     existing.QRCodeURL,
	}, nil
}

// resolveUserSelectedSpot validates and locks the driver-chosen spot.
// It acquires the Redis lock first, then verifies the spot is AVAILABLE in the DB.
func (u *ReservationUsecase) resolveUserSelectedSpot(ctx context.Context, req model.CreateReservationRequest) (*model.Spot, *apperror.AppError) {
	if req.SpotID == "" {
		return nil, apperror.New("validation_error", "spot_id is required for USER_SELECTED mode")
	}

	acquired, appErr := u.repo.AcquireSpotLock(ctx, req.SpotID, req.DriverID)
	if appErr != nil {
		return nil, appErr
	}
	if !acquired {
		return nil, apperror.New("conflict", "spot is currently being reserved by another driver")
	}

	spot, appErr := u.repo.GetSpotByID(ctx, req.SpotID)
	if appErr != nil {
		_ = u.repo.ReleaseSpotLock(ctx, req.SpotID)
		return nil, appErr
	}
	if spot.Status != model.SpotStatusAvailable {
		_ = u.repo.ReleaseSpotLock(ctx, req.SpotID)
		return nil, apperror.New("conflict", "spot is not available")
	}

	return spot, nil
}

// resolveSystemAssignedSpot finds any available spot for the given vehicle type and locks it.
func (u *ReservationUsecase) resolveSystemAssignedSpot(ctx context.Context, req model.CreateReservationRequest) (*model.Spot, *apperror.AppError) {
	spot, appErr := u.repo.GetAvailableSpot(ctx, req.VehicleType)
	if appErr != nil {
		return nil, appErr
	}

	acquired, appErr := u.repo.AcquireSpotLock(ctx, spot.ID, req.DriverID)
	if appErr != nil {
		return nil, appErr
	}
	if !acquired {
		return nil, apperror.New("conflict", "spot was taken, please retry")
	}

	return spot, nil
}
