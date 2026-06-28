<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
let { loaded, status } = $props();

let items = $state([]);
let search = $state("");
let sortBy = $state("Value"); let asc = $state(false);
let refreshing = $state(false);

// market account
let market = $state({ loggedIn: false, user: "", error: "" });
let email = $state(""); let password = $state(""); let connecting = $state(false);

// list modal
let modal = $state(null);        // the SellItem being listed
let listings = $state(null);     // top market listings | null while loading
let qty = $state(1);
let price = $state(0);
let listStatus = $state("");
let listing = $state(false);

async function reload() { items = (await Service.GetSellable()) || []; }
$effect(() => { if (loaded) reload(); });
Service.MarketStatus().then((m) => (market = m));

const sorters = {
  Value: (a, b) => price2(a) - price2(b),
  Name: (a, b) => a.name.localeCompare(b.name),
  Ducats: (a, b) => a.ducats - b.ducats,
  Qty: (a, b) => a.qty - b.qty,
};
function price2(it) { return it.live > 0 ? it.live : it.plat; }
function setSort(s) { if (sortBy === s) asc = !asc; else { sortBy = s; asc = false; } }

let shown = $derived(
  [...items]
    .filter((it) => !search || it.name.toLowerCase().includes(search.toLowerCase()))
    .sort((a, b) => (asc ? 1 : -1) * sorters[sortBy](a, b))
);

const statusLabel = { ingame: "in-game", online: "online", offline: "offline" };

async function refresh() {
  refreshing = true;
  await Service.RefreshLivePrices(shown.map((it) => it.name));
  await reload(); refreshing = false;
}
async function connect() {
  connecting = true; market = await Service.MarketLogin(email, password); connecting = false;
}
async function logout() { await Service.MarketLogout(); market = await Service.MarketStatus(); }

async function openModal(it) {
  modal = it; listings = null; listStatus = "";
  qty = 1; price = price2(it) || 1;
  const rows = (await Service.TopSellers(it.name, 5)) || [];
  // Only keep showing if the user hasn't already closed/switched the modal.
  if (modal === it) {
    listings = rows;
    if (rows.length && (!price || price === price2(it))) price = rows[0].platinum;
  }
}
function closeModal() { modal = null; listings = null; }

function clampQty(v) {
  const max = modal ? modal.qty : 1;
  qty = Math.max(1, Math.min(max, Math.round(v || 1)));
}

async function list() {
  if (!modal) return;
  listing = true; listStatus = "Listing on warframe.market…";
  const r = await Service.CreateSellOrder(modal.name, Math.round(price), qty);
  listing = false;
  if (r.error) { listStatus = "Error: " + r.error; return; }
  listStatus = `Listed ${qty}× ${modal.name} at ${Math.round(price)}p.`;
}
</script>

<header class="head">
  <h1>Trades</h1>
  <span class="muted">{refreshing ? "Fetching live prices…" : items.length + " sellable items"}</span>
  <button class="btn ghost spacer" onclick={refresh}>Live prices</button>
</header>

