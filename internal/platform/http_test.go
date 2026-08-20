package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

func TestDecodeJSONStrictObject(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	body := &trackingBody{Reader: strings.NewReader(`{"name":"Momobase"}`)}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = body
	got, err := DecodeJSON[payload](req)
	if err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}
	if got.Name != "Momobase" || !body.closed {
		t.Fatalf("DecodeJSON() = %+v, closed = %v", got, body.closed)
	}

	for _, raw := range []string{
		`{"name":"Momobase","unknown":true}`,
		`{"name":"one"} {"name":"two"}`,
		`{"name":`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(raw))
		if _, err := DecodeJSON[payload](req); err == nil {
			t.Fatalf("DecodeJSON() accepted %q", raw)
		}
	}
}

func TestJSONResponseHelpers(t *testing.T) {
	recorder := httptest.NewRecorder()
	JSON(recorder, http.StatusCreated, map[string]string{"id": "one"})
	if recorder.Code != http.StatusCreated || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("JSON() status/header = %d, %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	var response apiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	if !response.Success || response.Data == nil {
		t.Fatalf("JSON() response = %+v", response)
	}

	recorder = httptest.NewRecorder()
	Error(recorder, http.StatusBadRequest, "INVALID", "bad request")
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Success || response.Error == nil || response.Error.Code != "INVALID" {
		t.Fatalf("Error() response = %+v", response)
	}

	recorder = httptest.NewRecorder()
	RawJSON(recorder, http.StatusOK, map[string]bool{"ok": true})
	if !strings.Contains(recorder.Body.String(), `"ok":true`) {
		t.Fatalf("RawJSON() body = %s", recorder.Body.String())
	}
}

func TestPaginationAndPaginateSlice(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=2&per_page=500", nil)
	page, size := Pagination(req)
	if page != 2 || size != 100 {
		t.Fatalf("Pagination() = %d, %d", page, size)
	}
	req = httptest.NewRequest(http.MethodGet, "/?page=bad&per_page=-1", nil)
	page, size = Pagination(req)
	if page != 1 || size != 20 {
		t.Fatalf("Pagination() fallback = %d, %d", page, size)
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
