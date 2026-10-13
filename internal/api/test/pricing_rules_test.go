package test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	. "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

func TestPricingRulesGETReturnsModelAndNormalizedRulesRemoved(t *testing.T) {
	provider := &pricingStub{rules: []servicedto.PricingRule{{Key: "service_tier", Value: "priority", Multiplier: 2}}}
	router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")
	req := newPricingRequest(http.MethodGet, "/api/v1/pricing/rules?model=openai%2Fgpt-5.6", "")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assertPricingRemoved(t, resp)
	if provider.lastRules != nil {
		t.Fatal("retired rules called service")
	}
}

func TestPricingRulesPUTPreservesOmittedNullAndExplicitZeroMultipliersRemoved(t *testing.T) {
	for name, body := range map[string]string{
		"omitted": `{"model":"model-a","rules":[{"key":"service_tier","value":"priority"}]}`,
		"null":    `{"model":"model-a","rules":[{"key":"service_tier","value":"priority","multiplier":null}]}`,
		"zero":    `{"model":"model-a","rules":[{"key":"service_tier","value":"priority","multiplier":0}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			provider := &pricingStub{rules: []servicedto.PricingRule{{Key: "service_tier", Value: "priority", Multiplier: 1}}}
			router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")
			req := newPricingRequest(http.MethodPut, "/api/v1/pricing/rules", body)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			assertPricingRemoved(t, resp)
			if provider.lastRules != nil {
				t.Fatal("retired rules called service")
			}
		})
	}
}

func TestPricingRulesPUTAllowsEmptyArrayToClearRulesRemoved(t *testing.T) {
	assertRetiredPricingRequest(t, "PUT", "/api/v1/pricing/rules", `{"model":"model-a","rules":[]}`)
}

func TestPricingRulesRoutesMapValidationAndMissingModelErrorsRemoved(t *testing.T) {
	for _, test := range []struct {
		name        string
		method      string
		url         string
		body        string
		providerErr error
	}{
		{name: "missing query model", method: http.MethodGet, url: "/api/v1/pricing/rules"},
		{name: "missing body model", method: http.MethodPut, url: "/api/v1/pricing/rules", body: `{"rules":[]}`},
		{name: "unknown model", method: http.MethodGet, url: "/api/v1/pricing/rules?model=missing", providerErr: service.ErrPricingModelNotFound},
		{name: "unknown field", method: http.MethodPut, url: "/api/v1/pricing/rules", body: `{"model":"model-a","rules":[{"key":"provider","value":"openai","multiplier":2}]}`, providerErr: service.ErrInvalidPricingRule},
		{name: "duplicate", method: http.MethodPut, url: "/api/v1/pricing/rules", body: `{"model":"model-a","rules":[{"key":"service_tier","value":"priority","multiplier":2},{"key":"service_tier","value":"priority","multiplier":3}]}`, providerErr: service.ErrInvalidPricingRule},
		{name: "huge finite combination", method: http.MethodPut, url: "/api/v1/pricing/rules", body: `{"model":"model-a","rules":[{"key":"service_tier","value":"priority","multiplier":1e308}]}`, providerErr: service.ErrInvalidPricingRule},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &pricingStub{err: test.providerErr}
			router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")
			req := newPricingRequest(test.method, test.url, test.body)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			assertPricingRemoved(t, resp)
			if provider.lastRules != nil {
				t.Fatal("retired rules called service")
			}
		})
	}
}

func TestPricingRulesPUTRejectsNonJSONFiniteNumbersRemoved(t *testing.T) {
	for _, value := range []string{"NaN", "Infinity", "-Infinity"} {
		provider := &pricingStub{}
		router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")
		body := `{"model":"model-a","rules":[{"key":"service_tier","value":"priority","multiplier":` + value + `}]}`
		req := newPricingRequest(http.MethodPut, "/api/v1/pricing/rules", body)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assertPricingRemoved(t, resp)
		if provider.lastRules != nil {
			t.Fatal("retired rules called service")
		}
	}
}

func TestPricingRulesRoutesRequireAdminAuthentication(t *testing.T) {
	provider := &pricingStub{err: errors.New("must not be called")}
	config := AuthConfig{Enabled: true, LoginPassword: "secret"}
	router := NewRouter(nil, nil, nil, provider, config, NewAuthHandler(config, nil), "")
	req := newPricingRequest(http.MethodGet, "/api/v1/pricing/rules?model=model-a", "")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated pricing rules status 401, got %d %s", resp.Code, resp.Body.String())
	}
}
