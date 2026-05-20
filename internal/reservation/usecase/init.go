package usecase

import (
	"context"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/internal/reservation/repository"
	"parkir-pintar/services/reservation/pkg/apperror"
	"parkir-pintar/services/reservation/pkg/paymentclient"
)

// Reservation defines the business logic contract for the reservation domain
type Reservation interface {
	// CreateReservation handles the full reservation creation flow including spot locking and payment initiation
	CreateReservation(ctx context.Context, req model.CreateReservationRequest) (*model.CreateReservationResponse, *apperror.AppError)

	// GetReservation retrieves a reservation by its UUID
	GetReservation(ctx context.Context, reservationID string) (*model.GetReservationResponse, *apperror.AppError)

	// HandleBookingPaymentDone processes a payment.booking.done NATS event
	HandleBookingPaymentDone(ctx context.Context, reservationID string, status string) *apperror.AppError
}

// ReservationUsecase is the concrete implementation
type ReservationUsecase struct {
	repo          repository.Reservation
	natsConn      interface{ Publish(string, []byte) error }
	paymentClient paymentclient.PaymentService
}

// NewReservation creates a new ReservationUsecase with all required dependencies
func NewReservation(repo repository.Reservation, nc interface{ Publish(string, []byte) error }, pc paymentclient.PaymentService) Reservation {
	return &ReservationUsecase{
		repo:          repo,
		natsConn:      nc,
		paymentClient: pc,
	}
}
