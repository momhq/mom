package projection

import "testing"

// TestDecideFoldOutcome table-tests the fold's core safety decision against
// the documented incident cases — each row is a failure mode that has
// actually destroyed or poisoned a vault.
func TestDecideFoldOutcome(t *testing.T) {
	cases := []struct {
		name          string
		rebuild       bool
		head          uint64
		res           FoldResult
		wantWatermark uint64
		wantPrune     bool
		wantPartial   bool
	}{
		{
			// The fallout-overseer wipe: rebuild against a dead harness — the
			// first window fails, synthesis returns nothing. The watermark must
			// stay 0 (NOT be promoted to head) and pruning must not fire.
			name:    "rebuild fails at first window — no prune, watermark stays 0",
			rebuild: true, head: 1000,
			res:           FoldResult{FoldedThrough: 0},
			wantWatermark: 0, wantPrune: false, wantPartial: true,
		},
		{
			// Partial fold: watermark stops at the last consecutive success so
			// the next fold retries the remaining window.
			name:    "partial fold keeps honest watermark",
			rebuild: false, head: 1000,
			res:           FoldResult{FoldedThrough: 400},
			wantWatermark: 400, wantPrune: false, wantPartial: true,
		},
		{
			// L0 done but L1/L2 aborted (usage limit): a rebuild must NOT
			// prune — concept files were not regenerated into the fresh set.
			name:    "rebuild with pending synthesis — no prune",
			rebuild: true, head: 1000,
			res:           FoldResult{FoldedThrough: 1000, PendingSynthesis: true},
			wantWatermark: 1000, wantPrune: false, wantPartial: false,
		},
		{
			// The only pruning case: a rebuild that fully completed.
			name:    "complete rebuild prunes",
			rebuild: true, head: 1000,
			res:           FoldResult{FoldedThrough: 1000},
			wantWatermark: 1000, wantPrune: true, wantPartial: false,
		},
		{
			// A complete incremental fold never prunes.
			name:    "complete incremental fold never prunes",
			rebuild: false, head: 1000,
			res:           FoldResult{FoldedThrough: 1000},
			wantWatermark: 1000, wantPrune: false, wantPartial: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, prune, partial := decideFoldOutcome(tc.rebuild, tc.head, tc.res)
			if w != tc.wantWatermark || prune != tc.wantPrune || partial != tc.wantPartial {
				t.Errorf("got (watermark=%d prune=%v partial=%v), want (%d %v %v)",
					w, prune, partial, tc.wantWatermark, tc.wantPrune, tc.wantPartial)
			}
		})
	}
}
