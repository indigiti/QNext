import type { QNextBar } from "./types.js";

export function barKey(bar: Pick<QNextBar, "instrumentId" | "timeframe" | "openTimeMs">): string {
  return `${bar.instrumentId}|${bar.timeframe}|${bar.openTimeMs}`;
}

export class CanonicalBarBook {
  readonly #bars = new Map<string, QNextBar>();

  replaceHistory(bars: readonly QNextBar[]): void {
    this.#bars.clear();
    for (const bar of bars) this.apply(bar);
  }

  apply(incoming: QNextBar): boolean {
    const key = barKey(incoming);
    const current = this.#bars.get(key);
    if (!current) {
      this.#bars.set(key, incoming);
      return true;
    }

    if (incoming.revision < current.revision) return false;
    if (incoming.revision === current.revision && current.final) return false;

    this.#bars.set(key, incoming);
    return true;
  }

  snapshot(): QNextBar[] {
    return [...this.#bars.values()].sort((a, b) => a.openTimeMs - b.openTimeMs);
  }
}
