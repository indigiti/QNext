const nativeFetch = globalThis.fetch.bind(globalThis);
const ADMIN_API_MARKER = '/qnext/admin/api/index.php';
const TOKEN_HEADER = 'X-QNext-Ops-Token';
const SESSION_ROUTE = '/session';
const RESTORED_EVENT = 'qnext-ops-auth-restored';

let sessionExchange: Promise<boolean> | null = null;

function routeURL(base: URL, route: string): URL {
  const url = new URL(base.toString());
  url.searchParams.set('route', route);
  return url;
}

function adminURL(input: RequestInfo | URL): URL | null {
  try {
    const url = input instanceof Request
      ? new URL(input.url, window.location.href)
      : new URL(String(input), window.location.href);
    return url.pathname.endsWith(ADMIN_API_MARKER) ? url : null;
  } catch {
    return null;
  }
}

function clearBrowserToken(): void {
  try {
    sessionStorage.removeItem('qnext-ops-token');
  } catch {
    // Hardened browsers can deny storage access.
  }

  const tokenInput = document.querySelector<HTMLInputElement>('#token');
  if (tokenInput) tokenInput.value = '';
  try {
    window.dispatchEvent(new CustomEvent(RESTORED_EVENT));
  } catch {
    // Non-browser tests can omit CustomEvent.
  }
}

async function exchangeToken(base: URL, token: string): Promise<boolean> {
  if (sessionExchange) return sessionExchange;

  sessionExchange = (async () => {
    try {
      const response = await nativeFetch(routeURL(base, SESSION_ROUTE), {
        method: 'POST',
        credentials: 'same-origin',
        cache: 'no-store',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ token }),
      });
      if (!response.ok) return false;
      clearBrowserToken();
      return true;
    } catch {
      return false;
    }
  })();

  try {
    return await sessionExchange;
  } finally {
    sessionExchange = null;
  }
}

function requestParts(input: RequestInfo | URL, init?: RequestInit): {
  headers: Headers;
  method: string;
  route: string | null;
} {
  const request = input instanceof Request ? input : null;
  const headers = new Headers(request?.headers);
  if (init?.headers) {
    new Headers(init.headers).forEach((value, key) => headers.set(key, value));
  }
  const method = (init?.method ?? request?.method ?? 'GET').toUpperCase();
  const url = adminURL(input);
  return {
    headers,
    method,
    route: url?.searchParams.get('route') ?? null,
  };
}

export function installSessionFirstFetch(): void {
  if (typeof window === 'undefined') return;
  if ((globalThis.fetch as typeof fetch & { __qnextSessionFirst?: boolean }).__qnextSessionFirst) return;

  const wrapped: typeof fetch = async (input, init) => {
    const base = adminURL(input);
    if (!base) return nativeFetch(input, init);

    const { headers, route } = requestParts(input, init);
    if (route === SESSION_ROUTE) return nativeFetch(input, init);

    const token = headers.get(TOKEN_HEADER)?.trim() ?? '';
    if (!token) {
      return nativeFetch(input, {
        ...init,
        credentials: 'same-origin',
      });
    }

    const established = await exchangeToken(base, token);
    if (!established) {
      // Preserve the existing error path without repeatedly exposing the token.
      headers.delete(TOKEN_HEADER);
      return nativeFetch(input, {
        ...init,
        headers,
        credentials: 'same-origin',
      });
    }

    headers.delete(TOKEN_HEADER);
    return nativeFetch(input, {
      ...init,
      headers,
      credentials: 'same-origin',
    });
  };

  (wrapped as typeof fetch & { __qnextSessionFirst?: boolean }).__qnextSessionFirst = true;
  globalThis.fetch = wrapped;
}

installSessionFirstFetch();
