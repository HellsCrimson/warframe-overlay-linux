package main

import (
	"testing"

	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/wfdata"
)

func TestBuildSetIndexResolvesRewardToSet(t *testing.T) {
	items := []wfdata.Item{{
		UniqueName: "/mesa", Name: "Mesa Prime", Masterable: true, ProductCategory: "Suits",
		Components: []wfdata.Component{
			{Name: "Chassis", UniqueName: "/r/MesaPrimeChassis", ItemCount: 1},
			{Name: "Systems", UniqueName: "/r/MesaPrimeSystems", ItemCount: 1},
		},
	}}
	// Own the chassis blueprint; XP enough for rank 30 (Suits: 1000×30²) → mastered.
	inv, err := inventory.Parse([]byte(`{
		"Recipes":[{"ItemType":"/x/MesaPrimeChassisBlueprint","ItemCount":1}],
		"XPInfo":[{"ItemType":"/mesa","XP":900000}]}`))
	if err != nil {
		t.Fatalf("parse inventory: %v", err)
	}

	idx := buildSetIndex(wfdata.New(items), inv)

	// A relic reward drop name resolves to its parent set via the loose part key.
	e, ok := idx[partKey("Mesa Prime Chassis Blueprint")]
	if !ok {
		t.Fatalf("reward did not resolve to a set; keys = %v", idx)
	}
	if e.SetName != "Mesa Prime" || !e.Mastered || !e.Crafted {
		t.Errorf("entry = %+v, want Mesa Prime mastered+crafted", e)
	}
	var chassisOwned, systemsOwned, found int
	for _, p := range e.Parts {
		switch p.Name {
		case "Mesa Prime Chassis":
			found++
			if p.Owned {
				chassisOwned++
			}
		case "Mesa Prime Systems":
			found++
			if p.Owned {
				systemsOwned++
			}
		}
	}
	if found != 2 || chassisOwned != 1 || systemsOwned != 0 {
		t.Errorf("parts = %+v, want chassis owned, systems not", e.Parts)
	}
}
