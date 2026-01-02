package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/steipete/goplaces"
)

func TestRunSearchJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/places:searchText" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"places": [{"id": "abc"}]}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"search",
		"coffee",
		"--api-key", "test-key",
		"--base-url", server.URL,
		"--json",
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"results\"") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunSearchHuman(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"places": [{"id": "abc", "displayName": {"text": "Cafe"}}]}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"search",
		"coffee",
		"--api-key", "test-key",
		"--base-url", server.URL,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "Cafe") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunDetailsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/places/place-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id": "place-1"}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"details",
		"place-1",
		"--api-key", "test-key",
		"--base-url", server.URL,
		"--json",
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "\"place_id\"") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunDetailsHuman(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id": "place-2", "displayName": {"text": "Park"}}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"details",
		"place-2",
		"--api-key", "test-key",
		"--base-url", server.URL,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "Park") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunResolveHuman(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/places:searchText" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"places": [{"id": "loc-1", "displayName": {"text": "Downtown"}}]}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"resolve",
		"Downtown",
		"--api-key", "test-key",
		"--base-url", server.URL,
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "Downtown") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunResolveJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"places": [{"id": "loc-2"}]}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"resolve",
		"Downtown",
		"--api-key", "test-key",
		"--base-url", server.URL,
		"--json",
	}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "\"results\"") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--version"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout.String()) != Version {
		t.Fatalf("unexpected version: %s", stdout.String())
	}
}

func TestRunMissingCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code")
	}
}

func TestRunParseError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"search", "--api-key", "x"}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatalf("expected parse error")
	}
}

func TestRunLocationBiasError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"search", "coffee", "--lat", "1", "--api-key", "x"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected validation error exit code 2, got %d", exitCode)
	}
}

func TestRunHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if stdout.Len() == 0 {
		t.Fatalf("expected help output")
	}
}

func TestVersionFlagIsBool(t *testing.T) {
	var flag VersionFlag
	if !flag.IsBool() {
		t.Fatalf("expected IsBool true")
	}
}

func TestWriteJSONError(t *testing.T) {
	err := writeJSON(&bytes.Buffer{}, map[string]any{"bad": func() {}})
	if err == nil {
		t.Fatalf("expected json error")
	}
}

func TestWriteJSON(t *testing.T) {
	var out bytes.Buffer
	if err := writeJSON(&out, map[string]string{"ok": "true"}); err != nil {
		t.Fatalf("writeJSON error: %v", err)
	}
	if !strings.Contains(out.String(), "\"ok\"") {
		t.Fatalf("unexpected json output: %s", out.String())
	}
}

func TestHandleError(t *testing.T) {
	if code := handleError(&bytes.Buffer{}, nil); code != 0 {
		t.Fatalf("expected 0")
	}
	if code := handleError(&bytes.Buffer{}, goplaces.ValidationError{Field: "x", Message: "bad"}); code != 2 {
		t.Fatalf("expected validation exit 2")
	}
	if code := handleError(&bytes.Buffer{}, goplaces.ErrMissingAPIKey); code != 2 {
		t.Fatalf("expected missing api key exit 2")
	}
	if code := handleError(&bytes.Buffer{}, errors.New("boom")); code != 1 {
		t.Fatalf("expected generic exit 1")
	}
}
