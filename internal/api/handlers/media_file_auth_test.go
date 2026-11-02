package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apimw "github.com/prairie-server/prairie-server/internal/api/middleware"
	"github.com/prairie-server/prairie-server/internal/auth"
	"github.com/prairie-server/prairie-server/internal/catalog"
	"github.com/prairie-server/prairie-server/internal/scanner"
)

func TestMediaFileAuthorizerMapsMissingFileToNotFound(t *testing.T) {
	authorizer := &MediaFileAuthorizer{
		FileResolver: stubMediaFileResolver{err: scanner.ErrFileNotFound},
		ItemAccess:   stubItemAccessChecker{},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(apimw.SetClaims(req.Context(), &auth.Claims{
		UserID:    1,
		TokenType: auth.TokenTypeAccess,
	}))

	_, err := authorizer.Authorize(req, 99)
	if !errors.Is(err, catalog.ErrItemNotFound) {
		t.Fatalf("Authorize() error = %v, want ErrItemNotFound", err)
	}
}
