package platform

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
)

// apiResponse is the common envelope returned by JSON API endpoints.
type apiResponse struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *APIError `json:"error,omitempty"`
	Message string    `json:"message,omitempty"`
}

// APIError describes a machine-readable API failure.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PageData contains one page of items and its pagination metadata.
type PageData[T any] struct {
	Page  int `json:"page"`
	Total int `json:"total"`
	Items []T `json:"items"`
	Count int `json:"count"`
}

// JSON writes payload in a successful API response envelope.
func JSON(c fiber.Ctx, status int, payload any) error {
	return c.Status(status).JSON(apiResponse{Success: status < 400, Data: payload})
}

// RawJSON writes payload directly without an API response envelope.
func RawJSON(c fiber.Ctx, status int, payload any) error { return c.Status(status).JSON(payload) }

// Error writes a failed API response containing code and message.
func Error(c fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(apiResponse{Error: &APIError{Code: code, Message: message}, Message: message})
}

// Pagination reads and bounds the page and per_page query parameters.
func Pagination(c fiber.Ctx) (int, int) {
	page, size := positive(c.Query("page"), 1), positive(c.Query("per_page"), 20)
	if size > 100 {
		size = 100
	}
	return page, size
}

// PaginateSlice returns the requested bounded page from an in-memory slice.
func PaginateSlice[T any](items []T, page, size int) PageData[T] {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	total, start := len(items), (page-1)*size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return PageData[T]{Page: page, Total: total, Items: items[start:end], Count: end - start}
}
func positive(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
