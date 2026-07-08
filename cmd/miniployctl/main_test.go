package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/itsamenathan/miniploy/internal/config"
)

func TestHealthCommandDisabled(t *testing.T) {
	cfg := config.Config{HealthEnabled: false}

	if err := health(context.Background(), cfg); err != nil {
		t.Fatalf("health() error = %v, want nil", err)
	}
}

func TestHealthCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.Config{HealthEnabled: true, HealthAddr: server.URL}
	if err := health(context.Background(), cfg); err != nil {
		t.Fatalf("health() error = %v, want nil", err)
	}
}

func TestHealthCommandUnhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := config.Config{HealthEnabled: true, HealthAddr: server.URL}
	if err := health(context.Background(), cfg); err == nil {
		t.Fatal("health() error = nil, want error")
	}
}

func TestHealthURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "localhost address",
			addr: "127.0.0.1:8080",
			want: "http://127.0.0.1:8080/healthz",
		},
		{
			name: "port only",
			addr: ":8080",
			want: "http://127.0.0.1:8080/healthz",
		},
		{
			name: "all interfaces ipv4",
			addr: "0.0.0.0:8080",
			want: "http://127.0.0.1:8080/healthz",
		},
		{
			name: "all interfaces ipv6",
			addr: "[::]:8080",
			want: "http://127.0.0.1:8080/healthz",
		},
		{
			name: "full url",
			addr: "http://localhost:9000",
			want: "http://localhost:9000/healthz",
		},
		{
			name: "full url trailing slash",
			addr: "http://localhost:9000/",
			want: "http://localhost:9000/healthz",
		},
		{
			name: "https url",
			addr: "https://miniploy.example.com",
			want: "https://miniploy.example.com/healthz",
		},
		{
			name: "trim whitespace",
			addr: " 127.0.0.1:8080 ",
			want: "http://127.0.0.1:8080/healthz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := healthURL(tt.addr); got != tt.want {
				t.Fatalf("healthURL(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
