<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
import { Events } from "@wailsio/runtime";
let { loaded, status } = $props();

let builds = $state([]);
let now = $state(Date.now());
let toasts = $state([]); // {id, text}
let toastSeq = 0;

async function refresh() {
  builds = (await Service.GetFoundry()) || [];
}

$effect(() => {
  if (loaded) refresh();
});

// Tick every second so countdowns stay live.
$effect(() => {
  const t = setInterval(() => (now = Date.now()), 1000);
  return () => clearInterval(t);
});

// Backend re-loads the inventory when the game starts/changes.
Events.On("inventory:loaded", refresh);

// Backend fires this the moment a tracked build finishes crafting.
Events.On("foundry:done", (ev) => {
  const name = ev?.data ?? ev;
  const id = ++toastSeq;
  toasts = [...toasts, { id, text: `${name} is ready to claim` }];
  setTimeout(() => (toasts = toasts.filter((t) => t.id !== id)), 8000);
  refresh();
});

function remaining(ms) {
  return ms - now; // negative once done
}
function fmt(ms) {
  const left = remaining(ms);
  if (left <= 0) return "Ready";
  let s = Math.floor(left / 1000);
  const d = Math.floor(s / 86400); s -= d * 86400;
  const h = Math.floor(s / 3600); s -= h * 3600;
  const m = Math.floor(s / 60); s -= m * 60;
  const pad = (n) => String(n).padStart(2, "0");
  if (d > 0) return `${d}d ${pad(h)}:${pad(m)}:${pad(s)}`;
  return `${pad(h)}:${pad(m)}:${pad(s)}`;
}

let pending = $derived(builds.filter((b) => remaining(b.completionMs) > 0).length);
let ready = $derived(builds.length - pending);
</script>

<header class="head">
  <h1>Foundry</h1>
  <span class="muted">
    {#if loaded}{builds.length} building{#if ready > 0} · {ready} ready{/if}{:else}{status}{/if}
  </span>
  <button class="btn ghost spacer" onclick={refresh}>Refresh</button>
</header>

{#if loaded}
  {#if builds.length === 0}
    <div class="empty">Nothing is crafting right now.</div>
  {:else}
    <div class="scroll">
      <div class="grid">
        {#each builds as b (b.name + b.completionMs)}
          {@const done = remaining(b.completionMs) <= 0}
          <div class="card" class:owned={done}>
            {#if b.icon}<img class="cthumb" src={b.icon} alt="" loading="lazy" />{:else}<div class="cthumb ph"></div>{/if}
            <span class="cname">{b.name.replace(/ Blueprint$/, "")}</span>
            <span class="cbadge" class:rdy={done}>{fmt(b.completionMs)}</span>
          </div>
        {/each}
      </div>
    </div>
  {/if}
{:else}
  <div class="empty">{status}</div>
{/if}

<div class="toasts">
  {#each toasts as t (t.id)}
    <div class="toast">✓ {t.text}</div>
  {/each}
</div>

<style>
.cbadge { color: var(--dim); font-variant-numeric: tabular-nums; }
.cbadge.rdy { color: var(--green); font-weight: 700; }
.toasts { position: fixed; right: 18px; bottom: 18px; display: flex; flex-direction: column; gap: 8px; z-index: 50; }
.toast { background: var(--panel2); color: var(--fg); border-left: 3px solid var(--green);
  padding: 10px 14px; border-radius: 6px; box-shadow: 0 4px 16px rgba(0,0,0,0.4); font-size: 13px; }
</style>
