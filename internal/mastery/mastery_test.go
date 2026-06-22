package mastery

import "testing"

func TestRankPerClass(t *testing.T) {
	// Warframe: 1000×rank² => rank 30 at 900,000 affinity.
	if r := Rank(900000, "Suits"); r != 30 {
		t.Errorf("warframe rank at 900k = %d, want 30", r)
	}
	if r := Rank(1100000, "Suits"); r != 30 {
		t.Errorf("warframe rank past max should cap at 30, got %d", r)
	}
	if r := Rank(250000, "Suits"); r != 15 { // sqrt(250000/1000)=15.8 -> 15
		t.Errorf("warframe rank at 250k = %d, want 15", r)
	}
	// Weapon: 500×rank² => rank 30 at 450,000.
	if r := Rank(450000, "LongGuns"); r != 30 {
		t.Errorf("weapon rank at 450k = %d, want 30", r)
	}
	// Necramech caps at 40.
	if m := MaxRank("MechSuits"); m != 40 {
		t.Errorf("necramech max rank = %d, want 40", m)
	}
	if r := Rank(900000, "MechSuits"); r != 30 { // 1000×rank², sqrt(900)=30, not yet 40
		t.Errorf("necramech rank at 900k = %d, want 30", r)
	}
}

func TestRankZero(t *testing.T) {
	if r := Rank(0, "Suits"); r != 0 {
		t.Errorf("rank at 0 xp = %d, want 0", r)
	}
}
