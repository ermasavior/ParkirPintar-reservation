package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"parkir-pintar/services/reservation/internal/reservation"
	"parkir-pintar/services/reservation/pkg/config"
	"parkir-pintar/services/reservation/pkg/dotenv"
	"parkir-pintar/services/reservation/pkg/logger"
	pkgOtel "parkir-pintar/services/reservation/pkg/otel"
	"parkir-pintar/services/reservation/pkg/paymentclient"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func main() {
	dotenv.LoadEnv()

	cfg := config.Config{
		Log: config.LogConfig{
			Level:  dotenv.GetEnv("LOG_LEVEL", "info"),
			Format: dotenv.GetEnv("LOG_FORMAT", "json"),
		},
		OTEL: config.OTELConfig{
			ServiceName: dotenv.GetEnv("APP_NAME", "reservation-service"),
			Endpoint:    dotenv.GetEnv("OTEL_RECEIVER_OTLP_ENDPOINT", ""),
			Insecure:    true,
		},
	}
	logger.SetupLogger(cfg.Log)

	otel := pkgOtel.NewOpenTelemetry(cfg.OTEL.Endpoint, cfg.OTEL.ServiceName, dotenv.GetEnv("APP_ENV", "local"))

	ctx := context.Background()

	// PostgreSQL
	pool, err := pgxpool.New(ctx, dotenv.GetEnv("POSTGRES_DSN", ""))
	if err != nil {
		logger.Error(ctx, "failed to create postgres pool", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error(ctx, "failed to connect to postgres", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info(ctx, "connected to postgres")

	// Redis
	redisOpt, err := redis.ParseURL(dotenv.GetEnv("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		logger.Error(ctx, "failed to parse redis URL", slog.String("error", err.Error()))
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpt)
	defer redisClient.Close()

	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		logger.Error(ctx, "failed to connect to redis", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info(ctx, "connected to redis")

	// NATS
	natsURL := dotenv.GetEnv("NATS_URL", nats.DefaultURL)
	nc, err := nats.Connect(natsURL)
	if err != nil {
		logger.Error(ctx, "failed to connect to NATS", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer nc.Close()
	logger.Info(ctx, "connected to NATS")

	// Payment Service gRPC client
	paymentServiceURL := dotenv.GetEnv("PAYMENT_SERVICE_URL", "localhost:8086")
	pc, err := paymentclient.New(paymentServiceURL)
	if err != nil {
		logger.Error(ctx, "failed to create payment service client", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info(ctx, "payment service client created", slog.String("target", paymentServiceURL))

	// Reservation domain
	svc := reservation.New(pool, redisClient, nc, pc)

	if err := svc.Start(); err != nil {
		logger.Error(ctx, "failed to start reservation service", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer svc.Stop()

	// gRPC server
	port := dotenv.GetEnv("APP_PORT", "8081")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		logger.Error(ctx, "failed to listen", slog.String("port", port), slog.String("error", err.Error()))
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	svc.RegisterGRPC(grpcServer)

	go func() {
		logger.Info(ctx, "reservation gRPC server listening", slog.String("port", port))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error(ctx, "gRPC server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info(ctx, "shutting down reservation service...")
	grpcServer.GracefulStop()
	logger.Info(ctx, "reservation service stopped")

	if err := otel.EndAPM(ctx); err != nil {
		logger.Error(ctx, err.Error(), nil)
	}
}
