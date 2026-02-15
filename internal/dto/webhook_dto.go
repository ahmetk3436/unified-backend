package dto

type RevenueCatWebhook struct {
	APIVersion string          `json:"api_version"`
	Event      RevenueCatEvent `json:"event"`
}

type RevenueCatEvent struct {
	Type                     string   `json:"type"`
	ID                       string   `json:"id"`
	AppUserID                string   `json:"app_user_id"`
	ProductID                string   `json:"product_id"`
	EntitlementIDs           []string `json:"entitlement_ids"`
	PeriodType               string   `json:"period_type"`
	PurchasedAtMs            int64    `json:"purchased_at_ms"`
	ExpirationAtMs           int64    `json:"expiration_at_ms"`
	Environment              string   `json:"environment"`
	Store                    string   `json:"store"`
	OriginalAppUserID        string   `json:"original_app_user_id"`
	TransactionID            string   `json:"transaction_id"`
	OriginalTransactionID    string   `json:"original_transaction_id"`
	IsTrialConversion        bool     `json:"is_trial_conversion"`
	CountryCode              string   `json:"country_code"`
	Currency                 string   `json:"currency"`
	Price                    float64  `json:"price"`
	PriceInPurchasedCurrency float64  `json:"price_in_purchased_currency"`
}
