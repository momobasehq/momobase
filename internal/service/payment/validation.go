package payment

import (
	"encoding/json"

	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/platform"
)

// paymentRequestHash returns the canonical SHA-256 idempotency hash. It is taken over the
// normalized request and before RequestValidator, which is what decides what is a replay.
func paymentRequestHash(service string, req *dto.CreatePayment) string {
	data, _ := json.Marshal(struct {
		Service string
		Request *dto.CreatePayment
	}{service, req})
	return platform.SHA256Hex(string(data))
}
