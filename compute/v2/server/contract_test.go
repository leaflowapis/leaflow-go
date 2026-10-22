package computev2server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/go-faster/jx"
)

// 通过真实 ogen 解码与校验检查批准的 oneOf，不将服务端所有权判断伪装成 SDK 校验。
func TestNativeBootDiskDecoder(t *testing.T) {
	image := `{"type":"image","image_id":"image_00000000000000000000000001","disk_type_id":"disktype_00000000000000000000000001","size_gb":20,"billing":{"usage_type":"licensed","period":{"unit":"month","count":1},"termination_policy":"immediate"}}`
	disk := `{"type":"disk","disk_id":"disk_00000000000000000000000001","login_username":"ubuntu"}`
	for _, row := range []struct {
		name, body string
		valid      bool
	}{
		{"image", image, true}, {"disk", disk, true},
		{"unsupported type", strings.Replace(disk, `"disk"`, `"snapshot"`, 1), false},
		{"image plus disk source", image[:len(image)-1] + `,"disk_id":"disk_00000000000000000000000001"}`, false},
		{"disk plus image source", disk[:len(disk)-1] + `,"image_id":"image_00000000000000000000000001"}`, false},
		{"missing discriminator", `{"disk_id":"disk_00000000000000000000000001","login_username":"ubuntu"}`, false},
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

func TestNativeTerminationPolicyPresence(t *testing.T) {
	prefix := `{"usage_type":"licensed","period":{"unit":"month","count":1}`
	for _, row := range []struct {
		name, suffix   string
		valid, present bool
	}{
		{"omitted", `}`, true, false},
		{"immediate", `,"termination_policy":"immediate"}`, true, true},
		{"period end", `,"termination_policy":"period_end"}`, true, true},
		{"null", `,"termination_policy":null}`, false, true},
		{"unsupported", `,"termination_policy":"unknown"}`, false, true},
		{"client bad enum", `,"termination_policy":"<nil>"}`, false, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			var value BillingChoice
			err := value.Decode(jx.DecodeBytes([]byte(prefix + row.suffix)))
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != row.valid {
				t.Fatalf("valid=%v, decode/validate=%v", row.valid, err)
			}
			if row.valid {
				if _, present := value.TerminationPolicy.Get(); present != row.present {
					t.Fatal("policy presence changed")
				}
				wire, err := json.Marshal(&value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(wire), `"termination_policy"`) != row.present {
					t.Fatalf("policy presence not retained: %s", wire)
				}
			}
		})
	}
}

// 商业条件的文字描述不会自动成为 SDK 校验。
func TestNativeDocumentsSDKValidationBoundaries(t *testing.T) {
	for _, body := range []string{`{"usage_type":"metered","period":{"unit":"month","count":1}}`, `{"usage_type":"licensed"}`} {
		var value BillingChoice
		if err := value.Decode(jx.DecodeBytes([]byte(body))); err != nil {
			t.Fatal(err)
		}
		if err := value.Validate(); err != nil {
			t.Fatalf("SDK added a conditional rule: %v", err)
		}
	}
}

// 创建盘请求的周期须完整校验，计数不得被截断。
func TestNativeCreateDiskPreservesPeriod(t *testing.T) {
	for _, row := range []struct {
		count string
		valid bool
	}{{"1", true}, {"2147483647", true}, {"2147483648", false}, {"0", false}, {"-1", false}} {
		t.Run("period "+row.count, func(t *testing.T) {
			raw := `{"disk_type_id":"disktype_00000000000000000000000001","availability_zone_id":"zone_00000000000000000000000001","name":"disk","size_gb":20,"billing":{"usage_type":"licensed","period":{"unit":"month","count":` + row.count + `},"termination_policy":"immediate"}}`
			var value CreateDiskRequestBody
			err := value.Decode(jx.DecodeBytes([]byte(raw)))
			if err == nil {
				err = value.Validate()
			}
			if (err == nil) != row.valid {
				t.Fatalf("period inline=%v valid=%v", err, row.valid)
			}
			if row.valid {
				period, set := value.Billing.Period.Get()
				if !set || fmt.Sprint(period.Count) != row.count {
					t.Fatal("period count/presence changed")
				}
			}
		})
	}
}

// 对比 JSON 字段形状，防 nominal 类型变化引入 Value/Set 或额外包装层。
func TestNativeRequestSerdePreservesWireFields(t *testing.T) {
	for _, raw := range []string{
		`{"disk_type_id":"disktype_00000000000000000000000001","availability_zone_id":"zone_00000000000000000000000001","name":"disk","size_gb":20,"billing":{"usage_type":"licensed","period":{"unit":"month","count":1}}}`,
		`{"disk_type_id":"disktype_00000000000000000000000001","availability_zone_id":"zone_00000000000000000000000001","name":"disk","size_gb":20,"billing":{"usage_type":"licensed","period":{"unit":"month","count":1},"termination_policy":"period_end"},"checkout":{"mode":"deferred"}}`,
	} {
		var value CreateDiskRequestBody
		if err := value.Decode(jx.DecodeBytes([]byte(raw))); err != nil {
			t.Fatal(err)
		}
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
		wire, err := json.Marshal(&value)
		if err != nil {
			t.Fatal(err)
		}
		before, after := map[string]json.RawMessage{}, map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(raw), &before); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(wire, &after); err != nil {
			t.Fatal(err)
		}
		if len(before) != len(after) {
			t.Fatalf("field count changed: %s", wire)
		}
		for key, a := range before {
			b, present := after[key]
			if !present {
				t.Fatalf("field disappeared: %s", key)
			}
			var left, right bytes.Buffer
			if err := json.Compact(&left, a); err != nil {
				t.Fatal(err)
			}
			if err := json.Compact(&right, b); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(left.Bytes(), right.Bytes()) {
				t.Fatalf("field %s changed: %s -> %s", key, a, b)
			}
		}
	}
}
