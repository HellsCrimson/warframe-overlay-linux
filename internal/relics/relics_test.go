package relics

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// itemsJSON mimics the warframestat items endpoint: relics carry a rewards list,
// non-relics (and relics without rewards) must be ignored.
const itemsJSON = `[
  {"uniqueName":"/Lotus/Types/Game/Projections/T1VoidProjectionDBronze","name":"Lith D1 Intact","type":"Relic","rewards":[
    {"rarity":"Rare","chance":2,"item":{"name":"Mesa Prime Chassis Blueprint"}},
    {"rarity":"Common","chance":25.33,"item":{"name":"Boltor Prime Receiver"}}
  ]},
  {"uniqueName":"/Lotus/Types/Game/Projections/T1VoidProjectionDGold","name":"Lith D1 Radiant","type":"Relic","rewards":[
    {"rarity":"Rare","chance":10,"item":{"name":"Mesa Prime Chassis Blueprint"}}
  ]},
  {"uniqueName":"/Lotus/Powersuits/Mag/Mag","name":"Mag","type":"Warframe"},
  {"uniqueName":"/Lotus/Types/Game/Projections/EmptyRelic","name":"Empty","type":"Relic","rewards":[]}
]`

func newTestTables(t *testing.T, cacheDir string) *Tables {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(itemsJSON))
	}))
	t.Cleanup(srv.Close)
	old := itemsURLVar
	itemsURLVar = srv.URL
	t.Cleanup(func() { itemsURLVar = old })

	tables, err := Load(Options{CacheDir: cacheDir, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

func TestLoadParsesRelics(t *testing.T) {
	tables := newTestTables(t, t.TempDir())
	// Only the two relics with rewards are kept (warframe + empty relic dropped).
	if tables.Len() != 2 {
		t.Fatalf("relic count = %d, want 2", tables.Len())
	}
	intact := tables.Rewards("/Lotus/Types/Game/Projections/T1VoidProjectionDBronze")
	if len(intact) != 2 {
		t.Fatalf("intact rewards = %d, want 2", len(intact))
	}
	if intact[0].Part != "Mesa Prime Chassis Blueprint" || intact[0].Chance != 2 {
		t.Errorf("intact[0] = %+v", intact[0])
	}
	// The refined variant carries its own (higher) chances.
	if rad := tables.Rewards("/Lotus/Types/Game/Projections/T1VoidProjectionDGold"); len(rad) != 1 || rad[0].Chance != 10 {
		t.Errorf("radiant rewards = %+v, want one at 10%%", rad)
	}
	if got := tables.Rewards("/nope"); got != nil {
		t.Errorf("unknown relic = %+v, want nil", got)
	}

	// Name parsing splits era/code from refinement.
	r, ok := tables.Get("/Lotus/Types/Game/Projections/T1VoidProjectionDBronze")
	if !ok || r.Name != "Lith D1" || r.Era != "Lith" || r.Refinement != "Intact" {
		t.Errorf("Get intact = %+v ok=%v; want Lith D1 / Lith / Intact", r, ok)
	}
	if r, _ := tables.Get("/Lotus/Types/Game/Projections/T1VoidProjectionDGold"); r.Refinement != "Radiant" {
		t.Errorf("Get radiant refinement = %q, want Radiant", r.Refinement)
	}
}

func TestLoadServesStaleOnFailure(t *testing.T) {
	dir := t.TempDir()
	newTestTables(t, dir) // populates the cache

	// Point at a dead server; with a zero TTL the cache is stale, but Load must
	// still serve it rather than failing.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	old := itemsURLVar
	itemsURLVar = srv.URL
	defer func() { itemsURLVar = old }()

	tables, err := Load(Options{CacheDir: dir, TTL: 0})
	if err != nil {
		t.Fatalf("expected stale-served tables, got err: %v", err)
	}
	if tables.Len() != 2 {
		t.Errorf("stale relic count = %d, want 2", tables.Len())
	}
	if _, err := os.Stat(filepath.Join(dir, "relics.json")); err != nil {
		t.Errorf("cache file missing: %v", err)
	}
}
