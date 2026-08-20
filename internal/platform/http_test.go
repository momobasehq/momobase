package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// run drives one handler through a throwaway Fiber app. A fiber.Ctx cannot be
// constructed directly, so every helper that takes one is exercised through a request.
func run(t *testing.T, req *http.Request, handler fiber.Handler) *http.Response {
	t.Helper()
	app := fiber.New()
	app.All("/*", handler)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return res
}

func body(t *testing.T, res *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = res.Body.Close()
	return string(raw)
}

func jsonRequest(raw string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(raw))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	return req
}

func TestDecodeJSONStrictObject(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	res := run(t, jsonRequest(`{"name":"Momobase"}`), func(c fiber.Ctx) error {
		got, err := DecodeJSON[payload](c)
		if err != nil {
			return c.Status(http.StatusBadRequest).SendString(err.Error())
		}
		return c.SendString(got.Name)
	})
	if got := body(t, res); got != "Momobase" {
		t.Fatalf("DecodeJSON() = %q, want the decoded name", got)
	}

	for _, raw := range []string{
		`{"name":"Momobase","unknown":true}`,
		`{"name":"one"} {"name":"two"}`,
		`{"name":`,
	} {
		res := run(t, jsonRequest(raw), func(c fiber.Ctx) error {
			if _, err := DecodeJSON[payload](c); err != nil {
				return c.SendStatus(http.StatusBadRequest)
			}
			return c.SendStatus(http.StatusOK)
		})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("DecodeJSON() accepted %q", raw)
		}
	}
}

func TestJSONResponseHelpers(t *testing.T) {
	res := run(t, httptest.NewRequest(http.MethodGet, "/", nil), func(c fiber.Ctx) error {
		return JSON(c, http.StatusCreated, map[string]string{"id": "one"})
	})
	if res.StatusCode != http.StatusCreated ||
		!strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("JSON() status/header = %d, %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var response apiResponse
	if err := json.Unmarshal([]byte(body(t, res)), &response); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	if !response.Success || response.Data == nil {
		t.Fatalf("JSON() response = %+v", response)
	}

	res = run(t, httptest.NewRequest(http.MethodGet, "/", nil), func(c fiber.Ctx) error {
		return Error(c, http.StatusBadRequest, "INVALID", "bad request")
	})
	if err := json.Unmarshal([]byte(body(t, res)), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Success || response.Error == nil || response.Error.Code != "INVALID" {
		t.Fatalf("Error() response = %+v", response)
	}

	res = run(t, httptest.NewRequest(http.MethodGet, "/", nil), func(c fiber.Ctx) error {
		return RawJSON(c, http.StatusOK, map[string]bool{"ok": true})
	})
	if got := body(t, res); !strings.Contains(got, `"ok":true`) {
		t.Fatalf("RawJSON() body = %s", got)
	}
}

func TestPaginationAndPaginateSlice(t *testing.T) {
	pagination := func(target string) string {
		res := run(t, httptest.NewRequest(http.MethodGet, target, nil), func(c fiber.Ctx) error {
			page, size := Pagination(c)
			return c.SendString(strings.Join([]string{itoa(page), itoa(size)}, ","))
		})
		return body(t, res)
	}
	if got := pagination("/?page=2&per_page=500"); got != "2,100" {
		t.Fatalf("Pagination() = %s, want 2,100", got)
	}
	if got := pagination("/?page=bad&per_page=-1"); got != "1,20" {
		t.Fatalf("Pagination() fallback = %s, want 1,20", got)
	}

	data := PaginateSlice([]int{1, 2, 3, 4, 5}, 2, 2)
	if data.Total != 5 || data.Count != 2 || data.Items[0] != 3 || data.Items[1] != 4 {
		t.Fatalf("PaginateSlice() = %+v", data)
	}
	empty := PaginateSlice([]int{1}, 99, 0)
	if empty.Page != 99 || empty.Count != 0 || len(empty.Items) != 0 {
		t.Fatalf("PaginateSlice() out of range = %+v", empty)
	}
}

func itoa(value int) string { return strconv.Itoa(value) }
