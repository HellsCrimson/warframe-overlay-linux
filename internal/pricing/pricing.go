// Package pricing turns matched reward items into a ranked result, choosing the
// "best" pick the same way WFInfo does: by platinum value, with a small ducat
// contribution as a tiebreaker so high-ducat parts win among worthless-plat
// options.
package pricing

import "warframe-overlay-linux/internal/db"

// Reward is one reward choice plus its resolved values. Item is nil when OCR
// produced a name that did not match the database.
type Reward struct {
	OCRName string
	Item    *db.Item
}

// Plat returns the platinum value, or 0 for unmatched items.
func (r Reward) Plat() float64 {
	if r.Item == nil {
		return 0
	}
	return r.Item.Platinum
}

// Ducats returns the ducat value, or 0 for unmatched items.
func (r Reward) Ducats() int {
	if r.Item == nil {
		return 0
	}
	return r.Item.Ducats
}

// score is the ranking value: platinum dominates, with ducats/10 as a minor
// tiebreak (matching wfinfo-ng's max(platinum, ducats/10 + platinum/100)).
func (r Reward) score() float64 {
	plat := r.Plat()
	alt := float64(r.Ducats())/10.0 + plat/100.0
	if alt > plat {
		return alt
	}
	return plat
}

// Result is the evaluated reward set with the index of the best pick.
type Result struct {
	Rewards   []Reward
	BestIndex int // -1 when there are no rewards
}

// Evaluate pairs OCR names with database matches and selects the best reward.
func Evaluate(ocrNames []string, database *db.Database) Result {
	res := Result{BestIndex: -1}
	bestScore := -1.0
	for i, name := range ocrNames {
		r := Reward{OCRName: name}
		if database != nil {
			r.Item = database.Match(name)
		}
		res.Rewards = append(res.Rewards, r)
		if s := r.score(); s > bestScore {
			bestScore = s
			res.BestIndex = i
		}
	}
	return res
}
