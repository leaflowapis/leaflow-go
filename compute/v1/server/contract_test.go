package computev1server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/go-faster/jx"
)

// 通过真实 ogen 解码与校验检查批准的 oneOf，不将服务端所有权判断伪装成 SDK 校验。
func TestNativeBootDiskDecoder(t *testing.T) {
	image := `{"type":"image","image_id":"11111111-1111-1111-1111-111111111111","disk_type_id":"22222222-2222-2222-2222-222222222222","size_gb":20,"billing":{"mode":"prepaid","period":{"unit":"month","count":1},"termination_policy":"immediate"}}`
	disk := `{"type":"disk","disk_id":"33333333-3333-3333-3333-333333333333","login_username":"ubuntu"}`
	for _, row := range []struct {
		name, body string
		valid      bool
	}{
		{"image", image, true}, {"disk", disk, true},
		{"unsupported type", strings.Replace(disk, `"disk"`, `"snapshot"`, 1), false},
		{"image plus disk source", image[:len(image)-1] + `,"disk_id":"33333333-3333-3333-3333-333333333333"}`, false},
		{"disk plus image source", disk[:len(disk)-1] + `,"image_id":"11111111-1111-1111-1111-111111111111"}`, false},
		{"missing discriminator", `{"disk_id":"33333333-3333-3333-3333-333333333333","login_username":"ubuntu"}`, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			var value BootDisk
			err := value.Decode(jx.DecodeBytes([]byte(row.body)))
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != row.valid {
				t.Fatalf("valid=%v, decode/validate=%v", row.valid, err)
			}
		})
	}
}

func TestNativeBillingPeriodInt32(t *testing.T) {
	for _, row := range []struct {
		count string
		valid bool
	}{
		{"1", true}, {"2147483647", true}, {"2147483648", false}, {"0", false}, {"-1", false},
	} {
		t.Run(row.count, func(t *testing.T) {
			var value BillingPeriod
			body := fmt.Sprintf(`{"unit":"month","count":%s}`, row.count)
			err := value.Decode(jx.DecodeBytes([]byte(body)))
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != row.valid {
				t.Fatalf("valid=%v, decode/validate=%v", row.valid, err)
			}
			if row.valid && fmt.Sprint(value.Count) != row.count {
				t.Fatalf("count truncated: %d", value.Count)
			}
		})
	}
}

func TestNativeNullableTerminationPolicyRequired(t *testing.T) {
	prefix := `{"mode":"postpaid","period":null,"quantity_unit":"item","amount":null,"monthly_amount":null,"saving_percent":null,"unit_amount":"0.10","unit":"hour"`
	for _, row := range []struct {
		name, suffix string
		valid, null  bool
	}{
		{"required null", `,"termination_policy":null}`, true, true},
		{"immediate", `,"termination_policy":"immediate"}`, true, false},
		{"period end", `,"termination_policy":"period_end"}`, true, false},
		{"missing", `}`, false, false},
		{"unsupported", `,"termination_policy":"unknown"}`, false, false},
		{"client bad enum", `,"termination_policy":"<nil>"}`, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			var value PricingOption
			err := value.Decode(jx.DecodeBytes([]byte(prefix + row.suffix)))
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != row.valid {
				t.Fatalf("valid=%v, decode/validate=%v", row.valid, err)
			}
			if row.valid && value.TerminationPolicy.Null != row.null {
				t.Fatalf("null changed: %+v", value.TerminationPolicy)
			}
			if row.null {
				if _, ok := value.TerminationPolicy.Get(); ok {
					t.Fatal("null became a policy value")
				}
				wire, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(wire), `"termination_policy":null`) {
					t.Fatalf("null not retained: %s", wire)
				}
			}
		})
	}
}

// 记录现有生成边界：描述中的条件与 monetary 格式并不自动成为 ogen 校验；不修改契约设计。
func TestNativeDocumentsSDKValidationBoundaries(t *testing.T) {
	for _, body := range []string{`{"mode":"postpaid","period":{"unit":"month","count":1}}`, `{"mode":"prepaid"}`} {
		var value BillingChoice
		if err := value.Decode(jx.DecodeBytes([]byte(body))); err != nil {
			t.Fatal(err)
		}
		if err := value.Validate(); err != nil {
			t.Fatalf("SDK added a conditional rule: %v", err)
		}
	}
	var option PricingOption
	body := `{"mode":"postpaid","period":null,"quantity_unit":"item","amount":null,"monthly_amount":null,"saving_percent":null,"unit_amount":"1e3","unit":"hour","termination_policy":null}`
	if err := option.Decode(jx.DecodeBytes([]byte(body))); err != nil {
		t.Fatal(err)
	}
	if err := option.Validate(); err != nil {
		t.Fatalf("SDK added a money pattern: %v", err)
	}
}
