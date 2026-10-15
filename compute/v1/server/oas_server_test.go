package computev1server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-faster/jx"
	"github.com/ogen-go/ogen/ogenerrors"
)

type testScopedAuthKey struct{}

type testScopedAuthentication struct{}

func (testScopedAuthentication) HandleScopedTokenAuth(ctx context.Context, operation OperationName, token ScopedTokenAuth) (context.Context, error) {
	if token.Token != "test-scoped" {
		return nil, errors.New("test token refused")
	}
	return context.WithValue(ctx, testScopedAuthKey{}, true), nil
}

func (testScopedAuthentication) ScopedTokenAuth(ctx context.Context, operation OperationName) (ScopedTokenAuth, error) {
	return ScopedTokenAuth{Token: "test-scoped"}, nil
}

type testTypeCatalogHandler struct {
	UnimplementedHandler
	calls atomic.Int64
}

var testMachineTypeOffSale = errors.New("test machine type is not on sale")

func (handler *testTypeCatalogHandler) NewError(ctx context.Context, err error) *ErrorStatusCode {
	status := http.StatusBadRequest
	var security *ogenerrors.SecurityError
	if errors.As(err, &security) {
		status = http.StatusUnauthorized
	}
	if errors.Is(err, testMachineTypeOffSale) {
		status = http.StatusNotFound
	}
	return &ErrorStatusCode{StatusCode: status, Response: Error{Message: "test request refused", Status: int64(status)}}
}

func (handler *testTypeCatalogHandler) GetMachineType(ctx context.Context, params GetMachineTypeParams) (*MachineType, error) {
	if authenticated, _ := ctx.Value(testScopedAuthKey{}).(bool); !authenticated {
		return nil, errors.New("authentication context missing")
	}
	handler.calls.Add(1)
	if params.MachineTypeId != "mtype_00000000000000000000000001" {
		return nil, testMachineTypeOffSale
	}
	var item MachineType
	body := `{"id":"mtype_00000000000000000000000001","name":"small","region_id":"region_00000000000000000000000001","availability_zone_id":"zone_00000000000000000000000001","vcpus":1,"ram_mb":1024,"max_network_interfaces":1,"max_elastic_ips":1,"max_bandwidth_mbps":100,"network_egress_kbps":null,"network_ingress_kbps":null,"plan_id":"11111111-1111-1111-1111-111111111111"}`
	if err := item.Decode(jx.DecodeBytes([]byte(body))); err != nil {
		return nil, err
	}
	return &item, item.Validate()
}

func TestNativeGetMachineTypeHTTPReadAndAuthentication(t *testing.T) {
	for _, scenario := range []struct {
		name, identity, authorization string
		status                        int
		called                        bool
	}{
		{"on sale", "mtype_00000000000000000000000001", "Bearer test-scoped", http.StatusOK, true},
		{"off sale", "mtype_00000000000000000000000002", "Bearer test-scoped", http.StatusNotFound, true},
		{"missing authentication", "mtype_00000000000000000000000001", "", http.StatusUnauthorized, false},
		{"UUID", "11111111-1111-1111-1111-111111111111", "Bearer test-scoped", http.StatusBadRequest, false},
		{"wrong namespace", "disk_00000000000000000000000001", "Bearer test-scoped", http.StatusBadRequest, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			handler := &testTypeCatalogHandler{}
			server, err := NewServer(handler, testScopedAuthentication{})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/machine-types/"+scenario.identity, nil)
			if scenario.authorization != "" {
				request.Header.Set("Authorization", scenario.authorization)
			}
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != scenario.status || (handler.calls.Load() != 0) != scenario.called {
				t.Fatalf("status=%d, calls=%d, body=%s", response.Code, handler.calls.Load(), response.Body.String())
			}
			if response.Code == http.StatusOK {
				var item MachineType
				if err := item.Decode(jx.DecodeBytes(response.Body.Bytes())); err != nil {
					t.Fatal(err)
				}
				if err := item.Validate(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func (handler *testTypeCatalogHandler) ListElasticIPTypes(ctx context.Context, params ListElasticIPTypesParams) (*ElasticIPTypeListResponseBody, error) {
	if authenticated, _ := ctx.Value(testScopedAuthKey{}).(bool); !authenticated {
		return nil, errors.New("authentication context missing")
	}
	handler.calls.Add(1)
	return &ElasticIPTypeListResponseBody{
		Items:      []ElasticIPType{},
		Pagination: OffsetPagination{Page: 1, PageSize: 50},
	}, nil
}

func TestNativeCatalogHTTPAuthenticationAndFilters(t *testing.T) {
	for _, scenario := range []struct {
		name, query, authorization string
		valid                      bool
	}{
		{"missing token", "", "", false},
		{"wrong token", "", "Bearer wrong", false},
		{"optional region", "", "Bearer test-scoped", true},
		{"typed region", "?region_id=region_00000000000000000000000001", "Bearer test-scoped", true},
		{"UUID region", "?region_id=11111111-1111-1111-1111-111111111111", "Bearer test-scoped", false},
		{"page size limit", "?page_size=201", "Bearer test-scoped", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			handler := &testTypeCatalogHandler{}
			server, err := NewServer(handler, testScopedAuthentication{})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/elastic-ip-types"+scenario.query, nil)
			if scenario.authorization != "" {
				request.Header.Set("Authorization", scenario.authorization)
			}
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if accepted := response.Code >= 200 && response.Code < 300; accepted != scenario.valid {
				t.Fatalf("valid=%v, status=%d, response=%s", scenario.valid, response.Code, response.Body.String())
			}
			if called := handler.calls.Load() != 0; called != scenario.valid {
				t.Fatalf("valid=%v, handler calls=%d", scenario.valid, handler.calls.Load())
			}
		})
	}
}

func TestNativeCreateElasticIPClientRefusesInvalidInputsBeforeHTTP(t *testing.T) {
	var calls atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/api/v1/elastic-ips" || request.Header.Get("Authorization") != "Bearer test-scoped" {
			t.Error("generated client lost request path or authentication")
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusForbidden)
		_, _ = response.Write([]byte(`{"code":"TEST_REFUSAL","message":"test-only refusal"}`))
	}))
	t.Cleanup(backend.Close)
	client, err := NewClient(backend.URL, testScopedAuthentication{})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		address IPv4HostAddress
		typeID  ElasticIPTypeID
		valid   bool
	}{
		{"192.0.2.10", "eiptype_00000000000000000000000001", true},
		{"2001:db8::10", "eiptype_00000000000000000000000001", false},
		{"::ffff:192.0.2.10", "eiptype_00000000000000000000000001", false},
		{"192.00.2.10", "eiptype_00000000000000000000000001", false},
		{"256.0.0.1", "eiptype_00000000000000000000000001", false},
		{"192.0.2.10", "11111111-1111-1111-1111-111111111111", false},
	} {
		before := calls.Load()
		_, err := client.CreateElasticIP(t.Context(), &CreateElasticIPRequestBody{
			Address:               NewOptIPv4HostAddress(scenario.address),
			TypeID:                scenario.typeID,
			BandwidthMbps:         50,
			Billing:               BillingChoice{Mode: BillingChoiceModePostpaid},
		})
		if err == nil {
			t.Fatal("test backend or validation must refuse the request")
		}
		if issued := calls.Load() != before; issued != scenario.valid {
			t.Fatalf("valid=%v, HTTP issued=%v, error=%v", scenario.valid, issued, err)
		}
	}
}
