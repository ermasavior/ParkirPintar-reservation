package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/internal/reservation/repository"
	"parkir-pintar/services/reservation/pkg/logger"

	"github.com/nats-io/nats.go"
)

const (
	schedulerInterval      = 30 * time.Second
	subjectReservationExpired = "reservation.expired"
)

type ExpiryScheduler struct {
	repo repository.Reservation
	nc   *nats.Conn
	stop chan struct{}
}

func NewExpiryScheduler(repo repository.Reservation, nc *nats.Conn) *ExpiryScheduler {
	return &ExpiryScheduler{
		repo: repo,
		nc:   nc,
		stop: make(chan struct{}),
	}
}

func (s *ExpiryScheduler) Start() {
	go s.run()
	logger.Info(context.Background(), "ExpiryScheduler: started",
		slog.Duration("interval", schedulerInterval),
	)
}

func (s *ExpiryScheduler) Stop() {
	close(s.stop)
}

func (s *ExpiryScheduler) run() {
	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			logger.Info(context.Background(), "ExpiryScheduler: stopped")
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *ExpiryScheduler) tick() {
	ctx := context.Background()
	logger.Info(ctx, "ExpiryScheduler: scanning expired reservations")

	expired, appErr := s.repo.GetExpiredReservations(ctx)
	if appErr != nil {
		logger.Error(ctx, "ExpiryScheduler: failed to query expired reservations",
			slog.String("error", appErr.Error()),
		)
		return
	}

	if len(expired) == 0 {
		return
	}

	logger.Info(ctx, "ExpiryScheduler: processing expired reservations",
		slog.Int("count", len(expired)),
	)

	for _, res := range expired {
		if appErr := s.repo.ExpireReservationAndReleaseSpot(ctx, res.ID, res.SpotID); appErr != nil {
			logger.Error(ctx, "ExpiryScheduler: failed to expire reservation",
				slog.String("reservation_id", res.ID),
				slog.String("error", appErr.Error()),
			)
			// Continue processing remaining reservations — don't abort the batch
			continue
		}

		s.publishExpired(ctx, res)

		logger.Info(ctx, "ExpiryScheduler: reservation expired",
			slog.String("reservation_id", res.ID),
			slog.String("spot_id", res.SpotID),
		)
	}
}

func (s *ExpiryScheduler) publishExpired(ctx context.Context, res model.Reservation) {
	if s.nc == nil {
		return
	}

	event := model.NATSReservationEvent{
		ReservationID: res.ID,
		DriverID:      res.DriverID,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		logger.Error(ctx, "ExpiryScheduler: failed to marshal event",
			slog.String("reservation_id", res.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	if err := s.nc.Publish(subjectReservationExpired, payload); err != nil {
		logger.Error(ctx, "ExpiryScheduler: failed to publish reservation.expired",
			slog.String("reservation_id", res.ID),
			slog.String("error", err.Error()),
		)
	}
}
