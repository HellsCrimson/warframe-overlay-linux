package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"warframe-overlay-linux/internal/config"
	"warframe-overlay-linux/internal/db"
	"warframe-overlay-linux/internal/inventory"
	"warframe-overlay-linux/internal/mastery"
	"warframe-overlay-linux/internal/relicoverlay"
	"warframe-overlay-linux/internal/relics"
	"warframe-overlay-linux/internal/trades"
	"warframe-overlay-linux/internal/wfdata"
	"warframe-overlay-linux/internal/wfmarket"
)

// serviceOptions carries the process configuration (from CLI flags) into the
// service.
type serviceOptions struct {
	cfg           config.Config
	inventoryFile string // load inventory from this file instead of the game (dev)
	logger        *slog.Logger
}

// Service is the Wails-bound API: its exported methods are callable from the
// frontend. It wraps the pure-Go domain packages (shared with the overlay).
type Service struct {
	cfg     config.Config
	invFile string
	log     *slog.Logger

	mu         sync.Mutex
	inv        *inventory.Inventory
	names      *wfdata.DB
	prices     *db.Database
	relics     *relics.Tables
	market     *wfmarket.Client
	session    *wfmarket.Session
	tradeStore *trades.Store
	livePrices map[string]int
	invErr     string
}

// NewService constructs the service and kicks off background data loads,
// including the in-game relic-reward overlay.
func NewService(opts serviceOptions) *Service {
	if opts.logger == nil {
		opts.logger = slog.Default()
	}
	s := &Service{
		cfg:        opts.cfg,
		invFile:    opts.inventoryFile,
		log:        opts.logger,
		market:     wfmarket.New(config.DefaultCacheDir()),
		livePrices: map[string]int{},
	}
	go func() {
		if d, err := wfdata.Load(wfdata.Options{CacheDir: config.DefaultCacheDir()}); err == nil {
			s.mu.Lock()
			s.names = d
			s.mu.Unlock()
		}
	}()
	go func() {
		if d, err := db.Load(db.Options{CacheDir: config.DefaultCacheDir(), TTL: 24 * time.Hour}); err == nil {
			s.mu.Lock()
			s.prices = d
			s.mu.Unlock()
		}
	}()
	// Relic drop tables (for the mastery "farmable from owned relics" ordering).
	go func() {
		if t, err := relics.Load(relics.Options{CacheDir: config.DefaultCacheDir()}); err == nil {
			s.mu.Lock()
			s.relics = t
			s.mu.Unlock()
		}
	}()
	// Market item list (for part thumbnails + ids) in the background.
	go func() { _ = s.market.LoadItems() }()
	// Auto-login to warframe.market with saved credentials.
	if creds, ok := wfmarket.LoadCredentials(config.DefaultConfigDir()); ok {
		go func() {
			_ = s.market.LoadItems()
			if sess, err := s.market.Login(creds.Email, creds.Password); err == nil {
				s.mu.Lock()
				s.session = sess
				s.mu.Unlock()
			}
		}()
	}
	// Trade tracking: tail EE.log into the persistent store.
	if store, err := trades.OpenStore(config.DefaultConfigDir()); err == nil {
		s.tradeStore = store
		go trades.Watch(context.Background(), s.cfg.EELogPath, store, func() {})
	}
	// In-game relic-reward overlay: watch EE.log, capture, OCR, price and show.
	go s.runOverlay()
	return s
}

// runOverlay starts the relic-reward overlay pipeline, decorating rewards with
// ownership info from the loaded inventory.
func (s *Service) runOverlay() {
	err := relicoverlay.Run(context.Background(), relicoverlay.Options{
		EELogPath:        s.cfg.EELogPath,
		Monitor:          s.cfg.Monitor,
		DumpDir:          s.cfg.CapturePNGDir,
		NoOverlay:        s.cfg.NoOverlay,
		OverlayDuration:  s.cfg.OverlayDuration,
		PostTriggerDelay: s.cfg.PostTriggerDelay,
		CacheDir:         s.cfg.CacheDir,
		DataTTL:          s.cfg.DataTTL,
		Logger:           s.log,
		Owned: func(dropName string) (int, bool) {
			s.mu.Lock()
			inv := s.inv
			s.mu.Unlock()
			if inv == nil {
				return 0, false
			}
			return inv.Owned(dropName), true
		},
	})
	if err != nil {
		s.log.Error("relic overlay stopped", "err", err)
	}
}

// ---- Inventory --------------------------------------------------------------

