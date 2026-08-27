package public

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/momobasehq/momobase/internal/domain"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/identity"
	"github.com/momobasehq/momobase/internal/service/payment"
)

func authenticatedHandler(t *testing.T) (*Handler, *identity.AppAuthService, string) {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&domain.App{}, &domain.AppCredential{}, &domain.AppSession{}, &domain.Transaction{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	hash, err := platform.HashPassword("client-secret")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	app := domain.App{BaseModel: domain.BaseModel{ID: "app-1"}, Name: "App", Status: "active", Environment: "sandbox"}
	credential := domain.AppCredential{
		BaseModel:        domain.BaseModel{ID: "cred-1"},
		AppID:            app.ID,
		Name:             "Default",
		ClientID:         "client-1",
		ClientSecretHash: hash,
		Status:           "active",
		Scopes:           "collections:create transactions:read",
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("create app: %v", err)
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	manager, err := platform.NewTokenManager(strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	auth := identity.NewAppAuthService(repository.New(db), "client", "secret", time.Minute, time.Hour, manager)
	tokens, err := auth.IssueClientToken(context.Background(), "client-1", "client-secret")
	if err != nil {
		t.Fatalf("IssueClientToken() error = %v", err)
	}
	repos := repository.New(db)
	payments := payment.NewOrchestrator(repos, nil, nil, nil)
	return NewHandler(payments, nil, repos), auth, tokens.AccessToken
}

// response is one recorded reply. A fiber.Ctx cannot be built directly, so a handler is
// exercised by mounting it on a throwaway app and driving a request through it.
type response struct {
	Code int
	Body string
}

// serve mounts handler at pattern and runs req through it, optionally behind the app
// bearer middleware so the handler sees a resolved identity.
func serve(t *testing.T, pattern string, handler fiber.Handler, req *http.Request, guards ...fiber.Handler) response {
	t.Helper()
	app := fiber.New()
	chain := append(append([]fiber.Handler{}, guards...), handler)
	rest := make([]any, 0, len(chain)-1)
	for _, link := range chain[1:] {
		rest = append(rest, link)
	}
	app.All(pattern, chain[0], rest...)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	_ = res.Body.Close()
	return response{Code: res.StatusCode, Body: string(raw)}
}

func serveAuthenticated(
	t *testing.T,
	auth *identity.AppAuthService,
	token, pattern string,
	handler fiber.Handler,
	req *http.Request,
) response {
	t.Helper()
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	return serve(t, pattern, handler, req, authmw.WithAppBearer(auth))
}

func TestGetTransactionByIDAndReference(t *testing.T) {
	h, auth, token := authenticatedHandler(t)
	tx := domain.Transaction{
		BaseModel:      domain.BaseModel{ID: "txn-1"},
		AppID:          "app-1",
		ServiceType:    domain.ServiceCollection,
		PaymentMethod:  "momo",
		Amount:         1000,
		Currency:       "UGX",
		Country:        "UG",
		Reference:      "order-1",
		IdempotencyKey: "idem-1",
		Status:         domain.TxPending,
		ProviderFee:    80,
		PlatformFee:    120,
	}
	if err := h.repos.Transactions.Create(context.Background(), &tx); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/transactions/txn-1", nil)
	res := serveAuthenticated(t, auth, token, "/transactions/:id", h.GetTransaction, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body, "order-1") {
		t.Fatalf("GetTransaction() = %d %s", res.Code, res.Body)
	}
	if strings.Contains(res.Body, "provider_fee") || !strings.Contains(res.Body, `"platform_fee":120`) {
		t.Fatalf("GetTransaction() fee visibility = %s", res.Body)
	}

	const byReference = "/transactions/by-reference/:reference"
	req = httptest.NewRequest(http.MethodGet, "/transactions/by-reference/order-1", nil)
	res = serveAuthenticated(t, auth, token, byReference, h.GetTransactionByReference, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body, "txn-1") {
		t.Fatalf("GetTransactionByReference() = %d %s", res.Code, res.Body)
	}

	req = httptest.NewRequest(http.MethodGet, "/transactions/by-reference/missing", nil)
	res = serveAuthenticated(t, auth, token, byReference, h.GetTransactionByReference, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("GetTransactionByReference(missing) status = %d", res.Code)
	}
}

func TestCreateCollectionRequiresIdentityAndValidJSON(t *testing.T) {
	h, auth, token := authenticatedHandler(t)
	anonymous := httptest.NewRequest(http.MethodPost, "/collections", strings.NewReader(`{}`))
	if res := serve(t, "/collections", h.CreateCollection, anonymous); res.Code != http.StatusUnauthorized {
		t.Fatalf("CreateCollection(no identity) status = %d", res.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/collections", strings.NewReader(`{"unknown":true}`))
	res := serveAuthenticated(t, auth, token, "/collections", h.CreateCollection, req)
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body, "VALIDATION_ERROR") {
		t.Fatalf("CreateCollection(invalid JSON) = %d %s", res.Code, res.Body)
	}
}
