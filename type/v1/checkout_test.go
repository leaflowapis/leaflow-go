package typev1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-faster/jx"
)

// 核对原生共同类型真实解码；金额格式来自既有 CheckoutOptions pattern，不新增业务政策。
func TestNativeCheckoutAmountPresence(t *testing.T) {
	for _, row := range []struct {
		name, body, value string
		valid, present    bool
	}{
		{"absent", `{}`, "", true, false},
		{"zero", `{"expected_amount":"0"}`, "0", true, true},
		{"zero decimals", `{"expected_amount":"0.00"}`, "0.00", true, true},
		{"ten decimals", `{"expected_amount":"1.1234567890"}`, "1.1234567890", true, true},
		{"null", `{"expected_amount":null}`, "", false, false},
		{"JSON number", `{"expected_amount":0}`, "", false, false},
		{"empty", `{"expected_amount":""}`, "", false, false},
		{"exponent", `{"expected_amount":"1e3"}`, "", false, false},
		{"negative", `{"expected_amount":"-1"}`, "", false, false},
		{"negative zero", `{"expected_amount":"-0"}`, "", false, false},
		{"whitespace", `{"expected_amount":" 0 "}`, "", false, false},
		{"eleven decimals", `{"expected_amount":"1.12345678901"}`, "", false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			var value CheckoutOptions
			err := value.Decode(jx.DecodeBytes([]byte(row.body)))
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != row.valid {
				t.Fatalf("decode/validate=%v valid=%v", err, row.valid)
			}
			if !row.valid {
				return
			}
			got, present := value.ExpectedAmount.Get()
			if present != row.present || got != row.value {
				t.Fatalf("presence/value lost: %+v", value.ExpectedAmount)
			}
			wire, err := json.Marshal(&value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(wire), `"expected_amount"`) != row.present {
				t.Fatalf("wrong JSON presence: %s", wire)
			}
			var back CheckoutOptions
			if err := json.Unmarshal(wire, &back); err != nil {
				t.Fatal(err)
			}
			again, set := back.ExpectedAmount.Get()
			if set != present || again != got {
				t.Fatalf("round trip lost presence: %s", wire)
			}
		})
	}
}
