<script>
import Inventory from "./Inventory.svelte";
import Mastery from "./Mastery.svelte";
import Trades from "./Trades.svelte";
import Analytics from "./Analytics.svelte";
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";

const tabs = ["Inventory", "Mastery", "Trades", "Analytics"];
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
</script>

<div class="layout">
  <nav class="sidebar">
    <div class="brand">Companion</div>
    {#each tabs as t}
      <button class="nav {tab === t ? 'active' : ''}" onclick={() => (tab = t)}>{t}</button>
    {/each}
  </nav>
  <main class="content">
    {#if tab === "Inventory"}<Inventory {loaded} {status} {load} />
    {:else if tab === "Mastery"}<Mastery {loaded} {status} />
    {:else if tab === "Trades"}<Trades {loaded} {status} />
    {:else if tab === "Analytics"}<Analytics />
    {/if}
  </main>
</div>

<style>
.layout { display: flex; height: 100vh; }
.sidebar { width: 190px; background: var(--panel); padding: 14px 0; flex-shrink: 0; }
.brand { color: var(--gold); font-size: 19px; font-weight: 700; padding: 4px 18px 14px; }
.nav { display: block; width: 100%; text-align: left; padding: 10px 18px; background: none;
  border: none; color: #c4c6cf; font-size: 14px; cursor: pointer; }
.nav:hover { background: #20222a; }
.nav.active { background: var(--panel2); color: var(--gold); }
.content { flex: 1; padding: 16px 22px; display: flex; flex-direction: column; min-width: 0; overflow: hidden; }
</style>
