package logredact

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// Branches Prairie's per-package coverage gate requires: non-container JSON,
// scheme validation, and URL errors whose raw text is only partly present in
// the wrapping message.

func TestSanitizeJSONOmitsScalarsAndMasksPw(t *testing.T) {
	const omitted = "[body omitted: not a complete JSON object or array]"
	for _, body := range []string{`"just a string"`, `42`, `{"a":1} trailing`} {
		if got := string(SanitizeJSON([]byte(body))); got != omitted {
			t.Errorf("SanitizeJSON(%s) = %q, want omitted", body, got)
		}
	}
	got := string(SanitizeJSON([]byte(`{"Pw":"hunter2","list":[{"url":"/x?api_key=abc"}],"n":1.50}`)))
	if strings.Contains(got, "hunter2") || strings.Contains(got, "abc") {
		t.Fatalf("SanitizeJSON leaked a credential: %s", got)
	}
	if !strings.Contains(got, "1.50") {
		t.Fatalf("SanitizeJSON must keep exact JSON numbers: %s", got)
	}
}

func TestLooksLikeDiagnosticURL(t *testing.T) {
	cases := map[string]bool{
		"/Videos/1/stream?api_key=x": true,
		"https://example.test/a":     true,
		"svn+ssh://host/repo":        true,
		"://missing-scheme":          false,
		"1http://digit-first":        false,
		"ht_tp://bad-char":           false,
		"what? no":                   false,
	}
	for in, want := range cases {
		if got := looksLikeDiagnosticURL(in); got != want {
			t.Errorf("looksLikeDiagnosticURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSanitizeURLErrorWrapperWithoutRawText(t *testing.T) {
	raw := &url.Error{Op: "Get", URL: "https://example.test/x?api_key=secret", Err: errors.New("boom")}

	// Neither the raw error text nor the URL appears: fall back to the clone.
	opaque := &wrapped{msg: "opaque failure", cause: raw}
	if got := SanitizeURLError(opaque).Error(); strings.Contains(got, "secret") {
		t.Fatalf("opaque wrapper leaked the credential: %q", got)
	}

	// A single-cause wrapper whose message only names the URL.
	named := &wrapped{msg: "while fetching " + raw.URL, cause: raw}
	if got := SanitizeURLError(named).Error(); strings.Contains(got, "secret") || !strings.Contains(got, "while fetching") {
		t.Fatalf("URL-naming wrapper = %q", got)
	}
}

func TestSanitizeMultiURLErrorFallsBackToJoin(t *testing.T) {
	raw := &url.Error{Op: "Get", URL: "https://example.test/x?token=secret", Err: errors.New("boom")}
	multi := &multiWrapped{msg: "several things failed", errs: []error{errors.New("plain"), raw}}
	got := SanitizeURLError(multi)
	if strings.Contains(got.Error(), "secret") {
		t.Fatalf("multi-error fallback leaked the credential: %q", got)
	}
	unchanged := errors.Join(errors.New("a"), errors.New("b"))
	if SanitizeURLError(unchanged) != unchanged { //nolint:errorlint // identity check is the point
		t.Fatal("a multi-error without URL errors must be returned unchanged")
	}
}

type wrapped struct {
	msg   string
	cause error
}

func (w *wrapped) Error() string { return w.msg }
func (w *wrapped) Unwrap() error { return w.cause }

type multiWrapped struct {
	msg  string
	errs []error
}

func (m *multiWrapped) Error() string   { return m.msg }
func (m *multiWrapped) Unwrap() []error { return m.errs }
