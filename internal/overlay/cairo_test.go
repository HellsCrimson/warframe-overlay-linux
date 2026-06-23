package overlay

import (
	"strings"
	"testing"
)

func TestMarkupMasteredAndSetList(t *testing.T) {
	l := Label{
		Name: "Mesa Prime Chassis Blueprint", Price: "15p · 20d", Mastered: true,
		OwnedKnown: true, Owned: 1, SetName: "Mesa Prime",
		SetParts: []SetPart{
			{Name: "Mesa Prime Chassis", Owned: true},
			{Name: "Mesa Prime Systems", Owned: false},
		},
	}
	m := l.markup()
	if !strings.Contains(m, "✓ Mastered") {
		t.Errorf("missing mastered status line:\n%s", m)
	}
	if !strings.Contains(m, "Mesa Prime set") {
		t.Errorf("missing set header:\n%s", m)
	}
	// Set parts are shortened (prefix stripped) and marked owned/missing.
	if !strings.Contains(m, "✓ Chassis") || !strings.Contains(m, "· Systems") {
		t.Errorf("set checklist wrong:\n%s", m)
	}
}

func TestMarkupNewWhenUnowned(t *testing.T) {
	l := Label{Name: "Forma Blueprint", Price: "0p · 0d", OwnedKnown: true, Owned: 0}
	if m := l.markup(); !strings.Contains(m, "✦ NEW") {
		t.Errorf("missing NEW marker:\n%s", m)
	}
}
