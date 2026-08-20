package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/momobasehq/momobase/internal/platform"
)

// maxInboundRequestID bounds an adopted identifier. It reaches the logs, and an
// unbounded value from a caller would be a cheap way to bloat them.
const maxInboundRequestID = 64

// BoundRequestID drops an oversized inbound X-Request-Id so the requestid middleware
// behind it generates one instead.
//
// Fiber's requestid already refuses an identifier containing anything but visible
// ASCII, which is the injection half of the problem; it does not bound the length,
// which is the log-bloat half. This supplies only the missing half.
func BoundRequestID(c fiber.Ctx) error {
	if len(c.Get(fiber.HeaderXRequestID)) > maxInboundRequestID {
		c.Request().Header.Del(fiber.HeaderXRequestID)
	}
	return c.Next()
}

// JSONOnly rejects requests whose Content-Type is not application/json.
func JSONOnly(c fiber.Ctx) error {
	if !strings.HasPrefix(strings.ToLower(c.Get(fiber.HeaderContentType)), fiber.MIMEApplicationJSON) {
		return platform.Error(
			c,
			fiber.StatusUnsupportedMediaType,
			"UNSUPPORTED_MEDIA_TYPE",
			"Content-Type must be application/json",
		)
	}
	return c.Next()
}

// NoCache adds a Cache-Control no-store header to responses.
func NoCache(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Next()
}

// RequestContext gives handlers a context that can actually be cancelled.
//
// fiber.Ctx satisfies context.Context, but it is pooled and reused, so its Done channel
// is always nil and it can never be cancelled — Fiber's own documentation says to pass
// Context() to anything cancellation-aware. Everything below the HTTP layer takes a
// context.Context and expects cancellation to mean something: a caller that goes away
// should stop the database work its request started, and the provider executor
// deliberately distinguishes caller cancellation from a provider failure. The fasthttp
// request context carries that signal, so it becomes the request's context here, once,
// rather than every handler reaching past Fiber for it.
func RequestContext(c fiber.Ctx) error {
	c.SetContext(c.RequestCtx())
	return c.Next()
}
