package api

import (
	"strings"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/auth"
	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

func TestSessionFromClaimsDefaultsLegacyTokenToDefaultTenant(t *testing.T) {
	server := NewServer(config.Config{
		SMLAuthProvider:  "smlgoh",
		SMLAuthDataGroup: "sml",
	}, nil, nil)

	session := server.sessionFromClaims(auth.Claims{})
	if session.SMLTenant != store.DefaultSMLTenant {
		t.Fatalf("legacy token tenant = %q, want %q", session.SMLTenant, store.DefaultSMLTenant)
	}
	if session.SMLDataCode != strings.ToUpper(store.DefaultSMLTenant) {
		t.Fatalf("legacy token data code = %q, want default data code", session.SMLDataCode)
	}
	if session.AuthSource != "legacy" {
		t.Fatalf("legacy token auth source = %q, want legacy", session.AuthSource)
	}
}

func TestSessionFromClaimsPreservesSelectedSMLTenant(t *testing.T) {
	server := NewServer(config.Config{
		SMLAuthProvider:  "smlgoh",
		SMLAuthDataGroup: "sml",
	}, nil, nil)

	session := server.sessionFromClaims(auth.Claims{
		SMLProvider:  "smlgoh",
		SMLDataGroup: "sml",
		SMLDataCode:  "AMPACCOUNT",
		SMLTenant:    "AMP-ACCOUNT",
		AuthSource:   "sml",
	})
	if session.SMLTenant != "amp_account" {
		t.Fatalf("selected tenant = %q, want normalized amp_account", session.SMLTenant)
	}
	if session.SMLDataCode != "AMPACCOUNT" {
		t.Fatalf("data code = %q, want selected data code", session.SMLDataCode)
	}
	if session.AuthSource != "sml" {
		t.Fatalf("auth source = %q, want sml", session.AuthSource)
	}
}
