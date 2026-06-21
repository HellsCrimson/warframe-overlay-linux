package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const pricesJSON = `[
  {"name":"Mirage Prime Neuroptics", "custom_avg": 12.5},
  {"name":"Mirage Prime Neuroptics Blueprint", "custom_avg": 12.5},
  {"name":"Mirage Prime Systems Blueprint", "custom_avg": 4.0},
  {"name":"Akbolto Prime Link", "custom_avg": 30.0},
  {"name":"Forma Blueprint", "custom_avg": 0.0}
]`

const filteredJSON = `{
  "eqmt": {
    "Mirage Prime": {
      "type": "Warframes",
      "parts": {
        "Mirage Prime Neuroptics": {"ducats": 45},
        "Mirage Prime Systems": {"ducats": 15}
      }
    },
    "Akbolto Prime": {
      "type": "Pistols",
      "parts": {
        "Akbolto Prime Link": {"ducats": 100}
      }
    }
  },
  "ignored_items": {"Forma Blueprint": {}}
}`

func newTestDB(t *testing.T) *Database {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/prices/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(pricesJSON))
	})
	mux.HandleFunc("/filtered_items/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(filteredJSON))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Point the package URLs at the test server.
	oldP, oldF := pricesURLVar, filteredItemsURLVar
	pricesURLVar = srv.URL + "/prices/"
	filteredItemsURLVar = srv.URL + "/filtered_items/"
	t.Cleanup(func() { pricesURLVar, filteredItemsURLVar = oldP, oldF })

	d, err := Load(Options{CacheDir: t.TempDir(), TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestComponentDropNameAndDucats(t *testing.T) {
	d := newTestDB(t)

	// Warframe component "Systems" drops as "<...> Systems Blueprint".
	got := d.Match("Mirage Prime Systems Blueprint")
	if got == nil {
		t.Fatal("expected match for Mirage Prime Systems Blueprint")
	}
	if got.Ducats != 15 {
		t.Errorf("ducats = %d, want 15", got.Ducats)
	}
	if got.Platinum != 4.0 {
		t.Errorf("platinum = %v, want 4.0", got.Platinum)
	}
}

func TestIgnoredItemsAreMatchable(t *testing.T) {
	d := newTestDB(t)
	// "Forma Blueprint" comes from ignored_items; it must still resolve (as a
	// worthless reward) so the screen isn't left with an unmatched column.
	got := d.Match("Forma Blueprint")
	if got == nil || got.Name != "Forma Blueprint" {
		t.Fatalf("expected Forma Blueprint, got %+v", got)
	}
	if got.Platinum != 0 || got.Ducats != 0 {
		t.Errorf("Forma should be zero-valued, got %+v", got)
	}
}

func TestMatchThroughSurroundingNoise(t *testing.T) {
	d := newTestDB(t)
	// Real OCR wraps the name in icon/border garbage; containment should still
	// pull out the right item.
	if got := d.Match("4 - U 2 X Forma Blueprint i W"); got == nil || got.Name != "Forma Blueprint" {
		t.Fatalf("noise-surrounded Forma failed: %+v", got)
	}
	if got := d.Match("ORI w Akbolto Prime Link p Y"); got == nil || got.Name != "Akbolto Prime Link" {
		t.Fatalf("noise-surrounded Akbolto failed: %+v", got)
	}
}

func TestFuzzyMatchToleratesOCRError(t *testing.T) {
	d := newTestDB(t)
	// OCR commonly confuses characters; this should still resolve.
	got := d.Match("Akbo1to Prime L1nk") // 1-for-l/i substitutions
	if got == nil || got.Name != "Akbolto Prime Link" {
		t.Fatalf("fuzzy match failed, got %+v", got)
	}
}

func TestNoMatchForGarbage(t *testing.T) {
	d := newTestDB(t)
	if got := d.Match("xqzwooble"); got != nil {
		t.Errorf("expected nil for garbage, got %+v", got)
	}
}

// An item not in the DB that merely shares a long suffix with a real item (a
// brand-new prime "... Prime Link") must NOT be mispriced as that item.
func TestRejectsSharedSuffixNonMatch(t *testing.T) {
	d := newTestDB(t)
	if got := d.Match("Volnus Prime Link"); got != nil {
		t.Errorf("expected nil for unknown '... Prime Link', got %+v", got)
	}
}

// The live wfinfo prices feed encodes custom_avg as a quoted string; ensure we
// parse it.
func TestStringCustomAvg(t *testing.T) {
	var p priceItem
	if err := json.Unmarshal([]byte(`{"name":"X","custom_avg":"78.6"}`), &p); err != nil {
		t.Fatal(err)
	}
	if float64(p.CustomAvg) != 78.6 {
		t.Errorf("custom_avg = %v, want 78.6", float64(p.CustomAvg))
	}
	// Numeric form must still parse.
	if err := json.Unmarshal([]byte(`{"name":"Y","custom_avg":12.5}`), &p); err != nil {
		t.Fatal(err)
	}
	if float64(p.CustomAvg) != 12.5 {
		t.Errorf("numeric custom_avg = %v, want 12.5", float64(p.CustomAvg))
	}
}
