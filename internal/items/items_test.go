package items

import "testing"

func TestScaling(t *testing.T) {
	if got := Scaling(1920, 1080); got != 1.0 {
		t.Errorf("Scaling(1920,1080) = %v, want 1.0", got)
	}
	if got := Scaling(2560, 1440); got != 1440.0/1080.0 {
		t.Errorf("Scaling(2560,1440) = %v, want %v", got, 1440.0/1080.0)
	}
}

func TestRewardColumnsCountAndBounds(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4} {
		cols := RewardColumns(2560, 1440, n)
		if len(cols) != n {
			t.Fatalf("RewardColumns n=%d returned %d cols", n, len(cols))
		}
		for i, c := range cols {
			if c.Min.X < 0 || c.Max.X > 2560 || c.Min.Y < 0 || c.Max.Y > 1440 {
				t.Errorf("n=%d col %d out of frame: %v", n, i, c)
			}
			if c.Dx() <= 0 || c.Dy() <= 0 {
				t.Errorf("n=%d col %d empty: %v", n, i, c)
			}
		}
	}
}

func TestRewardColumnsCentered(t *testing.T) {
	strip := RewardStrip(1920, 1080)
	cols := RewardColumns(1920, 1080, 2)
	leftGap := cols[0].Min.X - strip.Min.X
	rightGap := strip.Max.X - cols[len(cols)-1].Max.X
	if abs(leftGap-rightGap) > 2 {
		t.Errorf("columns not centered: leftGap=%d rightGap=%d", leftGap, rightGap)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
