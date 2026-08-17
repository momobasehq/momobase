package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/momobasehq/momobase/internal/domain"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
)

func authenticatedHandler(t *testing.T) (*Handler, *services.AppAuthService, string) {
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
	auth := services.NewAppAuthService(db, "client", "secret", time.Minute, time.Hour, manager)
	tokens, err := auth.IssueClientToken(context.Background(), "client-1", "client-secret")
	if err != nil {
		t.Fatalf("IssueClientToken() error = %v", err)
	}
	return NewHandler(nil, nil, db), auth, tokens.AccessToken
}

func serveAuthenticated(auth *services.AppAuthService, token string, handler http.HandlerFunc, recorder *httptest.ResponseRecorder, req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+token)
	authmw.WithAppBearer(auth)(handler).ServeHTTP(recorder, req)
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
	}
	if err := h.db.Create(&tx).Error; err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/transactions/txn-1", nil)
	req.SetPathValue("id", "txn-1")
	recorder := httptest.NewRecorder()
	serveAuthenticated(auth, token, h.GetTransaction, recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "order-1") {
		t.Fatalf("GetTransaction() = %d %s", recorder.Code, recorder.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/transactions/by-reference/order-1", nil)
	req.SetPathValue("reference", "order-1")
	recorder = httptest.NewRecorder()
	serveAuthenticated(auth, token, h.GetTransactionByReference, recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "txn-1") {
		t.Fatalf("GetTransactionByReference() = %d %s", recorder.Code, recorder.Body.String())
	}

	req.SetPathValue("reference", "missing")
	recorder = httptest.NewRecorder()
	serveAuthenticated(auth, token, h.GetTransactionByReference, recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GetTransactionByReference(missing) status = %d", recorder.Code)
	}
}

func TestCreateCollectionRequiresIdentityAndValidJSON(t *testing.T) {
	h, auth, token := authenticatedHandler(t)
	recorder := httptest.NewRecorder()
	h.CreateCollection(recorder, httptest.NewRequest(http.MethodPost, "/collections", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("CreateCollection(no identity) status = %d", recorder.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/collections", strings.NewReader(`{"unknown":true}`))
	recorder = httptest.NewRecorder()
	serveAuthenticated(auth, token, h.CreateCollection, recorder, req)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("CreateCollection(invalid JSON) = %d %s", recorder.Code, recorder.Body.String())
	}
}