<!-- account -->
<div class="bar">
  {#if market.loggedIn}
    <span style="flex:1; color:var(--green)">warframe.market: signed in as {market.user || "your account"}</span>
    <button class="btn ghost" onclick={logout}>Sign out</button>
  {:else}
    <input class="input" placeholder="warframe.market email" bind:value={email} />
    <input class="input" type="password" placeholder="password" bind:value={password} />
    {#if market.error}<span style="color:var(--red)" class="muted">login failed</span>{/if}
    <button class="btn ghost" onclick={connect}>{connecting ? "Connecting…" : "Connect"}</button>
  {/if}
</div>

<input class="search" placeholder="Search sellable items…" bind:value={search} />
<div class="chips" style="margin:2px 0 8px; align-items:center">
  <span class="muted">Sort by:</span>
  {#each Object.keys(sorters) as s}
    <button class="badge" style="cursor:pointer; background:{sortBy===s?'var(--panel2)':'var(--panel)'}; color:{sortBy===s?'var(--gold)':'#c4c6cf'}"
      onclick={() => setSort(s)}>{s}{sortBy===s ? (asc ? " ↑" : " ↓") : ""}</button>
  {/each}
</div>

{#if loaded}
  <div class="scroll">
    {#each shown as it}
      <div class="row clickable" role="button" tabindex="0" title="List on warframe.market"
           onclick={() => openModal(it)} onkeydown={(e) => e.key === "Enter" && openModal(it)}>
        {#if it.icon}<img class="thumb" src={it.icon} alt="" loading="lazy" />{:else}<div class="thumb ph"></div>{/if}
        <span class="iname">{it.name}{#if it.qty > 1}<span class="muted"> ×{it.qty}</span>{/if}</span>
        <span class="muted" style="margin-right:14px">{it.ducats} ducats</span>
        <span style="color:{it.live > 0 ? 'var(--green)' : 'var(--gold)'}; width:60px; text-align:right">{price2(it)}p</span>
        <span class="hint">list ›</span>
      </div>
    {/each}
  </div>
{:else}
  <div class="empty">{status}</div>
{/if}

{#if modal}
  <div class="modal-bg" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) closeModal(); }}>
    <div class="modal" role="dialog" aria-modal="true" tabindex="-1">
      <div class="modal-head">
        {#if modal.icon}<img class="thumb" src={modal.icon} alt="" />{/if}
        <h2 style="flex:1">{modal.name} <span class="muted" style="font-weight:400">· {modal.qty} owned</span></h2>
        <button class="xbtn" onclick={closeModal} aria-label="Close">✕</button>
      </div>
      <div class="modal-body">
        <div class="muted" style="margin-bottom:6px">Cheapest current sellers</div>
        {#if listings === null}
          <div class="muted">Loading market prices…</div>
        {:else if !listings.length}
          <div class="muted">No current sellers found.</div>
        {:else}
          <div class="listrows">
            {#each listings as o}
              <button class="listrow {o.status}" title="Use this price"
                      onclick={() => (price = o.platinum)}>
                <span class="lp">{o.platinum}p</span>
                <span class="lq muted">×{o.quantity}</span>
                <span class="lstatus">{statusLabel[o.status] || o.status}</span>
              </button>
            {/each}
          </div>
        {/if}

        {#if market.loggedIn}
          <div class="listform">
            <label>Quantity
              <input type="number" min="1" max={modal.qty} bind:value={qty}
                     onchange={(e) => clampQty(+e.target.value)} />
              <span class="muted">/ {modal.qty}</span>
            </label>
            <label>Price (p)
              <input type="number" min="1" bind:value={price} />
            </label>
            <button class="btn green" disabled={listing} onclick={list}>
              {listing ? "Listing…" : `List ${qty}×`}
            </button>
          </div>
          {#if listStatus}<div class="muted" style="margin-top:8px">{listStatus}</div>{/if}
        {:else}
          <div class="muted" style="margin-top:10px">Sign in to warframe.market (above) to list this item.</div>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
.row.clickable { cursor: pointer; }
.hint { color: var(--dim); font-size: 12px; margin-left: 10px; }
.listrows { display: flex; flex-wrap: wrap; gap: 8px; }
.listrow { display: inline-flex; align-items: baseline; gap: 7px; background: var(--panel2);
  border: 1px solid #34384400; border-radius: 8px; padding: 6px 10px; cursor: pointer; color: var(--fg); font: inherit; }
.listrow:hover { border-color: var(--gold); }
.listrow .lp { color: var(--gold); font-weight: 600; }
.listrow .lstatus { font-size: 11px; color: var(--dim); }
.listrow.ingame .lstatus { color: var(--green); }
.listrow.online .lstatus { color: var(--blue); }
.listform { display: flex; align-items: flex-end; gap: 14px; margin-top: 16px; flex-wrap: wrap; }
.listform label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--dim); }
.listform input { width: 90px; background: var(--bg); border: none; border-radius: 6px; padding: 7px 9px;
  color: var(--fg); outline: none; font: inherit; }
</style>
