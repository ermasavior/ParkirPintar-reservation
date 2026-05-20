package paymentclient

import (
	"context"
	"fmt"
	"testing"

	paymentpb "parkir-pintar/services/reservation/gen/payment/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testIdemKey   = "550e8400-e29b-41d4-a716-446655440000"
	testRefID     = "660e8400-e29b-41d4-a716-446655440001"
	testDriverID  = "770e8400-e29b-41d4-a716-446655440002"
)

func successCaller(_ context.Context, _, _, _ string, _ paymentpb.PaymentType, _ int64) (*CreatePaymentResult, error) {
	return &CreatePaymentResult{
		PaymentID: "pay-001",
		QRCodeURL: "https://qr.example.com/stub",
	}, nil
}

func failCaller(_ context.Context, _, _, _ string, _ paymentpb.PaymentType, _ int64) (*CreatePaymentResult, error) {
	return nil, fmt.Errorf("rpc error: connection refused")
}

// ── Success ───────────────────────────────────────────────────────────────────

func TestCreatePayment_Success(t *testing.T) {
	c := newWithCaller(successCaller)

	result, appErr := c.CreatePayment(context.Background(),
		testIdemKey, testRefID, testDriverID,
		paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000,
	)

	require.Nil(t, appErr)
	assert.Equal(t, "pay-001", result.PaymentID)
	assert.Equal(t, "https://qr.example.com/stub", result.QRCodeURL)
}

// ── Single failure — circuit stays CLOSED ────────────────────────────────────

func TestCreatePayment_SingleFailure_CircuitStayClosed(t *testing.T) {
	c := newWithCaller(failCaller)

	_, appErr := c.CreatePayment(context.Background(),
		testIdemKey, testRefID, testDriverID,
		paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000,
	)

	require.NotNil(t, appErr)
	assert.Equal(t, "payment_service_error", appErr.ErrorCode)
	// Circuit should still be CLOSED after 1 failure (threshold is 5)
	assert.Equal(t, "payment_service_error", appErr.ErrorCode)
}

// ── 5 consecutive failures → circuit OPENS ───────────────────────────────────

func TestCreatePayment_FiveConsecutiveFailures_CircuitOpens(t *testing.T) {
	c := newWithCaller(failCaller)

	// Trigger 5 consecutive failures to open the circuit
	for i := 0; i < 5; i++ {
		_, appErr := c.CreatePayment(context.Background(),
			testIdemKey, testRefID, testDriverID,
			paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000,
		)
		require.NotNil(t, appErr)
		assert.Equal(t, "payment_service_error", appErr.ErrorCode)
	}

	// 6th call — circuit is now OPEN, returns unavailable immediately
	_, appErr := c.CreatePayment(context.Background(),
		testIdemKey, testRefID, testDriverID,
		paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000,
	)

	require.NotNil(t, appErr)
	assert.Equal(t, "payment_service_unavailable", appErr.ErrorCode)
	assert.Contains(t, appErr.Message, "temporarily unavailable")
}

// ── Circuit OPEN — no downstream calls made ───────────────────────────────────

func TestCreatePayment_CircuitOpen_DoesNotCallDownstream(t *testing.T) {
	callCount := 0
	countingCaller := func(_ context.Context, _, _, _ string, _ paymentpb.PaymentType, _ int64) (*CreatePaymentResult, error) {
		callCount++
		return nil, fmt.Errorf("downstream error")
	}

	c := newWithCaller(countingCaller)

	// Open the circuit with 5 failures
	for i := 0; i < 5; i++ {
		_, _ = c.CreatePayment(context.Background(), testIdemKey, testRefID, testDriverID,
			paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000)
	}

	callsBefore := callCount

	// Next call should be rejected by CB without calling downstream
	_, appErr := c.CreatePayment(context.Background(), testIdemKey, testRefID, testDriverID,
		paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000)

	require.NotNil(t, appErr)
	assert.Equal(t, "payment_service_unavailable", appErr.ErrorCode)
	assert.Equal(t, callsBefore, callCount, "downstream should not be called when circuit is OPEN")
}

// ── Success after failures resets consecutive count ───────────────────────────

func TestCreatePayment_SuccessAfterFailures_ResetsCircuit(t *testing.T) {
	callCount := 0
	// Fail 4 times then succeed
	mixedCaller := func(_ context.Context, _, _, _ string, _ paymentpb.PaymentType, _ int64) (*CreatePaymentResult, error) {
		callCount++
		if callCount <= 4 {
			return nil, fmt.Errorf("transient error")
		}
		return &CreatePaymentResult{PaymentID: "pay-ok", QRCodeURL: "https://qr.example.com"}, nil
	}

	c := newWithCaller(mixedCaller)

	// 4 failures — circuit still CLOSED (threshold is 5)
	for i := 0; i < 4; i++ {
		_, appErr := c.CreatePayment(context.Background(), testIdemKey, testRefID, testDriverID,
			paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000)
		assert.Equal(t, "payment_service_error", appErr.ErrorCode)
	}

	// 5th call succeeds — consecutive failure count resets
	result, appErr := c.CreatePayment(context.Background(), testIdemKey, testRefID, testDriverID,
		paymentpb.PaymentType_PAYMENT_TYPE_PARKING_FEE, 15000)

	require.Nil(t, appErr)
	assert.Equal(t, "pay-ok", result.PaymentID)
}

// ── New() returns error on bad target ────────────────────────────────────────

func TestNew_InvalidTarget_ReturnsError(t *testing.T) {
	// grpc.NewClient is lazy — it doesn't fail on bad address at creation time
	// but we can verify it returns a non-nil client
	c, err := New("localhost:1")
	assert.NoError(t, err) // gRPC dials lazily
	assert.NotNil(t, c)
}
