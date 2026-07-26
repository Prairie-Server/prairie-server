package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/prairie-server/prairie-server/internal/clientip"
	"github.com/prairie-server/prairie-server/internal/models"
)

type fakeOAuthClient struct {
	initResp     *pluginv1.InitAuthorizeResponse
	initErr      error
	exchangeResp *pluginv1.AuthenticateResponse
	exchangeErr  error

	gotInit     *pluginv1.InitAuthorizeRequest
	gotExchange *pluginv1.ExchangeCodeRequest
}

func (f *fakeOAuthClient) InitAuthorize(_ context.Context, in *pluginv1.InitAuthorizeRequest) (*pluginv1.InitAuthorizeResponse, error) {
	f.gotInit = in
	return f.initResp, f.initErr
}

func (f *fakeOAuthClient) ExchangeCode(_ context.Context, in *pluginv1.ExchangeCodeRequest) (*pluginv1.AuthenticateResponse, error) {
	f.gotExchange = in
	return f.exchangeResp, f.exchangeErr
}

type fakeCompleter struct {
	gotInput OAuthLoginInput
	pair     *TokenPair
	user     *models.User
	err      error
}

func (f *fakeCompleter) CompleteOAuthLogin(_ context.Context, in OAuthLoginInput) (*TokenPair, *models.User, error) {
	f.gotInput = in
	return f.pair, f.user, f.err
}

func newOAuthHandlerForTest(_ *testing.T, fc *fakeOAuthClient, fcomp *fakeCompleter) (*OAuthHandler, *InMemoryOAuthStore) {
	store := NewInMemoryOAuthStore()
	h := NewOAuthHandler(OAuthHandlerDeps{
		Store:       store,
		StateSecret: []byte("test-secret"),
		ResolveClient: func(_ context.Context, _ int) (OAuthClient, string, error) {
			return fc, "whmcs", nil
		},
		LoginCompleter:       fcomp,
		HostBaseURL:          "https://prairie.test",
		StateTTL:             10 * time.Minute,
		FrontendCompletePath: "/login/oauth-complete",
	})
	return h, store
}

