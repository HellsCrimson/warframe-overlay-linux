<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
let { loaded, status } = $props();

let view = $state(null);
let hideNotStarted = $state(true);
let selected = $state("");

$effect(() => { if (loaded) Service.GetMastery().then((v) => (view = v)); });

const statusColor = {
  "Built — rank up": "var(--gold)", "Ready to build": "var(--green)",
  "Collecting parts": "var(--blue)", "Not started": "#6a6d77",
};
let items = $derived(
  !view ? [] : view.items.filter((it) => !hideNotStarted || it.status !== "Not started")
);
let sel = $derived(items.find((it) => it.name === selected));

function detail(it) {
  if (it.status === "Built — rank up") return `rank ${it.rank} / ${it.maxRank}`;
  if (it.partsTotal > 0) return `${it.partsOwned} / ${it.partsTotal} parts`;
  return "";
}
function copyWTB() { if (sel?.wtb) navigator.clipboard.writeText(sel.wtb); }
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
  <label class="muted" style="margin:6px 0; display:flex; gap:8px; align-items:center; cursor:pointer">
    <input type="checkbox" bind:checked={hideNotStarted} /> Hide not-yet-started items
  </label>
  <div class="scroll">
    {#each items as it}
      <div class="row" class:active={it.name === selected}
           onclick={() => (selected = selected === it.name ? "" : it.name)}
           role="button" tabindex="0">
        {#if it.icon}<img class="thumb" src={it.icon} alt="" loading="lazy" />{:else}<div class="thumb ph"></div>{/if}
        <span class="iname">{it.name}</span>
        <span class="muted" style="margin-right:12px">{detail(it)}</span>
        <span class="badge" style="color:{statusColor[it.status] || '#888'}">{it.status}</span>
      </div>
      {#if it.name === selected && it.parts && it.parts.length}
        <div class="parts">
          {#each it.parts as p}
            <span class="part" class:have={p.have >= p.need} title={p.have >= p.need ? "owned" : `need ${p.need - p.have} more`}>
              {p.name}
              <b>{p.have}/{p.need}</b>
            </span>
          {/each}
        </div>
      {/if}
    {/each}
  </div>
  {#if sel && sel.wtb}
    <div class="bar">
      <div style="flex:1">
        <div class="muted">Buy missing parts for {sel.name}</div>
        <div>{sel.wtb}</div>
      </div>
      <button class="btn" onclick={copyWTB}>Copy WTB</button>
    </div>
  {/if}
{/if}

<style>
.row { cursor: pointer; }
.row.active { background: var(--panel2); }
.parts {
  display: flex; flex-wrap: wrap; gap: 6px;
  padding: 6px 12px 10px 56px; background: var(--panel2);
}
.part {
  display: inline-flex; align-items: baseline; gap: 5px;
  padding: 2px 8px; border-radius: 10px;
  font-size: 12px; color: #d98c5f; background: rgba(217,140,95,0.12);
}
.part.have { color: var(--green); background: rgba(120,190,120,0.12); }
.part b { font-weight: 600; }
</style>
