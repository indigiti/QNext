import test from "node:test";
import assert from "node:assert/strict";
import { CanonicalBarBook } from "../dist/qnext/reconcile.js";

const base = {
  instrumentId: "QNEXT:NIFTY",
  timeframe: "1m",
  openTimeMs: 1000,
  open: 100,
  high: 102,
  low: 99,
  close: 101,
  volume: 10,
  final: false,
  revision: 0,
  quality: "GOOD",
};

test("forming bars update at the same revision, final bars become stable", () => {
  const book = new CanonicalBarBook();
  assert.equal(book.apply(base), true);
  assert.equal(book.apply({ ...base, close: 101.5 }), true);
  assert.equal(book.apply({ ...base, close: 102, final: true }), true);
  assert.equal(book.apply({ ...base, close: 99, final: false }), false);
  assert.equal(book.snapshot()[0].close, 102);
});

test("higher revision corrections replace finalized history", () => {
  const book = new CanonicalBarBook();
  book.apply({ ...base, final: true, revision: 1 });
  assert.equal(book.apply({ ...base, close: 103, final: true, revision: 2 }), true);
  assert.equal(book.apply({ ...base, close: 80, final: true, revision: 1 }), false);
  assert.equal(book.snapshot()[0].close, 103);
});
