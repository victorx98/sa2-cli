package cli

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/victorx98/sa2-cli/internal/api"
	"github.com/victorx98/sa2-cli/internal/session"
)

const (
	mockPassword = " synthetic-password "
	mockAccess   = "synthetic-access"
	mockRefresh  = "synthetic-refresh-"
)

// The fake vault keeps synthetic fixtures outside the application's lock directory.
type fixtureVault struct {
	dir   string
	fault string
}

func (v fixtureVault) Read(_ context.Context, key string) ([]byte, error) {
	if v.fault == "read" {
		return nil, session.ErrUnavailable
	}
	raw, err := os.ReadFile(filepath.Join(v.dir, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, session.ErrNotFound
	}
	return raw, err
}

func (v fixtureVault) Write(_ context.Context, key string, value []byte) error {
	fault := "save"
	if bytes.Equal(value, []byte{1, 0}) {
		fault = "pending"
	}
	if v.fault == fault {
		return session.ErrUnavailable
	}
	return os.WriteFile(filepath.Join(v.dir, key), value, 0600)
}

func (v fixtureVault) Delete(_ context.Context, key string) error {
	err := os.Remove(filepath.Join(v.dir, key))
	if errors.Is(err, os.ErrNotExist) {
		return session.ErrNotFound
	}
	return err
}

func (fixtureVault) Close() {}

func TestChildProcess(t *testing.T) {
	if os.Getenv("SA2_TEST_CHILD") != "1" {
		return
	}
	certificate, err := os.ReadFile(os.Getenv("SA2_TEST_ROOT_CA"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		t.Fatal("invalid test CA")
	}
	client := api.NewHTTPClient()
	client.Transport.(*http.Transport).TLSClientConfig.RootCAs = roots
	if os.Getenv("SA2_TEST_TIMEOUT") == "1" {
		client.Timeout = 500 * time.Millisecond
	}
	app := App{
		In: os.Stdin, Out: os.Stdout, Err: os.Stderr, Version: "test",
		Origin: os.Getenv("SA2_API_URL"),
		API:    api.Client{HTTP: client},
		OpenSession: func(context.Context) (session.Vault, *session.Locker, error) {
			fault := os.Getenv("SA2_TEST_FAULT")
			if fault == "open" {
				return nil, nil, session.ErrUnavailable
			}
			return fixtureVault{dir: os.Getenv("SA2_TEST_VAULT"), fault: fault},
				session.NewLocker(os.Getenv("SA2_TEST_LOCKS")), nil
		},
	}
	args := slices.Index(os.Args, "--")
	if args < 0 {
		t.Fatal("missing CLI arguments")
	}
	os.Exit(app.Run(t.Context(), os.Args[args+1:]))
}

type harness struct {
	t      *testing.T
	dir    string
	origin string
}

func startHarness(t *testing.T, handler http.HandlerFunc) harness {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(filepath.Join(dir, "ca.pem"), cert, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "fixture-vault"), 0700); err != nil {
		t.Fatal(err)
	}
	return harness{t: t, dir: dir, origin: server.URL}
}

func (h harness) command(input, fault string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(h.t.Context(), os.Args[0], append([]string{"-test.run=^TestChildProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(),
		"SA2_TEST_CHILD=1",
		"SA2_API_URL="+h.origin,
		"SA2_TEST_ROOT_CA="+filepath.Join(h.dir, "ca.pem"),
		"SA2_TEST_VAULT="+filepath.Join(h.dir, "fixture-vault"),
		"SA2_TEST_LOCKS="+filepath.Join(h.dir, "locks"),
		"SA2_TEST_FAULT="+fault,
	)
	cmd.Stdin = strings.NewReader(input)
	return cmd
}

type observation struct {
	exit int
	data json.RawMessage
	code string
	err  error
}

func observe(cmd *exec.Cmd) observation {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exit := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exit = exitErr.ExitCode()
	} else if err != nil {
		return observation{err: err}
	}
	if stderr.Len() != 0 || bytes.Count(stdout.Bytes(), []byte("\n")) != 1 ||
		strings.Contains(stdout.String(), "synthetic-") {
		return observation{err: errors.New("invalid output streams")}
	}
	var result struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return observation{err: err}
	}
	return observation{exit: exit, data: result.Data, code: result.Error.Code}
}

func (h harness) expect(cmd *exec.Cmd, exit int, code string) json.RawMessage {
	h.t.Helper()
	result := observe(cmd)
	if result.err != nil || result.exit != exit || result.code != code {
		h.t.Fatalf("expected exit %d and %s, got %+v", exit, code, result)
	}
	return result.data
}

func (h harness) login() {
	h.t.Helper()
	h.expect(h.command(mockPassword+"\r\n", "", "login", "--email", "user@example.test", "--password-stdin"), 0, "")
}

func reply(w http.ResponseWriter, token string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accessToken": mockAccess, "refreshToken": token,
		"user": map[string]any{
			"id": "user-id", "email": "user@example.test",
			"roles": []map[string]string{{"enName": "student"}},
		},
	})
}

