// Package trades parses completed player-to-player trades out of Warframe's
// EE.log and records them, so the app can track platinum/ducat flow over time.
//
// A trade appears in the log as a confirmation dialog whose (multi-line)
// description lists what you offer and what you receive from the other player,
// followed — once accepted — by a "The trade was successful!" dialog:
//
//	Dialog::CreateOkCancel(description=Are you sure you want to accept this trade? You are offering:
//	Platinum x 7
//	and will receive from bog0bog the following:
//	Alternox Prime Stock, title= leftItem=… rightItem=…)
//	…
//	Dialog::CreateOk(description=The trade was successful!, title= …)
package trades

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Item is one line of a trade: a named item with a quantity. Platinum is
// represented as an item named "Platinum".
type Item struct {
	Name string
	Qty  int
}

// IsPlatinum reports whether this item is platinum.
func (i Item) IsPlatinum() bool { return strings.EqualFold(i.Name, "Platinum") }

// Trade is one completed exchange with another player.
type Trade struct {
	Time     time.Time
	Partner  string
	Gave     []Item // what you offered
	Received []Item // what you received
}

// PlatDelta is the net platinum from this trade (received minus given).
func (t Trade) PlatDelta() int {
	return platSum(t.Received) - platSum(t.Gave)
}

func platSum(items []Item) int {
	n := 0
	for _, it := range items {
		if it.IsPlatinum() {
			n += it.Qty
		}
	}
	return n
}

const (
	tradeStart   = "Are you sure you want to accept this trade?"
	tradeOK      = "The trade was successful!"
	dialogEnd    = "title=" // dialog params follow the description
	offeringTag  = "You are offering:"
	followingTag = "the following:"
)

var partnerRe = regexp.MustCompile(`and will receive from (.+?) the following:`)

// Parser is a stateful line-by-line trade detector. Feed it log lines (in
// order); it returns a completed *Trade when a trade is confirmed successful.
type Parser struct {
	// Now is injectable for tests; defaults to time.Now.
	Now func() time.Time

	accumulating bool
	buf          []string
	pending      *Trade
}

// Line feeds one log line to the parser, returning a finished Trade on success.
func (p *Parser) Line(line string) *Trade {
	if p.accumulating {
		p.buf = append(p.buf, line)
		if strings.Contains(line, dialogEnd) {
			p.accumulating = false
			p.pending = parseBlock(strings.Join(p.buf, "\n"))
			p.buf = nil
		}
		return nil
	}

	switch {
	case strings.Contains(line, tradeStart):
		p.accumulating = true
		p.buf = []string{line}
		// A single-line dialog (no embedded newlines) ends immediately.
		if strings.Contains(line, dialogEnd) {
			p.accumulating = false
			p.pending = parseBlock(line)
			p.buf = nil
		}
	case strings.Contains(line, tradeOK):
		if p.pending != nil {
			t := *p.pending
			if p.Now != nil {
				t.Time = p.Now()
			} else {
				t.Time = time.Now()
			}
			p.pending = nil
			return &t
		}
	}
	return nil
}

// parseBlock extracts the trade details from the joined dialog description.
func parseBlock(block string) *Trade {
	oi := strings.Index(block, offeringTag)
	pm := partnerRe.FindStringSubmatch(block)
	if oi < 0 || pm == nil {
		return nil
	}
	partner := strings.TrimSpace(pm[1])

	// "You are offering:" ... <offered> ... "and will receive from X the following:" ... <received> , title=
	rest := block[oi+len(offeringTag):]
	recvIdx := strings.Index(rest, "and will receive from")
	offeredText := rest
	receivedText := ""
	if recvIdx >= 0 {
		offeredText = rest[:recvIdx]
		afterRecv := rest[recvIdx:]
		if fi := strings.Index(afterRecv, followingTag); fi >= 0 {
			receivedText = afterRecv[fi+len(followingTag):]
		}
	}
	// Trim the trailing dialog params from the received section.
	if ti := strings.Index(receivedText, dialogEnd); ti >= 0 {
		// Drop the ", title=" and everything after on that line.
		if c := strings.LastIndex(receivedText[:ti], ","); c >= 0 {
			receivedText = receivedText[:c]
		} else {
			receivedText = receivedText[:ti]
		}
	}

	return &Trade{
		Partner:  partner,
		Gave:     parseItems(offeredText),
		Received: parseItems(receivedText),
	}
}

// parseItems turns a block of lines like "Platinum x 7" / "Alternox Prime Stock"
// into items, skipping blanks.
func parseItems(text string) []Item {
	var items []Item
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(strings.TrimRight(ln, ",")) // tidy trailing comma
		ln = stripLogPrefix(ln)
		if ln == "" {
			continue
		}
		name, qty := ln, 1
		// "<name> x <n>"
		if i := strings.LastIndex(ln, " x "); i >= 0 {
			if n, err := strconv.Atoi(strings.TrimSpace(ln[i+3:])); err == nil {
				name, qty = strings.TrimSpace(ln[:i]), n
			}
		}
		if name == "" {
			continue
		}
		items = append(items, Item{Name: name, Qty: qty})
	}
	return items
}

var logPrefixRe = regexp.MustCompile(`^\d+\.\d+\s+\w+\s+\[\w+\]:\s+`)

// stripLogPrefix removes a "812.525 Script [Info]: " style prefix if present
// (the first dialog line carries it; embedded continuation lines do not).
func stripLogPrefix(s string) string {
	s = logPrefixRe.ReplaceAllString(s, "")
	// Also drop the "Dialog.lua: Dialog::CreateOkCancel(description=..." lead-in
	// if it leaked into an item line (it won't for items, but be safe).
	return s
}
