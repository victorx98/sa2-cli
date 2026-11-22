package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

var ErrFeishuCallbackUnavailable = errors.New("local Feishu callback unavailable")

const feishuCallbackScript = `
const params = new URLSearchParams(location.hash.slice(1));
history.replaceState(null, '', location.pathname);
const code = params.get('code');
const proof = params.get('proof');
const message = document.getElementById('result');
if (code && proof) {
  fetch(location.pathname, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({code, proof})
  }).then(response => {
    if (response.ok) message.textContent = 'Authorization received. Return to your terminal for the result.';
  }).catch(() => {});
}
`

func waitForFeishuCode(ctx context.Context, origin string, out io.Writer) (string, error) {
	var entropy [32]byte
	rand.Read(entropy[:])
	proof := base64.RawURLEncoding.EncodeToString(entropy[:])
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", ErrFeishuCallbackUnavailable
	}
	defer listener.Close()
	addr := listener.Addr().(*net.TCPAddr)
	localOrigin := "http://" + addr.String()
	loginURL, err := url.Parse(origin + "/auth/cli")
	if err != nil {
		return "", err
	}
	loginURL.Fragment = "sa2-cli=" + strconv.Itoa(addr.Port) + "." + proof

	code := make(chan string, 1)
	var accepted atomic.Bool
	scriptHash := sha256.Sum256([]byte(feishuCallbackScript))
	policy := "default-src 'none'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; script-src 'sha256-" +
		base64.StdEncoding.EncodeToString(scriptHash[:]) + "'"
	server := &http.Server{
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 8192,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", policy)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if r.Host != addr.String() || r.URL.Path != "/feishu" || r.URL.RawQuery != "" {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprint(w, `<!doctype html><html lang="en"><meta charset="utf-8"><title>SA2 CLI</title>`+
					`<p id="result">If authorization does not complete, return to your terminal and start again.</p>`+
					`<script>`+feishuCallbackScript+`</script></html>`)
			case http.MethodPost:
				if r.Header.Get("Origin") != localOrigin || r.Header.Get("Content-Type") != "application/json" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				var input struct {
					Code  string `json:"code"`
					Proof string `json:"proof"`
				}
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
				defer clear(body)
				if err != nil || json.Unmarshal(body, &input) != nil ||
					len(input.Code) > 4096 || strings.TrimSpace(input.Code) == "" ||
					subtle.ConstantTimeCompare([]byte(input.Proof), []byte(proof)) != 1 {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if !accepted.CompareAndSwap(false, true) {
					w.WriteHeader(http.StatusConflict)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				code <- input.Code
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}),
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if server.Shutdown(shutdown) != nil {
			server.Close()
		}
	}()
	if _, err := fmt.Fprintf(out, "Open this URL in a browser on this computer, then scan with Feishu:\n%s\n", loginURL); err != nil {
		return "", err
	}
	openFeishuBrowser(out, loginURL.String())
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	select {
	case received := <-code:
		return received, ctx.Err()
	case <-ctx.Done():
		return "", ctx.Err()
	case <-serveDone:
		return "", ErrFeishuCallbackUnavailable
	}
}

func openFeishuBrowser(out io.Writer, loginURL string) {
	file, ok := out.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return
		}
		cmd = exec.Command("xdg-open", loginURL)
	case "darwin":
		cmd = exec.Command("open", loginURL)
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", loginURL)
	default:
		return
	}
	if cmd.Start() == nil {
		go func() { _ = cmd.Wait() }()
	}
}
