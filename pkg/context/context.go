package context

import "context"

// Context key for storing values in context.Context
type contextKey string

const (
	ContextDataKey contextKey = "contextdata"
)

// ContextData holds all context values as a struct
type ContextData struct {
	TransactionID string
	Msisdn        string
	AppVersion    string
	OSVersion     string
	DeviceID      string
}

// SetContextData stores the context data struct in context
func SetContextData(ctx context.Context, data ContextData) context.Context {
	return context.WithValue(ctx, ContextDataKey, data)
}

// GetContextData retrieves the context data struct from context
func GetContextData(ctx context.Context) ContextData {
	if val := ctx.Value(ContextDataKey); val != nil {
		if data, ok := val.(ContextData); ok {
			return data
		}
	}
	return ContextData{}
}