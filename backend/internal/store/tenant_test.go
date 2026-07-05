package store

import (
	"context"
	"strings"
	"testing"
)

func TestWithSMLTenantNormalizesTenantForScopedQueries(t *testing.T) {
	ctx := WithSMLTenant(context.Background(), "AMP-ACCOUNT")
	got := tenantFilterValue(ctx)
	if got != "amp_account" {
		t.Fatalf("tenantFilterValue() = %q, want normalized amp_account", got)
	}
}

func TestTenantSQLPredicateUsesExplicitTenantPlaceholder(t *testing.T) {
	predicate := tenantSQLPredicate("d", "ignored", 3)
	if !strings.Contains(predicate, "$3 = ''") || !strings.Contains(predicate, "d.sml_tenant = $3") {
		t.Fatalf("tenant predicate must bind the requested alias and placeholder, got %q", predicate)
	}
}
