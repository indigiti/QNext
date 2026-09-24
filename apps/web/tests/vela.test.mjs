import test from "node:test";
import assert from "node:assert/strict";
import { bindQNextProviderToVela } from "../dist/qnext/vela.js";

test("Vela binding forwards only provider presentation events", () => {
  let listener;
  const provider = {
    subscribe(fn) {
      listener = fn;
      fn({ type: "status", status: { phase: "idle" } });
      return () => { listener = undefined; };
    }
  };
  const seen = { bars: [], phases: [] };
  const chart = {
    replaceBars(bars) { seen.bars.push([...bars]); },
    setConnectionStatus(status) { seen.phases.push(status.phase); }
  };

  const unbind = bindQNextProviderToVela(provider, chart);
  listener({ type: "bars", bars: [{ instrumentId: "QNEXT:NIFTY", timeframe: "1m", openTimeMs: 1 }] });
  listener({ type: "status", status: { phase: "live" } });

  assert.equal(seen.bars.length, 1);
  assert.deepEqual(seen.phases, ["idle", "live"]);
  unbind();
});
