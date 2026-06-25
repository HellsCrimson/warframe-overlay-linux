<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
import { Browser } from "@wailsio/runtime";
let { item, onClose } = $props();

// Warframe wiki article path; page titles use underscores for spaces.
function wikiURL(name) {
  return "https://wiki.warframe.com/w/" + encodeURIComponent((name || "").replace(/ /g, "_"));
}

let tree = $state(null);
let hideDone = $state(false);
let selPart = $state("");       // name of the part whose sellers are shown
let sellers = $state(null);
let loadingSellers = $state(false);
let copied = $state("");

$effect(() => {
  tree = null;
  if (item?.name) Service.GetCraftingTree(item.name).then((t) => (tree = t));
});

// Collect leaf nodes (recipe inputs) for the "still needed" summary.
function leaves(n, acc = []) {
  if (!n) return acc;
  if (!n.children || !n.children.length) { acc.push(n); return acc; }
  for (const c of n.children) leaves(c, acc);
  return acc;
}
let missing = $derived(tree ? leaves(tree).filter((l) => !l.enough) : []);
let missingParts = $derived(missing.filter((l) => !l.isResource));
let missingRes = $derived(missing.filter((l) => l.isResource));
let hasRecipe = $derived(!!tree && tree.children && tree.children.length > 0);

// A node is "done" when it and every input beneath it are satisfied.
function allEnough(n) {
  if (!n) return true;
  if (n.children && n.children.length) return n.children.every(allEnough);
  return n.enough;
}

function pickPart(n) {
  if (n.isResource || n.enough || (n.children && n.children.length)) return;
  copied = "";
  if (selPart === n.name) { selPart = ""; sellers = null; return; }
  selPart = n.name; sellers = null; loadingSellers = true;
  Service.PartSellers(n.name).then((rows) => {
    if (selPart === n.name) { sellers = rows || []; loadingSellers = false; }
  });
}
function copy(t) { navigator.clipboard.writeText(t); copied = t; }
function onKey(e) { if (e.key === "Escape") onClose(); }
</script>

<svelte:window onkeydown={onKey} />

