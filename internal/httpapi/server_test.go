package httpapi

import (
	"allspark.local/fiscal/internal/store"
	"encoding/json"
	"os"
	"testing"
)

func TestCNPJ(t *testing.T) {
	for _, v := range []struct {
		v  string
		ok bool
	}{{"11222333000181", true}, {"00000000000000", false}, {"11222333000182", false}, {"../../etc", false}, {"", false}} {
		if validCNPJ(v.v) != v.ok {
			t.Fatalf("CNPJ %s", v.v)
		}
	}
}
func TestDocumentValidation(t *testing.T) {
	s := Server{}
	in := store.Input{CompanyID: "f964109e-c301-4a43-a8da-18018b84c013", Kind: "nfe", Operation: "generate", Environment: "homologation", Series: 1, Invoice: json.RawMessage(`[{"tag":"ide","values":{}}]`)}
	raw, _ := os.ReadFile("../../examples/document.json")
	_ = json.Unmarshal(raw, &in)
	in.CompanyID = "f964109e-c301-4a43-a8da-18018b84c013"
	if why := s.validate(in, "test-key-123"); why != "" {
		t.Fatal(why)
	}
	in.Operation = "issue"
	if s.validate(in, "test-key-123") == "" {
		t.Fatal("issuance enabled unexpectedly")
	}
	in.Operation = "generate"
	in.Environment = "production"
	if why := s.validate(in, "test-key-123"); why != "" {
		t.Fatalf("production should be allowed: %s", why)
	}
	in.Environment = "staging"
	if s.validate(in, "test-key-123") == "" {
		t.Fatal("invalid environment should be rejected")
	}
}
