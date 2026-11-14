package vault

import (
	"bytes"
	"testing"
)

func TestIsolationAndTampering(t *testing.T) {
	v, _ := New(bytes.Repeat([]byte{7}, 32))
	plain := []byte("private-key-and-password")
	sealed := v.Seal(plain, "tenant-a/cert-a")
	if bytes.Contains(sealed, plain) {
		t.Fatal("plaintext leaked")
	}
	got, e := v.Open(sealed, "tenant-a/cert-a")
	if e != nil || !bytes.Equal(got, plain) {
		t.Fatal(e)
	}
	if _, e = v.Open(sealed, "tenant-b/cert-a"); e == nil {
		t.Fatal("cross-tenant decrypt allowed")
	}
	sealed[len(sealed)-1] ^= 1
	if _, e = v.Open(sealed, "tenant-a/cert-a"); e == nil {
		t.Fatal("tampering accepted")
	}
	if _, e = New([]byte("weak")); e == nil {
		t.Fatal("weak key accepted")
	}
}
