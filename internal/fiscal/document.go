// Package fiscal define o vocabulário comum, independente do motor fiscal.
package fiscal

import "context"

type Kind string

const (
	NFe  Kind = "nfe"
	NFCe Kind = "nfce"
	NFSe Kind = "nfse"
)

type FiscalError struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Source       string `json:"source"`
	Retryable    bool   `json:"retryable"`
	ProviderCode string `json:"provider_code,omitempty"`
}

type Artifact struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // xml, pdf, protocol_xml, event_xml
	MediaType   string `json:"media_type"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"` // URL temporária, autorizada por tenant
}

type Protocol struct {
	Number     string `json:"number"`
	StatusCode string `json:"status_code"`
	Reason     string `json:"reason"`
	ReceivedAt string `json:"received_at"`
}

type Event struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Status    string     `json:"status"`
	Protocol  *Protocol  `json:"protocol"`
	Artifacts []Artifact `json:"artifacts"`
}

type Document struct {
	ID          string        `json:"id"`
	TenantID    string        `json:"tenant_id"`
	CompanyID   string        `json:"company_id"`
	Kind        Kind          `json:"kind"`
	Environment string        `json:"environment"`
	Status      string        `json:"status"`
	AccessKey   string        `json:"access_key,omitempty"`
	Protocol    *Protocol     `json:"protocol"`
	Artifacts   []Artifact    `json:"artifacts"`
	Events      []Event       `json:"events"`
	Errors      []FiscalError `json:"errors"`
}

// Engine é uma porta para adaptadores. A implementação remota será interna,
// sem certificados, senhas ou detalhes SOAP no contrato público.
type Engine interface {
	Submit(ctx context.Context, command SubmitCommand) (Document, error)
	Reconcile(ctx context.Context, document Document) (Document, error)
}

type SubmitCommand struct {
	TenantID       string
	CompanyID      string
	DocumentID     string
	IdempotencyKey string
	Kind           Kind
	Environment    string
	Payload        []byte
}
