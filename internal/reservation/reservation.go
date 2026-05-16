package reservation

import (
	pb "parkir-pintar/services/reservation/gen/reservation/v1"
	"parkir-pintar/services/reservation/internal/reservation/handler"
	"parkir-pintar/services/reservation/internal/reservation/repository"
	"parkir-pintar/services/reservation/internal/reservation/usecase"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

// RegisterGRPC wires up the reservation domain and registers it on the gRPC server
func RegisterGRPC(grpcServer *grpc.Server, db *pgxpool.Pool, redisClient *redis.Client, paymentBaseURL string) {
	repo := repository.NewReservation(db, redisClient)
	uc := usecase.NewReservation(repo, paymentBaseURL)
	srv := handler.NewReservationServer(uc)

	pb.RegisterReservationServiceServer(grpcServer, srv)
}
