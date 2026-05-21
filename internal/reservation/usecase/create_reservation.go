package usecase

import (
	"context"
	"log/slog"

	paymentpb "parkir-pintar/services/reservation/gen/payment/v1"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"
	"parkir-pintar/services/reservation/pkg/logger"

	"github.com/google/uuid"
)

const bookingFeeIDR int64 = 5000

func (u *ReservationUsecase) CreateReservation(ctx context.Context, req model.CreateReservationRequest) (*model.CreateReservationResponse, *apperror.AppError) {
	// Idempotency check
	if resp, appErr := u.checkIdempotency(ctx, req.IdempotencyKey); appErr != nil || resp != nil {
		return resp, appErr
	}

	// Validate driver has no active reservation
	hasActive, appErr := u.repo.HasActiveReservation(ctx, req.DriverID)
	if appErr != nil {
		return nil, appErr
	}
	if hasActive {
		return nil, apperror.New("conflict", "driver already has an active reservation")
	}

	// Resolve spot and acquire Redis lock
	var spot *model.Spot
	if req.Mode == model.AssignmentModeUserSelected {
		spot, appErr = u.resolveUserSelectedSpot(ctx, req)
	} else {
		spot, appErr = u.resolveSystemAssignedSpot(ctx, req)
	}
	if appErr != nil {
		return nil, appErr
	}

	reservationID := uuid.New().String()

	// Call Payment Service to create QRIS booking fee payment
	paymentResult, appErr := u.paymentClient.CreatePayment(ctx,
		req.IdempotencyKey,
		reservationID,
		req.DriverID,
		paymentpb.PaymentType_PAYMENT_TYPE_BOOKING_FEE,
		bookingFeeIDR,
	)
	if appErr != nil {
		logger.Error(ctx, "CreateReservation: payment service call failed",
			slog.String("error", appErr.Error()),
		)
		_ = u.repo.ReleaseSpotLock(ctx, spot.ID)
		return nil, appErr
	}
	qrCodeURL := paymentResult.QRCodeURL
	logger.Info(ctx, "CreateReservation: payment created",
		slog.String("payment_id", paymentResult.PaymentID),
	)

	// INSERT reservation + UPDATE spot=LOCKED in a single DB transaction
	reservation := &model.Reservation{
		ID:             reservationID,
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

	// Release Redis lock — spot is now protected by DB status=LOCKED
	if releaseErr := u.repo.ReleaseSpotLock(ctx, spot.ID); releaseErr != nil {
		logger.Warn(ctx, "CreateReservation: failed to release spot lock after DB commit",
			slog.String("spot_id", spot.ID),
			slog.String("error", releaseErr.Error()),
		)
	}

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
