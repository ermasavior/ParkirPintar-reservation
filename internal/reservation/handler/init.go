package handler

import (
	pb "parkir-pintar/services/reservation/gen/reservation/v1"
	"parkir-pintar/services/reservation/internal/reservation/usecase"
)

// ReservationServer implements the gRPC ReservationServiceServer interface
type ReservationServer struct {
	pb.UnimplementedReservationServiceServer
	uc usecase.Reservation
}

// NewReservationServer creates a new ReservationServer
func NewReservationServer(uc usecase.Reservation) *ReservationServer {
	return &ReservationServer{uc: uc}
}