func TestProcessSessionLifecycle(t *testing.T) {
	var mu sync.Mutex
	var refreshes []string
	var loginRequests int
	h := startHarness(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodPost {
			t.Error("non-POST authentication request")
		}
		switch r.URL.Path {
		case "/api/auth/login":
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil ||
				input["password"] != mockPassword || input["email"] != "user@example.test" {
				t.Error("login request shape or password changed")
			}
			loginRequests++
			reply(w, mockRefresh+"0")
		case "/api/auth/refresh":
			var input map[string]string
			token := fmt.Sprint(mockRefresh, len(refreshes))
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input["refreshToken"] != token {
				t.Error("refresh did not consume the latest token")
			}
			refreshes = append(refreshes, input["refreshToken"])
			reply(w, fmt.Sprint(mockRefresh, len(refreshes)))
		default:
			t.Errorf("unexpected API path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	h.expect(h.command("", "", "whoami"), 3, "LOGIN_REQUIRED")
	h.expect(h.command("", "open", "login", "--email", "user@example.test", "--password-stdin"), 1, "CREDENTIAL_STORE_UNAVAILABLE")
	h.expect(h.command(mockPassword, "read", "login", "--email", "user@example.test", "--password-stdin"), 1, "CREDENTIAL_STORE_UNAVAILABLE")
	h.expect(h.command("", "", "login", "--email", "user@example.test"), 2, "INVALID_ARGUMENT")
	h.expect(h.command("", "", "whoami", "--api-url", "http://127.0.0.1"), 2, "INVALID_ARGUMENT")
	h.login()

	results := make(chan observation, 2)
	for range 2 {
		go func() { results <- observe(h.command("", "", "whoami")) }()
	}
	for range 2 {
		got := <-results
		if got.err != nil || got.exit != 0 ||
			string(got.data) != `{"id":"user-id","email":"user@example.test","roles":["student"]}` {
			t.Fatalf("concurrent whoami failed: %+v", got)
		}
	}
	h.expect(h.command("", "", "--api-url", "https://127.0.0.1:1", "whoami"), 3, "LOGIN_REQUIRED")
	for range 2 {
		data := h.expect(h.command("", "", "logout"), 0, "")
		if string(data) != `{"loggedOut":true}` {
			t.Fatalf("unexpected logout result %s", data)
		}
	}
	h.expect(h.command("", "", "whoami"), 3, "LOGIN_REQUIRED")
	mu.Lock()
	defer mu.Unlock()
	if loginRequests != 1 || !slices.Equal(refreshes, []string{mockRefresh + "0", mockRefresh + "1"}) {
		t.Fatal("unexpected login or refresh count")
	}
	file, err := os.Stat(filepath.Join(h.dir, "locks", session.Key(h.origin)+".lock"))
	if err != nil || file.Size() != 0 {
		t.Fatal("application lock must exist and remain empty")
	}
}

func TestProcessRefreshFailure(t *testing.T) {
	cases := []struct {
		name, fault, code      string
		status, exit, requests int
		timeout                bool
	}{
		{name: "pending write", fault: "pending", code: "CREDENTIAL_STORE_UNAVAILABLE", exit: 1, requests: 0},
		{name: "save", fault: "save", code: "CREDENTIAL_STORE_UNAVAILABLE", exit: 1, requests: 1},
		{name: "unauthorized", status: 401, code: "LOGIN_REQUIRED", exit: 3, requests: 1},
		{name: "server unavailable", status: 503, code: "LOGIN_REQUIRED", exit: 3, requests: 1},
		{name: "timeout", timeout: true, code: "LOGIN_REQUIRED", exit: 3, requests: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			h := startHarness(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/auth/login" {
					reply(w, mockRefresh+"0")
					return
				}
				requests.Add(1)
				switch {
				case test.timeout:
					_, _ = io.Copy(io.Discard, r.Body)
					select {
					case <-r.Context().Done():
					case <-time.After(8 * time.Second):
						t.Error("timed out request connection was not closed")
					}
				case test.status != 0:
					w.WriteHeader(test.status)
					fmt.Fprint(w, mockRefresh+"error")
				default:
					reply(w, mockRefresh+"1")
				}
			})
			h.login()
			cmd := h.command("", test.fault, "whoami")
			if test.timeout {
				cmd.Env = append(cmd.Env, "SA2_TEST_TIMEOUT=1")
			}
			h.expect(cmd, test.exit, test.code)
			if test.requests > 0 {
				h.expect(h.command("", "", "whoami"), 3, "LOGIN_REQUIRED")
			}
			if observed := int(requests.Load()); observed != test.requests {
				t.Fatalf("refresh attempts = %d, want %d", observed, test.requests)
			}
		})
	}
}

func TestProcessInterruptedAfterSendingRefresh(t *testing.T) {
	received := make(chan struct{}, 2)
	h := startHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login" {
			reply(w, mockRefresh+"0")
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		received <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-time.After(8 * time.Second):
			t.Error("interrupted request connection was not closed")
		}
	})
	h.login()
	cmd := h.command("", "", "whoami")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	select {
	case <-received:
	case <-time.After(8 * time.Second):
		t.Fatal("refresh request was not received")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("killed command exited successfully")
	}
	h.expect(h.command("", "", "whoami"), 3, "LOGIN_REQUIRED")
	select {
	case <-received:
		t.Fatal("the next process replayed the previous refresh token")
	default:
	}
}

func TestProcessRejectsRedirect(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.WriteHeader(401)
	}))
	defer target.Close()
	h := startHarness(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	h.expect(h.command(mockPassword, "", "login", "--email", "user@example.test", "--password-stdin"), 1, "API_UNAVAILABLE")
	if redirected.Load() != 0 {
		t.Fatal("authentication request followed a redirect")
	}
}
