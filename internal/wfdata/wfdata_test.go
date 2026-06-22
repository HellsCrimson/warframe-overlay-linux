package wfdata

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const itemsJSON = `[
  {"uniqueName":"/Lotus/Powersuits/Necro/NekrosPrime","name":"Nekros Prime","masterable":true,"productCategory":"Suits","type":"Warframe"},
  {"uniqueName":"/Lotus/Weapons/Tenno/LongGuns/PrimeTigris/PrimeTigris","name":"Tigris Prime","masterable":true,"productCategory":"LongGuns","type":"Weapon"},
  {"uniqueName":"/Lotus/Types/Items/MiscItems/Ferrite","name":"Ferrite","masterable":false,"productCategory":"","type":"Resource"}
]`

func newTestDB(t *testing.T) *DB {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(itemsJSON))
	}))
	t.Cleanup(srv.Close)
	old := itemsURLVar
	itemsURLVar = srv.URL
	t.Cleanup(func() { itemsURLVar = old })

	db, err := Load(Options{CacheDir: t.TempDir(), TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestResolveName(t *testing.T) {
	db := newTestDB(t)
	// The internal "Prime first" ordering resolves to the canonical name.
	if n, ok := db.Name("/Lotus/Weapons/Tenno/LongGuns/PrimeTigris/PrimeTigris"); !ok || n != "Tigris Prime" {
		t.Errorf("resolve PrimeTigris = %q,%v; want Tigris Prime", n, ok)
	}
	if n, ok := db.Name("/Lotus/Powersuits/Necro/NekrosPrime"); !ok || n != "Nekros Prime" {
		t.Errorf("resolve NekrosPrime = %q,%v; want Nekros Prime", n, ok)
	}
	if _, ok := db.Name("/Lotus/Unknown/Thing"); ok {
		t.Error("unknown type should not resolve")
	}
}

func TestMasterableFilter(t *testing.T) {
	db := newTestDB(t)
	m := db.Masterable()
	if len(m) != 2 {
		t.Fatalf("masterable count = %d, want 2", len(m))
	}
	for _, it := range m {
		if !it.Masterable {
			t.Errorf("%s not masterable", it.Name)
		}
	}
}
