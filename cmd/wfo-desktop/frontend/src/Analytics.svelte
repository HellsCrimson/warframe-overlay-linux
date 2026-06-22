<script>
import { Service } from "../bindings/warframe-overlay-linux/cmd/wfo-desktop/index.js";
import * as echarts from "echarts";

let a = $state(null);
let chartEl;
let chart;

async function reload() { a = await Service.GetAnalytics(); }
reload();
const timer = setInterval(reload, 5000); // pick up new trades

$effect(() => {
  if (!a || !chartEl) return;
  if (!chart) chart = echarts.init(chartEl, null, { renderer: "canvas" });
  const data = a.cumulative || [];
  chart.setOption({
    grid: { left: 48, right: 16, top: 16, bottom: 24 },
    xAxis: { type: "category", data: data.map((_, i) => i + 1),
      axisLine: { lineStyle: { color: "#3a3d47" } }, axisLabel: { color: "#80838d" } },
    yAxis: { type: "value", name: "net platinum",
      axisLine: { lineStyle: { color: "#3a3d47" } }, splitLine: { lineStyle: { color: "#23262f" } },
      axisLabel: { color: "#80838d" }, nameTextStyle: { color: "#80838d" } },
    tooltip: { trigger: "axis" },
    series: [{ type: "line", data, smooth: false, symbol: "circle", symbolSize: 6,
      lineStyle: { color: "#f2b134", width: 2 }, itemStyle: { color: "#f2b134" },
      areaStyle: { color: "rgba(242,177,52,0.10)" } }],
  });
});

$effect(() => () => { clearInterval(timer); chart?.dispose(); });

function fmt(n) { return (n >= 0 ? "+" : "") + n; }
</script>

<header class="head"><h1>Analytics</h1></header>
{#if !a}
  <div class="empty">Loading…</div>
{:else}
  <div class="cards">
    <div class="statcard"><div class="lbl">Inventory value</div><div class="val" style="color:var(--gold)">{a.invValue} p</div></div>
    <div class="statcard"><div class="lbl">Total ducats</div><div class="val" style="color:var(--blue)">{a.ducats}</div></div>
    <div class="statcard"><div class="lbl">Sellable parts</div><div class="val" style="color:var(--green)">{a.sellable}</div></div>
    <div class="statcard"><div class="lbl">Trades</div><div class="val">{a.tradeCount}</div></div>
    <div class="statcard"><div class="lbl">Net platinum</div><div class="val" style="color:{a.netPlat>=0?'var(--green)':'var(--red)'}">{fmt(a.netPlat)} p</div></div>
    <div class="statcard"><div class="lbl">Earned / spent</div><div class="val muted" style="font-size:18px">{a.platIn} / {a.platOut} p</div></div>
  </div>

  {#if a.tradeCount > 0}
    <div class="cat">Platinum over time</div>
    <div bind:this={chartEl} style="height:200px; width:100%"></div>
    <div class="cat">Recent trades</div>
    <div class="scroll">
      {#each a.recent as t}
        <div class="row">
          <span class="iname">{t.partner} — gave {t.gave}, got {t.received}</span>
          <span style="color:{t.platDelta>=0?'var(--green)':'var(--red)'}">{fmt(t.platDelta)} p</span>
        </div>
      {/each}
    </div>
  {:else}
    <div class="empty">No trades recorded yet — they'll appear here after you trade in-game.</div>
  {/if}
{/if}