// LoadStatus reports whether the inventory is loaded and any error.
type LoadStatus struct {
	Loaded bool   `json:"loaded"`
	Error  string `json:"error"`
}

// LoadInventory loads the player inventory (from $WFO_INVENTORY_FILE if set, else
// by scraping the running game).
func (s *Service) LoadInventory() LoadStatus {
	var (
		inv *inventory.Inventory
		err error
	)
	if s.invFile != "" {
		inv, err = inventory.LoadFile(s.invFile)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		inv, err = inventory.Load(ctx)
	}
	s.mu.Lock()
	s.inv = inv
	s.invErr = friendlyErr(err)
	s.mu.Unlock()
	return LoadStatus{Loaded: inv != nil, Error: friendlyErr(err)}
}

// InvItem is a displayed inventory item.
type InvItem struct {
	Name     string `json:"name"`
	Rank     int    `json:"rank"`
	MaxRank  int    `json:"maxRank"`
	Mastered bool   `json:"mastered"`
	Icon     string `json:"icon"`
}

// InvCategory groups inventory items.
type InvCategory struct {
	Name  string    `json:"name"`
	Items []InvItem `json:"items"`
}

// GetInventory returns owned equipment grouped by category with names, ranks and
// thumbnails.
func (s *Service) GetInventory() []InvCategory {
	s.mu.Lock()
	inv, names := s.inv, s.names
	s.mu.Unlock()
	if inv == nil {
		return nil
	}
	var out []InvCategory
	for _, c := range inv.Categories() {
		cat := InvCategory{Name: c.Name}
		for _, it := range c.Items {
			name := it.Name
			if names != nil {
				if n, ok := names.Name(it.Type); ok {
					name = n
				}
			}
			xp := it.XP
			if mxp := inv.MasteryXP(it.Type); mxp > xp {
				xp = mxp
			}
			rank := mastery.Rank(xp, c.ProductCategory)
			max := mastery.MaxRank(c.ProductCategory)
			cat.Items = append(cat.Items, InvItem{
				Name: name, Rank: rank, MaxRank: max, Mastered: rank >= max,
				Icon: names.ImageURL(it.Type),
			})
		}
		out = append(out, cat)
	}
	return out
}

// ---- Mastery ----------------------------------------------------------------

type MasterySummary struct {
	Total, Mastered, BuiltUnranked, ReadyToBuild, PartsPartial, NotStarted int
}

// MasteryPart is one recipe component with how many the player owns vs needs
// (recipes can require more than one, e.g. two blades).
type MasteryPart struct {
	Name  string `json:"name"`
	Query string `json:"query"` // "<item> <component>" for market lookup
	Have  int    `json:"have"`
	Need  int    `json:"need"`
}

type MasteryItem struct {
	Name       string        `json:"name"`
	Category   string        `json:"category"`
	Status     string        `json:"status"` // "Mastered" etc (display string)
	Rank       int           `json:"rank"`
	MaxRank    int           `json:"maxRank"`
	PartsOwned int           `json:"partsOwned"`
	PartsTotal int           `json:"partsTotal"`
	Parts      []MasteryPart `json:"parts"` // per-component detail (collecting items)
	Icon       string        `json:"icon"`
	// BuildCost is the platinum needed to buy the still-missing parts.
	BuildCost int `json:"buildCost"`
	// CostKnown is false when a needed part has no market price (BuildCost partial).
	CostKnown bool `json:"costKnown"`
	// RelicCount is how many owned relics can drop a still-needed part.
	RelicCount int `json:"relicCount"`
	// RelicScore weights drop chance by owned-relic count (the "farmable" rank key).
	RelicScore float64 `json:"relicScore"`
	// BestChance is the best single-relic drop chance (%) for a needed part.
	BestChance float64 `json:"bestChance"`
}

type MasteryView struct {
	Summary MasterySummary `json:"summary"`
	Items   []MasteryItem  `json:"items"`
}

