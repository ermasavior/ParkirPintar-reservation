package reservation

import (
	"testing"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	"parkir-pintar/services/reservation/internal/reservation/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeConsumer struct {
	startErr error
	stopped  bool
}

func (f *fakeConsumer) Start() error { return f.startErr }
func (f *fakeConsumer) Stop()        { f.stopped = true }

type fakeScheduler struct {
	started bool
	stopped bool
}

func (f *fakeScheduler) Start() { f.started = true }
func (f *fakeScheduler) Stop()  { f.stopped = true }

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestService(
	repo *mockreservation.MockReservationRepository,
	uc *mockreservation.MockReservationUsecase,
	consumer bookingConsumer,
	scheduler expiryScheduler,
) *Service {
	return &Service{
		repo:            repo,
		uc:              uc,
		bookingConsumer: consumer,
		expiryScheduler: scheduler,
	}
}

// ── Start ─────────────────────────────────────────────────────────────────────

func TestService_Start_BookingConsumerError_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	uc := mockreservation.NewMockReservationUsecase(ctrl)
	consumer := &fakeConsumer{startErr: assert.AnError}
	scheduler := &fakeScheduler{}

	svc := newTestService(repo, uc, consumer, scheduler)
	err := svc.Start()

	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
	// Scheduler must NOT be started when consumer fails
	assert.False(t, scheduler.started)
}

func TestService_Start_Success_StartsScheduler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	// Allow any scheduler ticks that may fire during the test
	repo.EXPECT().GetExpiredReservations(gomock.Any()).
		Return([]model.Reservation{}, nil).AnyTimes()
	uc := mockreservation.NewMockReservationUsecase(ctrl)
	consumer := &fakeConsumer{}
	scheduler := &fakeScheduler{}

	svc := newTestService(repo, uc, consumer, scheduler)
	err := svc.Start()

	require.NoError(t, err)
	assert.True(t, scheduler.started)
}

// ── Stop ──────────────────────────────────────────────────────────────────────

func TestService_Stop_StopsConsumerAndScheduler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	uc := mockreservation.NewMockReservationUsecase(ctrl)
	consumer := &fakeConsumer{}
	scheduler := &fakeScheduler{}

	svc := newTestService(repo, uc, consumer, scheduler)

	assert.NotPanics(t, func() { svc.Stop() })
	assert.True(t, consumer.stopped)
	assert.True(t, scheduler.stopped)
}

// ── RegisterGRPC ──────────────────────────────────────────────────────────────

func TestService_RegisterGRPC_RegistersServiceWithoutPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	uc := mockreservation.NewMockReservationUsecase(ctrl)
	consumer := &fakeConsumer{}
	scheduler := &fakeScheduler{}

	svc := newTestService(repo, uc, consumer, scheduler)
	grpcServer := grpc.NewServer()

	assert.NotPanics(t, func() {
		svc.RegisterGRPC(grpcServer)
	})

	info := grpcServer.GetServiceInfo()
	_, registered := info["reservation.v1.ReservationService"]
	assert.True(t, registered, "ReservationService should be registered on the gRPC server")
}
