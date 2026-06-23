<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
import CraftTree from "./CraftTree.svelte";
let { loaded, status, load } = $props();

let categories = $state([]);
let search = $state("");
let modalItem = $state(null);

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
      <div class="grid">
        {#each cat.items as it}
          <div class="card" class:owned={!it.mastered} class:mastered={it.mastered}
               onclick={() => (modalItem = it)} role="button" tabindex="0" title="Show crafting tree">
            {#if it.mastered}<span class="star">★</span>{/if}
            {#if it.icon}<img class="cthumb" src={it.icon} alt="" loading="lazy" />{:else}<div class="cthumb ph"></div>{/if}
            <span class="cname">{it.name}</span>
            <span class="cbadge" class:mr={it.mastered}>{it.mastered ? `Mastered ${it.rank}` : `Rank ${it.rank}/${it.maxRank}`}</span>
          </div>
        {/each}
      </div>
    {/each}
  </div>
{:else}
  <div class="empty">{status}</div>
{/if}

{#if modalItem}
  <CraftTree item={modalItem} onClose={() => (modalItem = null)} />
{/if}

<style>
.cbadge { color: var(--dim); }
.cbadge.mr { color: var(--green); }
</style>
