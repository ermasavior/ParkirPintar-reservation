package handler

import (
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

func validateUUID(s string) bool {
	return validate.Var(s, "required,uuid") == nil
}
