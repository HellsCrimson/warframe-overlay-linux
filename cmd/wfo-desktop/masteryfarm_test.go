package main

import (
	"testing"

	"warframe-overlay-linux/internal/mastery"
)

func TestPartKeyMatchesRewardAndComponent(t *testing.T) {
	// A recipe component ("…Neuroptics") and its relic reward ("…Neuroptics
	// Blueprint") must produce the same key, regardless of word order.
	if a, b := partKey("Rhino Prime Neuroptics"), partKey("Rhino Prime Neuroptics Blueprint"); a != b {
		t.Errorf("component vs reward key mismatch: %q != %q", a, b)
	}
	if a, b := partKey("Boltor Prime Receiver"), partKey("Receiver Boltor Prime"); a != b {
		t.Errorf("order-dependent key: %q != %q", a, b)
	}
	if partKey("Forma Blueprint") != "forma" {
		t.Errorf("partKey dropped wrong tokens: %q", partKey("Forma Blueprint"))
	}
}

func TestAnnotateRelicScore(t *testing.T) {
	it := mastery.Item{
		Name: "Mesa Prime",
		Parts: []mastery.Part{
			{Query: "Mesa Prime Chassis", Need: 1, Have: 0},  // missing
			{Query: "Mesa Prime Systems", Need: 1, Have: 0},  // missing
			{Query: "Mesa Prime Blueprint", Need: 1, Have: 1}, // owned, ignored
		},
	}
	idx := relicIndex{
		partKey("Mesa Prime Chassis"): {
			{unique: "R1", chance: 2, count: 6},
			{unique: "R2", chance: 10, count: 2},
		},
		// R1 also drops Systems: its count must not be tallied twice.
		partKey("Mesa Prime Systems"): {
			{unique: "R1", chance: 5, count: 6},
		},
	}

	got := annotate(it, MasteryItem{Name: it.Name}, nil, idx)

	wantScore := 2.0/100*6 + 10.0/100*2 + 5.0/100*6 // 0.62
	if got.RelicScore < wantScore-1e-9 || got.RelicScore > wantScore+1e-9 {
		t.Errorf("RelicScore = %v, want %v", got.RelicScore, wantScore)
	}
	if got.RelicCount != 8 { // R1 (6) counted once + R2 (2)
		t.Errorf("RelicCount = %d, want 8", got.RelicCount)
	}
	if got.BestChance != 10 {
		t.Errorf("BestChance = %v, want 10", got.BestChance)
	}
	if !got.CostKnown || got.BuildCost != 0 {
		t.Errorf("with nil prices: CostKnown=%v BuildCost=%d, want true/0", got.CostKnown, got.BuildCost)
	}
}

func TestSortMastery(t *testing.T) {
	items := []MasteryItem{
		{Name: "a", BuildCost: 50, CostKnown: true, RelicScore: 0.1},
		{Name: "b", BuildCost: 0, CostKnown: true, RelicScore: 0.0},
		{Name: "c", CostKnown: false, RelicScore: 0.62},
	}

	cost := append([]MasteryItem(nil), items...)
	sortMastery(cost, sortCost)
	if got := []string{cost[0].Name, cost[1].Name, cost[2].Name}; got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Errorf("cost order = %v, want [b a c] (unknown price last)", got)
	}

	rel := append([]MasteryItem(nil), items...)
	sortMastery(rel, sortRelics)
	if got := []string{rel[0].Name, rel[1].Name, rel[2].Name}; got[0] != "c" || got[1] != "a" || got[2] != "b" {
		t.Errorf("relics order = %v, want [c a b] (highest score first)", got)
	}

	// "next" leaves the (already best-next) order untouched.
	next := append([]MasteryItem(nil), items...)
	sortMastery(next, sortNext)
	if next[0].Name != "a" {
		t.Errorf("next order changed: %v", next[0].Name)
	}
}
