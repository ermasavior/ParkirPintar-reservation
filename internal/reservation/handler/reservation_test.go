package handler

import (
	"context"
	"testing"
	"time"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	pb "parkir-pintar/services/reservation/gen/reservation/v1"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	validIdemKey       = "550e8400-e29b-41d4-a716-446655440000"
	validDriverID      = "660e8400-e29b-41d4-a716-446655440001"
	validSpotID        = "770e8400-e29b-41d4-a716-446655440002"
	validReservationID = "880e8400-e29b-41d4-a716-446655440003"
)

func newServer(uc *mockreservation.MockReservationUsecase) *ReservationServer {
	return &ReservationServer{uc: uc}
}

func grpcCode(err error) codes.Code {
	if s, ok := status.FromError(err); ok {
		return s.Code()
	}
	return codes.Unknown
}

// ── CreateReservation — validation ───────────────────────────────────────────

func TestCreateReservation_InvalidIdempotencyKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv := newServer(mockreservation.NewMockReservationUsecase(ctrl))

	_, err := srv.CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: "not-a-uuid",
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
	assert.Contains(t, status.Convert(err).Message(), "idempotency_key")
}

func TestCreateReservation_InvalidDriverID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv := newServer(mockreservation.NewMockReservationUsecase(ctrl))

	_, err := srv.CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       "bad-driver",
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
	assert.Contains(t, status.Convert(err).Message(), "driver_id")
}

func TestCreateReservation_UserSelected_InvalidSpotID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv := newServer(mockreservation.NewMockReservationUsecase(ctrl))

	_, err := srv.CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_USER_SELECTED,
		SpotId:         "not-a-uuid",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
	assert.Contains(t, status.Convert(err).Message(), "spot_id")
}

func TestCreateReservation_UserSelected_EmptySpotID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv := newServer(mockreservation.NewMockReservationUsecase(ctrl))

	_, err := srv.CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_USER_SELECTED,
		SpotId:         "", // empty = invalid UUID
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

// ── CreateReservation — usecase error mapping ─────────────────────────────────

func TestCreateReservation_Conflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().CreateReservation(gomock.Any(), gomock.Any()).
		Return(nil, apperror.New("conflict", "driver already has an active reservation"))

	_, err := newServer(uc).CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.Error(t, err)
	assert.Equal(t, codes.AlreadyExists, grpcCode(err))
}

func TestCreateReservation_NoSpotsAvailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().CreateReservation(gomock.Any(), gomock.Any()).
		Return(nil, apperror.New("no_spots_available", "no available spots"))

	_, err := newServer(uc).CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.Error(t, err)
	assert.Equal(t, codes.ResourceExhausted, grpcCode(err))
}

func TestCreateReservation_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().CreateReservation(gomock.Any(), gomock.Any()).
		Return(nil, apperror.New("not_found", "spot not found"))

	_, err := newServer(uc).CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestCreateReservation_ValidationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().CreateReservation(gomock.Any(), gomock.Any()).
		Return(nil, apperror.New("validation_error", "spot_id is required for USER_SELECTED mode"))

	_, err := newServer(uc).CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

func TestCreateReservation_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().CreateReservation(gomock.Any(), gomock.Any()).
		Return(nil, apperror.New("db_error", "failed to insert reservation"))

	_, err := newServer(uc).CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, grpcCode(err))
}

// ── CreateReservation — success ───────────────────────────────────────────────

func TestCreateReservation_SystemAssigned_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().CreateReservation(gomock.Any(), gomock.Any()).
		Return(&model.CreateReservationResponse{
			ReservationID: validReservationID,
			SpotID:        validSpotID,
			SpotCode:      "A1",
			FloorNumber:   1,
			Status:        model.ReservationStatusPendingPayment,
			QRCodeURL:     "https://qr.example.com/stub",
		}, nil)

	res, err := newServer(uc).CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED,
	})

	require.NoError(t, err)
	assert.Equal(t, validReservationID, res.ReservationId)
	assert.Equal(t, validSpotID, res.SpotId)
	assert.Equal(t, "A1", res.SpotCode)
	assert.Equal(t, int32(1), res.FloorNumber)
	assert.Equal(t, pb.ReservationStatus_RESERVATION_STATUS_PENDING_PAYMENT, res.Status)
	assert.Equal(t, "https://qr.example.com/stub", res.QrCodeUrl)
}

