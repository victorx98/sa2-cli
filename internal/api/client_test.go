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
