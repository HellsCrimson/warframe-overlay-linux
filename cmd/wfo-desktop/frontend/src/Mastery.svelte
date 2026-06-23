<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
import CraftTree from "./CraftTree.svelte";
let { loaded, status } = $props();

let view = $state(null);
let hideNotStarted = $state(true);
let sort = $state("next");       // next | cost | relics
let search = $state("");
let modalItem = $state(null);    // item whose crafting tree is open

// Refetch when the sort mode changes (the backend does the ordering).
$effect(() => { if (loaded) Service.GetMastery(sort).then((v) => (view = v)); });

const statusColor = {
  "Mastered": "var(--gold)", "Built — rank up": "var(--gold)", "Ready to build": "var(--green)",
  "Collecting parts": "var(--blue)", "Not started": "#6a6d77",
};
let items = $derived(
  !view ? [] : view.items.filter((it) =>
    (!hideNotStarted || it.status !== "Not started") &&
    (!search || it.name.toLowerCase().includes(search.toLowerCase())))
);

function shortStatus(it) {
  if (it.status === "Mastered") return `Mastered ${it.rank}`;
  if (it.status === "Built — rank up") return `Rank ${it.rank}/${it.maxRank}`;
  if (it.partsTotal > 0) return `${it.partsOwned}/${it.partsTotal} parts`;
  return it.status;
}
function costLabel(it) {
  if (it.buildCost <= 0) return it.costKnown ? "free" : "no price";
  return it.costKnown ? `${it.buildCost}p` : `${it.buildCost}p+`;
}
function relicLabel(it) {
  if (!it.relicCount) return "no owned relics";
  return `${it.relicCount} relics · ${it.bestChance.toFixed(1)}%`;
}
function metric(it) {
  if (it.status === "Mastered") return "";
  if (sort === "cost") return costLabel(it);
  if (sort === "relics") return relicLabel(it);
  return "";
}
</script>

<header class="head"><h1>Mastery</h1></header>
{#if !loaded || !view}
  <div class="empty">{status || "Computing mastery…"}</div>
{:else}
  <div class="chips" style="margin:8px 0 4px">
    <span class="chip" style="color:var(--gold)">{view.summary.Mastered} mastered</span>
    <span class="chip" style="color:var(--gold)">{view.summary.BuiltUnranked} to rank up</span>
    <span class="chip" style="color:var(--green)">{view.summary.ReadyToBuild} ready</span>
    <span class="chip" style="color:var(--blue)">{view.summary.PartsPartial} collecting</span>
    <span class="chip muted">{view.summary.NotStarted} not started</span>
  </div>
  <div class="controls">
    <label class="muted sortlabel">
      Sort by
      <select bind:value={sort}>
        <option value="next">Best to do next</option>
        <option value="cost">Cheapest to build</option>
        <option value="relics">Farmable from my relics</option>
      </select>
    </label>
    <input class="csearch" placeholder="Search…" bind:value={search} />
    <label class="muted" style="display:flex; gap:8px; align-items:center; cursor:pointer">
      <input type="checkbox" bind:checked={hideNotStarted} /> Hide not-yet-started
    </label>
  </div>
  <div class="scroll">
    <div class="grid">
      {#each items as it (it.name)}
        <div class="card" class:owned={it.owned} class:mastered={it.status === "Mastered"}
             onclick={() => (modalItem = it)} role="button" tabindex="0"
             title="Show crafting tree">
          {#if it.status === "Mastered"}<span class="star">★</span>{/if}
          {#if it.icon}<img class="cthumb" src={it.icon} alt="" loading="lazy" />{:else}<div class="cthumb ph"></div>{/if}
          <span class="cname">{it.name}</span>
          <div class="dots">
            {#if it.parts}
              {#each it.parts as p}
                <span class="pdot" class:have={p.have >= p.need} title="{p.name} {p.have}/{p.need}"></span>
              {/each}
            {/if}
          </div>
          <span class="cbadge" style="color:{statusColor[it.status] || '#888'}">{shortStatus(it)}</span>
          {#if metric(it)}<span class="cmetric">{metric(it)}</span>{/if}
        </div>
      {/each}
    </div>
  </div>
{/if}

{#if modalItem}
  <CraftTree item={modalItem} onClose={() => (modalItem = null)} />
{/if}

<style>
.controls { display: flex; flex-wrap: wrap; gap: 16px; align-items: center; margin: 6px 0; }
.sortlabel { display: flex; gap: 8px; align-items: center; }
.sortlabel select {
  font: inherit; color: var(--fg); background: var(--panel2);
  border: 1px solid #333; border-radius: 8px; padding: 3px 8px; cursor: pointer;
}
.csearch {
  background: var(--panel); border: none; border-radius: 8px; padding: 6px 10px;
  color: var(--fg); outline: none; min-width: 150px;
}
.cmetric { font-size: 11px; font-weight: 600; color: var(--gold); }
</style>
