<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
let { loaded, status } = $props();

let items = $state([]);
let search = $state("");
let sortBy = $state("Value"); let asc = $state(false);
let selected = $state(new Set());
let refreshing = $state(false);
let listStatus = $state("");

// market account
let market = $state({ loggedIn: false, user: "", error: "" });
let email = $state(""); let password = $state(""); let connecting = $state(false);

async function reload() { items = (await Service.GetSellable()) || []; }
$effect(() => { if (loaded) reload(); });
Service.MarketStatus().then((m) => (market = m));

// Per-item top-3 market listings, loaded on demand (the API is rate-limited,
// so we fetch only the row the user expands).
let expanded = $state("");          // name of the row whose listings are shown
let listings = $state({});          // name -> array of {platinum,status,...} | null (loading)
async function toggleListings(name) {
  if (expanded === name) { expanded = ""; return; }
  expanded = name;
  if (listings[name] === undefined) {
    listings = { ...listings, [name]: null };
    const rows = (await Service.TopSellers(name, 3)) || [];
    listings = { ...listings, [name]: rows };
  }
}
const statusLabel = { ingame: "in-game", online: "online", offline: "offline" };

const sorters = {
  Value: (a, b) => price(a) - price(b),
  Name: (a, b) => a.name.localeCompare(b.name),
  Ducats: (a, b) => a.ducats - b.ducats,
  Qty: (a, b) => a.qty - b.qty,
};
function price(it) { return it.live > 0 ? it.live : it.plat; }
function setSort(s) { if (sortBy === s) asc = !asc; else { sortBy = s; asc = false; } }

let shown = $derived(
  [...items]
    .filter((it) => !search || it.name.toLowerCase().includes(search.toLowerCase()))
    .sort((a, b) => (asc ? 1 : -1) * sorters[sortBy](a, b))
);
let wts = $derived(
  "WTS " + [...selected].map((n) => {
    const it = items.find((i) => i.name === n); return it ? `[${n}] ${price(it)}p` : "";
  }).filter(Boolean).join(" ")
);
function toggle(n) { selected.has(n) ? selected.delete(n) : selected.add(n); selected = new Set(selected); }

async function refresh() {
  refreshing = true;
  await Service.RefreshLivePrices(shown.map((it) => it.name));
  await reload(); refreshing = false;
}
function copyWTS() { if (selected.size) navigator.clipboard.writeText(wts); }
async function connect() {
  connecting = true; market = await Service.MarketLogin(email, password); connecting = false;
}
async function logout() { await Service.MarketLogout(); market = { loggedIn: false }; }
async function listOnWFM() {
  listStatus = "Listing on warframe.market…";
  const r = await Service.ListOnMarket([...selected]);
  listStatus = r.error ? "Error: " + r.error : `Listed ${r.listed} (${r.failed} failed)`;
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
      <div class="row">
        <input type="checkbox" checked={selected.has(it.name)} onchange={() => toggle(it.name)} />
        {#if it.icon}<img class="thumb" src={it.icon} alt="" loading="lazy" />{:else}<div class="thumb ph"></div>{/if}
        <span class="iname">{it.name}{#if it.qty > 1}<span class="muted"> ×{it.qty}</span>{/if}</span>
        <span class="muted" style="margin-right:14px">{it.ducats} ducats</span>
        <button class="prices" class:open={expanded === it.name} onclick={() => toggleListings(it.name)}
          title="Show current warframe.market sell prices">market ›</button>
        <span style="color:{it.live > 0 ? 'var(--green)' : 'var(--gold)'}; width:60px; text-align:right">{price(it)}p</span>
      </div>
      {#if expanded === it.name}
        <div class="listings">
          {#if listings[it.name] === null}
            <span class="muted">Loading market prices…</span>
          {:else if !listings[it.name]?.length}
            <span class="muted">No current sellers found.</span>
          {:else}
            <span class="muted">Top sellers:</span>
            {#each listings[it.name] as o}
              <span class="listing {o.status}">{o.platinum}p<span class="lstatus">{statusLabel[o.status] || o.status}</span></span>
            {/each}
          {/if}
        </div>
      {/if}
    {/each}
  </div>
{:else}
  <div class="empty">{status}</div>
{/if}

<div class="bar">
  <div style="flex:1">{listStatus || (selected.size ? wts : "Select items to build a WTS message.")}</div>
  {#if market.loggedIn && selected.size}<button class="btn green" onclick={listOnWFM}>List on WFM</button>{/if}
  <button class="btn" onclick={copyWTS}>Copy</button>
</div>
