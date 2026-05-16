package context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetAndGetContextData(t *testing.T) {
	ctx := context.Background()
	
	// Set context data
	data := ContextData{
		TransactionID: "A3P1251201113149000133910",
		Msisdn:        "081234567890",
		AppVersion:    "1.2.3",
		OSVersion:     "android|11.0",
		DeviceID:      "device-123",
	}
	ctx = SetContextData(ctx, data)
	
	// Get context data
	retrieved := GetContextData(ctx)
	assert.Equal(t, data, retrieved)
}

func TestGetContextDataEmpty(t *testing.T) {
	ctx := context.Background()
	
	// Test empty context returns empty struct
	retrieved := GetContextData(ctx)
	expected := ContextData{}
	assert.Equal(t, expected, retrieved)
}