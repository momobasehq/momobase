package platform

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type APIResponse struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *APIError `json:"error,omitempty"`
	Message string    `json:"message,omitempty"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type PageData[T any] struct {
	Page  int `json:"page"`
	Total int `json:"total"`
	Items []T `json:"items"`
	Count int `json:"count"`
}

func JSON(w http.ResponseWriter, status int, payload any) {
	writeJSON(w, status, APIResponse{Success: status < 400, Data: payload})
}
func RawJSON(w http.ResponseWriter, status int, payload any) { writeJSON(w, status, payload) }
func Error(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, APIResponse{Error: &APIError{Code: code, Message: message}, Message: message})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func Pagination(r *http.Request) (int, int) {
	page, size := positive(r.URL.Query().Get("page"), 1), positive(r.URL.Query().Get("per_page"), 20)
	if size > 100 {
		size = 100
	}
	return page, size
}
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
