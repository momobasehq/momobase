package providers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Capability struct {
	ServiceType   string `json:"service_type"`
	PaymentMethod string `json:"payment_method"`
}
type ProviderConfig map[string]any
type PaymentRequest struct {
	TransactionID, Currency, Country, Reference, Phone, Network, Description string
	Amount                                                                   int64
}
type ProviderPaymentResponse struct {
	ProviderReference string         `json:"provider_reference"`
	Status            string         `json:"status"`
	Message           string         `json:"message"`
	Raw               map[string]any `json:"raw,omitempty"`
}
type ProviderTransactionStatus struct {
	ProviderReference string `json:"provider_reference"`
	Status            string `json:"status"`
	Message           string `json:"message"`
}
type ProviderBalance struct {
	Currency  string `json:"currency"`
	Available int64  `json:"available"`
	Ledger    int64  `json:"ledger"`
}
type ProviderWebhookEvent struct {
	ProviderReference string         `json:"provider_reference"`
	Status            string         `json:"status"`
	EventType         string         `json:"event_type"`
	ExternalReference string         `json:"external_reference,omitempty"`
	Amount            *int64         `json:"amount,omitempty"`
	Currency          string         `json:"currency,omitempty"`
	Country           string         `json:"country,omitempty"`
	Phone             string         `json:"phone,omitempty"`
	Raw               map[string]any `json:"raw,omitempty"`
}
type PaymentProvider interface {
	Capabilities() []Capability
	Init(context.Context, ProviderConfig) error
	HealthCheck(context.Context) error
	Collect(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	Disburse(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	QueryTransaction(context.Context, string, string) (*ProviderTransactionStatus, error)
	QueryBalance(context.Context, string) (*ProviderBalance, error)
	VerifyWebhook(context.Context, []byte, map[string]string) (*ProviderWebhookEvent, error)
}

type TokenCache struct {
	mu      sync.Mutex
	value   string
	expires time.Time
}

func (c *TokenCache) Get(load func() (string, time.Duration, error)) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value != "" && time.Until(c.expires) > 30*time.Second {
		return c.value, nil
	}
	value, ttl, err := load()
	if err != nil {
		return "", err
	}
	if ttl < time.Minute {
		ttl = time.Minute
	}
	c.value, c.expires = value, time.Now().Add(ttl)
	return value, nil
}
func PaymentStatus(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TS", "SUCCESS", "SUCCESSFUL", "COMPLETED", "200":
		return "succeeded"
	case "TF", "FAILED", "FAILURE", "DECLINED":
		return "failed"
	case "TIP", "PENDING", "IN_PROGRESS", "PROCESSING", "":
		return "processing"
	default:
		return "unknown"
	}
}
func OptionalAmount(raw, currency string) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := ParseAmountToMinor(raw, currency)
	return &value, err
}
func Slash(value string) string {
	if strings.HasPrefix(value, "/") {
		return value
	}
	return "/" + value
}

var ErrCircuitOpen = errors.New("provider circuit breaker is open")

type Factory func(*slog.Logger) PaymentProvider
type Registry map[string]Factory

func NewRegistry() Registry                        { return Registry{} }
func (r Registry) Register(code string, f Factory) { r[code] = f }
func (r Registry) Create(code string, log *slog.Logger) (PaymentProvider, error) {
	if f := r[code]; f != nil {
		return f(log), nil
	}
	return nil, fmt.Errorf("provider factory not registered: %s", code)
}
func Supports(caps []Capability, service, method string) bool {
	for _, c := range caps {
		if c.ServiceType == service && c.PaymentMethod == method {
			return true
		}
	}
	return false
}
func FirstError(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}
func First(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
func text(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
func String(c ProviderConfig, key string) string { return text(c[key]) }
func Bool(c ProviderConfig, key string) bool {
	value := strings.ToLower(text(c[key]))
	return value == "true" || value == "1"
}
func Int(c ProviderConfig, key string) int { value, _ := strconv.Atoi(text(c[key])); return value }
func Path(values map[string]any, path string) string {
	var value any = values
	for _, key := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value = object[key]
	}
	return text(value)
}
func Redact(value string) string {
	lower := strings.ToLower(value)
	for _, secret := range []string{"token", "secret", "api_key", "subscription_key", "password", "authorization", "bearer"} {
		if strings.Contains(lower, secret) {
			return "[redacted provider error]"
		}
	}
	if len(value) > 500 {
		return value[:500]
	}
	return value
}
func DoJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("provider api %s: %s", resp.Status, Redact(strings.TrimSpace(string(raw))))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
func RandomRef(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(raw[:])
}
func UUID() string {
	raw := RandomRef("")
	if len(raw) != 32 {
		return raw
	}
	return raw[:8] + "-" + raw[8:12] + "-4" + raw[13:16] + "-a" + raw[17:20] + "-" + raw[20:]
}
func exponent(currency string) int {
	if strings.Contains(" BIF CLP DJF GNF ISK JPY KMF KRW PYG RWF UGX VND VUV XAF XOF XPF ", " "+strings.ToUpper(strings.TrimSpace(currency))+" ") {
		return 0
	}
	return 2
}
func FormatAmountMinor(amount int64, currency string) string {
	if exponent(currency) == 0 {
		return strconv.FormatInt(amount, 10)
	}
	fraction := amount % 100
	if fraction < 0 {
		fraction = -fraction
	}
	return fmt.Sprintf("%d.%02d", amount/100, fraction)
}
func ParseAmountToMinor(raw, currency string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	if exponent(currency) == 0 {
		return strconv.ParseInt(value, 10, 64)
	}
	sign := int64(1)
	if strings.HasPrefix(value, "-") {
		sign, value = -1, value[1:]
	} else {
		value = strings.TrimPrefix(value, "+")
	}
	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 2 && len(parts[1]) > 2 {
		return 0, fmt.Errorf("amount %q has too many decimals for %s", raw, strings.ToUpper(currency))
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	whole, err := strconv.ParseInt(First(parts[0], "0"), 10, 64)
	if err != nil {
		return 0, err
	}
	minor, err := strconv.ParseInt(First(fraction+strings.Repeat("0", 2-len(fraction)), "0"), 10, 64)
	return sign * (whole*100 + minor), err
}
