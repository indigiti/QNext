# Staging-first QNeXT admin authentication hardening

- Unauthenticated admin sign-in: 12 attempts per 15-minute window, keyed on PHP's REMOTE_ADDR.
- One-time recovery: 6 attempts per 15-minute window. HTTP 429 includes Retry-After; success clears the counter.
- Do not trust arbitrary X-Forwarded-For; on shared proxies validate true client-IP configuration before rollout.
- Authenticated POST, PUT and DELETE requests relying on the admin session cookie must include X-QNext-Admin-Request: 1. Token-based API clients remain supported.
- Source UI request helpers send the header. This is a CSRF defense-in-depth header, not a credential.
- Public SYN+ paper markers are disabled by default unless PHP web runtime explicitly enables QNEXT_PUBLIC_PAPER_MARKERS=1. This opt-in publishes executions to unauthenticated viewers.

## Staging acceptance
1. Login/logout/recovery must work for correct credentials; invalid credentials should receive 403 then 429 after repeated attempts.
2. Cookie-authenticated mutations without the header must be rejected, while header-bearing requests succeed.
3. Header-token API callers remain unaffected.
4. Public paper-marker snapshots remain empty without opt-in.
5. Check proxy IP attribution and shared-user lockouts before promoting production.

Rollback by returning to the prior versioned release. No persistent market-data migration is needed.
