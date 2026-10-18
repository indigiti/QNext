# Dhan Dynamic Synthetic Resilience

Status: **CI candidate**

QNext synthetic option baskets are discovered from Upstox and may roll at runtime as ATM or expiry changes. Static Dhan option mappings therefore cannot be the authority for auto-leg mode.

## Authority model

Every active market underlying is always registered with an Upstox authority policy.

Dhan is capability-based:

- when an active underlying has a configured Dhan mapping, that underlying receives Upstox-primary / Dhan-secondary authority;
- an active underlying with no Dhan mapping remains Upstox-only and is not dropped;
- fixed synthetic legs receive Dhan authority only when an explicit Dhan mapping exists;
- auto synthetic legs initially stay on the existing Upstox path while Dhan discovery runs asynchronously.

## Dynamic option mapping

For an auto synthetic option leg, QNext uses the canonical option identity:

```text
EXCHANGE:UNDERLYING:YYYY-MM-DD:STRIKE:CE|PE
```

The Dhan mapper:

1. identifies the active QNext market from exchange + normalized underlying alias;
2. uses that market's configured Dhan underlying security ID;
3. fetches Dhan's option chain for the canonical expiry;
4. caches the entire underlying/expiry chain;
5. resolves CE/PE security IDs for each observed canonical strike;
6. registers the Dhan provider mapping without changing the canonical instrument ID;
7. adds the option key to the live Dhan LTP poll set;
8. adds Upstox-primary / Dhan-secondary authority policy for that exact leg.

The mapping work runs outside the provider tick path. Until a leg is mapped, Upstox ticks continue to feed the synthetic engine exactly as before.

## Rate and polling boundaries

Dhan option-chain discovery is throttled to one request every three seconds and cached per underlying/expiry. Dhan market-quote polling remains at the configured interval (minimum one second) and uses one dynamic key snapshot, allowing new option security IDs to join without restarting Market Core.

## Recovery

This slice provides **live quote failover** for dynamically mapped option legs. Dynamic option legs are intentionally non-recoverable through Dhan historical candles in this phase. Historical canonical corrections continue through QNext's existing recovery/revision path.

## Failure behavior

Failure to resolve a Dhan option security ID does not interrupt Upstox, candle formation, synthetic calculation, or WebSocket fan-out. The mapping error is observable through Dhan provider error metrics/logging and may be retried when the option leg is observed again.
