import test from "node:test";
import assert from "node:assert/strict";
import { QNextProvider } from "../dist/qnext/QNextProvider.js";

class FakeSocket {
  readyState = 1;
  sent = [];
  listeners = new Map();
  send(data) { this.sent.push(JSON.parse(data)); }
  close() { this.emit("close", {}); }
  addEventListener(type, fn) {
    const list = this.listeners.get(type) ?? [];
    list.push(fn);
    this.listeners.set(type, list);
  }
  emit(type, event) {
    for (const fn of this.listeners.get(type) ?? []) fn(event);
  }
}

function history(close = 100) {
  return new Response(JSON.stringify({ bars: [{
    instrument_id: "QNEXT:NIFTY",
    timeframe: "1m",
    open_time_ms: 1000,
    open: 99, high: 101, low: 98, close, volume: 10,
    final: true, revision: 0, quality: "GOOD"
  }]}), { status: 200, headers: { "content-type": "application/json" } });
}

test("provider loads history before subscribing to live stream", async () => {
  const socket = new FakeSocket();
  const provider = new QNextProvider({
    restBaseUrl: "https://example.test",
    streamUrl: "wss://example.test/api/v1/stream",
    fetch: async () => history(),
    webSocketFactory: () => socket,
  });

  await provider.start({ symbol: "QNEXT:NIFTY", timeframe: "1m" });
  assert.equal(provider.bars().length, 1);
  socket.emit("open", {});
  assert.deepEqual(socket.sent[0], { op: "hello", protocol: "QNEXT.STREAM/1" });
  assert.deepEqual(socket.sent[1], {
    op: "subscribe", channel: "bars", symbol: "QNEXT:NIFTY", timeframe: "1m"
  });
});

test("sequence gaps force REST resync and a fresh subscription", async () => {
  const socket = new FakeSocket();
  let historyCalls = 0;
  const provider = new QNextProvider({
    restBaseUrl: "https://example.test",
    streamUrl: "wss://example.test/api/v1/stream",
    fetch: async () => { historyCalls += 1; return history(historyCalls === 1 ? 100 : 105); },
    webSocketFactory: () => socket,
  });

  await provider.start({ symbol: "QNEXT:NIFTY", timeframe: "1m" });
  socket.emit("open", {});
  socket.emit("message", { data: JSON.stringify({
    op: "subscribed", stream_id: "s1", channel: "bars",
    symbol: "QNEXT:NIFTY", timeframe: "1m"
  })});
  await new Promise(resolve => setTimeout(resolve, 0));
  socket.emit("message", { data: JSON.stringify({
    channel: "bars", stream_id: "s1", seq: 2,
    symbol: "QNEXT:NIFTY", timeframe: "1m", quality: "GOOD",
    bar: { time: 2000, open: 100, high: 101, low: 99, close: 100.5, volume: 5, final: true, revision: 0 }
  })});
  await new Promise(resolve => setTimeout(resolve, 10));

  assert.equal(historyCalls, 2);
  assert.equal(provider.bars()[0].close, 105);
  assert.equal(socket.sent.at(-1).op, "subscribe");
});

test("reconnect prefers RESUME when a cursor exists", async () => {
  const sockets = [new FakeSocket(), new FakeSocket()];
  let index = 0;
  const provider = new QNextProvider({
    restBaseUrl: "https://example.test",
    streamUrl: "wss://example.test/api/v1/stream",
    fetch: async () => history(),
    webSocketFactory: () => sockets[index++],
  });

  await provider.start({ symbol: "QNEXT:NIFTY", timeframe: "1m" });
  sockets[0].emit("open", {});
  sockets[0].emit("message", { data: JSON.stringify({
    op: "subscribed", stream_id: "s1", channel: "bars",
    symbol: "QNEXT:NIFTY", timeframe: "1m"
  })});
  sockets[0].emit("message", { data: JSON.stringify({
    channel: "bars", stream_id: "s1", seq: 1,
    symbol: "QNEXT:NIFTY", timeframe: "1m", quality: "GOOD",
    bar: { time: 2000, open: 100, high: 101, low: 99, close: 100.5, volume: 5, final: false, revision: 0 }
  })});
  await new Promise(resolve => setTimeout(resolve, 0));

  provider.reconnect();
  sockets[1].emit("open", {});
  assert.deepEqual(sockets[1].sent[1], { op: "resume", stream_id: "s1", after_seq: 1 });
});
