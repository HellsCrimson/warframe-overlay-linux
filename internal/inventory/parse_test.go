package inventory

import "testing"

func TestFoundryCountsTowardParts(t *testing.T) {
	// Ash Chassis is being crafted (its blueprint was consumed from Recipes into
	// PendingRecipes); Ash Systems sits owned as a built component. Both must count
	// as parts the player effectively has, even though neither is a prime.
	raw := []byte(`{
		"MiscItems": [
			{"ItemType":"/Lotus/Types/Recipes/WarframeRecipes/AshSystemsComponent","ItemCount":1}
		],
		"PendingRecipes": [
			{
				"ItemType":"/Lotus/Types/Recipes/WarframeRecipes/AshChassisBlueprint",
				"CompletionDate":{"$date":{"$numberLong":"1782085762000"}},
				"ItemId":{"$oid":"6a37cfc22b0458c8c504d6ba"}
			}
		]
	}`)
	inv, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := inv.PartCount("Ash Chassis"); got != 1 {
		t.Errorf("Ash Chassis (building) part count = %d, want 1", got)
	}
	if got := inv.PartCount("Ash Systems"); got != 1 {
		t.Errorf("Ash Systems (built) part count = %d, want 1", got)
	}
	f := inv.Foundry()
	if len(f) != 1 {
		t.Fatalf("foundry builds = %d, want 1", len(f))
	}
	if f[0].Name != "Ash Chassis Blueprint" {
		t.Errorf("build name = %q, want %q", f[0].Name, "Ash Chassis Blueprint")
	}
	if f[0].ID != "6a37cfc22b0458c8c504d6ba" {
		t.Errorf("build id = %q", f[0].ID)
	}
	if got := f[0].Completion.UnixMilli(); got != 1782085762000 {
		t.Errorf("completion = %d, want 1782085762000", got)
	}
}

func TestSignatureOrderAndPunctuation(t *testing.T) {
	// Display name and internal-type tokenizations must collide.
	cases := [][2]string{
		{signature([]string{"Bronco", "Prime", "Receiver"}), signature([]string{"Bronco", "Prime", "Receiver"})},
	}
	_ = cases

	// Word order differs between "Tigris Prime Receiver" and "PrimeTigrisReceiver".
	if a, b := signature([]string{"Tigris", "Prime", "Receiver"}), signature([]string{"Prime", "Tigris", "Receiver"}); a != b {
		t.Errorf("order-independent signature failed: %q != %q", a, b)
	}
	// "&" punctuation is dropped: "Cobra & Crane Prime Handle" == "CobraCranePrimeHandle".
	disp := signature([]string{"Cobra", "&", "Crane", "Prime", "Handle"})
	internal := signature([]string{"Cobra", "Crane", "Prime", "Handle"})
	if disp != internal {
		t.Errorf("ampersand not normalized: %q != %q", disp, internal)
	}
}

func TestParseAndOwned(t *testing.T) {
	raw := []byte(`{
		"MiscItems": [
			{"ItemType":"/Lotus/Types/Recipes/Weapons/WeaponParts/BroncoPrimeReceiver","ItemCount":1},
			{"ItemType":"/Lotus/Types/Recipes/Weapons/WeaponParts/CobraCranePrimeHandle","ItemCount":2},
			{"ItemType":"/Lotus/Types/Items/MiscItems/Ferrite","ItemCount":99999}
		],
		"Recipes": [
			{"ItemType":"/Lotus/Types/Recipes/WarframeRecipes/SevagothPrimeBlueprint","ItemCount":3}
		]
	}`)
	inv, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := inv.Owned("Bronco Prime Receiver"); got != 1 {
		t.Errorf("Bronco Prime Receiver owned = %d, want 1", got)
	}
	if got := inv.Owned("Cobra & Crane Prime Handle"); got != 2 {
		t.Errorf("Cobra & Crane owned = %d, want 2", got)
	}
	if got := inv.Owned("Sevagoth Prime Blueprint"); got != 3 {
		t.Errorf("Sevagoth Prime Blueprint owned = %d, want 3", got)
	}
	// Non-prime junk must not be counted as a prime part.
	if got := inv.Owned("Ferrite"); got != 0 {
		t.Errorf("Ferrite owned = %d, want 0", got)
	}
	if got := inv.Owned("Mesa Prime Systems Blueprint"); got != 0 {
		t.Errorf("unowned item = %d, want 0", got)
	}
}

func TestParseRelics(t *testing.T) {
	raw := []byte(`{
		"MiscItems": [
			{"ItemType":"/Lotus/Types/Game/Projections/T1VoidProjectionDBronze","ItemCount":6},
			{"ItemType":"/Lotus/Types/Game/Projections/T3VoidProjectionMGold","ItemCount":2},
			{"ItemType":"/Lotus/Types/Recipes/Weapons/WeaponParts/BroncoPrimeReceiver","ItemCount":1}
		]
	}`)
	inv, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	rl := inv.Relics()
	if len(rl) != 2 {
		t.Fatalf("relic types = %d, want 2", len(rl))
	}
	if got := rl["/Lotus/Types/Game/Projections/T1VoidProjectionDBronze"]; got != 6 {
		t.Errorf("Lith D1 Intact count = %d, want 6", got)
	}
	if got := rl["/Lotus/Types/Game/Projections/T3VoidProjectionMGold"]; got != 2 {
		t.Errorf("Neo M Radiant count = %d, want 2", got)
	}
	// Relics must not leak into the prime-part counts, and parts not into relics.
	if got := inv.Owned("Bronco Prime Receiver"); got != 1 {
		t.Errorf("Bronco Prime Receiver owned = %d, want 1", got)
	}
}
