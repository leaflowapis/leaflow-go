package billingv1server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type contactPathSecurity struct{}

func (contactPathSecurity) AccessTokenAuth(context.Context, OperationName) (AccessTokenAuth, error) {
	return AccessTokenAuth{Token: "contact-test-token"}, nil
}

func TestContactClientAccountPaths(t *testing.T) {
	id := uuid.MustParse("60000000-0000-4000-8000-000000000001")
	expected := fmt.Sprintf("/api/v1/billing-accounts/42/contacts/%s", id)
	type observed struct{ method, path, authorization, body string }
	seen := make(chan observed, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		raw, _ := io.ReadAll(request.Body)
		seen <- observed{request.Method, request.URL.EscapedPath(), request.Header.Get("Authorization"), string(raw)}
		w.Header().Set("Content-Type", "application/json")
		if request.URL.EscapedPath() != expected {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"BILLING_CONTACT_NOT_FOUND","message":"contact is not in this account"}`))
			return
		}
		if request.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		name := "Finance"
		if request.Method == http.MethodPatch {
			name = "Updated"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id.String(), "billing_account_id": 42, "name": name,
			"active": true, "tax_exempt": false, "created_at": "2026-10-06T00:00:00Z",
		})
	}))
	defer server.Close()
	client, err := NewClient(server.URL, contactPathSecurity{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	contact, err := client.GetContact(ctx, GetContactParams{AccountId: 42, ContactId: id})
	if err != nil {
		t.Fatal(err)
	}
	if contact.BillingAccountID != 42 || contact.ID != id {
		t.Fatalf("wrong contact: %+v", contact)
	}
	contact, err = client.UpdateContact(ctx, &ContactUpdate{Name: NewOptString("Updated")}, UpdateContactParams{AccountId: 42, ContactId: id})
	if err != nil {
		t.Fatal(err)
	}
	if contact.Name != "Updated" {
		t.Fatalf("wrong update: %q", contact.Name)
	}
	if err = client.DeleteContact(ctx, DeleteContactParams{AccountId: 42, ContactId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetContact(ctx, GetContactParams{AccountId: 99, ContactId: id}); err == nil {
		t.Fatal("wrong parent must preserve the 404 response")
	}
	for index, method := range []string{"GET", "PATCH", "DELETE", "GET"} {
		request := <-seen
		route := expected
		if index == 3 {
			route = fmt.Sprintf("/api/v1/billing-accounts/99/contacts/%s", id)
		}
		if request.method != method || request.path != route {
			t.Fatalf("wrong request: %+v", request)
		}
		if request.authorization != "Bearer contact-test-token" {
			t.Fatal("authorization was not forwarded")
		}
		if method == "PATCH" {
			var body map[string]any
			if err = json.Unmarshal([]byte(request.body), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body["name"] != "Updated" {
				t.Fatalf("wrong patch body: %#v", body)
			}
		} else if request.body != "" {
			t.Fatalf("unexpected body for %s", method)
		}
	}
	if len(seen) != 0 {
		t.Fatal("client retried through another route")
	}
}
