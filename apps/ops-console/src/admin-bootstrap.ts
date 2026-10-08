import './admin-auth-gate.css';

declare global {
  interface Window {
    __QNEXT_OPS_CONFIG__?: { apiBase?: string };
  }
}

type SessionStatus = {
  authenticated: boolean;
  expires_in_seconds?: number;
};

type SetupStatus = {
  initialized: boolean;
  recovery_configured?: boolean;
};

const root = document.querySelector<HTMLDivElement>('#app');
if (!root) throw new Error('QNext Admin root not found');

const apiBase = (window.__QNEXT_OPS_CONFIG__?.apiBase ?? '/qnext/admin/api/index.php').replace(/\/$/, '');

function routeURL(route: string): string {
  const separator = apiBase.includes('?') ? '&' : '?';
  return apiBase + separator + 'route=' + encodeURIComponent(route);
}

async function request<T>(route: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body) headers.set('Content-Type', 'application/json');

  const response = await fetch(routeURL(route), {
    ...init,
    headers,
    credentials: 'same-origin',
    cache: 'no-store',
  });

  const text = await response.text();
  let payload: unknown = {};
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      throw new Error(`Admin authentication endpoint returned non-JSON HTTP ${response.status}`);
    }
  }

  if (!response.ok) {
    const message = typeof payload === 'object'
      && payload !== null
      && 'error' in payload
      && typeof (payload as { error?: unknown }).error === 'string'
      ? (payload as { error: string }).error
      : `HTTP ${response.status}`;
    throw new Error(message);
  }

  return payload as T;
}

function escapeHTML(value: unknown): string {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;');
}

async function sessionStatus(): Promise<SessionStatus> {
  return request<SessionStatus>('/session');
}

async function setupStatus(): Promise<SetupStatus> {
  return request<SetupStatus>('/setup-status');
}

