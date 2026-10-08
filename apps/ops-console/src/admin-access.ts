export {};

type SetupStatus = {
  initialized: boolean;
  recovery_configured?: boolean;
};

type RecoveryCodeResult = {
  recovery_code: string;
  generated_at: string;
};

type RecoveryResult = {
  authenticated: boolean;
  recovery_consumed: boolean;
  expires_in_seconds: number;
};

type RuntimeConfig = {
  apiBase?: string;
};

const runtime = ((window as unknown as { __QNEXT_OPS_CONFIG__?: RuntimeConfig }).__QNEXT_OPS_CONFIG__ ?? {});
const apiBase = (runtime.apiBase ?? '/qnext/admin/api/index.php').replace(/\/$/, '');

function routeURL(path: string): string {
  const separator = apiBase.includes('?') ? '&' : '?';
  return `${apiBase}${separator}route=${encodeURIComponent(path)}`;
}

async function apiRequest<T>(
  path: string,
  init: RequestInit = {},
  _includeAdminToken = false,
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body) headers.set('Content-Type', 'application/json');

  const response = await fetch(routeURL(path), {
    ...init,
    headers,
    credentials: 'same-origin',
  });
  const text = await response.text();
  let payload: unknown = {};
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      throw new Error(`Admin access API returned non-JSON HTTP ${response.status}`);
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

function setMessage(element: HTMLElement, message: string, bad = false): void {
  element.textContent = message;
  element.classList.toggle('bad', bad);
}

