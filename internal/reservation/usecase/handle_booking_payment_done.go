package usecase

import (
	"context"
	"log/slog"

	"parkir-pintar/services/reservation/pkg/apperror"
	"parkir-pintar/services/reservation/pkg/logger"
)

// HandleBookingPaymentDone processes a payment.booking.done NATS event.
//
// Flow:
//   - SUCCESS → reservation=CONFIRMED, expires_at=now()+1h
//   - FAILED  → reservation=CANCELLED, spot=AVAILABLE
//   - EXPIRED → reservation=CANCELLED, spot=AVAILABLE
func (u *ReservationUsecase) HandleBookingPaymentDone(ctx context.Context, reservationID string, status string) *apperror.AppError {
	// Fetch reservation to get spot_id
	reservation, appErr := u.repo.GetByID(ctx, reservationID)
	if appErr != nil {
		return appErr
	}

	switch status {
	case "SUCCESS":
		if appErr := u.repo.ConfirmReservation(ctx, reservationID); appErr != nil {
			return appErr
		}
		logger.Info(ctx, "HandleBookingPaymentDone: reservation confirmed",
			slog.String("reservation_id", reservationID),
		)

	case "FAILED", "EXPIRED":
		if appErr := u.repo.CancelReservationAndReleaseSpot(ctx, reservationID, reservation.SpotID); appErr != nil {
			return appErr
		}
		logger.Info(ctx, "HandleBookingPaymentDone: reservation cancelled, spot released",
			slog.String("reservation_id", reservationID),
			slog.String("spot_id", reservation.SpotID),
			slog.String("status", status),
		)

	default:
		return apperror.New("validation_error", "unknown payment status: "+status)
	}

	return nil
}
