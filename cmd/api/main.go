package main

import (
	"allspark.local/fiscal/internal/engine"
	"allspark.local/fiscal/internal/httpapi"
	"allspark.local/fiscal/internal/observability"
	"allspark.local/fiscal/internal/platform"
	"allspark.local/fiscal/internal/queue"
	"allspark.local/fiscal/internal/store"
	"allspark.local/fiscal/internal/vault"
	"allspark.local/fiscal/internal/worker"
	"context"
	"encoding/json"
	"errors"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if e := run(ctx); e != nil && !errors.Is(e, context.Canceled) {
		log.Fatal(e)
	}
}
func run(ctx context.Context) error {
	s, e := store.Open(ctx, platform.DatabaseURL(), platform.Int("DB_POOL_MAX", 10))
	if e != nil {
		return errors.New("database unavailable")
	}
	defer s.DB.Close()
	role := platform.Env("ROLE", "api")
	if role == "migrate" {
		password := platform.Secret("APP_PASSWORD")
		if _, e = s.DB.Exec(ctx, "ALTER ROLE allspark PASSWORD '"+strings.ReplaceAll(password, "'", "''")+"'"); e != nil {
			return errors.New("cannot provision database application role")
		}

		return s.Migrate(ctx, "/app/migrations")
	}
	if role == "tenant-create" {
		name := platform.Env("TENANT_NAME", "")
		if strings.TrimSpace(name) == "" {
			return errors.New("TENANT_NAME required")
		}
		id, token, e := s.CreateTenant(ctx, name)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"tenant_id": id, "api_key": token})
	}
	if role == "key-revoke" {
		id := platform.Env("KEY_ID", "")
		tag, e := s.DB.Exec(ctx, "UPDATE api_keys SET revoked_at=now() WHERE id=$1", id)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return errors.New("key not found")
		}
		return nil
	}
	if role == "dispatcher" {
		go func() {
			tick := time.NewTicker(15 * time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					if e := worker.Recover(ctx, s); e != nil {
						log.Print("lease recovery temporarily unavailable")
					}
				}
			}
		}()
		return brokerLoop(ctx, func(b *queue.Broker) error { return b.Dispatch(ctx, s) })
	}
	v, e := vault.FromFile(platform.Env("MASTER_KEY_FILE", "/run/secrets/master_key"))
	if e != nil {
		return errors.New("invalid master key")
	}
	objects := store.Objects{Dir: platform.Env("OBJECTS_DIR", "/data/objects"), Vault: v}
	if platform.Env("STORAGE_DRIVER", "filesystem") == "s3" {
		client, err := minio.New(platform.Env("S3_ENDPOINT", ""), &minio.Options{Creds: credentials.NewStaticV4(platform.Env("S3_ACCESS_KEY", ""), platform.Secret("S3_SECRET_KEY"), ""), Secure: platform.Env("S3_TLS", "true") == "true", Region: platform.Env("S3_REGION", "us-east-1")})
		if err != nil {
			return errors.New("invalid S3 configuration")
		}
		objects.S3 = client
		objects.Bucket = platform.Env("S3_BUCKET", "allspark-fiscal")
	}
	eng := engine.New(platform.Env("ENGINE_URL", "http://engine:9000"), platform.Secret("ENGINE_TOKEN"))
	var cache *redis.Client
	if rp := platform.Secret("REDIS_PASSWORD"); rp != "" || platform.Env("REDIS_ADDR", "") != "" {
		cache = redis.NewClient(&redis.Options{Addr: platform.Env("REDIS_ADDR", "redis:6379"), Password: rp, PoolSize: 16, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second})
		defer cache.Close()
	}
	if role == "worker" {
		w := worker.Worker{Store: s, Engine: eng, Vault: v, Objects: objects, Redis: cache, Concurrency: platform.Int("WORKER_CONCURRENCY", 8)}
		return brokerLoop(ctx, func(b *queue.Broker) error { return w.Run(ctx, b) })
	}
	if role != "api" {
		return errors.New("unknown ROLE")
	}
	if cache == nil {
		cache = redis.NewClient(&redis.Options{Addr: platform.Env("REDIS_ADDR", "redis:6379"), Password: platform.Secret("REDIS_PASSWORD"), PoolSize: 16, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second})
		defer cache.Close()
	}
	metrics := &observability.Metrics{}
	api := &httpapi.Server{Metrics: metrics, Store: s, Redis: cache, Vault: v, Objects: objects, Engine: eng, Issuance: platform.Env("ENABLE_ISSUANCE", "false") == "true", Rate: platform.Int("TENANT_RATE_PER_SECOND", 200), MaxPending: platform.Int("MAX_PENDING_JOBS", 100000)}
	metricsServer := &http.Server{Addr: ":9090", Handler: metrics.Handler(s), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Print("metrics server stopped")
		}
	}()
	defer metricsServer.Close()
	server := &http.Server{Addr: platform.Env("ALLSPARK_LISTEN", ":8080"), Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 110 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Print("AllSpark Fiscal API ready")
	e = server.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
func brokerLoop(ctx context.Context, run func(*queue.Broker) error) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		b, e := queue.Open(platform.RabbitURL())
		if e == nil {
			e = run(b)
			b.Close()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Print("broker connection interrupted; reconnecting")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
