<script>
import Inventory from "./Inventory.svelte";
import Mastery from "./Mastery.svelte";
import Relics from "./Relics.svelte";
import Foundry from "./Foundry.svelte";
import Trades from "./Trades.svelte";
import Analytics from "./Analytics.svelte";
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
import { Events } from "@wailsio/runtime";

const tabs = ["Inventory", "Mastery", "Relics", "Foundry", "Trades", "Analytics"];
let tab = $state(new URLSearchParams(location.search).get("tab") || "Inventory");

// Load inventory once at startup; tabs react to it.
let status = $state("Getting your inventory…");
let loaded = $state(false);
async function load() {
  status = "Getting your inventory…"; loaded = false;
  const st = await Service.LoadInventory();
  if (!st.loaded) { status = st.error || "Failed to load."; return; }
  loaded = true; status = "";
}
load();

// The backend auto-loads the inventory when Warframe starts after the app; pick
// it up here (no re-scrape — InventoryStatus just reads the held inventory).
Events.On("inventory:loaded", async () => {
  const st = await Service.InventoryStatus();
  if (st.loaded) { loaded = true; status = ""; }
});

// warframe.market account + online-status control (shown on every page).
let market = $state({ loggedIn: false, user: "", statusMode: "auto" });
async function refreshMarket() { market = await Service.MarketStatus(); }
refreshMarket();
// Re-sync when switching tabs (e.g. after signing in on the Trades tab).
$effect(() => { tab; refreshMarket(); });
async function setStatusMode(e) { market = await Service.SetMarketStatusMode(e.target.value); }
</script>

<div class="layout">
  <nav class="sidebar">
    <div class="brand">Companion</div>
    {#each tabs as t}
      <button class="nav {tab === t ? 'active' : ''}" onclick={() => (tab = t)}>{t}</button>
    {/each}
    <div class="statusbox">
      {#if market.loggedIn}
        <div class="slabel">WFM · {market.user || "signed in"}</div>
        <select value={market.statusMode} onchange={setStatusMode} title="warframe.market online status">
          <option value="auto">Auto (follow game)</option>
          <option value="online">Online</option>
          <option value="ingame">In game</option>
          <option value="invisible">Invisible</option>
        </select>
      {:else}
        <div class="slabel">Not signed in</div>
        <div class="shint">Sign in on the Trades tab</div>
      {/if}
    </div>
  </nav>
  <main class="content">
    {#if tab === "Inventory"}<Inventory {loaded} {status} {load} />
    {:else if tab === "Mastery"}<Mastery {loaded} {status} />
    {:else if tab === "Relics"}<Relics {loaded} {status} />
    {:else if tab === "Foundry"}<Foundry {loaded} {status} />
    {:else if tab === "Trades"}<Trades {loaded} {status} />
    {:else if tab === "Analytics"}<Analytics />
    {/if}
  </main>
</div>

<style>
.layout { display: flex; height: 100vh; }
.sidebar { width: 190px; background: var(--panel); padding: 14px 0; flex-shrink: 0;
  display: flex; flex-direction: column; }
.brand { color: var(--gold); font-size: 19px; font-weight: 700; padding: 4px 18px 14px; }
.statusbox { margin-top: auto; padding: 12px 14px 4px; border-top: 1px solid #23262f; }
.statusbox .slabel { color: var(--dim); font-size: 12px; margin-bottom: 6px; }
.statusbox .shint { color: #5a5d66; font-size: 11px; }
.statusbox select { width: 100%; font: inherit; font-size: 13px; color: var(--fg);
  background: var(--panel2); border: 1px solid #333; border-radius: 8px; padding: 5px 7px; cursor: pointer; }
.nav { display: block; width: 100%; text-align: left; padding: 10px 18px; background: none;
  border: none; color: #c4c6cf; font-size: 14px; cursor: pointer; }
.nav:hover { background: #20222a; }
.nav.active { background: var(--panel2); color: var(--gold); }
.content { flex: 1; padding: 16px 22px; display: flex; flex-direction: column; min-width: 0; overflow: hidden; }
</style>
