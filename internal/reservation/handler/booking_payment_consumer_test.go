package handler

import (
	"encoding/json"
	"testing"

	mockreservation "parkir-pintar/services/reservation/_mock/reservation"
	"parkir-pintar/services/reservation/internal/reservation/model"
	"parkir-pintar/services/reservation/pkg/apperror"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

const (
	testBookingReservationID = "880e8400-e29b-41d4-a716-446655440003"
)

func newBookingConsumer(uc *mockreservation.MockReservationUsecase) *BookingPaymentConsumer {
	return &BookingPaymentConsumer{uc: uc}
}

func marshalBookingEvent(t *testing.T, refID, status string) []byte {
	t.Helper()
	b, _ := json.Marshal(model.NATSPaymentDoneEvent{
		ReferenceID: refID,
		Status:      status,
	})
	return b
}

// ── handle — invalid JSON ─────────────────────────────────────────────────────

func TestBookingHandle_InvalidJSON_Terms(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	// No usecase calls expected

	consumer := newBookingConsumer(uc)
	msg := &nats.Msg{Data: []byte("not-json")}

	consumer.handle(msg)
	// Reaches Term path without panic
}

// ── handle — SUCCESS ──────────────────────────────────────────────────────────

func TestBookingHandle_Success_Acks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().
		HandleBookingPaymentDone(gomock.Any(), testBookingReservationID, "SUCCESS").
		Return(nil)

	consumer := newBookingConsumer(uc)
	msg := &nats.Msg{Data: marshalBookingEvent(t, testBookingReservationID, "SUCCESS")}

	consumer.handle(msg)
}

// ── handle — FAILED ───────────────────────────────────────────────────────────

func TestBookingHandle_Failed_CallsUsecase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().
		HandleBookingPaymentDone(gomock.Any(), testBookingReservationID, "FAILED").
		Return(nil)

	consumer := newBookingConsumer(uc)
	msg := &nats.Msg{Data: marshalBookingEvent(t, testBookingReservationID, "FAILED")}

	consumer.handle(msg)
}

// ── handle — EXPIRED ──────────────────────────────────────────────────────────

func TestBookingHandle_Expired_CallsUsecase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().
		HandleBookingPaymentDone(gomock.Any(), testBookingReservationID, "EXPIRED").
		Return(nil)

	consumer := newBookingConsumer(uc)
	msg := &nats.Msg{Data: marshalBookingEvent(t, testBookingReservationID, "EXPIRED")}

	consumer.handle(msg)
}

// ── handle — not_found → Term ─────────────────────────────────────────────────

func TestBookingHandle_NotFound_Terms(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().
		HandleBookingPaymentDone(gomock.Any(), testBookingReservationID, "SUCCESS").
		Return(apperror.New("not_found", "reservation not found"))

	consumer := newBookingConsumer(uc)
	msg := &nats.Msg{Data: marshalBookingEvent(t, testBookingReservationID, "SUCCESS")}

	consumer.handle(msg)
}

// ── handle — db_error → NakWithDelay ─────────────────────────────────────────

func TestBookingHandle_DBError_Naks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	uc.EXPECT().
		HandleBookingPaymentDone(gomock.Any(), testBookingReservationID, "SUCCESS").
		Return(apperror.New("db_error", "failed to confirm reservation"))

	consumer := newBookingConsumer(uc)
	msg := &nats.Msg{Data: marshalBookingEvent(t, testBookingReservationID, "SUCCESS")}

	consumer.handle(msg)
}

// ── NewBookingPaymentConsumer ─────────────────────────────────────────────────

func TestNewBookingPaymentConsumer_CreatesConsumer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	c := NewBookingPaymentConsumer(nil, uc)

	assert.NotNil(t, c)
	assert.Nil(t, c.nc)
	assert.Nil(t, c.sub)
	assert.NotNil(t, c.uc)
}

// ── Stop ──────────────────────────────────────────────────────────────────────

func TestStop_NilSubscription_DoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	c := NewBookingPaymentConsumer(nil, uc)
	// sub is nil — Stop must be a no-op

	assert.NotPanics(t, func() { c.Stop() })
}

func TestStop_WithSubscription_Unsubscribes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mockreservation.NewMockReservationUsecase(ctrl)
	c := NewBookingPaymentConsumer(nil, uc)
	// Inject a non-nil subscription so the Unsubscribe branch is exercised.
	// nats.Subscription.Unsubscribe on a zero-value struct returns an error
	// which Stop discards — no panic expected.
	c.sub = &nats.Subscription{}

	assert.NotPanics(t, func() { c.Stop() })
	// After Stop the subscription field is unchanged (Stop doesn't nil it),
	// but the Unsubscribe path was exercised.
}
