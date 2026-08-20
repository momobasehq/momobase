package middleware

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// RequestLogger records the method, path, response status, duration, request
// identifier, and client address of each request using the supplied structured logger.
//
// Fiber ships a logger middleware, but it writes a formatted line rather than slog
// attributes, and every other log line in Momobase is structured. The client address is
// whatever Fiber's proxy-trust configuration resolved, so a log line and a 429 from the
// limiter name the same address.
func RequestLogger(log *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		log.Info(
			"http_request",
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", responseStatus(c, err)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("ip", c.IP()),
			slog.String("request_id", requestid.FromContext(c)),
		)
		return err
	}
}

// responseStatus reports the status the caller will see. A handler that returned an
// error has not been through the error handler yet, so the recorded status is still
// whatever was set before it failed rather than what is about to be sent.
func responseStatus(c fiber.Ctx, err error) int {
	if err == nil {
		return c.Response().StatusCode()
	}
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return fiberErr.Code
	}
	return fiber.StatusInternalServerError
}
