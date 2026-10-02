package computev1server

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/go-faster/jx"
)

type financialCase struct {
	Name     string                     `json:"name"`
	Input    map[string]json.RawMessage `json:"input"`
	Expected bool                       `json:"expected"`
}

func loadFinancialCases(t *testing.T) []financialCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/financial_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []financialCase
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// 从批准源的共同 64-case corpus 复用金额案例，直接核对本 SDK 实际 CheckoutOptions。
func TestFinancialSourceCheckoutCases(t *testing.T) {
	for _, row := range loadFinancialCases(t) {
		t.Run(row.Name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]json.RawMessage{"expected_amount": row.Input["expected_amount"]})
			if err != nil {
				t.Fatal(err)
			}
			var value CheckoutOptions
			err = value.Decode(jx.DecodeBytes(raw))
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != row.Expected {
				t.Fatalf("accepted=%v expected=%v error=%v", err == nil, row.Expected, err)
			}
		})
	}
}
