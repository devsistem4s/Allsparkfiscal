package platform

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func Env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func Int(k string, d int) int {
	v, e := strconv.Atoi(Env(k, strconv.Itoa(d)))
	if e != nil || v < 1 {
		panic("invalid " + k)
	}
	return v
}
func Secret(k string) string {
	p := os.Getenv(k + "_FILE")
	if p == "" {
		panic(k + "_FILE is required")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		panic("cannot read " + k + "_FILE")
	}
	v := strings.TrimSpace(string(b))
	if len(v) < 32 {
		panic(k + " too short")
	}
	return v
}
func DatabaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s/allspark?sslmode=%s", url.QueryEscape(Env("DB_USER", "allspark")), url.QueryEscape(Secret("DB_PASSWORD")), Env("DB_HOST", "postgres:5432"), Env("DB_SSLMODE", "disable"))
}
func RabbitURL() string {
	return fmt.Sprintf("amqp://allspark:%s@%s/", url.QueryEscape(Secret("RABBIT_PASSWORD")), Env("RABBIT_HOST", "rabbitmq:5672"))
}
