package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
)

// seedTransactions writes three transactions a minute apart, oldest first, so tests can
// assert filters, search, sorting and the created_at range against known rows.
func seedTransactions(t *testing.T, h *Handler) time.Time {
	t.Helper()
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := []domain.Transaction{
		{
			AppID:           "app-1",
			ServiceType:     "collection",
			PaymentMethod:   "momo",
			Amount:          500,
			Status:          "succeeded",
			Reference:       "ref-alpha",
			CustomerAccount: "256700000001",
		},
		{
			AppID:           "app-1",
			ServiceType:     "disbursement",
			PaymentMethod:   "momo",
			Amount:          900,
			Status:          "failed",
			Reference:       "ref-beta_1",
			CustomerAccount: "256700000002",
		},
		{
			AppID:           "app-2",
			ServiceType:     "collection",
			PaymentMethod:   "card",
			Amount:          100,
			Status:          "pending",
			Reference:       "ref-gamma",
			CustomerAccount: "4242",
		},
	}
	for index := range rows {
		row := rows[index]
		row.ID = "tx-" + string(rune('a'+index))
		row.IdempotencyKey = row.ID
		row.Currency, row.Country = "UGX", "UG"
		row.CreatedAt = start.Add(time.Duration(index) * time.Minute)
		if err := h.repos.Transactions.Create(context.Background(), &row); err != nil {
			t.Fatalf("create transaction: %v", err)
		}
	}
	return start
}

func listTransactions(t *testing.T, h *Handler, query string) response {
	t.Helper()
	return serve(t, "/transactions", h.ListTransactions, httptest.NewRequest(http.MethodGet, "/transactions?"+query, nil))
}

func TestListTransactionsFiltersSearchesAndSorts(t *testing.T) {
	h := testHandler(t)
	start := seedTransactions(t, h)

	cases := []struct {
		name, query string
		total       string
		order       []string
	}{
		{name: "default newest first", query: "", total: `"total":3`, order: []string{"tx-c", "tx-b", "tx-a"}},
		{name: "exact filter", query: "status=failed", total: `"total":1`, order: []string{"tx-b"}},
		{name: "comma filter matches any", query: "status=failed,pending", total: `"total":2`, order: []string{"tx-c", "tx-b"}},
		{name: "filters combine", query: "app_id=app-1&service_type=collection", total: `"total":1`, order: []string{"tx-a"}},
		{name: "search is case-insensitive", query: "q=REF-GAM", total: `"total":1`, order: []string{"tx-c"}},
		{name: "search covers customer account", query: "q=4242", total: `"total":1`, order: []string{"tx-c"}},
		// Unescaped, "_" would match the "l" in "alpha"; escaped, it matches only itself.
		{name: "search escapes wildcards", query: "q=a_p", total: `"total":0`},
		{name: "sort ascending", query: "sort=amount", total: `"total":3`, order: []string{"tx-c", "tx-a", "tx-b"}},
		{name: "sort descending", query: "sort=amount&order=desc", total: `"total":3`, order: []string{"tx-b", "tx-a", "tx-c"}},
		{
			name:  "created_at range",
			query: "from=" + start.Add(time.Minute).Format(time.RFC3339) + "&to=" + start.Add(2*time.Minute).Format(time.RFC3339),
			total: `"total":1`, order: []string{"tx-b"},
		},
		// The total is the filtered total, not the table's, so paging stays honest.
		{name: "total counts the filter, not the page", query: "app_id=app-1&per_page=1", total: `"total":2`, order: []string{"tx-b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := listTransactions(t, h, tc.query)
			if res.Code != http.StatusOK || !strings.Contains(res.Body, tc.total) {
				t.Fatalf("response = %d %s, want %s", res.Code, res.Body, tc.total)
			}
			last := -1
			for _, id := range tc.order {
				at := strings.Index(res.Body, `"id":"`+id+`"`)
				if at <= last {
					t.Fatalf("%s out of order or missing in %s", id, res.Body)
				}
				last = at
			}
		})
	}
}

func TestListRejectsUndeclaredQueries(t *testing.T) {
	h := testHandler(t)
	for _, query := range []string{
		"password_hash=x",   // not a declared filter
		"sort=request_hash", // not a declared sort
		"order=sideways",    // not a direction
		"from=yesterday",    // not RFC 3339
		"status=",           // a filter with no value
	} {
		if res := listTransactions(t, h, query); res.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d %s, want 400", query, res.Code, res.Body)
		}
	}
	res := serve(t, "/providers", h.ListProviders, httptest.NewRequest(http.MethodGet, "/providers?active=maybe", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("bool filter: status = %d %s, want 400", res.Code, res.Body)
	}
}

func TestListCredentialsCannotWidenTheAppScope(t *testing.T) {
	h := testHandler(t)
	for _, credential := range []domain.AppCredential{
		{BaseModel: domain.BaseModel{ID: "cred-1"}, AppID: "app-1", Name: "one", ClientID: "client-1", Status: "active"},
		{BaseModel: domain.BaseModel{ID: "cred-2"}, AppID: "app-2", Name: "two", ClientID: "client-2", Status: "active"},
	} {
		if err := h.repos.AppCredentials.Create(context.Background(), &credential); err != nil {
			t.Fatalf("create credential: %v", err)
		}
	}
	// app_id is not a credential filter, so it cannot reach another app's rows.
	res := serve(t, "/apps/:id/credentials", h.ListCredentials,
		httptest.NewRequest(http.MethodGet, "/apps/app-1/credentials?app_id=app-2", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want 400", res.Code, res.Body)
	}
	res = serve(t, "/apps/:id/credentials", h.ListCredentials,
		httptest.NewRequest(http.MethodGet, "/apps/app-1/credentials?q=client", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body, "cred-1") || strings.Contains(res.Body, "cred-2") {
		t.Fatalf("response = %d %s", res.Code, res.Body)
	}
}

func TestGetTransaction(t *testing.T) {
	h := testHandler(t)
	seedTransactions(t, h)
	res := serve(t, "/transactions/:id", h.GetTransaction, httptest.NewRequest(http.MethodGet, "/transactions/tx-b", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body, "ref-beta_1") {
		t.Fatalf("response = %d %s", res.Code, res.Body)
	}
	res = serve(t, "/transactions/:id", h.GetTransaction, httptest.NewRequest(http.MethodGet, "/transactions/missing", nil))
	if res.Code != http.StatusNotFound {
		t.Fatalf("missing: status = %d", res.Code)
	}
}