// GetMastery computes the mastery view, ordered by sortMode: "next" (best to do
// next, the default), "cost" (cheapest missing parts to buy first) or "relics"
// (most farmable from relics the player already owns first).
func (s *Service) GetMastery(sortMode string) MasteryView {
	s.mu.Lock()
	inv, names, prices, tables := s.inv, s.names, s.prices, s.relics
	s.mu.Unlock()
	if inv == nil || names == nil {
		return MasteryView{}
	}
	res := mastery.Compute(names.Masterable(), inv)
	idx := buildRelicIndex(inv, tables)
	view := MasteryView{Summary: MasterySummary(res.Summary)}
	for _, it := range res.Items {
		var parts []MasteryPart
		for _, p := range it.Parts {
			parts = append(parts, MasteryPart{Name: p.Name, Query: p.Query, Have: p.Have, Need: p.Need})
		}
		mi := MasteryItem{
			Name: it.Name, Category: it.Category, Status: it.Status.String(),
			Rank: it.Rank, MaxRank: it.MaxRank,
			PartsOwned: it.PartsOwned, PartsTotal: it.PartsTotal,
			Parts: parts,
			Icon:  names.ImageURLByName(it.Name),
		}
		view.Items = append(view.Items, annotate(it, mi, prices, idx))
	}
	sortMastery(view.Items, sortMode)
	return view
}

// MarketSeller is one warframe.market seller of a part, with a ready-to-send
// in-game whisper to buy it.
type MarketSeller struct {
	User     string `json:"user"`
	Platinum int    `json:"platinum"`
	Quantity int    `json:"quantity"`
	Status   string `json:"status"` // ingame | online | offline
	Whisper  string `json:"whisper"`
}

// PartSellers looks up warframe.market sellers for a part (by its "<item>
// <component>" query) and returns each with a copyable purchase whisper.
func (s *Service) PartSellers(query string) []MarketSeller {
	s.mu.Lock()
	prices := s.prices
	s.mu.Unlock()

	name := query
	if prices != nil {
		if item := prices.FindPart(query); item != nil {
			// Use the plain part name: warframe.market lists components
			// without the "Blueprint" suffix that DropName carries for
			// warframe parts (relic-drop naming), so DropName misses the slug.
			name = item.Name
		}
	}
	orders, err := s.market.SellOrders(name, 12)
	if err != nil {
		return nil
	}
	out := make([]MarketSeller, 0, len(orders))
	for _, o := range orders {
		out = append(out, MarketSeller{
			User: o.Seller, Platinum: o.Platinum, Quantity: o.Quantity, Status: o.Status,
			Whisper: fmt.Sprintf("/w %s Hi! I want to buy: \"%s\" for %d platinum. (warframe.market from warframe-overlay-linux)",
				o.Seller, name, o.Platinum),
		})
	}
	return out
}

// ---- Trades -----------------------------------------------------------------

type SellItem struct {
	Name   string `json:"name"`
	Qty    int    `json:"qty"`
	Plat   int    `json:"plat"`
	Live   int    `json:"live"`
	Ducats int    `json:"ducats"`
	Icon   string `json:"icon"`
}

