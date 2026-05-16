package apperror

const (
	InternalServerErrorMessage = "An internal server error occurred"
)

// AppError represents application-specific errors with error codes
type AppError struct {
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	return e.Message
}

// New creates a new AppError
func New(errorCode, message string) *AppError {
	return &AppError{
		ErrorCode: errorCode,
		Message:   message,
	}
}
