package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/gofiber/fiber/v3"
)

// DecodeJSON reads one strict JSON object and rejects trailing content.
//
// It decodes the buffered body itself rather than going through Fiber's binder,
// because the binder accepts unknown fields: a request naming a field the payload
// does not have would be silently ignored instead of refused.
func DecodeJSON[T any](c fiber.Ctx) (*T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(c.Body()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid json: multiple values")
	}
	return &v, nil
}