// GetSellable lists owned tradeable parts with prices and thumbnails.
func (s *Service) GetSellable() []SellItem {
	s.mu.Lock()
	inv, prices, names, live := s.inv, s.prices, s.names, s.livePrices
	s.mu.Unlock()
	if inv == nil || prices == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []SellItem
	for _, it := range prices.Items() {
		if seen[it.DropName] {
			continue
		}
		qty := inv.Owned(it.DropName)
		if qty <= 0 {
			continue
		}
		seen[it.DropName] = true
		// Parts aren't in warframestat's item list, so use warframe.market
		// thumbnails (falling back to warframestat by name where present).
		icon := s.market.IconURL(it.DropName)
		if icon == "" && names != nil {
			icon = names.ImageURLByName(it.DropName)
		}
		out = append(out, SellItem{
			Name: it.DropName, Qty: qty, Plat: int(it.Platinum + 0.5),
			Live: live[it.DropName], Ducats: it.Ducats, Icon: icon,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		pi, pj := priceOf(out[i]), priceOf(out[j])
		if pi != pj {
			return pi > pj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func priceOf(s SellItem) int {
	if s.Live > 0 {
		return s.Live
	}
	return s.Plat
}

// RefreshLivePrices fetches warframe.market median prices for the given items.
func (s *Service) RefreshLivePrices(names []string) {
	if err := s.market.LoadItems(); err != nil {
		return
	}
	for _, name := range names {
		if p, err := s.market.PriceByName(name); err == nil && p > 0 {
			s.mu.Lock()
			s.livePrices[name] = p
			s.mu.Unlock()
		}
	}
}

// ---- warframe.market account -------------------------------------------------

type MarketStatus struct {
	LoggedIn bool   `json:"loggedIn"`
	User     string `json:"user"`
	Error    string `json:"error"`
}

func (s *Service) MarketStatus() MarketStatus {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil {
		return MarketStatus{}
	}
	return MarketStatus{LoggedIn: true, User: sess.UserName}
}

// MarketLogin signs in and (on success) saves the credentials.
func (s *Service) MarketLogin(email, password string) MarketStatus {
	_ = s.market.LoadItems()
	sess, err := s.market.Login(email, password)
	if err != nil {
		return MarketStatus{Error: err.Error()}
	}
	s.mu.Lock()
	s.session = sess
	s.mu.Unlock()
	_ = wfmarket.SaveCredentials(config.DefaultConfigDir(), wfmarket.Credentials{Email: email, Password: password})
	return MarketStatus{LoggedIn: true, User: sess.UserName}
}

func (s *Service) MarketLogout() {
	s.mu.Lock()
	s.session = nil
	s.mu.Unlock()
	_ = wfmarket.ClearCredentials(config.DefaultConfigDir())
}

// ListResult reports the outcome of posting sell orders.
type ListResult struct {
	Listed int    `json:"listed"`
	Failed int    `json:"failed"`
	Error  string `json:"error"`
}

// ListOnMarket posts visible sell orders for the named items at their price.
func (s *Service) ListOnMarket(names []string) ListResult {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil {
		return ListResult{Error: "not signed in"}
	}
	byName := map[string]SellItem{}
	for _, it := range s.GetSellable() {
		byName[it.Name] = it
	}
	var res ListResult
	for _, name := range names {
		it, ok := byName[name]
		if !ok {
			res.Failed++
			continue
		}
		id, ok := s.market.ItemID(name)
		if !ok {
			res.Failed++
			continue
		}
		if err := s.market.AddSellOrder(sess, id, priceOf(it), it.Qty); err != nil {
			res.Failed++
		} else {
			res.Listed++
		}
	}
	return res
}

// ---- Analytics --------------------------------------------------------------

type TradeRow struct {
	Partner   string `json:"partner"`
	Gave      string `json:"gave"`
	Received  string `json:"received"`
	PlatDelta int    `json:"platDelta"`
}

type Analytics struct {
	InvValue   int        `json:"invValue"`
	Ducats     int        `json:"ducats"`
	Sellable   int        `json:"sellable"`
	TradeCount int        `json:"tradeCount"`
	NetPlat    int        `json:"netPlat"`
	PlatIn     int        `json:"platIn"`
	PlatOut    int        `json:"platOut"`
	Cumulative []int      `json:"cumulative"`
	Recent     []TradeRow `json:"recent"`
}

// GetAnalytics returns inventory value plus tracked-trade stats.
func (s *Service) GetAnalytics() Analytics {
	s.mu.Lock()
	inv, prices, live, store := s.inv, s.prices, s.livePrices, s.tradeStore
	s.mu.Unlock()

	var a Analytics
	if inv != nil && prices != nil {
		seen := map[string]bool{}
		for _, it := range prices.Items() {
			if seen[it.DropName] {
				continue
			}
			qty := inv.Owned(it.DropName)
			if qty <= 0 {
				continue
			}
			seen[it.DropName] = true
			unit := int(it.Platinum + 0.5)
			if lp := live[it.DropName]; lp > 0 {
				unit = lp
			}
			a.InvValue += unit * qty
			a.Ducats += it.Ducats * qty
			a.Sellable++
		}
	}
	if store != nil {
		ts := store.All()
		sum := trades.Summarize(ts)
		a.TradeCount, a.NetPlat, a.PlatIn, a.PlatOut = sum.Count, sum.NetPlat, sum.PlatIn, sum.PlatOut
		a.Cumulative = sum.Cumulative
		for i := len(ts) - 1; i >= 0; i-- { // newest first
			t := ts[i]
			a.Recent = append(a.Recent, TradeRow{
				Partner: t.Partner, Gave: itemsSummary(t.Gave),
				Received: itemsSummary(t.Received), PlatDelta: t.PlatDelta(),
			})
		}
	}
	return a
}

func itemsSummary(items []trades.Item) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		if it.Qty > 1 {
			parts = append(parts, fmt.Sprintf("%s ×%d", it.Name, it.Qty))
		} else {
			parts = append(parts, it.Name)
		}
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

// ---- helpers ----------------------------------------------------------------

func friendlyErr(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, inventory.ErrNotRunning):
		return "Warframe isn't running. Start the game and reload."
	case errors.Is(err, inventory.ErrPermission):
		return "Can't read game memory — run: sudo sysctl kernel.yama.ptrace_scope=0 (or grant CAP_SYS_PTRACE)."
	case errors.Is(err, inventory.ErrAuthNotFound):
		return "Couldn't find your session — are you logged in?"
	default:
		return err.Error()
	}
}
