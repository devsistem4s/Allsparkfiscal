// Package vault implements envelope encryption with AES-256-GCM.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

type Vault struct{ aead cipher.AEAD }

func New(key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, errors.New("master key must contain 32 bytes")
	}
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	a, e := cipher.NewGCM(b)
	return &Vault{a}, e
}
func FromFile(path string) (*Vault, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	k, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if e != nil {
		return nil, e
	}
	return New(k)
}
func nonce(a cipher.AEAD) []byte {
	n := make([]byte, a.NonceSize())
	if _, e := rand.Read(n); e != nil {
		panic(e)
	}
	return n
}
func (v *Vault) Seal(plain []byte, ctx string) []byte {
	dek := make([]byte, 32)
	if _, e := rand.Read(dek); e != nil {
		panic(e)
	}
	defer clear(dek)
	data, _ := New(dek)
	wrapNonce := nonce(v.aead)
	dataNonce := nonce(data.aead)
	wrapped := v.aead.Seal(wrapNonce, wrapNonce, dek, []byte("v2:dek:"+ctx))
	payload := data.aead.Seal(dataNonce, dataNonce, plain, []byte("v2:data:"+ctx))
	return append(append([]byte{2}, wrapped...), payload...)
}
func (v *Vault) Open(sealed []byte, ctx string) ([]byte, error) {
	n := v.aead.NonceSize()
	if len(sealed) < 1+n+v.aead.Overhead() {
		return nil, errors.New("invalid ciphertext")
	}
	if sealed[0] == 1 {
		return v.aead.Open(nil, sealed[1:1+n], sealed[1+n:], []byte("v1:"+ctx))
	}
	wrappedLen := n + 32 + v.aead.Overhead()
	if sealed[0] != 2 || len(sealed) < 1+wrappedLen+n+v.aead.Overhead() {
		return nil, errors.New("invalid ciphertext")
	}
	dek, e := v.aead.Open(nil, sealed[1:1+n], sealed[1+n:1+wrappedLen], []byte("v2:dek:"+ctx))
	if e != nil {
		return nil, e
	}
	defer clear(dek)
	data, e := New(dek)
	if e != nil {
		return nil, e
	}
	p := sealed[1+wrappedLen:]
	return data.aead.Open(nil, p[:n], p[n:], []byte("v2:data:"+ctx))
}