function renderGate(setup: SetupStatus, message = ''): void {
  const firstTime = !setup.initialized;
  root.innerHTML = `
    <main class="admin-gate" aria-labelledby="admin-gate-title">
      <section class="admin-gate-card">
        <div class="admin-gate-brand">
          <div class="admin-gate-mark" aria-hidden="true">QN</div>
          <div>
            <p class="admin-gate-eyebrow">QNeXT Control Plane</p>
            <h1 id="admin-gate-title">${firstTime ? 'Initialize Admin access' : 'Admin sign in'}</h1>
          </div>
        </div>
        <p class="admin-gate-copy">
          ${firstTime
            ? 'Enter the configured first-time bootstrap token. The Admin application will not load until authentication succeeds.'
            : 'Enter the Admin token to establish a secure browser session. The token is used only for this exchange and is not stored in browser storage.'}
        </p>

        <form id="admin-gate-form" class="admin-gate-form" autocomplete="off">
          <label for="admin-gate-token">${firstTime ? 'Bootstrap token' : 'Admin token'}</label>
          <input
            id="admin-gate-token"
            type="password"
            minlength="16"
            maxlength="512"
            autocomplete="current-password"
            autocapitalize="off"
            spellcheck="false"
            required
            autofocus
          />
          <button type="submit">${firstTime ? 'Initialize & open Admin' : 'Sign in'}</button>
        </form>

        <div id="admin-gate-message" class="admin-gate-message ${message ? 'bad' : ''}" role="status">
          ${escapeHTML(message)}
        </div>

        ${setup.recovery_configured ? `
          <details class="admin-gate-recovery">
            <summary>Recover access</summary>
            <form id="admin-recovery-gate-form" class="admin-gate-form" autocomplete="off">
              <label for="admin-recovery-code">Recovery code</label>
              <input id="admin-recovery-code" type="password" autocomplete="off" required />
              <label for="admin-recovery-token">New Admin token</label>
              <input id="admin-recovery-token" type="password" minlength="16" maxlength="512" autocomplete="new-password" required />
              <label for="admin-recovery-confirm">Confirm new token</label>
              <input id="admin-recovery-confirm" type="password" minlength="16" maxlength="512" autocomplete="new-password" required />
              <button type="submit" class="secondary">Recover & open Admin</button>
            </form>
          </details>
        ` : ''}

        <div class="admin-gate-foot">
          <span>HttpOnly session</span>
          <span>SameSite=Strict</span>
          <span>Admin UI loads after authentication</span>
        </div>
      </section>
    </main>
  `;

  const form = document.querySelector<HTMLFormElement>('#admin-gate-form')!;
  const tokenInput = document.querySelector<HTMLInputElement>('#admin-gate-token')!;
  const status = document.querySelector<HTMLDivElement>('#admin-gate-message')!;
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const token = tokenInput.value.trim();
    tokenInput.value = '';
    if (token.length < 16) {
      status.textContent = 'Token must be at least 16 characters.';
      status.classList.add('bad');
      return;
    }

    const button = form.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    button.disabled = true;
    status.textContent = firstTime ? 'Initializing secure Admin session…' : 'Authenticating…';
    status.classList.remove('bad');

    try {
      if (firstTime) {
        await request('/setup', {
          method: 'POST',
          body: JSON.stringify({ token }),
        });
      } else {
        await request('/session', {
          method: 'POST',
          body: JSON.stringify({ token }),
        });
      }
      await loadAdmin();
    } catch (error) {
      status.textContent = (error as Error).message;
      status.classList.add('bad');
      button.disabled = false;
      tokenInput.focus();
    }
  });

  document.querySelector<HTMLFormElement>('#admin-recovery-gate-form')?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const recoveryCode = document.querySelector<HTMLInputElement>('#admin-recovery-code')!.value.trim();
    const newTokenInput = document.querySelector<HTMLInputElement>('#admin-recovery-token')!;
    const confirmInput = document.querySelector<HTMLInputElement>('#admin-recovery-confirm')!;
    const newToken = newTokenInput.value.trim();
    const confirm = confirmInput.value.trim();
    newTokenInput.value = '';
    confirmInput.value = '';

    if (newToken.length < 16 || newToken !== confirm) {
      status.textContent = newToken !== confirm
        ? 'New token confirmation does not match.'
        : 'New token must be at least 16 characters.';
      status.classList.add('bad');
      return;
    }

    status.textContent = 'Recovering secure Admin access…';
    status.classList.remove('bad');
    try {
      await request('/auth/recover', {
        method: 'POST',
        body: JSON.stringify({ recovery_code: recoveryCode, new_token: newToken }),
      });
      await loadAdmin();
    } catch (error) {
      status.textContent = (error as Error).message;
      status.classList.add('bad');
    }
  });
}

let adminLoaded = false;

async function loadAdmin(): Promise<void> {
  if (adminLoaded) return;
  adminLoaded = true;
  document.documentElement.classList.add('ops-authenticated');
  root.innerHTML = '';

  await import('./main');
  await import('./admin-router');
  await import('./admin-shell');
  await import('./admin-access');
  await import('./syn-plus-panel');
  await import('./workspace-settings-panel');
  await import('./observability');
  await import('./canonical-5s-admin');
  await import('./five-second-health-v2');
  await import('./intelligence-panel.mts');
  await import('./intelligence-lab-panel.mts');
  await import('./admin-ready');

  document.querySelector<HTMLButtonElement>('#admin-logout')?.addEventListener('click', async () => {
    try {
      await request('/session', { method: 'DELETE' });
    } finally {
      window.location.reload();
    }
  });

  window.addEventListener('qnext-ops-auth-rejected', () => {
    window.location.reload();
  }, { once: true });
}

async function bootstrap(): Promise<void> {
  try {
    const session = await sessionStatus();
    if (session.authenticated) {
      await loadAdmin();
      return;
    }
  } catch {
    // Fall through to the explicit sign-in gate.
  }

  try {
    renderGate(await setupStatus());
  } catch (error) {
    renderGate({ initialized: true }, `Admin authentication service unavailable: ${(error as Error).message}`);
  }
}

void bootstrap();
