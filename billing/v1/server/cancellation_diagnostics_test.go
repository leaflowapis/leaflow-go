package billingv1server

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/go-faster/jx"
)

func TestTenantCancellationDiagnosticWire(t *testing.T) {
	if _, exists := reflect.TypeOf(CancellationItem{}).FieldByName("FailureReason"); exists {
		t.Fatal("tenant cancellation item exposes operator reason")
	}
	for _, row := range []struct {
		name, code string
		set        bool
	}{
		{name: "absent"}, {name: "explicit empty", set: true},
		{name: "stopped", code: "COMPUTE_RELEASE_FAILED", set: true},
		{name: "unknown outcome", code: "COMPUTE_RELEASE_RESULT_UNKNOWN", set: true},
	} {
		t.Run(row.name, func(t *testing.T) {
			fields := map[string]any{
				"id":              "bb3b9c54-99ea-4aef-b375-ec61df426d5c",
				"subscription_id": "74825f39-3b5a-47be-ae44-0b8f55a39f0b",
				"plan_id":         "2289b2ca-cfa8-4ae9-9b11-c76b93a78593",
				"plan_name":       "Plan", "billing_type": "prepaid", "status": "releasing",
				"failure_reason": "Private operator diagnostic",
			}
			if row.set {
				fields["failure_code"] = row.code
			}
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			var item CancellationItem
			if err = item.Decode(jx.DecodeBytes(raw)); err != nil {
				t.Fatal(err)
			}
			if err = item.Validate(); err != nil {
				t.Fatal(err)
			}
			code, set := item.FailureCode.Get()
			if set != row.set || (set && code != row.code) {
				t.Fatal("code presence or value changed")
			}
			if item.Status != CancellationItemStatusReleasing || item.EffectiveAt.IsSet() || item.BalanceAmount.IsSet() || item.CreditAmount.IsSet() || item.GatewayAmount.IsSet() {
				t.Fatal("diagnostic inferred completion or refund")
			}
			encoded, err := item.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			var back map[string]any
			if err = json.Unmarshal(encoded, &back); err != nil {
				t.Fatal(err)
			}
			value, set := back["failure_code"]
			if set != row.set || (set && value != row.code) {
				t.Fatal("code changed during encode")
			}
			if _, leaked := back["failure_reason"]; leaked {
				t.Fatal("operator diagnostic leaked into tenant output")
			}
		})
	}
}
