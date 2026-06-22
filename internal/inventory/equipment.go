package inventory

import "strings"

// OwnedItem is a single owned piece of equipment.
type OwnedItem struct {
	Name string // prettified display name
	Type string // raw /Lotus/... ItemType
	XP   int    // accumulated affinity (mastery progress)
}

// Category groups owned equipment of one kind (Warframes, Primary, …).
type Category struct {
	Name  string
	Items []OwnedItem
}

type equipEntry struct {
	ItemType string `json:"ItemType"`
	XP       int    `json:"XP"`
}

// equipJSON decodes just the mastery-bearing equipment lists from the inventory
// response (the response has ~200 mixed-type keys, so we name only these).
type equipJSON struct {
	Suits           []equipEntry `json:"Suits"`
	LongGuns        []equipEntry `json:"LongGuns"`
	Pistols         []equipEntry `json:"Pistols"`
	Melee           []equipEntry `json:"Melee"`
	SpaceSuits      []equipEntry `json:"SpaceSuits"`
	SpaceGuns       []equipEntry `json:"SpaceGuns"`
	SpaceMelee      []equipEntry `json:"SpaceMelee"`
	Sentinels       []equipEntry `json:"Sentinels"`
	SentinelWeapons []equipEntry `json:"SentinelWeapons"`
	MoaPets         []equipEntry `json:"MoaPets"`
	KubrowPets      []equipEntry `json:"KubrowPets"`
	MechSuits       []equipEntry `json:"MechSuits"`
	OperatorAmps    []equipEntry `json:"OperatorAmps"`
	Hoverboards     []equipEntry `json:"Hoverboards"`
}

// buildCategories turns the decoded equipment lists into ordered, display-ready
// categories.
func buildCategories(eq equipJSON) []Category {
	ordered := []struct {
		label   string
		entries []equipEntry
	}{
		{"Warframes", eq.Suits},
		{"Primary", eq.LongGuns},
		{"Secondary", eq.Pistols},
		{"Melee", eq.Melee},
		{"Archwing", eq.SpaceSuits},
		{"Arch-Gun", eq.SpaceGuns},
		{"Arch-Melee", eq.SpaceMelee},
		{"Sentinels", eq.Sentinels},
		{"Sentinel Weapons", eq.SentinelWeapons},
		{"MOA Companions", eq.MoaPets},
		{"Beast Companions", eq.KubrowPets},
		{"Necramechs", eq.MechSuits},
		{"Amps", eq.OperatorAmps},
		{"K-Drives", eq.Hoverboards},
	}
	var cats []Category
	for _, o := range ordered {
		if len(o.entries) == 0 {
			continue
		}
		items := make([]OwnedItem, 0, len(o.entries))
		for _, e := range o.entries {
			if e.ItemType == "" {
				continue
			}
			items = append(items, OwnedItem{
				Name: prettifyLeaf(e.ItemType),
				Type: e.ItemType,
				XP:   e.XP,
			})
		}
		cats = append(cats, Category{Name: o.label, Items: items})
	}
	return cats
}

// prettifyLeaf turns the last path segment of an ItemType into a spaced display
// name, e.g. "/Lotus/.../NekrosPrime" -> "Nekros Prime". Word resolution is
// approximate (the internal type ordering differs from in-game display for some
// items); a proper name database can refine this later.
func prettifyLeaf(itemType string) string {
	leaf := itemType[strings.LastIndexByte(itemType, '/')+1:]
	return strings.Join(pascalRe.FindAllString(leaf, -1), " ")
}