func TestCreateReservation_UserSelected_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().CreateReservation(gomock.Any(), gomock.Any()).
		Return(&model.CreateReservationResponse{
			ReservationID: validReservationID,
			SpotID:        validSpotID,
			SpotCode:      "B2",
			FloorNumber:   2,
			Status:        model.ReservationStatusPendingPayment,
			QRCodeURL:     "https://qr.example.com/stub",
		}, nil)

	res, err := newServer(uc).CreateReservation(context.Background(), &pb.CreateReservationRequest{
		IdempotencyKey: validIdemKey,
		DriverId:       validDriverID,
		VehicleType:    pb.VehicleType_VEHICLE_TYPE_CAR,
		Mode:           pb.AssignmentMode_ASSIGNMENT_MODE_USER_SELECTED,
		SpotId:         validSpotID,
	})

	require.NoError(t, err)
	assert.Equal(t, validReservationID, res.ReservationId)
	assert.Equal(t, "B2", res.SpotCode)
	assert.Equal(t, int32(2), res.FloorNumber)
}

// ── GetReservation — validation ───────────────────────────────────────────────

func TestGetReservation_InvalidReservationID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv := newServer(mockreservation.NewMockReservationUsecase(ctrl))

	_, err := srv.GetReservation(context.Background(), &pb.GetReservationRequest{
		ReservationId: "not-a-uuid",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
	assert.Contains(t, status.Convert(err).Message(), "reservation_id")
}

func TestGetReservation_EmptyReservationID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	srv := newServer(mockreservation.NewMockReservationUsecase(ctrl))

	_, err := srv.GetReservation(context.Background(), &pb.GetReservationRequest{
		ReservationId: "",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, grpcCode(err))
}

// ── GetReservation — usecase error mapping ────────────────────────────────────

func TestGetReservation_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().GetReservation(gomock.Any(), validReservationID).
		Return(nil, apperror.New("not_found", "reservation not found"))

	_, err := newServer(uc).GetReservation(context.Background(), &pb.GetReservationRequest{
		ReservationId: validReservationID,
	})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, grpcCode(err))
}

func TestGetReservation_InternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().GetReservation(gomock.Any(), validReservationID).
		Return(nil, apperror.New("db_error", "failed to query reservation"))

	_, err := newServer(uc).GetReservation(context.Background(), &pb.GetReservationRequest{
		ReservationId: validReservationID,
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, grpcCode(err))
}

// ── GetReservation — success ──────────────────────────────────────────────────

func TestGetReservation_Success_WithOptionalFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	now := time.Now()
	confirmedAt := now.Add(-30 * time.Minute)
	expiresAt := now.Add(30 * time.Minute)

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().GetReservation(gomock.Any(), validReservationID).
		Return(&model.GetReservationResponse{
			ReservationID:  validReservationID,
			DriverID:       validDriverID,
			SpotID:         validSpotID,
			SpotCode:       "A1",
			FloorNumber:    1,
			VehicleType:    model.VehicleTypeCar,
			AssignmentMode: model.AssignmentModeSystemAssigned,
			Status:         model.ReservationStatusConfirmed,
			ConfirmedAt:    &confirmedAt,
			ExpiresAt:      &expiresAt,
			CreatedAt:      now,
		}, nil)

	res, err := newServer(uc).GetReservation(context.Background(), &pb.GetReservationRequest{
		ReservationId: validReservationID,
	})

	require.NoError(t, err)
	assert.Equal(t, validReservationID, res.ReservationId)
	assert.Equal(t, validDriverID, res.DriverId)
	assert.Equal(t, validSpotID, res.SpotId)
	assert.Equal(t, "A1", res.SpotCode)
	assert.Equal(t, int32(1), res.FloorNumber)
	assert.Equal(t, pb.VehicleType_VEHICLE_TYPE_CAR, res.VehicleType)
	assert.Equal(t, pb.AssignmentMode_ASSIGNMENT_MODE_SYSTEM_ASSIGNED, res.AssignmentMode)
	assert.Equal(t, pb.ReservationStatus_RESERVATION_STATUS_CONFIRMED, res.Status)
	assert.NotNil(t, res.ConfirmedAt)
	assert.NotNil(t, res.ExpiresAt)
}

func TestGetReservation_Success_NilOptionalFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().GetReservation(gomock.Any(), validReservationID).
		Return(&model.GetReservationResponse{
			ReservationID:  validReservationID,
			DriverID:       validDriverID,
			SpotID:         validSpotID,
			SpotCode:       "C3",
			FloorNumber:    3,
			VehicleType:    model.VehicleTypeCar,
			AssignmentMode: model.AssignmentModeUserSelected,
			Status:         model.ReservationStatusPendingPayment,
			ConfirmedAt:    nil,
			ExpiresAt:      nil,
			CreatedAt:      time.Now(),
		}, nil)

	res, err := newServer(uc).GetReservation(context.Background(), &pb.GetReservationRequest{
		ReservationId: validReservationID,
	})

	require.NoError(t, err)
	assert.Nil(t, res.ConfirmedAt)
	assert.Nil(t, res.ExpiresAt)
	assert.Equal(t, pb.ReservationStatus_RESERVATION_STATUS_PENDING_PAYMENT, res.Status)
}
