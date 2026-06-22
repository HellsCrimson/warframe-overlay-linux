package trades

import (
	"strings"
	"testing"
)

// Real EE.log excerpt of a completed trade (buying Alternox Prime Stock for 7p).
const sample = `809.248 Sys [Info]: Created /Lotus/Interface/Dialog.swf
809.248 Script [Info]: Dialog.lua: Dialog::CreateOkCancel(description=Are you sure you want to accept this trade? You are offering:

Platinum x 7



and will receive from bog0bog the following:

Alternox Prime Stock, title= leftItem=/Menu/Confirm_Item_Ok, rightItem=/Menu/Confirm_Item_Cancel)
812.525 Script [Info]: Dialog.lua: SendResult_MENU_SELECT()
812.525 Script [Info]: Dialog.lua: Dialog::SendResult(4)
813.445 Sys [Info]: Created /Lotus/Interface/Dialog.swf
813.445 Script [Info]: Dialog.lua: Dialog::CreateOk(description=The trade was successful!, title= leftItem=/Menu/Confirm_Item_Ok)
813.869 Script [Info]: Dialog.lua: SendResult_MENU_SELECT()`

func feed(p *Parser, s string) []*Trade {
	var got []*Trade
	for _, ln := range strings.Split(s, "\n") {
		if t := p.Line(ln); t != nil {
			got = append(got, t)
		}
	}
	return got
}

func TestParseRealTrade(t *testing.T) {
	p := &Parser{}
	got := feed(p, sample)
	if len(got) != 1 {
		t.Fatalf("expected 1 completed trade, got %d", len(got))
	}
	tr := got[0]
	if tr.Partner != "bog0bog" {
		t.Errorf("partner = %q, want bog0bog", tr.Partner)
	}
	if len(tr.Gave) != 1 || !tr.Gave[0].IsPlatinum() || tr.Gave[0].Qty != 7 {
		t.Errorf("gave = %+v, want [Platinum x7]", tr.Gave)
	}
	if len(tr.Received) != 1 || tr.Received[0].Name != "Alternox Prime Stock" || tr.Received[0].Qty != 1 {
		t.Errorf("received = %+v, want [Alternox Prime Stock x1]", tr.Received)
	}
	if tr.PlatDelta() != -7 {
		t.Errorf("plat delta = %d, want -7 (a purchase)", tr.PlatDelta())
	}
}

func TestNoTradeUntilSuccess(t *testing.T) {
	// The confirmation dialog without the success line yields nothing (cancelled).
	cancelled := strings.Split(sample, "813.445 Sys")[0]
	p := &Parser{}
	if got := feed(p, cancelled); len(got) != 0 {
		t.Errorf("expected no trade without success line, got %d", len(got))
	}
}

func TestSaleDirection(t *testing.T) {
	// Selling: you offer the item, receive platinum.
	sale := `1.0 Script [Info]: Dialog.lua: Dialog::CreateOkCancel(description=Are you sure you want to accept this trade? You are offering:

Loki Prime Systems

and will receive from buyer123 the following:

Platinum x 45, title= leftItem=/Menu/Confirm_Item_Ok)
2.0 Script [Info]: Dialog.lua: Dialog::CreateOk(description=The trade was successful!, title=)`
	p := &Parser{}
	got := feed(p, sale)
	if len(got) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(got))
	}
	if got[0].PlatDelta() != 45 {
		t.Errorf("sale plat delta = %d, want +45", got[0].PlatDelta())
	}
	if len(got[0].Gave) != 1 || got[0].Gave[0].Name != "Loki Prime Systems" {
		t.Errorf("gave = %+v, want [Loki Prime Systems]", got[0].Gave)
	}
}
