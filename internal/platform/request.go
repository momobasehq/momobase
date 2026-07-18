package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// DecodeJSON reads one strict JSON object and rejects trailing content.
func DecodeJSON[T any](r *http.Request) (*T, error) {
	defer func() { _ = r.Body.Close() }()
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid json: multiple values")
	}
	return &v, nil
}
