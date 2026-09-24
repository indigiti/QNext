# Golden Fixtures

Golden fixtures are immutable test inputs used for parity and certification.

Initial fixture families:

- provider ticks
- canonical ticks
- NIFTY candles
- NIFTY-SYN input option legs
- NIFTY-SYN expected synthetic values
- history/live boundary cases
- duplicate/late/out-of-order ticks
- provider failover sequences

Every fixture must declare its schema version and expected engine version.
