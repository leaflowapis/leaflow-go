package swas_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	api "github.com/leaflowapis/leaflow-go/swas/v1/server"
)

// These are versioned SWAS SDK compatibility checks, not shared CLI behavior tests.
func TestPriceOnlyCheckoutHandoff(t *testing.T) {
	id := uuid.MustParse("10000000-0000-4000-8000-000000000001")
	request := api.PurchaseCreate{CatalogRevision: 1, ProviderPlanID: "synthetic-plan", BillingPriceID: id, ImageID: "synthetic-image", Quantity: 1, RegionID: "synthetic-region"}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	seen := false
	client, err := api.NewClient("https://synthetic.invalid", security{}, api.WithClient(&http.Client{Transport: fakeTransport(func(r *http.Request) (*http.Response, error) {
		seen = true
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err = json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"expected_quote", "amount", "currency", "billing_account_id"} {
			if _, ok := body[forbidden]; ok {
				t.Fatalf("unexpected SWAS monetary authority: %s", forbidden)
			}
		}
		if body["billing_price_id"] != id.String() || r.Header.Get("Idempotency-Key") != "synthetic-intent" {
			t.Fatal("Price or intent key lost")
		}
		return &http.Response{StatusCode: 202, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"10000000-0000-4000-8000-000000000001","state":"checkout_open","checkout_session_id":"20000000-0000-4000-8000-000000000001","delivered":false,"billing_settled":false}`)), Request: r}, nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CreatePurchase(context.Background(), &request, api.CreatePurchaseParams{IdempotencyKey: "synthetic-intent"})
	if err != nil || !seen {
		t.Fatalf("handoff failed: %v", err)
	}
	purchase, ok := result.(*api.Purchase)
	if !ok || purchase.State != "checkout_open" || purchase.CheckoutSessionID == uuid.Nil || purchase.Delivered || purchase.BillingSettled {
		t.Fatalf("checkout intent claimed completed delivery: %#v", result)
	}
	t.Log("Price-only request; deferred Billing session returned; no quote/confirmation/payment call")
}

func TestPriceMetadataMayOmitAmount(t *testing.T) {
	var option api.PriceOption
	raw := strings.Replace(optionJSON, `,"amount":"10.01"`, "", 1)
	if err := json.Unmarshal([]byte(raw), &option); err != nil {
		t.Fatal(err)
	}
	if err := option.Validate(); err != nil {
		t.Fatal(err)
	}
	if option.Amount.Set {
		t.Fatal("omitted display amount was manufactured")
	}
	encoded, err := json.Marshal(&option)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"amount"`) {
		t.Fatalf("absent amount leaked: %s", encoded)
	}
	t.Log("Price identity and period validate without quote amount")
}

func TestPriceOnlyInstanceOmitsHistoricalAmount(t *testing.T) {
	var value api.InstancePrice
	if err := json.Unmarshal([]byte(`{"currency":"USD","period_months":12}`), &value); err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if value.Amount.Set {
		t.Fatal("unknown historical price became an amount")
	}
	raw, err := json.Marshal(&value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"amount"`) {
		t.Fatalf("unknown amount exposed: %s", raw)
	}
	t.Log("instance period retained; unavailable historical amount omitted, not zero")
}
