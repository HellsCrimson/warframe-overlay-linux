package pricing

import (
	"testing"

	"warframe-overlay-linux/internal/db"
)

// fakeDB matching is exercised via db's own tests; here we pass nil and rely on
// pre-set items by constructing Rewards through Evaluate with a stub matcher is
// not possible (Match needs a real Database), so we test scoring directly.

func TestScoreoringPrefersPlatThenDucats(t *testing.T) {
	highPlat := Reward{Item: &db.Item{Platinum: 20, Ducats: 15}}
	highDucat := Reward{Item: &db.Item{Platinum: 2, Ducats: 100}}
	cheap := Reward{Item: &db.Item{Platinum: 1, Ducats: 15}}

	if highPlat.score() <= highDucat.score() {
		t.Errorf("expected plat-heavy reward to outrank ducat-heavy one")
	}
	if highDucat.score() <= cheap.score() {
		t.Errorf("expected ducat tiebreak to lift higher-ducat reward")
	}
}

func TestEvaluateBestIndexAndUnmatched(t *testing.T) {
	// nil database => no items match => all unmatched, best is first.
	res := Evaluate([]string{"a", "b", "c"}, nil)
	if len(res.Rewards) != 3 {
		t.Fatalf("got %d rewards", len(res.Rewards))
	}
	if res.BestIndex != 0 {
		t.Errorf("BestIndex = %d, want 0 (all zero score)", res.BestIndex)
	}
	for _, r := range res.Rewards {
		if r.Item != nil {
			t.Errorf("expected unmatched item for %q", r.OCRName)
		}
		if r.Plat() != 0 || r.Ducats() != 0 {
			t.Errorf("unmatched reward should be zero-valued")
		}
	}
}

func TestEvaluateEmpty(t *testing.T) {
	res := Evaluate(nil, nil)
	if res.BestIndex != -1 {
		t.Errorf("empty result BestIndex = %d, want -1", res.BestIndex)
	}
}
