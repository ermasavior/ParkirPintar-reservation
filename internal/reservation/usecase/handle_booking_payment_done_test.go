package usecase

import (
	"context"
	"testing"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testHandleReservationID = "880e8400-e29b-41d4-a716-446655440003"
	testHandleSpotID        = "770e8400-e29b-41d4-a716-446655440002"
	testHandleDriverID      = "660e8400-e29b-41d4-a716-446655440001"
)

func newUsecaseNoNATS(repo *mockreservation.MockReservationRepository) *ReservationUsecase {
	return &ReservationUsecase{repo: repo, natsConn: nil, paymentClient: nil}
}

func validReservation() *model.Reservation {
	return &model.Reservation{
		ID:       testHandleReservationID,
		DriverID: testHandleDriverID,
		SpotID:   testHandleSpotID,
		Status:   model.ReservationStatusPendingPayment,
	}
}

// ── SUCCESS ───────────────────────────────────────────────────────────────────

func TestHandleBookingPaymentDone_Success_Confirms(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testHandleReservationID).Return(validReservation(), nil)
	repo.EXPECT().ConfirmReservation(gomock.Any(), testHandleReservationID).Return(nil)

	appErr := newUsecaseNoNATS(repo).HandleBookingPaymentDone(context.Background(), testHandleReservationID, "SUCCESS")

	require.Nil(t, appErr)
}

func TestHandleBookingPaymentDone_Success_ConfirmFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testHandleReservationID).Return(validReservation(), nil)
	repo.EXPECT().ConfirmReservation(gomock.Any(), testHandleReservationID).
		Return(apperror.New("db_error", "failed to confirm reservation"))

	appErr := newUsecaseNoNATS(repo).HandleBookingPaymentDone(context.Background(), testHandleReservationID, "SUCCESS")

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
}

// ── FAILED ────────────────────────────────────────────────────────────────────

func TestHandleBookingPaymentDone_Failed_CancelsAndReleasesSpot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testHandleReservationID).Return(validReservation(), nil)
	repo.EXPECT().CancelReservationAndReleaseSpot(gomock.Any(), testHandleReservationID, testHandleSpotID).Return(nil)

	appErr := newUsecaseNoNATS(repo).HandleBookingPaymentDone(context.Background(), testHandleReservationID, "FAILED")

	require.Nil(t, appErr)
}

func TestHandleBookingPaymentDone_Failed_CancelFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testHandleReservationID).Return(validReservation(), nil)
	repo.EXPECT().CancelReservationAndReleaseSpot(gomock.Any(), testHandleReservationID, testHandleSpotID).
		Return(apperror.New("db_error", "failed to cancel reservation"))

	appErr := newUsecaseNoNATS(repo).HandleBookingPaymentDone(context.Background(), testHandleReservationID, "FAILED")

	require.NotNil(t, appErr)
	assert.Equal(t, "db_error", appErr.ErrorCode)
}

// ── EXPIRED ───────────────────────────────────────────────────────────────────

func TestHandleBookingPaymentDone_Expired_CancelsAndReleasesSpot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testHandleReservationID).Return(validReservation(), nil)
	repo.EXPECT().CancelReservationAndReleaseSpot(gomock.Any(), testHandleReservationID, testHandleSpotID).Return(nil)

	appErr := newUsecaseNoNATS(repo).HandleBookingPaymentDone(context.Background(), testHandleReservationID, "EXPIRED")

	require.Nil(t, appErr)
}

// ── Reservation not found ─────────────────────────────────────────────────────

func TestHandleBookingPaymentDone_ReservationNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testHandleReservationID).
		Return(nil, apperror.New("not_found", "reservation not found"))

	appErr := newUsecaseNoNATS(repo).HandleBookingPaymentDone(context.Background(), testHandleReservationID, "SUCCESS")

	require.NotNil(t, appErr)
	assert.Equal(t, "not_found", appErr.ErrorCode)
}

// ── Unknown status ────────────────────────────────────────────────────────────

func TestHandleBookingPaymentDone_UnknownStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mockreservation.NewMockReservationRepository(ctrl)
	repo.EXPECT().GetByID(gomock.Any(), testHandleReservationID).Return(validReservation(), nil)

	appErr := newUsecaseNoNATS(repo).HandleBookingPaymentDone(context.Background(), testHandleReservationID, "UNKNOWN")

	require.NotNil(t, appErr)
	assert.Equal(t, "validation_error", appErr.ErrorCode)
}
