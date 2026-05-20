// Package paymentclient provides a gRPC client for the Payment Service
// with a circuit breaker to prevent cascading failures.
package paymentclient

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	paymentpb "parkir-pintar/services/reservation/gen/payment/v1"
	"parkir-pintar/services/reservation/pkg/apperror"
	"parkir-pintar/services/reservation/pkg/logger"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// CreatePaymentResult holds the result of a CreatePayment call
type CreatePaymentResult struct {
	PaymentID string
	QRCodeURL string
}

// PaymentService is the interface the reservation usecase depends on.
type PaymentService interface {
	CreatePayment(ctx context.Context,
		idempotencyKey, referenceID, driverID string,
		paymentType paymentpb.PaymentType,
		amountIDR int64,
	) (*CreatePaymentResult, *apperror.AppError)
}

// caller is the function that actually makes the gRPC call — injectable for tests
type caller func(ctx context.Context,
	idempotencyKey, referenceID, driverID string,
	paymentType paymentpb.PaymentType,
	amountIDR int64,
) (*CreatePaymentResult, error)

// Client wraps the Payment Service gRPC client with a circuit breaker
type Client struct {
	cb     *gobreaker.CircuitBreaker[*CreatePaymentResult]
	callFn caller
}

// compile-time check
var _ PaymentService = (*Client)(nil)

// New creates a Client connected to the given address.
func New(target string) (*Client, error) {
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("paymentclient: failed to connect to %s: %w", target, err)
	}

	grpcClient := paymentpb.NewPaymentServiceClient(conn)

	callFn := func(ctx context.Context,
		idempotencyKey, referenceID, driverID string,
		paymentType paymentpb.PaymentType,
		amountIDR int64,
	) (*CreatePaymentResult, error) {
		resp, err := grpcClient.CreatePayment(ctx, &paymentpb.CreatePaymentRequest{
			IdempotencyKey: idempotencyKey,
			ReferenceId:    referenceID,
			PaymentType:    paymentType,
			AmountIdr:      amountIDR,
			DriverId:       driverID,
			Method:         paymentpb.PaymentMethod_PAYMENT_METHOD_QRIS,
		})
		if err != nil {
			return nil, err
		}
		return &CreatePaymentResult{
			PaymentID: resp.PaymentId,
			QRCodeURL: resp.QrCodeUrl,
		}, nil
	}

	return newWithCaller(callFn), nil
}

// newWithCaller creates a Client with an injected caller — used in tests
func newWithCaller(fn caller) *Client {
	cb := gobreaker.NewCircuitBreaker[*CreatePaymentResult](gobreaker.Settings{
		Name:        "payment-service",
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Info(context.Background(), "paymentclient: circuit breaker state changed",
				slog.String("name", name),
				slog.String("from", from.String()),
				slog.String("to", to.String()),
			)
		},
	})
	return &Client{cb: cb, callFn: fn}
}

// CreatePayment calls PaymentService.CreatePayment with circuit breaker protection.
func (c *Client) CreatePayment(ctx context.Context,
	idempotencyKey, referenceID, driverID string,
	paymentType paymentpb.PaymentType,
	amountIDR int64,
) (*CreatePaymentResult, *apperror.AppError) {

	result, err := c.cb.Execute(func() (*CreatePaymentResult, error) {
		return c.callFn(ctx, idempotencyKey, referenceID, driverID, paymentType, amountIDR)
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			logger.Error(ctx, "paymentclient: circuit breaker is OPEN — payment service unavailable",
				slog.String("reference_id", referenceID),
			)
			return nil, apperror.New("payment_service_unavailable", "payment service is temporarily unavailable, please retry later")
		}
		logger.Error(ctx, "paymentclient: CreatePayment failed",
			slog.String("reference_id", referenceID),
			slog.String("error", err.Error()),
		)
		return nil, apperror.New("payment_service_error", "failed to create payment: "+err.Error())
	}

	return result, nil
}
