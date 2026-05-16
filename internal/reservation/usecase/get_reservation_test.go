package usecase

import (
	"context"
	"testing"
	"time"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetReservation_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	now := time.Now()
	confirmedAt := now.Add(-30 * time.Minute)
	expiresAt := now.Add(30 * time.Minute)

	repo := mockreservation.NewMockReservation(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testReservationID).Return(&model.Reservation{
		ID:             testReservationID,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		SpotCode:       "A1",
		FloorNumber:    1,
		VehicleType:    model.VehicleTypeCar,
		AssignmentMode: model.AssignmentModeSystemAssigned,
		Status:         model.ReservationStatusConfirmed,
		ConfirmedAt:    &confirmedAt,
		ExpiresAt:      &expiresAt,
		CreatedAt:      now,
	}, nil)

	res, appErr := newUsecase(repo).GetReservation(context.Background(), testReservationID)

	require.Nil(t, appErr)
	assert.Equal(t, testReservationID, res.ReservationID)
	assert.Equal(t, testDriverID, res.DriverID)
	assert.Equal(t, testSpotID, res.SpotID)
	assert.Equal(t, "A1", res.SpotCode)
	assert.Equal(t, 1, res.FloorNumber)
	assert.Equal(t, model.VehicleTypeCar, res.VehicleType)
	assert.Equal(t, model.AssignmentModeSystemAssigned, res.AssignmentMode)
	assert.Equal(t, model.ReservationStatusConfirmed, res.Status)
	assert.NotNil(t, res.ConfirmedAt)
	assert.NotNil(t, res.ExpiresAt)
}

func TestGetReservation_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservation(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testReservationID).
		Return(nil, apperror.New("not_found", "reservation not found"))

	_, appErr := newUsecase(repo).GetReservation(context.Background(), testReservationID)

	require.NotNil(t, appErr)
	assert.Equal(t, "not_found", appErr.ErrorCode)
}

func TestGetReservation_DBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservation(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testReservationID).
		Return(nil, apperror.New("db_error", "failed to query reservation"))

	_, appErr := newUsecase(repo).GetReservation(context.Background(), testReservationID)

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
}

func TestGetReservation_NilOptionalFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservation(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testReservationID).Return(&model.Reservation{
		ID:             testReservationID,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		SpotCode:       "B3",
		FloorNumber:    2,
		VehicleType:    model.VehicleTypeCar,
		AssignmentMode: model.AssignmentModeUserSelected,
		Status:         model.ReservationStatusPendingPayment,
		ConfirmedAt:    nil,
		ExpiresAt:      nil,
		CreatedAt:      time.Now(),
	}, nil)

	res, appErr := newUsecase(repo).GetReservation(context.Background(), testReservationID)

	require.Nil(t, appErr)
	assert.Nil(t, res.ConfirmedAt)
	assert.Nil(t, res.ExpiresAt)
	assert.Equal(t, model.ReservationStatusPendingPayment, res.Status)
}
