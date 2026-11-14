package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMutationScopes(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if got := requiredScope(method, []string{"v1", "companies", "id", "certificate"}); got != "certificates:write" {
			t.Fatal(got)
		}
		if got := requiredScope(method, []string{"v1", "companies", "id"}); got != "companies:write" {
			t.Fatal(got)
		}
	}
}
func TestPaginationBounds(t *testing.T) {
	for _, q := range []string{"limit=0", "limit=101", "cursor=bad", "limit=abc"} {
		_, _, ok := pagination(httptest.NewRequest("GET", "/v1/companies?"+q, nil))
		if ok {
			t.Fatal(q)
		}
	}
}
func TestCompanyPatchRejectsIdentity(t *testing.T) {
	r := httptest.NewRequest("PATCH", "/", strings.NewReader(`{"cnpj":"11222333000181"}`))
	r.Header.Set("Content-Type", "application/json")
	var dst struct {
		Name string `json:"name"`
	}
	if decode(httptest.NewRecorder(), r, &dst) == nil {
		t.Fatal("identity accepted")
	}
}
