package store

import (
	"allspark.local/fiscal/internal/vault"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Objects struct {
	Dir    string
	Vault  *vault.Vault
	S3     *minio.Client
	Bucket string
}

func (o Objects) Put(tenant, doc, kind, media string, data []byte) (Artifact, error) {
	id := uuid.NewString()
	sum := sha256.Sum256(data)
	a := Artifact{ID: id, Kind: kind, MediaType: media, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	if o.S3 != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		encrypted := o.Vault.Seal(data, tenant+"/"+doc+"/"+id)
		_, e := o.S3.PutObject(ctx, o.Bucket, tenant+"/"+doc+"/"+id, bytes.NewReader(encrypted), int64(len(encrypted)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
		return a, e
	}
	if e := os.MkdirAll(o.Dir, 0700); e != nil {
		return a, e
	}
	f, e := os.OpenFile(filepath.Join(o.Dir, id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return a, e
	}
	sealed := o.Vault.Seal(data, tenant+"/"+doc+"/"+id)
	_, e = f.Write(sealed)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		_ = os.Remove(filepath.Join(o.Dir, id))
	}
	return a, e
}
func (o Objects) Get(tenant, doc, id string) ([]byte, error) {
	if _, e := uuid.Parse(id); e != nil {
		return nil, e
	}
	var b []byte
	var e error
	if o.S3 != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		obj, err := o.S3.GetObject(ctx, o.Bucket, tenant+"/"+doc+"/"+id, minio.GetObjectOptions{})
		if err != nil {
			return nil, err
		}
		defer obj.Close()
		b, e = io.ReadAll(io.LimitReader(obj, 16<<20))
	} else {
		b, e = os.ReadFile(filepath.Join(o.Dir, id))
	}
	if e != nil {
		return nil, e
	}
	return o.Vault.Open(b, tenant+"/"+doc+"/"+id)
}
func (s *Store) Artifact(ctx context.Context, tenant, doc, id string) (Artifact, error) {
	var a Artifact
	e := s.DB.QueryRow(ctx, "SELECT id::text,kind,media_type,sha256,size FROM artifacts WHERE tenant_id=$1 AND document_id=$2 AND id=$3", tenant, doc, id).Scan(&a.ID, &a.Kind, &a.MediaType, &a.SHA256, &a.Size)
	return a, e
}
