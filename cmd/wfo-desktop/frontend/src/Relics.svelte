<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
let { loaded, status } = $props();

let view = $state(null);
let sort = $state("era");      // era | value | count
let selected = $state("");     // "name|refinement" of the expanded relic

// Fetch on load and whenever the sort changes (the backend orders the rows).
// The relic drop tables load in the background and can lag the inventory, so
// retry a few times while the result is still empty before settling.
$effect(() => {
  const mode = sort;
  if (!loaded) return;
  let cancelled = false, tries = 0;
  const go = () => Service.GetRelics(mode).then((v) => {
    if (cancelled) return;
    view = v;
    if (!v?.items?.length && tries++ < 8) setTimeout(go, 1500);
  });
  view = null;
  go();
  return () => { cancelled = true; };
});

const eraColor = {
  Lith: "#5fc7a0", Meso: "#5c9bd6", Neo: "#c79bff", Axi: "#f2b134", Requiem: "#e0815a",
};
const rarityColor = { Common: "#b06a3a", Uncommon: "#aeb6c0", Rare: "var(--gold)" };

function key(r) { return r.name + "|" + r.refinement; }
function pick(r) { const k = key(r); selected = selected === k ? "" : k; }
</script>

<header class="head"><h1>Relics</h1></header>
{#if !loaded || !view}
  <div class="empty">{status || "Reading your relics…"}</div>
{:else if !view.items.length}
  <div class="empty">No relics found in your inventory.</div>
{:else}
  <div class="chips" style="margin:8px 0 4px">
    <span class="chip" style="color:var(--gold)">{view.total} relics</span>
    <span class="chip muted">{view.types} distinct</span>
  </div>
  <div class="controls">
    <label class="muted sortlabel">
      Sort by
      <select bind:value={sort}>
        <option value="era">Era</option>
        <option value="value">Value (best)</option>
        <option value="count">Count</option>
      </select>
    </label>
  </div>
  <div class="scroll">
    {#each view.items as r (key(r))}
      <div class="row" class:active={key(r) === selected}
           onclick={() => pick(r)} role="button" tabindex="0">
        <span class="era" style="background:{eraColor[r.era] || '#555'}22; color:{eraColor[r.era] || '#aaa'}">{r.era}</span>
        <span class="iname">{r.name}</span>
        <span class="refine" class:radiant={r.refinement === 'Radiant'}>{r.refinement}</span>
        <span class="muted" style="margin-right:12px">≈{r.value}p</span>
        <span class="badge count">×{r.count}</span>
      </div>
      {#if key(r) === selected && r.rewards}
        <div class="rewards">
          {#each r.rewards as rw}
            <div class="rw">
              <span class="dot" style="background:{rarityColor[rw.rarity] || '#777'}" title={rw.rarity}></span>
              <span class="rname" class:owned={rw.owned > 0}>{rw.part}</span>
              {#if rw.owned > 0}<span class="have" title="you own {rw.owned}">✓{rw.owned > 1 ? ' ×' + rw.owned : ''}</span>{/if}
              <span class="muted chance">{rw.chance}%</span>
              <span class="val">{rw.plat ? rw.plat + 'p' : '—'}{#if rw.ducats} · {rw.ducats}d{/if}</span>
            </div>
          {/each}
        </div>
      {/if}
    {/each}
  </div>
{/if}

<style>
.controls { display: flex; flex-wrap: wrap; gap: 18px; align-items: center; margin: 6px 0; }
.sortlabel { display: flex; gap: 8px; align-items: center; }
.sortlabel select {
  font: inherit; color: var(--fg); background: var(--panel2);
  border: 1px solid #333; border-radius: 8px; padding: 3px 8px; cursor: pointer;
}
.row { cursor: pointer; }
.row.active { background: var(--panel2); }
.era {
  flex: none; width: 52px; text-align: center; font-size: 11px; font-weight: 700;
  padding: 3px 0; border-radius: 6px; letter-spacing: 0.3px;
}
.refine { font-size: 12px; color: var(--dim); margin-right: 12px; }
.refine.radiant { color: var(--gold); font-weight: 600; }
.badge.count { color: var(--fg); background: var(--panel2); }
.rewards {
  display: flex; flex-direction: column; gap: 5px;
  padding: 6px 12px 12px 64px; background: var(--panel2);
}
.rw { display: flex; align-items: center; gap: 10px; font-size: 13px; }
.dot { width: 9px; height: 9px; border-radius: 50%; flex: none; }
.rw .rname { min-width: 0; }
.rw .rname.owned { color: var(--green); }
.rw .have { color: var(--green); font-size: 12px; font-weight: 600; }
.rw .chance { margin-left: auto; min-width: 44px; text-align: right; }
.rw .val { min-width: 70px; text-align: right; color: var(--gold); }
</style>
