<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
let { loaded, status, load } = $props();

let categories = $state([]);
let search = $state("");

$effect(() => {
  if (loaded) Service.GetInventory().then((c) => (categories = c || []));
});

let total = $derived(categories.reduce((n, c) => n + c.items.length, 0));
let filtered = $derived(
  categories.map((c) => ({
    name: c.name,
    items: c.items.filter((it) => !search || it.name.toLowerCase().includes(search.toLowerCase())),
  })).filter((c) => c.items.length > 0)
);
</script>

<header class="head">
  <h1>Inventory</h1>
  <span class="muted">{#if loaded}{total} items across {categories.length} categories{:else}{status}{/if}</span>
  <button class="btn spacer" onclick={load}>Reload</button>
</header>
<input class="search" placeholder="Search items…" bind:value={search} />

{#if loaded}
  <div class="scroll">
    {#each filtered as cat}
      <div class="cat">{cat.name} <span class="muted">({cat.items.length})</span></div>
      {#each cat.items as it}
        <div class="row">
          {#if it.icon}<img class="thumb" src={it.icon} alt="" loading="lazy" />{:else}<div class="thumb ph"></div>{/if}
          <span class="iname">{it.name}</span>
          <span class="rank" class:mastered={it.mastered}>{it.mastered ? "★ " + it.rank : "rank " + it.rank}</span>
        </div>
      {/each}
    {/each}
  </div>
{:else}
  <div class="empty">{status}</div>
{/if}

<style>
.rank { color: var(--dim); font-size: 13px; }
.rank.mastered { color: var(--green); }
</style>
