<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
let { loaded, status } = $props();

let view = $state(null);
let sort = $state("era");      // era | value | count
let search = $state("");
let detail = $state(null);     // relic whose full drop table is open

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
function rwStatus(rw) {
  if (rw.mastered) return "mastered";
  if (rw.crafted) return "crafted";
  if (rw.owned > 0) return "owned";
  return "new";
}
function rwTitle(rw) {
  const s = { mastered: "set mastered", crafted: "set built/owned", owned: `owned ×${rw.owned}`, new: "new" }[rwStatus(rw)];
  return `${rw.part} — ${s}`;
}
let items = $derived(
  !view ? [] : view.items.filter((r) => !search || r.name.toLowerCase().includes(search.toLowerCase()))
);
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
    <input class="csearch" placeholder="Search…" bind:value={search} />
  </div>
  <div class="scroll">
    <div class="grid relics">
      {#each items as r (key(r))}
        <div class="card relic" onclick={() => (detail = r)} role="button" tabindex="0" title="Show full drop table">
          <div class="rhead">
            <span class="era" style="background:{eraColor[r.era] || '#555'}22; color:{eraColor[r.era] || '#aaa'}">{r.era}</span>
            <span class="rname">{r.name}</span>
            <span class="rcount">×{r.count}</span>
          </div>
          <div class="rsub">
            <span class="refine" class:radiant={r.refinement === 'Radiant'}>{r.refinement}</span>
            <span class="rval">≈{r.value}p</span>
          </div>
          <div class="ricons">
            {#each r.rewards as rw}
              <div class="ric {rwStatus(rw)}" title={rwTitle(rw)}>
                {#if rw.icon}<img src={rw.icon} alt="" loading="lazy" class:dim={rwStatus(rw) === 'new'} />{:else}<span class="ph"></span>{/if}
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
{/if}

{#if detail}
  <div class="modal-bg" role="presentation" onclick={(e) => { if (e.target === e.currentTarget) detail = null; }}>
    <div class="modal" role="dialog" aria-modal="true" tabindex="-1">
      <div class="modal-head">
        <span class="era" style="background:{eraColor[detail.era] || '#555'}22; color:{eraColor[detail.era] || '#aaa'}">{detail.era}</span>
        <h2>{detail.name} <span class="muted" style="font-weight:400">{detail.refinement} · ×{detail.count} · ≈{detail.value}p</span></h2>
        <button class="xbtn" onclick={() => (detail = null)} aria-label="Close">✕</button>
      </div>
      <div class="modal-body">
        {#each detail.rewards as rw}
          <div class="rw">
            <span class="dot" style="background:{rarityColor[rw.rarity] || '#777'}" title={rw.rarity}></span>
            {#if rw.icon}<img class="rwimg" class:dim={rwStatus(rw) === 'new'} src={rw.icon} alt="" loading="lazy" />{/if}
            <span class="rname2" class:done={rwStatus(rw) !== 'new'}>{rw.part}</span>
            {#if rw.mastered}<span class="tag mastered">★ Mastered</span>
            {:else if rw.crafted}<span class="tag crafted">✓ Crafted</span>
            {:else if rw.owned > 0}<span class="tag owned">✓ Owned{rw.owned > 1 ? ' ×' + rw.owned : ''}</span>
            {:else}<span class="tag new">✦ New</span>{/if}
            <span class="muted chance">{rw.chance}%</span>
            <span class="val">{rw.plat ? rw.plat + 'p' : '—'}{#if rw.ducats} · {rw.ducats}d{/if}</span>
          </div>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
.controls { display: flex; flex-wrap: wrap; gap: 16px; align-items: center; margin: 6px 0; }
.sortlabel { display: flex; gap: 8px; align-items: center; }
.sortlabel select {
  font: inherit; color: var(--fg); background: var(--panel2);
  border: 1px solid #333; border-radius: 8px; padding: 3px 8px; cursor: pointer;
}
.csearch { background: var(--panel); border: none; border-radius: 8px; padding: 6px 10px; color: var(--fg); outline: none; min-width: 150px; }

.grid.relics { grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); }
.card.relic { align-items: stretch; gap: 9px; padding: 11px 12px; cursor: pointer; }
.rhead { display: flex; align-items: center; gap: 8px; }
.era { flex: none; font-size: 11px; font-weight: 700; padding: 2px 7px; border-radius: 6px; letter-spacing: 0.3px; }
.rname { flex: 1; font-weight: 600; font-size: 14px; min-width: 0; }
.rcount { color: var(--fg); font-weight: 600; font-size: 13px; }
.rsub { display: flex; align-items: center; justify-content: space-between; font-size: 12px; }
.refine { color: var(--dim); }
.refine.radiant { color: var(--gold); font-weight: 600; }
.rval { color: var(--gold); font-weight: 600; }
.ricons { display: flex; flex-wrap: wrap; gap: 6px; }
.ric { width: 34px; height: 34px; border-radius: 7px; background: #20222a; display: grid; place-items: center;
  border: 1.5px solid #34384400; box-shadow: inset 0 0 0 1.5px #34384400; }
.ric img { width: 30px; height: 30px; object-fit: contain; }
.ric img.dim { opacity: 0.45; }
.ric .ph { width: 18px; height: 18px; border-radius: 4px; background: #2c2f39; }
.ric.mastered { box-shadow: inset 0 0 0 2px var(--gold); }
.ric.crafted { box-shadow: inset 0 0 0 2px var(--blue); }
.ric.owned { box-shadow: inset 0 0 0 2px var(--green); }
.ric.new { opacity: 0.92; }

.modal-head .era { margin-right: 2px; }
.rw { display: flex; align-items: center; gap: 10px; font-size: 13px; padding: 3px 0; }
.dot { width: 9px; height: 9px; border-radius: 50%; flex: none; }
.rwimg { width: 28px; height: 28px; object-fit: contain; flex: none; }
.rwimg.dim { opacity: 0.4; }
.rname2 { flex: 1; min-width: 0; }
.rname2.done { color: var(--green); }
.tag { font-size: 11px; font-weight: 600; padding: 1px 7px; border-radius: 9px; white-space: nowrap; }
.tag.mastered { color: var(--gold); background: rgba(242,177,52,0.14); }
.tag.crafted { color: var(--blue); background: rgba(92,155,214,0.14); }
.tag.owned { color: var(--green); background: rgba(95,199,160,0.14); }
.tag.new { color: #6a6d77; background: rgba(255,255,255,0.04); }
.chance { min-width: 44px; text-align: right; }
.val { min-width: 70px; text-align: right; color: var(--gold); }
</style>
