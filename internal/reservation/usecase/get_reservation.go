package usecase

import (
	"context"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"
)

func (u *ReservationUsecase) GetReservation(ctx context.Context, reservationID string) (*model.GetReservationResponse, *apperror.AppError) {
	reservation, appErr := u.repo.GetByID(ctx, reservationID)
	if appErr != nil {
		return nil, appErr
	}

	return &model.GetReservationResponse{
		ReservationID:  reservation.ID,
		DriverID:       reservation.DriverID,
		SpotID:         reservation.SpotID,
		SpotCode:       reservation.SpotCode,
		FloorNumber:    reservation.FloorNumber,
		VehicleType:    reservation.VehicleType,
		AssignmentMode: reservation.AssignmentMode,
		Status:         reservation.Status,
		ConfirmedAt:    reservation.ConfirmedAt,
		ExpiresAt:      reservation.ExpiresAt,
		CreatedAt:      reservation.CreatedAt,
	}, nil
}
