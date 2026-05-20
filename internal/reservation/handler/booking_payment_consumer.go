package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/internal/reservation/usecase"
	"parkir-pintar/services/reservation/pkg/logger"

	"github.com/nats-io/nats.go"
)

const (
	subjectBookingDone    = "payment.booking.done"
	bookingDurableName    = "reservation-service"
	bookingStreamName     = "PAYMENTS"
)

type BookingPaymentConsumer struct {
	uc  usecase.Reservation
	nc  *nats.Conn
	sub *nats.Subscription
}

func NewBookingPaymentConsumer(nc *nats.Conn, uc usecase.Reservation) *BookingPaymentConsumer {
	return &BookingPaymentConsumer{nc: nc, uc: uc}
}

func (c *BookingPaymentConsumer) Start() error {
	js, err := c.nc.JetStream()
	if err != nil {
		return err
	}

	// Ensure stream exists (idempotent — payment service creates it too)
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     bookingStreamName,
		Subjects: []string{"payment.booking.done", "payment.parking.done"},
	})
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		logger.Warn(context.Background(), "BookingPaymentConsumer: AddStream warning",
			slog.String("error", err.Error()),
		)
	}

	sub, err := js.Subscribe(subjectBookingDone, c.handle,
		nats.Durable(bookingDurableName),
		nats.ManualAck(),
		nats.AckExplicit(),
	)
	if err != nil {
		return err
	}

	c.sub = sub
	logger.Info(context.Background(), "BookingPaymentConsumer: subscribed",
		slog.String("subject", subjectBookingDone),
		slog.String("durable", bookingDurableName),
	)
	return nil
}

func (c *BookingPaymentConsumer) Stop() {
	if c.sub != nil {
		_ = c.sub.Unsubscribe()
	}
}

func (c *BookingPaymentConsumer) handle(msg *nats.Msg) {
	ctx := context.Background()

	var event model.NATSPaymentDoneEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		logger.Error(ctx, "BookingPaymentConsumer: failed to unmarshal event",
			slog.String("error", err.Error()),
		)
		// Malformed message — terminate, never redeliver
		_ = msg.Term()
		return
	}

	logger.Info(ctx, "BookingPaymentConsumer: received event",
		slog.String("reservation_id", event.ReferenceID),
		slog.String("status", event.Status),
	)

	appErr := c.uc.HandleBookingPaymentDone(ctx, event.ReferenceID, event.Status)
	if appErr != nil {
		logger.Error(ctx, "BookingPaymentConsumer: HandleBookingPaymentDone failed",
			slog.String("error", appErr.Error()),
			slog.String("reservation_id", event.ReferenceID),
		)
		switch appErr.ErrorCode {
		case "not_found":
			_ = msg.Term()
		default:
			// Transient error (db_error) — redeliver after delay
			_ = msg.NakWithDelay(5 * time.Second)
		}
		return
	}

	_ = msg.Ack()
}
