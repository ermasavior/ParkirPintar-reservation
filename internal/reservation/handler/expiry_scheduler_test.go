package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// ── fakePublisher ─────────────────────────────────────────────────────────────

type fakePublisher struct {
	published [][]byte
	err       error
}

func (f *fakePublisher) Publish(_ string, data []byte) error {
	f.published = append(f.published, data)
	return f.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newScheduler(repo *mockreservation.MockReservationRepository) *ExpiryScheduler {
	return &ExpiryScheduler{repo: repo, nc: nil, stop: make(chan struct{})}
}

func newSchedulerWithPublisher(repo *mockreservation.MockReservationRepository, pub natsPublisher) *ExpiryScheduler {
	return &ExpiryScheduler{repo: repo, nc: pub, stop: make(chan struct{})}
}

const (
	testExpiredReservationID = "880e8400-e29b-41d4-a716-446655440003"
	testExpiredSpotID        = "770e8400-e29b-41d4-a716-446655440002"
	testExpiredDriverID      = "660e8400-e29b-41d4-a716-446655440001"
)

// ── NewExpiryScheduler ────────────────────────────────────────────────────────

func TestNewExpiryScheduler_InitialisesFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	s := NewExpiryScheduler(repo, nil)

	assert.NotNil(t, s)
	assert.NotNil(t, s.stop)
	assert.Nil(t, s.nc)
}

// ── tick — no expired reservations ───────────────────────────────────────────

func TestTick_NoExpiredReservations_DoesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{}, nil)

	newScheduler(repo).tick()
}

// ── tick — query fails ────────────────────────────────────────────────────────

func TestTick_QueryFails_LogsAndReturns(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).
		Return(nil, apperror.New("db_error", "failed to query expired reservations"))

	newScheduler(repo).tick()
}

// ── tick — one expired reservation ───────────────────────────────────────────

func TestTick_OneExpiredReservation_ExpiresAndReleasesSpot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{
		{ID: testExpiredReservationID, DriverID: testExpiredDriverID, SpotID: testExpiredSpotID},
	}, nil)
	repo.EXPECT().ExpireReservationAndReleaseSpot(gomock.Any(), testExpiredReservationID, testExpiredSpotID).
		Return(nil)

	newScheduler(repo).tick()
}

// ── tick — multiple expired reservations ─────────────────────────────────────

func TestTick_MultipleExpiredReservations_ProcessesAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id1 := "aaa00000-e29b-41d4-a716-446655440001"
	id2 := "bbb00000-e29b-41d4-a716-446655440002"
	spot1 := "ccc00000-e29b-41d4-a716-446655440003"
	spot2 := "ddd00000-e29b-41d4-a716-446655440004"

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{
		{ID: id1, DriverID: testExpiredDriverID, SpotID: spot1},
		{ID: id2, DriverID: testExpiredDriverID, SpotID: spot2},
	}, nil)
	repo.EXPECT().ExpireReservationAndReleaseSpot(gomock.Any(), id1, spot1).Return(nil)
	repo.EXPECT().ExpireReservationAndReleaseSpot(gomock.Any(), id2, spot2).Return(nil)

	newScheduler(repo).tick()
}

// ── tick — one fails, continues with rest ────────────────────────────────────

func TestTick_OneExpireFails_ContinuesWithRest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id1 := "aaa00000-e29b-41d4-a716-446655440001"
	id2 := "bbb00000-e29b-41d4-a716-446655440002"
	spot1 := "ccc00000-e29b-41d4-a716-446655440003"
	spot2 := "ddd00000-e29b-41d4-a716-446655440004"

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{
		{ID: id1, DriverID: testExpiredDriverID, SpotID: spot1},
		{ID: id2, DriverID: testExpiredDriverID, SpotID: spot2},
	}, nil)
	repo.EXPECT().ExpireReservationAndReleaseSpot(gomock.Any(), id1, spot1).
		Return(apperror.New("db_error", "failed to expire reservation"))
	repo.EXPECT().ExpireReservationAndReleaseSpot(gomock.Any(), id2, spot2).Return(nil)

	newScheduler(repo).tick()
}

// ── publishExpired — nc is nil ────────────────────────────────────────────────

func TestPublishExpired_NilConn_DoesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	s := newScheduler(repo) // nc == nil

	// Must not panic
	assert.NotPanics(t, func() {
		s.publishExpired(context.Background(), model.Reservation{
			ID:       testExpiredReservationID,
			DriverID: testExpiredDriverID,
		})
	})
}

// ── publishExpired — publish succeeds ────────────────────────────────────────

func TestPublishExpired_Success_PublishesEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	pub := &fakePublisher{}
	s := newSchedulerWithPublisher(repo, pub)

	s.publishExpired(context.Background(), model.Reservation{
		ID:       testExpiredReservationID,
		DriverID: testExpiredDriverID,
	})

	assert.Len(t, pub.published, 1)
	assert.Contains(t, string(pub.published[0]), testExpiredReservationID)
}

// ── publishExpired — publish fails ───────────────────────────────────────────

func TestPublishExpired_PublishError_LogsAndContinues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	pub := &fakePublisher{err: errors.New("nats: connection closed")}
	s := newSchedulerWithPublisher(repo, pub)

	// Must not panic even when Publish returns an error
	assert.NotPanics(t, func() {
		s.publishExpired(context.Background(), model.Reservation{
			ID:       testExpiredReservationID,
			DriverID: testExpiredDriverID,
		})
	})
}

// ── tick — publishes event after successful expire ────────────────────────────

func TestTick_PublishesEventAfterExpire(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{
		{ID: testExpiredReservationID, DriverID: testExpiredDriverID, SpotID: testExpiredSpotID},
	}, nil)
	repo.EXPECT().ExpireReservationAndReleaseSpot(gomock.Any(), testExpiredReservationID, testExpiredSpotID).
		Return(nil)

	pub := &fakePublisher{}
	s := newSchedulerWithPublisher(repo, pub)
	s.tick()

	assert.Len(t, pub.published, 1)
}

// ── Start / Stop — stop channel branch in run() ───────────────────────────────

func TestStartStop_DoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{}, nil).AnyTimes()

	s := NewExpiryScheduler(repo, nil)
	s.Start()
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	assert.NotNil(t, s)
}

// TestRun_StopChannelBranch exercises the stop case in run() directly by
// using a scheduler with a very short ticker so the stop fires cleanly.
func TestRun_StopChannelBranch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{}, nil).AnyTimes()

	s := &ExpiryScheduler{
		repo: repo,
		nc:   nil,
		stop: make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		s.run()
		close(done)
	}()

	// Close stop immediately — run() should exit via the stop branch
	close(s.stop)

	select {
	case <-done:
		// run() exited cleanly via stop branch
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not exit after stop channel was closed")
	}
}
