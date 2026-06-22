<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
let { loaded, status } = $props();

let view = $state(null);
let hideNotStarted = $state(true);
let sort = $state("next");       // next | cost | relics
let selected = $state("");
let selPart = $state("");        // query of the part whose sellers are shown
let sellers = $state(null);      // seller rows, or null while loading
let loadingSellers = $state(false);
let copied = $state("");         // whisper just copied (for feedback)

// Refetch when the sort mode changes (the backend does the ordering).
$effect(() => { if (loaded) Service.GetMastery(sort).then((v) => (view = v)); });

const statusColor = {
  "Built — rank up": "var(--gold)", "Ready to build": "var(--green)",
  "Collecting parts": "var(--blue)", "Not started": "#6a6d77",
};
let items = $derived(
  !view ? [] : view.items.filter((it) => !hideNotStarted || it.status !== "Not started")
);

function detail(it) {
  if (it.status === "Built — rank up") return `rank ${it.rank} / ${it.maxRank}`;
  if (it.partsTotal > 0) return `${it.partsOwned} / ${it.partsTotal} parts`;
  return "";
}

// Sort-specific metric shown on each row (cost to finish / owned-relic odds).
function costLabel(it) {
  if (it.buildCost <= 0) return it.costKnown ? "free" : "no price";
  return it.costKnown ? `${it.buildCost}p` : `${it.buildCost}p+`;
}
function relicLabel(it) {
  if (!it.relicCount) return "no owned relics";
  return `${it.relicCount} relics · ${it.bestChance.toFixed(1)}%`;
}

function pickItem(it) {
  selPart = ""; sellers = null; copied = "";
  selected = selected === it.name ? "" : it.name;
}

function pickPart(p) {
  copied = "";
  if (selPart === p.query) { selPart = ""; sellers = null; return; }
  selPart = p.query;
  sellers = null;
  loadingSellers = true;
  Service.PartSellers(p.query).then((rows) => {
    if (selPart === p.query) { sellers = rows || []; loadingSellers = false; }
  });
}

function copy(text) { navigator.clipboard.writeText(text); copied = text; }
</script>

<header class="head"><h1>Mastery</h1></header>
{#if !loaded || !view}
  <div class="empty">{status || "Computing mastery…"}</div>
{:else}
  <div class="chips" style="margin:8px 0 4px">
    <span class="chip" style="color:var(--green)">{view.summary.Mastered} mastered</span>
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
    <label class="muted" style="display:flex; gap:8px; align-items:center; cursor:pointer">
      <input type="checkbox" bind:checked={hideNotStarted} /> Hide not-yet-started items
    </label>
  </div>
  <div class="scroll">
    {#each items as it}
      <div class="row" class:active={it.name === selected}
           onclick={() => pickItem(it)} role="button" tabindex="0">
        {#if it.icon}<img class="thumb" src={it.icon} alt="" loading="lazy" />{:else}<div class="thumb ph"></div>{/if}
        <span class="iname">{it.name}</span>
        {#if sort === "cost"}
          <span class="metric" class:dim={!it.costKnown && it.buildCost <= 0}>{costLabel(it)}</span>
        {:else if sort === "relics"}
          <span class="metric" class:dim={!it.relicCount}>{relicLabel(it)}</span>
        {/if}
        <span class="muted" style="margin-right:12px">{detail(it)}</span>
        <span class="badge" style="color:{statusColor[it.status] || '#888'}">{it.status}</span>
      </div>
      {#if it.name === selected && it.parts && it.parts.length}
        <div class="parts">
          {#each it.parts as p}
            <button type="button" class="part" class:have={p.have >= p.need} class:sel={selPart === p.query}
                    title={p.have >= p.need ? "owned" : `need ${p.need - p.have} more — click for sellers`}
                    onclick={() => pickPart(p)}>
              {p.name}
              <b>{p.have}/{p.need}</b>
            </button>
          {/each}
        </div>
        {#if selPart && (loadingSellers || sellers)}
          <div class="sellers">
            {#if loadingSellers}
              <div class="muted">Loading sellers from warframe.market…</div>
            {:else if !sellers.length}
              <div class="muted">No sellers found on warframe.market.</div>
            {:else}
              {#each sellers as o}
                <div class="seller">
                  <span class="dot {o.status}" title={o.status}></span>
                  <span class="sname">{o.user}</span>
                  <span class="muted">{o.platinum}p · ×{o.quantity}</span>
                  <button class="btn copy" class:done={copied === o.whisper}
                          onclick={() => copy(o.whisper)}>
                    {copied === o.whisper ? "Copied ✓" : "Copy whisper"}
                  </button>
                </div>
              {/each}
            {/if}
          </div>
        {/if}
      {/if}
    {/each}
  </div>
{/if}

<style>
.controls {
  display: flex; flex-wrap: wrap; gap: 18px; align-items: center; margin: 6px 0;
}
.sortlabel { display: flex; gap: 8px; align-items: center; }
.sortlabel select {
  font: inherit; color: var(--text); background: var(--panel2);
  border: 1px solid var(--border, #333); border-radius: 8px; padding: 3px 8px; cursor: pointer;
}
.metric {
  margin-right: 12px; font-size: 12px; font-weight: 600;
  color: var(--gold); white-space: nowrap;
}
.metric.dim { color: #6a6d77; font-weight: 400; }
.row { cursor: pointer; }
.row.active { background: var(--panel2); }
.parts {
  display: flex; flex-wrap: wrap; gap: 6px;
  padding: 6px 12px 10px 56px; background: var(--panel2);
}
.part {
  display: inline-flex; align-items: baseline; gap: 5px;
  padding: 3px 9px; border-radius: 10px; cursor: pointer;
  font: inherit; font-size: 12px; color: #d98c5f;
  background: rgba(217,140,95,0.12); border: 1px solid transparent;
}
.part:hover { border-color: currentColor; }
.part.sel { border-color: currentColor; }
.part.have { color: var(--green); background: rgba(120,190,120,0.12); }
.part b { font-weight: 600; }
.sellers {
  display: flex; flex-direction: column; gap: 4px;
  padding: 4px 12px 12px 56px; background: var(--panel2);
}
.seller { display: flex; align-items: center; gap: 10px; }
.seller .sname { font-weight: 600; min-width: 0; }
.seller .muted { margin-right: auto; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: #6a6d77; flex: none; }
.dot.ingame { background: var(--green); }
.dot.online { background: var(--gold); }
.btn.copy { padding: 2px 10px; font-size: 12px; }
.btn.copy.done { color: var(--green); }
</style>
