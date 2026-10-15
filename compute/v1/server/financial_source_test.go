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

func TestFinancialSourceAmountsCannotEnterCreateDisk(t *testing.T) {
	for _, row := range loadFinancialCases(t) {
		t.Run(row.Name, func(t *testing.T) {
			checkout, err := json.Marshal(row.Input)
			if err != nil {
				t.Fatal(err)
			}
			raw := []byte(`{"disk_type_id":"disktype_00000000000000000000000001","name":"disk","size_gb":20,"billing":{"mode":"postpaid"},"checkout":` + string(checkout) + `}`)
			var value CreateDiskRequestBody
			err = value.Decode(jx.DecodeBytes(raw))
			if err == nil {
				err = value.Validate()
			}
			if err == nil {
				t.Fatal("Compute accepted checkout fields that belong to Billing")
			}
		})
	}
}
