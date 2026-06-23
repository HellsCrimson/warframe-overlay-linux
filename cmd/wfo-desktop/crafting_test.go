package main

import (
	"testing"

	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/wfdata"
)

func findChild(n CraftNode, name string) (CraftNode, bool) {
	for _, c := range n.Children {
		if c.Name == name {
			return c, true
		}
	}
	return CraftNode{}, false
}

func TestGetCraftingTreeRecursesIntoResources(t *testing.T) {
	items := []wfdata.Item{
		{UniqueName: "/excalibur", Name: "Excalibur", Masterable: true, Components: []wfdata.Component{
			{Name: "Chassis", UniqueName: "/chassis", ItemCount: 1},
		}},
		{UniqueName: "/chassis", Name: "Excalibur Chassis", Components: []wfdata.Component{
			{Name: "Ferrite", UniqueName: "/Lotus/Types/Items/MiscItems/Ferrite", ItemCount: 500},
			{Name: "Rubedo", UniqueName: "/Lotus/Types/Items/MiscItems/Rubedo", ItemCount: 300},
		}},
	}
	inv, err := inventory.Parse([]byte(`{"MiscItems":[
		{"ItemType":"/Lotus/Types/Items/MiscItems/Ferrite","ItemCount":600},
		{"ItemType":"/Lotus/Types/Items/MiscItems/Rubedo","ItemCount":100}]}`))
	if err != nil {
		t.Fatalf("parse inventory: %v", err)
	}
	s := &Service{names: wfdata.New(items), inv: inv}

	tree := s.GetCraftingTree("Excalibur")
	if tree.Name != "Excalibur" || len(tree.Children) != 1 {
		t.Fatalf("root = %q with %d children, want Excalibur/1", tree.Name, len(tree.Children))
	}
	chassis := tree.Children[0]
	if chassis.Name != "Excalibur Chassis" || len(chassis.Children) != 2 {
		t.Fatalf("chassis = %q with %d children, want Excalibur Chassis/2", chassis.Name, len(chassis.Children))
	}

	ferrite, ok := findChild(chassis, "Ferrite")
	if !ok {
		t.Fatal("missing Ferrite leaf")
	}
	if !ferrite.IsResource || ferrite.Need != 500 || ferrite.Have != 600 || !ferrite.Enough {
		t.Errorf("Ferrite = %+v, want resource need500 have600 enough", ferrite)
	}
	rubedo, _ := findChild(chassis, "Rubedo")
	if rubedo.Need != 300 || rubedo.Have != 100 || rubedo.Enough {
		t.Errorf("Rubedo = %+v, want need300 have100 not-enough", rubedo)
	}
}

func TestGetCraftingTreePartLeaf(t *testing.T) {
	// A prime part component with no sub-recipe is a leaf, named by its parent and
	// counted via loose part matching (owned "…Blueprint" satisfies "…Chassis").
	items := []wfdata.Item{
		{UniqueName: "/mesaprime", Name: "Mesa Prime", Masterable: true, Components: []wfdata.Component{
			{Name: "Chassis", UniqueName: "/Lotus/Recipes/MesaPrimeChassis", ItemCount: 1},
		}},
	}
	inv, err := inventory.Parse([]byte(`{"Recipes":[
		{"ItemType":"/Lotus/Types/Recipes/MesaPrimeChassisBlueprint","ItemCount":1}]}`))
	if err != nil {
		t.Fatalf("parse inventory: %v", err)
	}
	s := &Service{names: wfdata.New(items), inv: inv}

	tree := s.GetCraftingTree("Mesa Prime")
	chassis, ok := findChild(tree, "Mesa Prime Chassis")
	if !ok {
		t.Fatalf("missing Mesa Prime Chassis leaf in %+v", tree.Children)
	}
	if chassis.IsResource || chassis.Have != 1 || !chassis.Enough {
		t.Errorf("Chassis leaf = %+v, want part have1 enough", chassis)
	}
}
