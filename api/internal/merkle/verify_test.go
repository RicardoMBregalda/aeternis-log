package merkle

import (
	"testing"

	"github.com/RicardoMBregalda/aeternis-log/go-api/internal/models"
)

// TestBuildVerifyResponse covers the F01 decision matrix: when a batch is
// anchored, the on-chain root is authoritative and the locally stored root is
// irrelevant — so tampering BOTH the content and the stored root is still
// detected. When the ledger cannot be consulted, the result falls back to a
// local-consistency check and discloses that via AnchorStatus.
func TestBuildVerifyResponse(t *testing.T) {
	const (
		onChain    = "aaaa"
		recomputed = "bbbb" // != onChain  => content was tampered
	)

	tests := []struct {
		name          string
		stored        string
		recalculated  string
		onChainRoot   string
		anchorStatus  string
		wantValid     bool
		wantIntegrity string
	}{
		{
			name:          "anchored and matches on-chain",
			stored:        onChain,
			recalculated:  onChain,
			onChainRoot:   onChain,
			anchorStatus:  models.AnchorAnchored,
			wantValid:     true,
			wantIntegrity: models.IntegrityValid,
		},
		{
			// The core F01 case: an attacker tampered the content AND rewrote the
			// stored root to match it. Pre-fix this returned VALID; now the
			// on-chain root is the source of truth, so it must be CORRUPTED.
			name:          "anchored but content+stored-root tampered to agree",
			stored:        recomputed, // attacker set stored == recomputed
			recalculated:  recomputed,
			onChainRoot:   onChain,
			anchorStatus:  models.AnchorAnchored,
			wantValid:     false,
			wantIntegrity: models.IntegrityCorrupted,
		},
		{
			name:          "not anchored on the ledger",
			stored:        recomputed,
			recalculated:  recomputed,
			onChainRoot:   "",
			anchorStatus:  models.AnchorUnanchored,
			wantValid:     false,
			wantIntegrity: models.IntegrityUnanchored,
		},
		{
			name:          "ledger unknown, local consistency holds",
			stored:        recomputed,
			recalculated:  recomputed,
			onChainRoot:   "",
			anchorStatus:  models.AnchorUnknown,
			wantValid:     true,
			wantIntegrity: models.IntegrityValid,
		},
		{
			name:          "ledger unknown, local mismatch",
			stored:        onChain,
			recalculated:  recomputed,
			onChainRoot:   "",
			anchorStatus:  models.AnchorUnknown,
			wantValid:     false,
			wantIntegrity: models.IntegrityCorrupted,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildVerifyResponse("batch-1", 3, tc.stored, tc.recalculated, tc.onChainRoot, tc.anchorStatus)

			if got.IsValid != tc.wantValid {
				t.Errorf("IsValid = %v, want %v", got.IsValid, tc.wantValid)
			}
			if got.Integrity != tc.wantIntegrity {
				t.Errorf("Integrity = %q, want %q", got.Integrity, tc.wantIntegrity)
			}
			if got.AnchorStatus != tc.anchorStatus {
				t.Errorf("AnchorStatus = %q, want %q", got.AnchorStatus, tc.anchorStatus)
			}
			if got.OnChainMerkleRoot != tc.onChainRoot {
				t.Errorf("OnChainMerkleRoot = %q, want %q", got.OnChainMerkleRoot, tc.onChainRoot)
			}
		})
	}
}
