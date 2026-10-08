import {
  captureActiveChartLabSnapshot,
  type LabChartSnapshot,
} from './intelligence-lab-snapshot';

export const INTELLIGENCE_LAB_PENDING_KEY = 'qnext-intelligence-lab-pending-v1';

interface WorkspaceLike {
  getState: () => unknown;
  chart: {
    indicators: () => Array<{
      id: string;
      title: string;
      visible: boolean;
      source?: string;
      nativeType?: string;
      inputs?: readonly { key: string; defval?: unknown }[];
      inputValues?: () => Record<string, unknown>;
      context: (select?: string[]) => Promise<unknown>;
    }>;
  };
}

export function mountIntelligenceLabBridge(workspace: WorkspaceLike): () => void {
  const existing = document.querySelector<HTMLDivElement>('#qnext-intelligence-lab-bridge');
  existing?.remove();

  const root = document.createElement('div');
  root.id = 'qnext-intelligence-lab-bridge';
  root.style.cssText = [
    'position:fixed',
    'right:18px',
    'bottom:18px',
    'z-index:1000',
    'display:flex',
    'align-items:center',
    'gap:8px',
    'padding:8px',
    'border:1px solid rgba(255,255,255,.14)',
    'border-radius:10px',
    'background:rgba(11,13,18,.92)',
    'box-shadow:0 8px 30px rgba(0,0,0,.35)',
    'backdrop-filter:blur(10px)',
  ].join(';');

  const status = document.createElement('span');
  status.textContent = 'Lab';
  status.style.cssText = 'font:12px system-ui;color:#9ca7b5;max-width:220px';

  const button = document.createElement('button');
  button.type = 'button';
  button.textContent = 'Send to Lab';
  button.title = 'Capture indicators enabled on the active chart for Intelligence Lab';
  button.style.cssText = [
    'border:1px solid rgba(99,102,241,.55)',
    'border-radius:7px',
    'padding:7px 10px',
    'background:#151a28',
    'color:#eef2ff',
    'font:600 12px system-ui',
    'cursor:pointer',
  ].join(';');

  button.addEventListener('click', async () => {
    button.disabled = true;
    status.textContent = 'Capturing enabled indicators…';
    try {
      const snapshot = await captureActiveChartLabSnapshot(workspace);
      persistPending(snapshot);
      status.textContent = `${snapshot.indicators.length} indicators · queueing…`;

      const response = await fetch('/qnext/admin/api/index.php?route=%2Fintelligence-lab%2Fimport', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ snapshot }),
      });

      if (response.ok) {
        localStorage.removeItem(INTELLIGENCE_LAB_PENDING_KEY);
        status.textContent = 'Sent to Lab';
        return;
      }

      if (response.status === 403) {
        status.textContent = 'Saved · open Admin to import';
        window.open('/qnext/admin/#intelligence-lab', '_blank', 'noopener');
        return;
      }

      const text = await response.text();
      throw new Error(text || `HTTP ${response.status}`);
    } catch (error) {
      status.textContent = `Lab error: ${error instanceof Error ? error.message : String(error)}`;
    } finally {
      button.disabled = false;
    }
  });

  root.append(status, button);
  document.body.append(root);
  return () => root.remove();
}

function persistPending(snapshot: LabChartSnapshot): void {
  const encoded = JSON.stringify(snapshot);
  try {
    localStorage.setItem(INTELLIGENCE_LAB_PENDING_KEY, encoded);
  } catch {
    throw new Error('chart snapshot is too large for browser handoff; sign in to Admin and retry');
  }
}
