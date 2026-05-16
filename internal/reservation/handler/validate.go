package handler

import (
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// validateUUID returns true if s is a valid UUID v4 string.
func validateUUID(s string) bool {
	return validate.Var(s, "required,uuid") == nil
}
