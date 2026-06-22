package fabric

import "testing"

// TestGatewayForSelectsTenantIdentity pins F14's per-tenant credential routing:
// a tenant with its own configured identity transacts under it; everyone else
// (including the empty/unauthenticated tenant) uses the default identity. This
// keeps the change backward-compatible — with no tenant identities configured,
// every call resolves to the default gateway, exactly as before.
func TestGatewayForSelectsTenantIdentity(t *testing.T) {
	def := &gatewayConn{}
	acme := &gatewayConn{}
	b := &gatewayBackend{
		defaultGw: def,
		tenantGws: map[string]*gatewayConn{"acme": acme},
	}

	if got := b.gatewayFor("acme"); got != acme {
		t.Errorf("gatewayFor(acme) = %p, want the acme identity %p", got, acme)
	}
	if got := b.gatewayFor("globex"); got != def {
		t.Errorf("gatewayFor(globex) = %p, want the default identity %p", got, def)
	}
	if got := b.gatewayFor(""); got != def {
		t.Errorf("gatewayFor(\"\") = %p, want the default identity %p", got, def)
	}
}
