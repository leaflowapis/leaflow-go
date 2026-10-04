package billingv1server

import (
	"encoding/json"
	"github.com/go-faster/jx"
	"testing"
)

func TestCancelSubscriptionRefundInput(t *testing.T) {
	for _, row := range []struct {
		name, amount string
		valid        bool
	}{
		{"zero", `"0"`, true}, {"exact decimal", `"0.0000000001"`, true},
		{"missing", "", false}, {"null", "null", false}, {"number", "0", false},
		{"negative", `"-1"`, false}, {"exponent", `"1e2"`, false},
		{"fraction overflow", `"0.00000000001"`, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			fields := map[string]json.RawMessage{"mode": json.RawMessage(`"immediate"`), "reason": json.RawMessage(`"User requested cancellation"`)}
			if row.amount != "" {
				fields["expected_refundable_amount"] = json.RawMessage(row.amount)
			}
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			var req CancelSubscriptionRequest
			err = req.Decode(jx.DecodeBytes(raw))
			if err == nil {
				err = req.Validate()
			}
			if (err == nil) != row.valid {
				t.Fatalf("accepted=%v want %v: %v", err == nil, row.valid, err)
			}
			if row.valid {
				var amount string
				if err := json.Unmarshal([]byte(row.amount), &amount); err != nil {
					t.Fatal(err)
				}
				if req.ExpectedRefundableAmount != amount {
					t.Fatal("refund amount changed")
				}
			}
		})
	}
}

func TestCancellationRefundAbsence(t *testing.T) {
	for _, row := range []struct {
		name, amount string
		set          bool
	}{
		{"cleanup pending", "", false}, {"completed without refund", `,"balance_amount":"0","credit_amount":"0","gateway_amount":"0"`, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			var item CancellationItem
			raw := `{"id":"bb3b9c54-99ea-4aef-b375-ec61df426d5c","subscription_id":"74825f39-3b5a-47be-ae44-0b8f55a39f0b","plan_id":"2289b2ca-cfa8-4ae9-9b11-c76b93a78593","plan_name":"plan","billing_type":"prepaid","status":"` + map[bool]string{false: "releasing", true: "completed"}[row.set] + `"` + row.amount + `}`
			if err := item.Decode(jx.DecodeBytes([]byte(raw))); err != nil {
				t.Fatal(err)
			}
			if err := item.Validate(); err != nil {
				t.Fatal(err)
			}
			for _, amount := range []OptMoney{item.BalanceAmount, item.CreditAmount, item.GatewayAmount} {
				v, set := amount.Get()
				if set != row.set || (set && v != Money("0")) {
					t.Fatal("absent refund confused with confirmed zero")
				}
			}
		})
	}
}