func withInstallID(r *http.Request, id string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("install_id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func TestOAuthInit_Redirects302WithAuthorizeURL(t *testing.T) {
	authorizeURL := "https://billing.example/oauth/authorize.php?client_id=x"
	pState, _ := structpb.NewStruct(map[string]any{"pkce_verifier": "abc"})
	fc := &fakeOAuthClient{initResp: &pluginv1.InitAuthorizeResponse{AuthorizeUrl: authorizeURL, ProviderState: pState}}
	fcomp := &fakeCompleter{}
	h, store := newOAuthHandlerForTest(t, fc, fcomp)

	r := withInstallID(httptest.NewRequest("POST", "/api/v1/auth/oauth/42/init?next=/me", nil), "42")
	w := httptest.NewRecorder()
	h.HandleInit(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("code = %d body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != authorizeURL {
		t.Errorf("Location = %q", got)
	}

	// Exactly one row inserted; next_url preserved.
	if got := len(store.rows); got != 1 {
		t.Errorf("rows = %d, want 1", got)
	}
	for _, row := range store.rows {
		if row.InstallID != "42" {
			t.Errorf("InstallID = %q", row.InstallID)
		}
		if row.NextURL != "/me" {
			t.Errorf("NextURL = %q", row.NextURL)
		}
		if !strings.Contains(string(row.ProviderState), "pkce_verifier") {
			t.Errorf("ProviderState missing PKCE: %s", row.ProviderState)
		}
	}

	// Plugin received our state.
	if fc.gotInit == nil || fc.gotInit.GetState() == "" {
		t.Error("plugin not invoked or empty state")
	}
}

func TestOAuthInit_NormalizesUnsafeNextURL(t *testing.T) {
	authorizeURL := "https://billing.example/oauth/authorize.php?client_id=x"
	fc := &fakeOAuthClient{initResp: &pluginv1.InitAuthorizeResponse{AuthorizeUrl: authorizeURL}}
	h, store := newOAuthHandlerForTest(t, fc, &fakeCompleter{})

	r := withInstallID(httptest.NewRequest("POST", "/api/v1/auth/oauth/42/init?next=//evil.example/path", nil), "42")
	w := httptest.NewRecorder()
	h.HandleInit(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("code = %d body = %s", w.Code, w.Body.String())
	}
	for _, row := range store.rows {
		if row.NextURL != "/" {
			t.Fatalf("NextURL = %q, want /", row.NextURL)
		}
	}
}

func TestOAuthInit_RejectsBadInstallID(t *testing.T) {
	h, _ := newOAuthHandlerForTest(t, &fakeOAuthClient{}, &fakeCompleter{})
	cases := []string{"", "abc", "-1", "0"}
	for _, c := range cases {
		r := withInstallID(httptest.NewRequest("POST", "/init", nil), c)
		w := httptest.NewRecorder()
		h.HandleInit(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("install_id=%q → code %d, want 400", c, w.Code)
		}
	}
}

func TestOAuthInit_PluginErrorReturns502(t *testing.T) {
	fc := &fakeOAuthClient{initErr: errors.New("upstream db dsn leaked")}
	h, _ := newOAuthHandlerForTest(t, fc, &fakeCompleter{})
	r := withInstallID(httptest.NewRequest("POST", "/init", nil), "42")
	w := httptest.NewRecorder()
	h.HandleInit(w, r)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", w.Code)
	}
	if strings.Contains(w.Body.String(), "upstream db dsn leaked") {
		t.Errorf("response leaked internal error: %q", w.Body.String())
	}
}

func TestOAuthInit_EmptyAuthorizeURLIs502(t *testing.T) {
	fc := &fakeOAuthClient{initResp: &pluginv1.InitAuthorizeResponse{AuthorizeUrl: ""}}
	h, _ := newOAuthHandlerForTest(t, fc, &fakeCompleter{})
	r := withInstallID(httptest.NewRequest("POST", "/init", nil), "42")
	w := httptest.NewRecorder()
	h.HandleInit(w, r)
	if w.Code != http.StatusBadGateway {
		t.Errorf("code = %d, want 502", w.Code)
	}
}

func TestOAuthCallback_HappyPath_DeliversOneTimeCompletionCode(t *testing.T) {
	pState, _ := structpb.NewStruct(map[string]any{"pkce_verifier": "abc"})
	fc := &fakeOAuthClient{
		initResp:     &pluginv1.InitAuthorizeResponse{AuthorizeUrl: "https://a", ProviderState: pState},
		exchangeResp: &pluginv1.AuthenticateResponse{ExternalSubject: "ws-1", Email: "u@x.com", DisplayName: "U"},
	}
	fcomp := &fakeCompleter{
		pair: &TokenPair{AccessToken: "acc.tok", RefreshToken: "ref.tok", ExpiresIn: 900},
		user: &models.User{ID: 7},
	}
	h, _ := newOAuthHandlerForTest(t, fc, fcomp)

	// Init first to populate the store + capture the signed state.
	rInit := withInstallID(httptest.NewRequest("POST", "/api/v1/auth/oauth/42/init?next=/me", nil), "42")
	wInit := httptest.NewRecorder()
	h.HandleInit(wInit, rInit)
	if wInit.Code != http.StatusFound {
		t.Fatalf("init code = %d body=%s", wInit.Code, wInit.Body.String())
	}
	state := fc.gotInit.GetState()

	// Callback with that state.
	rCb := withInstallID(httptest.NewRequest("GET", "/api/v1/auth/oauth/42/callback?code=auth-code&state="+url.QueryEscape(state), nil), "42")
	wCb := httptest.NewRecorder()
	h.HandleCallback(wCb, rCb)

	if wCb.Code != http.StatusFound {
		t.Fatalf("callback code = %d body = %s", wCb.Code, wCb.Body.String())
	}
	loc := wCb.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://prairie.test/login/oauth-complete?") {
		t.Fatalf("redirect = %q", loc)
	}
	if strings.Contains(loc, "acc.tok") || strings.Contains(loc, "ref.tok") {
		t.Fatalf("redirect leaked tokens: %q", loc)
	}
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	code := parsed.Query().Get("code")
	if code == "" {
		t.Fatalf("completion code missing from redirect: %q", loc)
	}

	rComplete := httptest.NewRequest("POST", "/api/v1/auth/oauth/complete", strings.NewReader(`{"code":"`+code+`"}`))
	wComplete := httptest.NewRecorder()
	h.HandleComplete(wComplete, rComplete)
	if wComplete.Code != http.StatusOK {
		t.Fatalf("complete code = %d body=%s", wComplete.Code, wComplete.Body.String())
	}
	var completed OAuthCompleteResponse
	if err := json.NewDecoder(wComplete.Body).Decode(&completed); err != nil {
		t.Fatalf("decode complete response: %v", err)
	}
	if completed.AccessToken != "acc.tok" {
		t.Errorf("access_token = %q", completed.AccessToken)
	}
	if completed.RefreshToken != "ref.tok" {
		t.Errorf("refresh_token = %q", completed.RefreshToken)
	}
	if completed.NextURL != "/me" {
		t.Errorf("next = %q", completed.NextURL)
	}

	// Completer received the right inputs.
	if fcomp.gotInput.InstallationID != 42 {
		t.Errorf("InstallationID = %d", fcomp.gotInput.InstallationID)
	}
	if fcomp.gotInput.Response.GetExternalSubject() != "ws-1" {
		t.Errorf("ExternalSubject = %q", fcomp.gotInput.Response.GetExternalSubject())
	}
	if fcomp.gotInput.CapabilityID != "whmcs" {
		t.Errorf("CapabilityID = %q", fcomp.gotInput.CapabilityID)
	}
}

