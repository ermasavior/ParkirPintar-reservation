package reservation

import (
	pb "parkir-pintar/services/reservation/gen/reservation/v1"
	"parkir-pintar/services/reservation/internal/reservation/handler"
	"parkir-pintar/services/reservation/internal/reservation/repository"
	"parkir-pintar/services/reservation/internal/reservation/usecase"
	"parkir-pintar/services/reservation/pkg/paymentclient"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Service struct {
	repo            repository.Reservation
	uc              usecase.Reservation
	bookingConsumer *handler.BookingPaymentConsumer
	expiryScheduler *handler.ExpiryScheduler
}

func New(db *pgxpool.Pool, redisClient *redis.Client, nc *nats.Conn, pc paymentclient.PaymentService) *Service {
	repo := repository.NewReservation(db, redisClient)
	uc := usecase.NewReservation(repo, nc, pc)
	return &Service{
		repo:            repo,
		uc:              uc,
		bookingConsumer: handler.NewBookingPaymentConsumer(nc, uc),
		expiryScheduler: handler.NewExpiryScheduler(repo, nc),
	}
}

func (s *Service) Start() error {
	if err := s.bookingConsumer.Start(); err != nil {
		return err
	}
	s.expiryScheduler.Start()
	return nil
}

func (s *Service) Stop() {
	s.bookingConsumer.Stop()
	s.expiryScheduler.Stop()
}

func (s *Service) RegisterGRPC(grpcServer *grpc.Server) {
	srv := handler.NewReservationServer(s.uc)
	pb.RegisterReservationServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)
}
