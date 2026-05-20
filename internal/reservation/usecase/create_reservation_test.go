package usecase

import (
	"context"
	"testing"
	"time"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	mockpaymentclient "parkir-pintar/services/reservation/_mock/pkg/paymentclient"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"
	"parkir-pintar/services/reservation/pkg/paymentclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testIdempotencyKey = "550e8400-e29b-41d4-a716-446655440000"
	testDriverID       = "660e8400-e29b-41d4-a716-446655440001"
	testSpotID         = "770e8400-e29b-41d4-a716-446655440002"
	testReservationID  = "880e8400-e29b-41d4-a716-446655440003"
)

func newUsecase(repo *mockreservation.MockReservationRepository, ctrl *gomock.Controller) *ReservationUsecase {
	pc := mockpaymentclient.NewMockPaymentService(ctrl)
	pc.EXPECT().CreatePayment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&paymentclient.CreatePaymentResult{
			PaymentID: "stub-payment-id",
			QRCodeURL: "https://qr.example.com/stub",
		}, nil).AnyTimes()
	return &ReservationUsecase{repo: repo, paymentClient: pc}
}
func validSpot() *model.Spot {
	return &model.Spot{
		ID:          testSpotID,
		SpotCode:    "A1",
		FloorNumber: 1,
		VehicleType: model.VehicleTypeCar,
		Status:      model.SpotStatusAvailable,
	}
}

func validCreatedReservation() *model.Reservation {
	return &model.Reservation{
		ID:             testReservationID,
		IdempotencyKey: testIdempotencyKey,
		DriverID:       testDriverID,
		SpotID:         testSpotID,
		SpotCode:       "A1",
		FloorNumber:    1,
		Status:         model.ReservationStatusPendingPayment,
		QRCodeURL:      "https://payment-gateway.example.com/qris/stub",
		CreatedAt:      time.Now(),
	}
}

func baseRequest(mode model.AssignmentMode) model.CreateReservationRequest {
	return model.CreateReservationRequest{
		IdempotencyKey: testIdempotencyKey,
		DriverID:       testDriverID,
		VehicleType:    model.VehicleTypeCar,
		Mode:           mode,
	}
}

// ── Idempotency ──────────────────────────────────────────────────────────────

func TestCreateReservation_IdempotencyReplay(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	existing := &model.Reservation{
		ID:          testReservationID,
		SpotID:      testSpotID,
		SpotCode:    "A1",
		FloorNumber: 1,
		Status:      model.ReservationStatusPendingPayment,
		QRCodeURL:   "https://payment-gateway.example.com/qris/stub",
	}

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(existing, nil)

	res, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.Nil(t, appErr)
	assert.Equal(t, testReservationID, res.ReservationID)
	assert.Equal(t, "A1", res.SpotCode)
	assert.Equal(t, 1, res.FloorNumber)
	assert.Equal(t, "https://payment-gateway.example.com/qris/stub", res.QRCodeURL)
	assert.Equal(t, model.ReservationStatusPendingPayment, res.Status)
}

func TestCreateReservation_IdempotencyDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).
		Return(nil, apperror.New("db_error", "failed to query reservation by idempotency key"))

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
}

// ── Active reservation guard ─────────────────────────────────────────────────

func TestCreateReservation_DriverHasActiveReservation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(true, nil)

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.NotNil(t, appErr)
	assert.Equal(t, "conflict", appErr.ErrorCode)
	assert.Contains(t, appErr.Message, "active reservation")
}

func TestCreateReservation_HasActiveReservationDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).
		Return(false, apperror.New("db_error", "failed to check active reservation"))

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
}

// ── USER_SELECTED mode ───────────────────────────────────────────────────────

func TestCreateReservation_UserSelected_MissingSpotID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)

	req := baseRequest(model.AssignmentModeUserSelected)
	req.SpotID = ""

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), req)

	require.NotNil(t, appErr)
	assert.Equal(t, "validation_error", appErr.ErrorCode)
}

func TestCreateReservation_UserSelected_LockNotAcquired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(false, nil)

	req := baseRequest(model.AssignmentModeUserSelected)
	req.SpotID = testSpotID

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), req)

	require.NotNil(t, appErr)
	assert.Equal(t, "conflict", appErr.ErrorCode)
}

func TestCreateReservation_UserSelected_LockAcquireError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).
		Return(false, apperror.New("redis_error", "failed to acquire spot lock"))

	req := baseRequest(model.AssignmentModeUserSelected)
	req.SpotID = testSpotID

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), req)

	require.NotNil(t, appErr)
	assert.Equal(t, "redis_error", appErr.ErrorCode)
}

func TestCreateReservation_UserSelected_SpotNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(true, nil)
	repo.EXPECT().GetSpotByID(gomock.Any(), testSpotID).
		Return(nil, apperror.New("not_found", "spot not found"))
	repo.EXPECT().ReleaseSpotLock(gomock.Any(), testSpotID).Return(nil)

	req := baseRequest(model.AssignmentModeUserSelected)
	req.SpotID = testSpotID

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), req)

	require.NotNil(t, appErr)
	assert.Equal(t, "not_found", appErr.ErrorCode)
}

