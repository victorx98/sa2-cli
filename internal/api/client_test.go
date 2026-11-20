package api

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAuthenticationReachesLoopbackHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/auth/login" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	origin, err := Origin(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := Client{HTTP: NewHTTPClient()}
	_, err = client.Login(t.Context(), origin, "user@example.test", []byte("test password"))
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("login error = %v; want ErrRejected from the server response", err)
	}
}

func TestAuthenticationRejectsUntrustedTLS(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(401)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()

	client := Client{HTTP: NewHTTPClient()}
	_, err := client.Login(t.Context(), server.URL, "user@example.test", []byte("test password"))
	if !errors.Is(err, ErrUnavailable) || requests.Load() != 0 {
		t.Fatal("authentication reached an untrusted TLS endpoint", err)
	}
}
