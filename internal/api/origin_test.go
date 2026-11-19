package api

import "testing"

func TestOrigin(t *testing.T) {
	valid := map[string]string{
		"https://Example.Test:00443/": "https://example.test",
		"https://example.test.":       "https://example.test",
		"https://127.0.0.1:8443/":     "https://127.0.0.1:8443",
		"https://[0000::1]:8443/":     "https://[::1]:8443",
	}
	for input, want := range valid {
		got, err := Origin(input)
		if err != nil || got != want {
			t.Errorf("Origin(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	invalid := []string{
		"", "http://localhost", "https://user:secret@example.test", "https://example.test/path",
		"https://example.test?", "https://example.test/#", "https://example.test/%2F",
		"https://[fe80::1%25eth0]", "https://a..b", "https://example.test:",
		"https://example.test:65536", "https://example.test:0", "https://example.test/../",
	}
	for _, input := range invalid {
		if _, err := Origin(input); err == nil {
			t.Errorf("accepted invalid origin %q", input)
		}
	}
}