func TestCreateReservation_UserSelected_SpotNotAvailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	lockedSpot := validSpot()
	lockedSpot.Status = model.SpotStatusLocked

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(true, nil)
	repo.EXPECT().GetSpotByID(gomock.Any(), testSpotID).Return(lockedSpot, nil)
	repo.EXPECT().ReleaseSpotLock(gomock.Any(), testSpotID).Return(nil)

	req := baseRequest(model.AssignmentModeUserSelected)
	req.SpotID = testSpotID

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), req)

	require.NotNil(t, appErr)
	assert.Equal(t, "conflict", appErr.ErrorCode)
	assert.Contains(t, appErr.Message, "not available")
}

func TestCreateReservation_UserSelected_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(true, nil)
	repo.EXPECT().GetSpotByID(gomock.Any(), testSpotID).Return(validSpot(), nil)
	repo.EXPECT().CreateReservationAndLockSpot(gomock.Any(), gomock.Any()).Return(validCreatedReservation(), nil)
	repo.EXPECT().ReleaseSpotLock(gomock.Any(), testSpotID).Return(nil)

	req := baseRequest(model.AssignmentModeUserSelected)
	req.SpotID = testSpotID

	res, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), req)

	require.Nil(t, appErr)
	assert.Equal(t, testReservationID, res.ReservationID)
	assert.Equal(t, testSpotID, res.SpotID)
	assert.Equal(t, "A1", res.SpotCode)
	assert.Equal(t, 1, res.FloorNumber)
	assert.Equal(t, model.ReservationStatusPendingPayment, res.Status)
	assert.NotEmpty(t, res.QRCodeURL)
}

// ── SYSTEM_ASSIGNED mode ─────────────────────────────────────────────────────

func TestCreateReservation_SystemAssigned_NoSpotsAvailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().GetAvailableSpot(gomock.Any(), model.VehicleTypeCar).
		Return(nil, apperror.New("no_spots_available", "no available spots for the requested vehicle type"))

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.NotNil(t, appErr)
	assert.Equal(t, "no_spots_available", appErr.ErrorCode)
}

func TestCreateReservation_SystemAssigned_LockNotAcquired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().GetAvailableSpot(gomock.Any(), model.VehicleTypeCar).Return(validSpot(), nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(false, nil)

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.NotNil(t, appErr)
	assert.Equal(t, "conflict", appErr.ErrorCode)
	assert.Contains(t, appErr.Message, "retry")
}

func TestCreateReservation_SystemAssigned_DBInsertError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().GetAvailableSpot(gomock.Any(), model.VehicleTypeCar).Return(validSpot(), nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(true, nil)
	repo.EXPECT().CreateReservationAndLockSpot(gomock.Any(), gomock.Any()).
		Return(nil, apperror.New("db_error", "failed to insert reservation"))
	repo.EXPECT().ReleaseSpotLock(gomock.Any(), testSpotID).Return(nil)

	_, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
}

func TestCreateReservation_SystemAssigned_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().GetAvailableSpot(gomock.Any(), model.VehicleTypeCar).Return(validSpot(), nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(true, nil)
	repo.EXPECT().CreateReservationAndLockSpot(gomock.Any(), gomock.Any()).Return(validCreatedReservation(), nil)
	repo.EXPECT().ReleaseSpotLock(gomock.Any(), testSpotID).Return(nil)

	res, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), baseRequest(model.AssignmentModeSystemAssigned))

	require.Nil(t, appErr)
	assert.Equal(t, testReservationID, res.ReservationID)
	assert.Equal(t, testSpotID, res.SpotID)
	assert.Equal(t, "A1", res.SpotCode)
	assert.Equal(t, 1, res.FloorNumber)
	assert.Equal(t, model.ReservationStatusPendingPayment, res.Status)
	assert.NotEmpty(t, res.QRCodeURL)
}

func TestCreateReservation_SystemAssigned_Motorcycle_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	motoSpot := &model.Spot{
		ID:          testSpotID,
		SpotCode:    "M1",
		FloorNumber: 2,
		VehicleType: model.VehicleTypeMotorcycle,
		Status:      model.SpotStatusAvailable,
	}
	created := validCreatedReservation()
	created.SpotCode = "M1"
	created.FloorNumber = 2

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByIdempotencyKey(gomock.Any(), testIdempotencyKey).Return(nil, nil)
	repo.EXPECT().HasActiveReservation(gomock.Any(), testDriverID).Return(false, nil)
	repo.EXPECT().GetAvailableSpot(gomock.Any(), model.VehicleTypeMotorcycle).Return(motoSpot, nil)
	repo.EXPECT().AcquireSpotLock(gomock.Any(), testSpotID, testDriverID).Return(true, nil)
	repo.EXPECT().CreateReservationAndLockSpot(gomock.Any(), gomock.Any()).Return(created, nil)
	repo.EXPECT().ReleaseSpotLock(gomock.Any(), testSpotID).Return(nil)

	req := baseRequest(model.AssignmentModeSystemAssigned)
	req.VehicleType = model.VehicleTypeMotorcycle

	res, appErr := newUsecase(repo, ctrl).CreateReservation(context.Background(), req)

	require.Nil(t, appErr)
	assert.Equal(t, "M1", res.SpotCode)
	assert.Equal(t, 2, res.FloorNumber)
}
