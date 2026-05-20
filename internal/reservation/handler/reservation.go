package handler

import (
	"context"
	"log/slog"

	pb "parkir-pintar/services/reservation/gen/reservation/v1"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/logger"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *ReservationServer) CreateReservation(ctx context.Context, req *pb.CreateReservationRequest) (*pb.CreateReservationResponse, error) {
	if !validateUUID(req.IdempotencyKey) {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key must be a valid UUID")
	}
	if !validateUUID(req.DriverId) {
		return nil, status.Error(codes.InvalidArgument, "driver_id must be a valid UUID")
	}
	if req.Mode == pb.AssignmentMode_ASSIGNMENT_MODE_USER_SELECTED {
		if !validateUUID(req.SpotId) {
			return nil, status.Error(codes.InvalidArgument, "spot_id must be a valid UUID for USER_SELECTED mode")
		}
	}

	ucReq := model.CreateReservationRequest{
		IdempotencyKey: req.IdempotencyKey,
		DriverID:       req.DriverId,
		VehicleType:    model.VehicleType(req.VehicleType),
		Mode:           model.AssignmentMode(req.Mode),
		SpotID:         req.SpotId,
	}

	res, appErr := s.uc.CreateReservation(ctx, ucReq)
	if appErr != nil {
		logger.Error(ctx, "CreateReservation failed", slog.String("error", appErr.Error()))
		switch appErr.ErrorCode {
		case "conflict":
			return nil, status.Error(codes.AlreadyExists, appErr.Message)
		case "no_spots_available":
			return nil, status.Error(codes.ResourceExhausted, appErr.Message)
		case "not_found":
			return nil, status.Error(codes.NotFound, appErr.Message)
		case "validation_error":
			return nil, status.Error(codes.InvalidArgument, appErr.Message)
		default:
			return nil, status.Error(codes.Internal, appErr.Message)
		}
	}

	return &pb.CreateReservationResponse{
		ReservationId: res.ReservationID,
		SpotId:        res.SpotID,
		SpotCode:      res.SpotCode,
		FloorNumber:   int32(res.FloorNumber),
		Status:        pb.ReservationStatus(res.Status),
		QrCodeUrl:     res.QRCodeURL,
	}, nil
}

func (s *ReservationServer) GetReservation(ctx context.Context, req *pb.GetReservationRequest) (*pb.GetReservationResponse, error) {
	if !validateUUID(req.ReservationId) {
		return nil, status.Error(codes.InvalidArgument, "reservation_id must be a valid UUID")
	}

	res, appErr := s.uc.GetReservation(ctx, req.ReservationId)
	if appErr != nil {
		logger.Error(ctx, "GetReservation failed", slog.String("error", appErr.Error()))
		switch appErr.ErrorCode {
		case "not_found":
			return nil, status.Error(codes.NotFound, appErr.Message)
		default:
			return nil, status.Error(codes.Internal, appErr.Message)
		}
	}

	pbRes := &pb.GetReservationResponse{
		ReservationId:  res.ReservationID,
		DriverId:       res.DriverID,
		SpotId:         res.SpotID,
		SpotCode:       res.SpotCode,
		FloorNumber:    int32(res.FloorNumber),
		VehicleType:    pb.VehicleType(res.VehicleType),
		AssignmentMode: pb.AssignmentMode(res.AssignmentMode),
		Status:         pb.ReservationStatus(res.Status),
		CreatedAt:      timestamppb.New(res.CreatedAt),
	}

	if res.ConfirmedAt != nil {
		pbRes.ConfirmedAt = timestamppb.New(*res.ConfirmedAt)
	}
	if res.ExpiresAt != nil {
		pbRes.ExpiresAt = timestamppb.New(*res.ExpiresAt)
	}

	return pbRes, nil
}