<div class="modal-bg" role="presentation"
     onclick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
  <div class="modal" role="dialog" aria-modal="true" tabindex="-1">
    <div class="modal-head">
      {#if item?.icon}<img class="hthumb" src={item.icon} alt="" />{/if}
      <h2>{item?.name} — crafting tree</h2>
      <button class="btn ghost wiki" title="Open this item on the Warframe wiki"
              onclick={() => Browser.OpenURL(wikiURL(item?.name))}>Wiki ↗</button>
      <label class="muted tg" title="Hide recipes already completed">
        <input type="checkbox" bind:checked={hideDone} /> Hide completed
      </label>
      <button class="xbtn" onclick={onClose} aria-label="Close">✕</button>
    </div>

    <div class="modal-body">
      {#if !tree}
        <div class="muted">Loading recipe…</div>
      {:else if !hasRecipe}
        <div class="muted">No recipe data for this item.</div>
      {:else}
        {@render node(tree, 0)}
      {/if}
    </div>

    {#if hasRecipe}
      <div class="modal-foot">
        <div class="needline">
          <span class="muted nlabel">Blueprints / parts needed:</span>
          {#if missingParts.length}
            {#each missingParts as p}<span class="need part">{p.name} <b>×{p.need - p.have}</b></span>{/each}
          {:else}<span class="ok">none — ready to build ✓</span>{/if}
        </div>
        <div class="needline">
          <span class="muted nlabel">Resources missing:</span>
          {#if missingRes.length}
            {#each missingRes as r}<span class="need res">{r.name} <b>×{r.need - r.have}</b></span>{/each}
          {:else}<span class="ok">none ✓</span>{/if}
        </div>
      </div>
    {/if}
  </div>
</div>

{#snippet node(n, depth)}
  {#if !hideDone || !allEnough(n)}
    {@const isLeaf = !n.children || !n.children.length}
    <div class="cnode">
      <button class="cleaf" class:enough={n.enough} class:branch={!isLeaf}
              class:clickable={isLeaf && !n.isResource && !n.enough}
              class:sel={selPart === n.name}
              onclick={() => pickPart(n)}
              disabled={!isLeaf || n.isResource || n.enough}>
        {#if n.icon}<img class="lthumb" src={n.icon} alt="" loading="lazy" />{:else}<span class="lthumb ph"></span>{/if}
        <span class="lname">{n.name}</span>
        {#if isLeaf && depth > 0 && n.need > 0}
          <span class="lqty" class:ok={n.enough}>{n.have}/{n.need}</span>
        {/if}
        {#if isLeaf && !n.isResource && !n.enough}<span class="hint">find sellers ›</span>{/if}
      </button>

      {#if selPart === n.name && (loadingSellers || sellers)}
        <div class="sellers">
          {#if loadingSellers}
            <div class="muted">Loading sellers from warframe.market…</div>
          {:else if !sellers.length}
            <div class="muted">No sellers found.</div>
          {:else}
            {#each sellers as o}
              <div class="seller">
                <span class="dot {o.status}" title={o.status}></span>
                <span class="sname">{o.user}</span>
                <span class="muted">{o.platinum}p · ×{o.quantity}</span>
                <button class="btn copy" class:done={copied === o.whisper} onclick={() => copy(o.whisper)}>
                  {copied === o.whisper ? "Copied ✓" : "Copy whisper"}
                </button>
              </div>
            {/each}
          {/if}
        </div>
      {/if}

      {#if !isLeaf}
        <div class="ckids">
          {#each n.children as c}{@render node(c, depth + 1)}{/each}
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

<style>
.hthumb { width: 34px; height: 34px; object-fit: contain; }
.tg { display: flex; gap: 7px; align-items: center; cursor: pointer; }
.cnode { display: flex; flex-direction: column; }
.ckids { margin-left: 14px; padding-left: 12px; border-left: 1px solid #2a2d37; }
.cleaf {
  display: flex; align-items: center; gap: 9px; width: 100%; text-align: left;
  background: transparent; border: 1px solid transparent; border-radius: 8px;
  padding: 5px 8px; color: var(--fg); font: inherit; margin: 2px 0;
}
.cleaf.branch { font-weight: 600; }
.cleaf.enough { color: var(--green); }
.cleaf.clickable { cursor: pointer; }
.cleaf.clickable:hover { border-color: #3a3d49; background: #1f2128; }
.cleaf.sel { border-color: var(--gold); }
.cleaf:disabled { cursor: default; }
.lthumb { width: 30px; height: 30px; object-fit: contain; flex: none; }
.lthumb.ph { background: #20222a; border-radius: 4px; }
.lname { flex: 1; min-width: 0; }
.lqty { font-size: 12px; font-weight: 600; color: var(--red); }
.lqty.ok { color: var(--green); }
.hint { font-size: 11px; color: var(--gold); opacity: 0.85; }
.sellers { display: flex; flex-direction: column; gap: 4px; padding: 4px 8px 8px 48px; }
.seller { display: flex; align-items: center; gap: 10px; }
.seller .sname { font-weight: 600; min-width: 0; }
.seller .muted { margin-right: auto; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: #6a6d77; flex: none; }
.dot.ingame { background: var(--green); }
.dot.online { background: var(--gold); }
.btn.wiki { padding: 4px 10px; font-size: 12px; }
.btn.copy { padding: 2px 10px; font-size: 12px; }
.btn.copy.done { color: var(--green); }
.needline { display: flex; flex-wrap: wrap; gap: 8px; align-items: baseline; margin: 3px 0; }
.nlabel { min-width: 168px; }
.need { font-size: 12px; padding: 2px 8px; border-radius: 9px; }
.need.part { color: #d98c5f; background: rgba(217,140,95,0.12); }
.need.res { color: var(--blue); background: rgba(92,155,214,0.12); }
.need b { font-weight: 700; }
.ok { color: var(--green); font-size: 13px; }
</style>
