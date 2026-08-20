// Package dto holds the request payloads the HTTP API accepts, the rules each one
// must satisfy, and the normalization applied before those rules are checked.
//
// A payload validates itself. Nothing below the HTTP layer re-checks a field's shape,
// so a service reads a value that is already trimmed, cased, and known to be
// well-formed, and is left with the decisions only it can make: whether the row
// exists, whether the caller may change it, and what to do about the provider.
//
// Normalize runs before Validate, always. Trimming and casing decide what the value
// is; the rules then judge that value rather than whatever arrived on the wire.
package dto

import (
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"

	"github.com/momobasehq/momobase/internal/utils"
)

// Payload is a request body that normalizes itself before it is judged.
type Payload interface{ Normalize() }

var (
	once     sync.Once
	validate *validator.Validate
)

// validator returns the shared validator, with Momobase's own rules registered.
//
// The rules are the ones the engine already had: they wrap internal/utils rather than
// restating its definitions as struct tags, so the shape a payload must have and the
// shape the engine enforces cannot drift apart.
func instance() *validator.Validate {
	once.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
		// Report a field by the name the caller sent it under. Naming the Go field
		// would tell an integrator to fix something they never wrote.
		validate.RegisterTagNameFunc(func(field reflect.StructField) string {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				return field.Name
			}
			return name
		})
		_ = validate.RegisterValidation("identifier", func(field validator.FieldLevel) bool {
			return utils.ValidIdentifier(field.Field().String())
		})
		_ = validate.RegisterValidation("account", func(field validator.FieldLevel) bool {
			return utils.ValidAccount(field.Field().String())
		})
		_ = validate.RegisterValidation("country", func(field validator.FieldLevel) bool {
			_, err := utils.NormalizeOptionalCountry(field.Field().String())
			return err == nil
		})
	})
	return validate
}

// Check normalizes a payload and then validates it, in that order.
//
// The order is load-bearing on the payment path: the idempotency hash is taken over
// the normalized request, so two spellings of one currency are one request rather than
// two. Doing it here rather than at each call site is what keeps that true everywhere.
func Check(payload Payload) error {
	if payload == nil {
		return errors.New("request body is required")
	}
	payload.Normalize()
	return Validate(payload)
}

// Validate reports the first rule a payload fails, in a message naming the field.
func Validate(payload any) error {
	err := instance().Struct(payload)
	if err == nil {
		return nil
	}
	var invalid validator.ValidationErrors
	if !errors.As(err, &invalid) || len(invalid) == 0 {
		return err
	}
	return errors.New(describe(invalid[0]))
}

// describe renders one failed rule as a sentence a caller can act on. The generic
// "failed on the 'max' tag" the library produces names the rule rather than the
// requirement, which tells an integrator nothing they can fix.
func describe(failure validator.FieldError) string {
	field := jsonName(failure)
	switch failure.Tag() {
	case "required":
		return field + " is required"
	case "identifier":
		return field + " may contain only lowercase letters, digits, and _-. and must not exceed 64 characters"
	case "account":
		return field + " must not exceed 255 characters or contain control characters"
	case "country":
		return field + " must be an ISO 3166-1 alpha-2 country code"
	case "len":
		return field + " must be exactly " + failure.Param() + " characters"
	case "max":
		return field + " must not exceed " + failure.Param() + " characters"
	case "gt":
		return field + " must be greater than " + failure.Param()
	case "gte", "min":
		return field + " must be at least " + failure.Param()
	case "oneof":
		return field + " must be one of: " + strings.ReplaceAll(failure.Param(), " ", ", ")
	case "email":
		return field + " must be an email address"
	default:
		return field + " is invalid"
	}
}

// jsonName reports the field by the name the caller sent it under, not the Go one.
func jsonName(failure validator.FieldError) string {
	name := failure.Field()
	if name == "" {
		return "request"
	}
	return name
}
