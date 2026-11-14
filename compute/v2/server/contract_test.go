package computev2server

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/go-faster/jx"
)

// 通过真实 ogen 解码与校验检查批准的 oneOf，不将服务端所有权判断伪装成 SDK 校验。
func TestNativeBootVolumeDecoder(t *testing.T) {
	image := `{"type":"image","image_id":"image_00000000000000000000000001","volume_type_id":"voltype_00000000000000000000000001","size_gib":20,"billing":{"price_id":"11111111-1111-4111-8111-111111111111"}}`
	volume := `{"type":"volume","volume_id":"vol_00000000000000000000000001","login_username":"ubuntu"}`
	for _, row := range []struct {
		name, body string
		valid      bool
	}{
		{"image", image, true}, {"volume", volume, true},
		{"unsupported type", strings.Replace(volume, `"volume"`, `"snapshot"`, 1), false},
		{"image plus volume source", image[:len(image)-1] + `,"volume_id":"vol_00000000000000000000000001"}`, false},
		{"volume plus image source", volume[:len(volume)-1] + `,"image_id":"image_00000000000000000000000001"}`, false},
		{"missing discriminator", `{"volume_id":"vol_00000000000000000000000001","login_username":"ubuntu"}`, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			var value BootVolume
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

func TestNativeBillingChoiceRequiresPriceIDAndRejectsOldTerms(t *testing.T) {
	for _, row := range []struct {
		body  string
		valid bool
	}{
		{`{"price_id":"11111111-1111-4111-8111-111111111111"}`, true},
		{`{}`, false},
		{`{"price_id":"bad"}`, false},
		{`{"price_id":null}`, false},
		{`{"price_id":"11111111-1111-4111-8111-111111111111","period":{"unit":"month","count":1}}`, false},
		{`{"usage_type":"licensed","period":{"unit":"month","count":1}}`, false},
	} {
		var value BillingChoice
		err := value.Decode(jx.DecodeBytes([]byte(row.body)))
		if (err == nil) != row.valid {
			t.Fatalf("body=%s valid=%v error=%v", row.body, row.valid, err)
		}
	}
}

// 对比 JSON 字段形状，防 nominal 类型变化引入 Value/Set 或额外包装层。
func TestNativeRequestSerdePreservesWireFields(t *testing.T) {
	for _, raw := range []string{
		`{"volume_type_id":"voltype_00000000000000000000000001","name":"volume","size_gib":20,"billing":{"price_id":"11111111-1111-4111-8111-111111111111"}}`,
		`{"volume_type_id":"voltype_00000000000000000000000001","name":"volume","size_gib":20,"billing":{"price_id":"11111111-1111-4111-8111-111111111111"},"checkout":{"mode":"deferred","auto_renew":false}}`,
	} {
		var value CreateVolumeRequestBody
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
