package billingprojectv1server

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

// 同一批准源 corpus 核对实际 QuoteLine quantity 的 28+10/正值/尾部字符边界。
func TestFinancialSourceQuantityCases(t *testing.T) {
	for _, row := range loadFinancialCases(t) {
		t.Run(row.Name, func(t *testing.T) {
			raw, err := json.Marshal(row.Input)
			if err != nil {
				t.Fatal(err)
			}
			var value QuoteLine
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