func TestOAuthCallback_RejectsTamperedState(t *testing.T) {
	h, _ := newOAuthHandlerForTest(t, &fakeOAuthClient{}, &fakeCompleter{})
	r := withInstallID(httptest.NewRequest("GET", "/cb?code=x&state=tampered.junk", nil), "42")
	w := httptest.NewRecorder()
	h.HandleCallback(w, r)
	if w.Code != http.StatusFound {
		t.Fatalf("code = %d", w.Code)
	}
	if got := w.Header().Get("Location"); !strings.Contains(got, "error=oauth_failed") {
		t.Errorf("Location = %q", got)
	}
}

func TestOAuthCallback_RejectsMissingCodeOrState(t *testing.T) {
	h, _ := newOAuthHandlerForTest(t, &fakeOAuthClient{}, &fakeCompleter{})
	r := withInstallID(httptest.NewRequest("GET", "/cb", nil), "42")
	w := httptest.NewRecorder()
	h.HandleCallback(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
}

func TestOAuthCallback_InstallMismatchRedirectsToLoginError(t *testing.T) {
	pState, _ := structpb.NewStruct(map[string]any{})
	fc := &fakeOAuthClient{initResp: &pluginv1.InitAuthorizeResponse{AuthorizeUrl: "https://a", ProviderState: pState}}
	h, _ := newOAuthHandlerForTest(t, fc, &fakeCompleter{})

	// Init for install 42.
	rInit := withInstallID(httptest.NewRequest("POST", "/init", nil), "42")
	wInit := httptest.NewRecorder()
	h.HandleInit(wInit, rInit)
	state := fc.gotInit.GetState()

	// Callback against install 99 with state signed for 42 → mismatch.
	rCb := withInstallID(httptest.NewRequest("GET", "/cb?code=c&state="+url.QueryEscape(state), nil), "99")
	wCb := httptest.NewRecorder()
	h.HandleCallback(wCb, rCb)
	if wCb.Code != http.StatusFound {
		t.Errorf("code = %d", wCb.Code)
	}
	if got := wCb.Header().Get("Location"); !strings.Contains(got, "install_mismatch") {
		t.Errorf("Location = %q", got)
	}
}

func TestOAuthCallback_ExchangeFailRedirectsToLoginError(t *testing.T) {
	pState, _ := structpb.NewStruct(map[string]any{})
	fc := &fakeOAuthClient{
		initResp:    &pluginv1.InitAuthorizeResponse{AuthorizeUrl: "https://a", ProviderState: pState},
		exchangeErr: errors.New("token endpoint returned 400 with secret context"),
	}
	h, _ := newOAuthHandlerForTest(t, fc, &fakeCompleter{})

	rInit := withInstallID(httptest.NewRequest("POST", "/init", nil), "42")
	h.HandleInit(httptest.NewRecorder(), rInit)
	state := fc.gotInit.GetState()

	rCb := withInstallID(httptest.NewRequest("GET", "/cb?code=c&state="+url.QueryEscape(state), nil), "42")
	wCb := httptest.NewRecorder()
	h.HandleCallback(wCb, rCb)
	if wCb.Code != http.StatusFound {
		t.Errorf("code = %d", wCb.Code)
	}
	if got := wCb.Header().Get("Location"); !strings.Contains(got, "reason=exchange_failed") {
		t.Errorf("Location = %q", got)
	} else if strings.Contains(got, "token endpoint") || strings.Contains(got, "secret context") {
		t.Errorf("Location leaked internal error: %q", got)
	}
}

func TestOAuthCallback_CompleteLoginErrorRedirectsWithGenericReason(t *testing.T) {
	pState, _ := structpb.NewStruct(map[string]any{})
	fc := &fakeOAuthClient{
		initResp:     &pluginv1.InitAuthorizeResponse{AuthorizeUrl: "https://a", ProviderState: pState},
		exchangeResp: &pluginv1.AuthenticateResponse{ExternalSubject: "ws-1"},
	}
	fcomp := &fakeCompleter{err: errors.New("insert users failed: private db detail")}
	h, _ := newOAuthHandlerForTest(t, fc, fcomp)

	rInit := withInstallID(httptest.NewRequest("POST", "/init", nil), "42")
	h.HandleInit(httptest.NewRecorder(), rInit)
	state := fc.gotInit.GetState()

	rCb := withInstallID(httptest.NewRequest("GET", "/cb?code=c&state="+url.QueryEscape(state), nil), "42")
	wCb := httptest.NewRecorder()
	h.HandleCallback(wCb, rCb)
	if wCb.Code != http.StatusFound {
		t.Errorf("code = %d", wCb.Code)
	}
	if got := wCb.Header().Get("Location"); !strings.Contains(got, "reason=login_failed") {
		t.Errorf("Location = %q", got)
	} else if strings.Contains(got, "private db detail") {
		t.Errorf("Location leaked internal error: %q", got)
	}
}

func TestClientIPUsesResolvedContextAndPreservesIPv6(t *testing.T) {
	tests := []struct {
		name       string
		contextIP  string
		remoteAddr string
		want       string
	}{
		{
			name:       "middleware context wins",
			contextIP:  "2001:db8::7",
			remoteAddr: "10.0.0.1:1234",
			want:       "2001:db8::7",
		},
		{
			name:       "bracketed ipv6 with port",
			remoteAddr: "[::1]:8080",
			want:       "::1",
		},
		{
			name:       "bare ipv6 from middleware remote addr",
			remoteAddr: "::1",
			want:       "::1",
		},
		{
			name:       "ipv4 with port",
			remoteAddr: "192.0.2.10:8080",
			want:       "192.0.2.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.contextIP != "" {
				r = r.WithContext(clientip.SetContext(r.Context(), tt.contextIP))
			}
			if got := clientIP(r); got != tt.want {
				t.Fatalf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
