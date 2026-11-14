package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FiscalProvider interface {
	Call(context.Context, string, any, any) error
}

type Client struct {
	URL, Token string
	HTTP       *http.Client
}

func New(url, token string) *Client {
	return &Client{url, token, &http.Client{Timeout: 90 * time.Second}}
}

type Error struct {
	Code      string `json:"code"`
	SefazCode string `json:"sefaz_code,omitempty"`
	Message   string `json:"message,omitempty"`
	Action    string `json:"action,omitempty"`
	Retryable bool   `json:"retryable"`
}

func (e *Error) Error() string { return e.Code }
func (c *Client) Call(ctx context.Context, action string, input any, output any) error {
	b, e := json.Marshal(input)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.URL+"/"+action, bytes.NewReader(b))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, e := c.HTTP.Do(req)
	if e != nil {
		return &Error{Code: "ENGINE_UNAVAILABLE", Retryable: true}
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if e != nil {
		return &Error{Code: "ENGINE_UNAVAILABLE", Retryable: true}
	}
	if res.StatusCode != 200 {
		var er Error
		if json.Unmarshal(raw, &er) != nil || er.Code == "" {
			er.Code = fmt.Sprintf("ENGINE_HTTP_%d", res.StatusCode)
		}
		return &er
	}
	if e = json.Unmarshal(raw, output); e != nil {
		return errors.New("invalid engine response")
	}
	return nil
}

type Certificate struct {
	PFX      string `json:"pfx_base64"`
	Password string `json:"password"`
	CSC      string `json:"csc,omitempty"`
	CSCID    string `json:"csc_id,omitempty"`
}
type CertInfo struct {
	CNPJ        string    `json:"cnpj"`
	Fingerprint string    `json:"fingerprint"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type Artifact struct {
	Kind      string `json:"kind"`
	MediaType string `json:"media_type"`
	Base64    string `json:"base64"`
}
type Result struct {
	Status    string     `json:"status"`
	AccessKey string     `json:"access_key"`
	Artifacts []Artifact `json:"artifacts"`
	Protocol  any        `json:"protocol"`
	Errors    []Error    `json:"errors"`
}