function installAdminAccessPanel(): void {
  const existing = document.querySelector('#admin-access-card');
  if (existing) return;

  const grid = document.querySelector<HTMLElement>('main.grid');
  if (!grid) {
    window.setTimeout(installAdminAccessPanel, 50);
    return;
  }

  const section = document.createElement('section');
  section.id = 'admin-access-card';
  section.className = 'card span-3';
  section.innerHTML = `
    <div class="card-head">
      <div>
        <p class="eyebrow">Admin Access</p>
        <h2>Web recovery</h2>
        <p class="muted">Generate a one-time recovery code while signed in. Store it offline. If the Admin token is lost later, use that code here to set a new token without SSH or File Manager.</p>
      </div>
      <span id="admin-recovery-status" class="badge">CHECKING</span>
    </div>

    <div class="actions">
      <button id="generate-admin-recovery" type="button">Generate / rotate recovery code</button>
      <button id="copy-admin-recovery" type="button" class="secondary" disabled>Copy code</button>
      <button id="revoke-admin-recovery" type="button" class="secondary">Revoke recovery code</button>
    </div>
    <div class="stack">
      <span class="muted">Recovery code is shown only when generated. Generating a new code invalidates the previous one.</span>
      <code id="admin-recovery-output">No recovery code shown in this browser session.</code>
      <span id="admin-recovery-message" class="muted"></span>
    </div>

    <details style="margin-top: 1rem">
      <summary>Recover access with a one-time code</summary>
      <form id="admin-recovery-form" class="stack" autocomplete="off" style="margin-top: 1rem">
        <label>
          <span>Recovery code</span>
          <input id="admin-recovery-code" name="recoveryCode" type="password" autocomplete="off" required />
        </label>
        <label>
          <span>New Admin token</span>
          <input id="admin-recovery-new-token" name="newToken" type="password" minlength="16" maxlength="512" autocomplete="new-password" required />
        </label>
        <label>
          <span>Confirm new Admin token</span>
          <input id="admin-recovery-confirm-token" name="confirmToken" type="password" minlength="16" maxlength="512" autocomplete="new-password" required />
        </label>
        <div class="actions">
          <button type="submit">Recover Admin access</button>
          <span class="muted">Successful recovery consumes the code and invalidates old Admin sessions.</span>
        </div>
      </form>
    </details>
  `;
  grid.prepend(section);

  const status = section.querySelector<HTMLSpanElement>('#admin-recovery-status')!;
  const output = section.querySelector<HTMLElement>('#admin-recovery-output')!;
  const message = section.querySelector<HTMLSpanElement>('#admin-recovery-message')!;
  const generate = section.querySelector<HTMLButtonElement>('#generate-admin-recovery')!;
  const copy = section.querySelector<HTMLButtonElement>('#copy-admin-recovery')!;
  const revoke = section.querySelector<HTMLButtonElement>('#revoke-admin-recovery')!;
  const form = section.querySelector<HTMLFormElement>('#admin-recovery-form')!;

  let visibleRecoveryCode = '';

  const refreshStatus = async () => {
    try {
      const result = await apiRequest<SetupStatus>('/setup-status');
      status.textContent = result.recovery_configured ? 'RECOVERY READY' : 'NO RECOVERY CODE';
      status.classList.toggle('good', Boolean(result.recovery_configured));
      status.classList.toggle('warn', !result.recovery_configured);
    } catch (error) {
      status.textContent = 'STATUS ERROR';
      status.classList.add('bad');
      setMessage(message, (error as Error).message, true);
    }
  };

  generate.addEventListener('click', async () => {
    generate.disabled = true;
    try {
      const result = await apiRequest<RecoveryCodeResult>('/auth/recovery-code', { method: 'POST' }, true);
      visibleRecoveryCode = result.recovery_code;
      output.textContent = visibleRecoveryCode;
      copy.disabled = false;
      setMessage(message, `Generated ${result.generated_at}. Copy this code now and store it offline; QNext stores only its digest.`);
      await refreshStatus();
    } catch (error) {
      setMessage(message, `Could not generate recovery code: ${(error as Error).message}. Sign in with the current Admin token first.`, true);
    } finally {
      generate.disabled = false;
    }
  });

  copy.addEventListener('click', async () => {
    if (!visibleRecoveryCode) return;
    try {
      await navigator.clipboard.writeText(visibleRecoveryCode);
      setMessage(message, 'Recovery code copied. Store it somewhere separate from the server.');
    } catch {
      setMessage(message, 'Clipboard access failed. Copy the displayed recovery code manually.', true);
    }
  });

  revoke.addEventListener('click', async () => {
    revoke.disabled = true;
    try {
      await apiRequest('/auth/recovery-code', { method: 'DELETE' }, true);
      visibleRecoveryCode = '';
      output.textContent = 'Recovery code revoked.';
      copy.disabled = true;
      setMessage(message, 'Recovery code revoked. Generate another one before relying on web recovery.');
      await refreshStatus();
    } catch (error) {
      setMessage(message, `Could not revoke recovery code: ${(error as Error).message}`, true);
    } finally {
      revoke.disabled = false;
    }
  });

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const recoveryCode = section.querySelector<HTMLInputElement>('#admin-recovery-code')!.value.trim();
    const newToken = section.querySelector<HTMLInputElement>('#admin-recovery-new-token')!.value.trim();
    const confirmToken = section.querySelector<HTMLInputElement>('#admin-recovery-confirm-token')!.value.trim();

    if (newToken.length < 16) {
      setMessage(message, 'New Admin token must be at least 16 characters.', true);
      return;
    }
    if (newToken !== confirmToken) {
      setMessage(message, 'New Admin token confirmation does not match.', true);
      return;
    }

    const submit = form.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    submit.disabled = true;
    try {
      const result = await apiRequest<RecoveryResult>('/auth/recover', {
        method: 'POST',
        body: JSON.stringify({ recovery_code: recoveryCode, new_token: newToken }),
      });
      if (!result.authenticated) throw new Error('Recovery did not establish an Admin session');
      setMessage(message, 'Admin access recovered. Reloading the authenticated session…');
      window.setTimeout(() => window.location.reload(), 350);
    } catch (error) {
      setMessage(message, `Recovery failed: ${(error as Error).message}`, true);
      submit.disabled = false;
    }
  });

  void refreshStatus();
}

installAdminAccessPanel();
