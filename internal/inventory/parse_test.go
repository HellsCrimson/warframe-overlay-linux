package inventory

import "testing"

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
