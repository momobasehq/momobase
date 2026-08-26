package mtn

type sandboxUserRequest struct {
	ProviderCallbackHost string `json:"providerCallbackHost"`
}

type sandboxAPIKeyResponse struct {
	APIKey string `json:"apiKey"`
}

type party struct {
	PartyIDType string `json:"partyIdType"`
	PartyID     string `json:"partyId"`
}

type paymentRequest struct {
	Amount       string `json:"amount"`
	Currency     string `json:"currency"`
	ExternalID   string `json:"externalId"`
	Payer        *party `json:"payer,omitempty"`
	Payee        *party `json:"payee,omitempty"`
	PayerMessage string `json:"payerMessage"`
	PayeeNote    string `json:"payeeNote"`
	TransferType string `json:"transferType,omitempty"`
}

type reason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type transactionStatus struct {
	Amount                 string  `json:"amount"`
	Currency               string  `json:"currency"`
	FinancialTransactionID string  `json:"financialTransactionId"`
	ExternalID             string  `json:"externalId"`
	Payer                  *party  `json:"payer,omitempty"`
	Payee                  *party  `json:"payee,omitempty"`
	PayerMessage           string  `json:"payerMessage"`
	PayeeNote              string  `json:"payeeNote"`
	Status                 string  `json:"status"`
	Reason                 *reason `json:"reason,omitempty"`
}

func (s transactionStatus) message() string {
	if s.Reason != nil {
		if s.Reason.Message != "" {
			return s.Reason.Message
		}
		if s.Reason.Code != "" {
			return s.Reason.Code
		}
	}
	return s.Status
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type balanceResponse struct {
	AvailableBalance string `json:"availableBalance"`
	Currency         string `json:"currency"`
}
