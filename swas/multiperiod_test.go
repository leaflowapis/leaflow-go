package swas_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	api "github.com/leaflowapis/leaflow-go/swas/v1/server"
)

const planJSON = `{"provider_plan_id":"plan-1","region_id":"region-1","name":"Synthetic","catalog_revision":1,"buyable":false,"price":null,"image_ids":["image-1"],"cpu":2,"memory_mb":1024,"disk_gb":40}`
const optionJSON = `{"billing_plan_id":"10000000-0000-4000-8000-000000000001","billing_price_id":"20000000-0000-4000-8000-000000000001","currency":"USD","amount":"10.01","period_months":1}`

type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type security struct{}

func (security) ScopedToken(context.Context, api.OperationName) (api.ScopedToken, error) {
	return api.ScopedToken{Token: "synthetic-token"}, nil
}

func TestMultiPeriodPlanOptionalPrices(t *testing.T) {
	var p api.Plan
	if err := json.Unmarshal([]byte(planJSON), &p); err != nil {
		t.Fatal(err)
	}
	if p.Prices != nil {
		t.Fatal("default decoder created optional prices")
	}
	b, err := json.Marshal(&p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"prices"`) {
		t.Fatalf("default prices leaked: %s", b)
	}
	p.Prices = []api.PriceOption{}
	b, err = json.Marshal(&p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"prices":[]`) {
		t.Fatalf("explicit empty prices omitted: %s", b)
	}
	var option api.PriceOption
	if err := json.Unmarshal([]byte(optionJSON), &option); err != nil {
		t.Fatal(err)
	}
	for _, period := range []api.PriceOptionPeriodMonths{1, 3, 6, 12, 24, 36} {
		option.PeriodMonths = period
		if err := option.Validate(); err != nil {
			t.Fatalf("period %d: %v", period, err)
		}
	}
	p.Prices = []api.PriceOption{option}
	b, err = json.Marshal(&p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded api.Plan
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Prices) != 1 || decoded.Prices[0].PeriodMonths != 36 || decoded.Prices[0].Amount.Value != "10.01" || !decoded.Prices[0].Amount.Set {
		t.Fatalf("option roundtrip: %+v", decoded.Prices)
	}
	for _, bad := range []string{`"period_months":2`, `"period_months":null`} {
		raw := strings.Replace(optionJSON, `"period_months":1`, bad, 1)
		var rejected api.PriceOption
		if err := json.Unmarshal([]byte(raw), &rejected); err == nil && rejected.Validate() == nil {
			t.Fatalf("invalid period accepted: %s", raw)
		}
	}
	t.Log("nil omitted; explicit empty retained; six periods and exact amount round-trip; invalid periods rejected")
}

func TestMultiPeriodListQueryNoNetwork(t *testing.T) {
	for _, tt := range []struct {
		name    string
		value   api.OptBool
		want    string
		present bool
	}{
		{"omitted", api.OptBool{}, "", false}, {"false", api.NewOptBool(false), "false", true}, {"true", api.NewOptBool(true), "true", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			seen := false
			client, err := api.NewClient("https://synthetic.invalid", security{}, api.WithClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
				seen = true
				if r.URL.Query().Has("include_prices") != tt.present || r.URL.Query().Get("include_prices") != tt.want {
					t.Errorf("query: %s", r.URL.RawQuery)
				}
				if r.URL.Query().Get("region_id") != "region-1" {
					t.Error("region lost")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"items":[],"source_observed_at":"2026-10-25T00:00:00Z","complete":true}`)), Request: r}, nil
			})}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.ListPlans(context.Background(), api.ListPlansParams{RegionID: "region-1", IncludePrices: tt.value}); err != nil {
				t.Fatal(err)
			}
			if !seen {
				t.Fatal("transport not called")
			}
			t.Logf("include_prices present=%v value=%q; synthetic transport only", tt.present, tt.want)
		})
	}
}

func TestMultiPeriodPreservesCanonicalNestedError(t *testing.T) {
	raw := []byte(`{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"Invalid synthetic request","reason":"INVALID_ARGUMENT"}}`)
	var parsed api.Error
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Error.Code != 400 || parsed.Error.Status != "INVALID_ARGUMENT" || parsed.Error.Message != "Invalid synthetic request" {
		t.Fatalf("canonical error shape: %+v", parsed)
	}
	encoded, err := json.Marshal(&parsed)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(encoded, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if _, ok := roundtrip["error"]; !ok {
		t.Fatalf("nested error lost: %s", encoded)
	}
	if _, ok := roundtrip["status"]; ok {
		t.Fatalf("stale flat error leaked: %s", encoded)
	}
	t.Log("Google nested Error wire representation retained; no legacy codec touched")
}
