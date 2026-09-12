package compute_test

import (
	"encoding/json"
	"testing"

	computev1server "github.com/leaflowapis/leaflow-go/compute/v1/server"
)

func TestPriceReferenceRequiresExactlyOneSelector(t *testing.T) {
	for _, test := range []struct {
		name, payload string
		valid         bool
	}{
		{"id", `{"id":"018f0310-650f-7425-927e-11d0af2fcb1f"}`, true},
		{"lookup", `{"lookup_key":"standard-metered"}`, true},
		{"empty", `{}`, false},
		{"ambiguous", `{"id":"018f0310-650f-7425-927e-11d0af2fcb1f","lookup_key":"standard-metered"}`, false},
		{"empty lookup", `{"lookup_key":""}`, false},
		{"legacy key", `{"price_key":"standard-metered"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value computev1server.CatalogReference
			err := json.Unmarshal([]byte(test.payload), &value)
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}

func TestOrderFundingRequiresDecimalStrings(t *testing.T) {
	for _, test := range []struct {
		name, payload string
		valid         bool
	}{
		{"split", `{"idempotency_key":"purchase-123","payment_plan":{"balance_amount":"40.0000000001","provider_amount":"60"}}`, true},
		{"numeric money", `{"idempotency_key":"purchase-123","payment_plan":{"balance_amount":40,"provider_amount":"60"}}`, false},
		{"partial split", `{"idempotency_key":"purchase-123","payment_plan":{"balance_amount":"40"}}`, false},
		{"missing idempotency", `{}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value computev1server.OrderOptions
			err := json.Unmarshal([]byte(test.payload), &value)
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}

func TestLaunchRejectsRetiredPaymentFields(t *testing.T) {
	const body = `{"name":"example","instance_type_id":"018f0310-650f-7425-927e-11d0af2fcb1f","order":{"idempotency_key":"purchase-123"},"price":{"lookup_key":"standard-metered"}`
	for _, field := range []string{`"term":"P1M"`, `"payment_method":"online"`, `"promotion_code":"old"`} {
		var value computev1server.LaunchInstanceRequestBody
		if err := json.Unmarshal([]byte(body+","+field+"}"), &value); err == nil {
			t.Fatalf("retired field accepted: %s", field)
		}
	}
}
