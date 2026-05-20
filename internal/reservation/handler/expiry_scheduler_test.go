package handler

import (
	"testing"
	"time"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func newScheduler(repo *mockreservation.MockReservationRepository) *ExpiryScheduler {
	return &ExpiryScheduler{repo: repo, nc: nil, stop: make(chan struct{})}
}

const (
	testExpiredReservationID = "880e8400-e29b-41d4-a716-446655440003"
	testExpiredSpotID        = "770e8400-e29b-41d4-a716-446655440002"
	testExpiredDriverID      = "660e8400-e29b-41d4-a716-446655440001"
)

// ── tick — no expired reservations ───────────────────────────────────────────

func TestTick_NoExpiredReservations_DoesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{}, nil)
	// No ExpireReservationAndReleaseSpot calls expected

	newScheduler(repo).tick()
}

// ── tick — query fails ────────────────────────────────────────────────────────

func TestTick_QueryFails_LogsAndReturns(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetExpiredReservations(gomock.Any()).
		Return(nil, apperror.New("db_error", "failed to query expired reservations"))
	// No ExpireReservationAndReleaseSpot calls expected

	newScheduler(repo).tick() // should not panic
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
	// id2 must still be processed despite id1 failing
	repo.EXPECT().ExpireReservationAndReleaseSpot(gomock.Any(), id2, spot2).Return(nil)

	newScheduler(repo).tick()
}

// ── Start / Stop lifecycle ────────────────────────────────────────────────────

func TestStartStop_DoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	// Allow any number of GetExpiredReservations calls during the brief run
	repo.EXPECT().GetExpiredReservations(gomock.Any()).Return([]model.Reservation{}, nil).AnyTimes()

	s := NewExpiryScheduler(repo, nil)
	s.Start()

	// Let it run briefly then stop
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	assert.NotNil(t, s)
}
