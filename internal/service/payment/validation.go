package payment

import (
	"encoding/json"

	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/platform"
)

// paymentRequestHash returns the canonical SHA-256 request hash used for idempotency checks.
//
// It is taken over the normalized request and before the selected provider's
// RequestValidator may rewrite the account or scheme. That ordering is what decides
// what counts as a replay: two spellings this side of normalization are one request,
// and a provider's later rewrite cannot change the identity of a request already made.
func paymentRequestHash(service string, req *dto.CreatePayment) string {
	data, _ := json.Marshal(struct {
		Service string
		Request *dto.CreatePayment
	}{service, req})
	return platform.SHA256Hex(string(data))
}
