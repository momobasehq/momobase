package common

import (
	"github.com/gofiber/fiber/v3"

	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/service/identity"
)

// Token wraps a form-encoded grant handler with shared validation and JSON response handling.
func Token(grant string, issue func(fiber.Ctx) (*identity.TokenResponse, error)) fiber.Handler {
	return func(c fiber.Ctx) error {
		actual := c.FormValue("grant_type")
		if actual != grant && (grant != "password" || actual != "") {
			return platform.Error(c, fiber.StatusBadRequest, "UNSUPPORTED_GRANT", "grant_type must be "+grant)
		}
		out, err := issue(c)
		if err != nil {
			return platform.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", err.Error())
		}
		return platform.RawJSON(c, fiber.StatusOK, out)
	}
}
