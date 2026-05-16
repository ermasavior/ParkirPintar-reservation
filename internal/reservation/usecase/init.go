package usecase

import (
	"context"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/internal/reservation/repository"
	"parkir-pintar/services/reservation/pkg/apperror"
)

// Reservation defines the business logic contract for the reservation domain
type Reservation interface {
	// CreateReservation handles the full reservation creation flow including spot locking and payment initiation
	CreateReservation(ctx context.Context, req model.CreateReservationRequest) (*model.CreateReservationResponse, *apperror.AppError)

	// GetReservation retrieves a reservation by its UUID
	GetReservation(ctx context.Context, reservationID string) (*model.GetReservationResponse, *apperror.AppError)
}

// ReservationUsecase is the concrete implementation
type ReservationUsecase struct {
	repo           repository.Reservation
	paymentBaseURL string // base URL of the Payment Service (gRPC or HTTP)
}

// NewReservation creates a new ReservationUsecase
func NewReservation(repo repository.Reservation, paymentBaseURL string) Reservation {
	return &ReservationUsecase{
		repo:           repo,
		paymentBaseURL: paymentBaseURL,
	}
}
