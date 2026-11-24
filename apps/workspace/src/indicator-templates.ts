import { registerIcon, registerWidgetAction } from '@luxalgo/vela/plugin';
import type { ChartCell, ChartState, VelaWorkspace, WorkspaceState } from '@luxalgo/vela/workspace';

export const INDICATOR_TEMPLATES_STORAGE_KEY = 'qnext.indicator-templates.v1';
const STORAGE_VERSION = 1;
const MAX_TEMPLATES = 100;
const MAX_INDICATORS = 200;
const MAX_INPUTS = 128;
const MAX_NAME_LENGTH = 64;

export type IndicatorSnapshot = NonNullable<ChartState['indicators']>;
export type IndicatorTemplateScope = 'active' | 'all';

export interface IndicatorTemplate {
  id: string;
  name: string;
  indicators: IndicatorSnapshot;
  updatedAt: number;
  favorite: boolean;
  lastUsedAt?: number;
}

export interface IndicatorTemplateStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function sanitizeInputValues(value: unknown): Record<string, unknown> | null {
  if (!isRecord(value)) return null;
  const entries = Object.entries(value);
  if (entries.length > MAX_INPUTS) return null;

  const result: Record<string, unknown> = {};
  for (const [key, item] of entries) {
    if (!key || key.length > 128 || key === '__proto__' || key === 'constructor' || key === 'prototype') return null;
    if (
      typeof item !== 'string' &&
      typeof item !== 'boolean' &&
      !(typeof item === 'number' && Number.isFinite(item))
    ) return null;
    if (typeof item === 'string' && item.length > 2_000) return null;
    result[key] = item;
  }
  return result;
}

function sanitizeManifestEntry(value: unknown): IndicatorSnapshot['manifest'][number] | null {
  if (typeof value === 'string') {
    return value.length > 0 && value.length <= 256 ? value : null;
  }
  if (!isRecord(value) || typeof value.name !== 'string' || !value.name.trim() || value.name.length > 256) return null;

  const inputs = value.inputs === undefined ? undefined : sanitizeInputValues(value.inputs);
  const props = value.props === undefined ? undefined : sanitizeInputValues(value.props);
  if ((value.inputs !== undefined && inputs === null) || (value.props !== undefined && props === null)) return null;
  if (value.hidden !== undefined && typeof value.hidden !== 'boolean') return null;

  return {
    name: value.name,
    ...(inputs ? { inputs } : {}),
    ...(props ? { props } : {}),
    ...(value.hidden === true ? { hidden: true } : {}),
  };
}

function sanitizeNativeEntry(value: unknown): IndicatorSnapshot['natives'][number] | null {
  if (typeof value === 'string') {
    return value.length > 0 && value.length <= 128 ? value : null;
  }
  if (!isRecord(value) || typeof value.type !== 'string' || !value.type.trim() || value.type.length > 128) return null;

  const inputs = value.inputs === undefined ? undefined : sanitizeInputValues(value.inputs);
  if (value.inputs !== undefined && inputs === null) return null;
  if (value.hidden !== undefined && typeof value.hidden !== 'boolean') return null;

  return {
    type: value.type,
    ...(inputs ? { inputs } : {}),
    ...(value.hidden === true ? { hidden: true } : {}),
  };
}

export function sanitizeIndicatorSnapshot(value: unknown): IndicatorSnapshot | null {
  if (!isRecord(value) || !Array.isArray(value.manifest) || !Array.isArray(value.natives)) return null;
  if (value.manifest.length > MAX_INDICATORS || value.natives.length > MAX_INDICATORS) return null;

  const manifest = value.manifest.map(sanitizeManifestEntry);
  const natives = value.natives.map(sanitizeNativeEntry);
  if (manifest.some((entry) => entry === null) || natives.some((entry) => entry === null)) return null;

  return {
    manifest: manifest as IndicatorSnapshot['manifest'],
    natives: natives as IndicatorSnapshot['natives'],
  };
}

export function sanitizeIndicatorTemplates(value: unknown): IndicatorTemplate[] {
  if (!isRecord(value) || value.version !== STORAGE_VERSION || !Array.isArray(value.templates)) return [];

  const templates: IndicatorTemplate[] = [];
  const ids = new Set<string>();
  for (const raw of value.templates.slice(0, MAX_TEMPLATES)) {
    if (!isRecord(raw)) continue;
    if (
      typeof raw.id !== 'string' || !raw.id || raw.id.length > 128 || ids.has(raw.id) ||
      typeof raw.name !== 'string' || !raw.name.trim() || raw.name.trim().length > MAX_NAME_LENGTH ||
      typeof raw.updatedAt !== 'number' || !Number.isFinite(raw.updatedAt) || raw.updatedAt <= 0
    ) continue;

    const indicators = sanitizeIndicatorSnapshot(raw.indicators);
    if (!indicators) continue;

    ids.add(raw.id);
    templates.push({
      id: raw.id,
      name: raw.name.trim(),
      indicators,
      updatedAt: raw.updatedAt,
      favorite: raw.favorite === true,
      ...(typeof raw.lastUsedAt === 'number' && Number.isFinite(raw.lastUsedAt) && raw.lastUsedAt > 0
        ? { lastUsedAt: raw.lastUsedAt }
        : {}),
    });
  }
  return templates;
}

export function readIndicatorTemplates(storage: Pick<IndicatorTemplateStorage, 'getItem'>): IndicatorTemplate[] {
  try {
    const raw = storage.getItem(INDICATOR_TEMPLATES_STORAGE_KEY);
    return raw ? sanitizeIndicatorTemplates(JSON.parse(raw) as unknown) : [];
  } catch {
    return [];
  }
}

export function writeIndicatorTemplates(
  storage: Pick<IndicatorTemplateStorage, 'setItem'>,
  templates: readonly IndicatorTemplate[],
): boolean {
  try {
    const safe = sanitizeIndicatorTemplates({ version: STORAGE_VERSION, templates });
    if (safe.length !== templates.length) return false;
    storage.setItem(INDICATOR_TEMPLATES_STORAGE_KEY, JSON.stringify({ version: STORAGE_VERSION, templates: safe }));
    return true;
  } catch {
    return false;
  }
}

function cloneJson<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function activeChart(state: WorkspaceState): ChartState | undefined {
  return state.charts.find((chart) => chart.id === state.activeCellId) ?? state.charts[0];
}

export function activeChartIndicatorSnapshot(state: WorkspaceState): IndicatorSnapshot {
  const snapshot = activeChart(state)?.indicators;
  return snapshot ? cloneJson(snapshot) : { manifest: [], natives: [] };
}

export function applyIndicatorTemplateToState(
  state: WorkspaceState,
  template: Pick<IndicatorTemplate, 'indicators'>,
  scope: IndicatorTemplateScope = 'active',
): WorkspaceState {
  const next = cloneJson(state);
  const activeId = activeChart(next)?.id;
  const targetIds = scope === 'all'
    ? new Set(next.charts.map((chart) => chart.id))
    : new Set(activeId ? [activeId] : []);

  next.charts = next.charts.map((chart) => targetIds.has(chart.id)
    ? { ...chart, indicators: cloneJson(template.indicators) }
    : chart);
  return next;
}

/** The Vela workspace ledger deliberately omits default-valued inputs. For a named
 * template, freeze the values currently stored by each live handle, including false/0.
 * Manifest identity comes from the shell rather than from the script's display title. */
export function captureLiveIndicatorSnapshot(cell: Pick<ChartCell, 'instances' | 'chart'>, expected: IndicatorSnapshot): IndicatorSnapshot {
  const manifest = cell.instances.filter(instance => !instance.external);
  const natives = cell.chart.indicators().filter(handle => handle.nativeType !== undefined);
  if (cell.instances.some(instance => instance.external) ||
      manifest.length !== expected.manifest.length || natives.length !== expected.natives.length ||
      manifest.some(instance => !instance.handle)) {
    throw new Error('Indicators are loading or contain an unsupported external study.');
  }
  const snapshot = sanitizeIndicatorSnapshot({
    manifest: manifest.map(instance => ({
      name: instance.entry.name,
      inputs: instance.handle!.inputValues(),
      props: instance.handle!.propValues(),
      hidden: !instance.handle!.visible,
    })),
    natives: natives.map(handle => ({
      type: handle.nativeType,
      inputs: handle.inputValues(),
      hidden: !handle.visible,
    })),
  });
  if (!snapshot) throw new Error('Indicator settings exceed the supported template format.');
  return snapshot;
}

/** Fail before replacing any indicator if the current picker cannot resolve a study. */
export function unavailableTemplateIndicators(snapshot: IndicatorSnapshot, library: ReturnType<ChartCell['libraryRows']>): string[] {
  const scripts = new Set(library.filter(row => !row.native).map(row => row.name));
  const natives = new Set(library.filter(row => row.native).map(row => row.nativeType));
  return [...new Set([
    ...snapshot.manifest.map(item => typeof item === 'string' ? item : item.name).filter(name => !scripts.has(name)),
    ...snapshot.natives.map(item => typeof item === 'string' ? item : item.type).filter(type => !natives.has(type)),
  ])];
}

function browserStorage(doc: Document): Storage | null {
  try {
    return doc.defaultView?.localStorage ?? null;
  } catch {
    return null;
  }
}

function newTemplateId(doc: Document): string {
  const cryptoApi = doc.defaultView?.crypto;
  if (cryptoApi && typeof cryptoApi.randomUUID === 'function') return cryptoApi.randomUUID();
  return 'qnext-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10);
}

function makeButton(doc: Document, label: string, className: string, ariaLabel?: string): HTMLButtonElement {
  const button = doc.createElement('button');
  button.type = 'button';
  button.className = className;
  button.textContent = label;
  if (ariaLabel) button.setAttribute('aria-label', ariaLabel);
  return button;
}

function templateSort(a: IndicatorTemplate, b: IndicatorTemplate): number {
  if (a.favorite !== b.favorite) return a.favorite ? -1 : 1;
  return (b.lastUsedAt ?? b.updatedAt) - (a.lastUsedAt ?? a.updatedAt);
}

function indicatorCount(snapshot: IndicatorSnapshot): number {
  return snapshot.manifest.length + snapshot.natives.length;
}

function showIndicatorTemplates(ctx: Parameters<Parameters<typeof registerWidgetAction>[0]['run']>[0], getWorkspace: () => VelaWorkspace | undefined): void {
  const workspace = getWorkspace();
  if (!workspace) {
    ctx.toast('The chart workspace is not ready yet.', 'error');
    return;
  }

  const doc = ctx.host.ownerDocument;
  const previous = doc.getElementById('qnext-indicator-template-dialog') as HTMLDialogElement | null;
  if (previous?.open) previous.close();
  previous?.remove();

  const dialog = doc.createElement('dialog');
  dialog.id = 'qnext-indicator-template-dialog';
  dialog.className = 'qnext-indicator-template-dialog';
  dialog.setAttribute('aria-labelledby', 'qnext-indicator-template-title');

  const panel = doc.createElement('div');
  panel.className = 'qnext-indicator-template-panel';

  const header = doc.createElement('header');
  header.className = 'qnext-indicator-template-header';
  const title = doc.createElement('h2');
  title.id = 'qnext-indicator-template-title';
  title.textContent = 'Indicator templates';
  const close = makeButton(doc, '×', 'qnext-template-icon-button', 'Close indicator templates');
  close.addEventListener('click', () => dialog.close());
  header.append(title, close);

  const description = doc.createElement('p');
  description.className = 'qnext-indicator-template-description';
  description.textContent = 'Save indicators with their custom inputs. Applying a template replaces indicators only; the symbol, timeframe and drawings stay unchanged.';

  const saveRow = doc.createElement('div');
  saveRow.className = 'qnext-indicator-template-save';
  const nameInput = doc.createElement('input');
  nameInput.type = 'text';
  nameInput.maxLength = MAX_NAME_LENGTH;
  nameInput.placeholder = 'Template name (e.g. EMA + VWAP + ADX)';
  nameInput.setAttribute('aria-label', 'Indicator template name');
  const saveButton = makeButton(doc, 'Save current', 'qnext-template-primary-button');
  saveRow.append(nameInput, saveButton);

  const listHeading = doc.createElement('div');
  listHeading.className = 'qnext-indicator-template-list-heading';
  listHeading.textContent = 'Saved templates';

  const list = doc.createElement('div');
  list.className = 'qnext-indicator-template-list';
  list.setAttribute('aria-live', 'polite');

  const feedback = doc.createElement('p');
  feedback.className = 'qnext-indicator-template-feedback';
  feedback.setAttribute('role', 'status');
  panel.append(header, description, saveRow, feedback, listHeading, list);
  dialog.append(panel);
  doc.body.append(dialog);

  const storage = browserStorage(doc);
  const load = (): IndicatorTemplate[] => storage ? readIndicatorTemplates(storage) : [];
  const persist = (templates: readonly IndicatorTemplate[]): boolean =>
    storage ? writeIndicatorTemplates(storage, templates) : false;

  const report = (message: string, kind: 'info' | 'success' | 'error' = 'info'): void => {
    feedback.textContent = message;
    feedback.dataset.kind = kind;
    ctx.toast(message, kind);
  };
  const renderList = (): void => {
    list.replaceChildren();
    const templates = load().sort(templateSort);
    if (templates.length === 0) {
      const empty = doc.createElement('p');
      empty.className = 'qnext-indicator-template-empty';
      empty.textContent = storage
        ? 'No templates saved yet. Configure your indicators, then save the current set above.'
        : 'Browser storage is unavailable. Enable site storage to save reusable templates.';
      list.append(empty);
      return;
    }

    for (const template of templates) {
      const row = doc.createElement('article');
      row.className = 'qnext-indicator-template-row';

      const details = doc.createElement('div');
      details.className = 'qnext-indicator-template-details';
      const name = doc.createElement('strong');
      name.textContent = template.name;
      const meta = doc.createElement('span');
      const count = indicatorCount(template.indicators);
      meta.textContent = count + (count === 1 ? ' indicator' : ' indicators') +
        ' · saved ' + new Date(template.updatedAt).toLocaleDateString();
      details.append(name, meta);

      const actions = doc.createElement('div');
      actions.className = 'qnext-indicator-template-actions';
      const applyButton = makeButton(doc, 'Apply', 'qnext-template-primary-button');
      applyButton.addEventListener('click', () => applyTemplate(template));
      actions.append(applyButton);


      const favoriteButton = makeButton(
        doc,
        template.favorite ? '★' : '☆',
        'qnext-template-icon-button',
        template.favorite ? 'Unpin template' : 'Pin template',
      );
      favoriteButton.setAttribute('aria-pressed', String(template.favorite));
      favoriteButton.addEventListener('click', () => {
        const next = load().map((item) => item.id === template.id
          ? { ...item, favorite: !item.favorite }
          : item);
        if (!persist(next)) {
          report('Could not update the template in browser storage.', 'error');
          return;
        }
        renderList();
      });

      const renameButton = makeButton(doc, 'Rename', 'qnext-template-secondary-button');
      renameButton.addEventListener('click', () => {
        const nextName = doc.defaultView?.prompt('Rename indicator template', template.name)?.trim();
        if (!nextName) return;
        if (nextName.length > MAX_NAME_LENGTH) {
          report('Template names must be 64 characters or fewer.', 'error');
          return;
        }
        const current = load();
        if (current.some((item) => item.id !== template.id && item.name.toLowerCase() === nextName.toLowerCase())) {
          report('A template with that name already exists.', 'error');
          return;
        }
        const next = current.map((item) => item.id === template.id
          ? { ...item, name: nextName, updatedAt: Date.now() }
          : item);
        if (!persist(next)) {
          report('Could not rename the template in browser storage.', 'error');
          return;
        }
        renderList();
      });

      const deleteButton = makeButton(doc, 'Delete', 'qnext-template-danger-button');
      deleteButton.addEventListener('click', () => {
        const confirmed = doc.defaultView?.confirm('Delete the "' + template.name + '" indicator template?') ?? false;
        if (!confirmed) return;
        if (!persist(load().filter((item) => item.id !== template.id))) {
          report('Could not delete the template from browser storage.', 'error');
          return;
        }
        renderList();
      });

      const duplicateButton = makeButton(doc, 'Duplicate', 'qnext-template-secondary-button');
      duplicateButton.addEventListener('click', () => {
        const current = load();
        const name = doc.defaultView?.prompt('Name the copied template', (template.name + ' copy').slice(0, MAX_NAME_LENGTH))?.trim();
        if (!name) return;
        if (name.length > MAX_NAME_LENGTH || current.length >= MAX_TEMPLATES ||
            current.some(item => item.name.toLowerCase() === name.toLowerCase())) {
          report('Use a unique name of up to 64 characters; maximum 100 templates.', 'error');
          return;
        }
        const copy: IndicatorTemplate = {
          id: newTemplateId(doc), name, indicators: cloneJson(template.indicators),
          updatedAt: Date.now(), favorite: false,
        };
        if (!persist([copy, ...current])) {
          report('Could not duplicate the template.', 'error');
          return;
        }
        renderList();
      });
      actions.append(favoriteButton, renameButton, duplicateButton, deleteButton);
      row.append(details, actions);
      list.append(row);
    }
  };

  const applyTemplate = (template: IndicatorTemplate): void => {
    const currentWorkspace = getWorkspace();
    if (!currentWorkspace) {
      report('The chart workspace is no longer available.', 'error');
      return;
    }
    try {
      const cell = currentWorkspace.active;
      const state = activeChart(currentWorkspace.getState());
      if (!state || state.id !== cell.id) throw new Error('Active chart changed; try again.');
      if (cell.instances.some(instance => instance.external)) throw new Error('Remove external studies before replacing indicators.');
      const missing = unavailableTemplateIndicators(template.indicators, cell.libraryRows());
      if (missing.length) throw new Error('Unavailable indicators: ' + missing.join(', '));
      // Rehydrate ONLY the active cell, without reapplying renderer or drawing state.
      // Preserve the requested market/session and extension state; the Vela ledger
      // replaces manifest instances and converges native instances without a new chart.
      cell.rehydrate({
        symbol: state.symbol, provider: state.provider, timeframe: state.timeframe,
        session: state.session, bars: state.bars, ext: state.ext,
        indicators: cloneJson(template.indicators),
      });
      ctx.stateChanged();
      const next = load().map((item) => item.id === template.id
        ? { ...item, lastUsedAt: Date.now() }
        : item);
      if (storage) persist(next);
      dialog.close();
      ctx.toast('Template loaded; indicators are recalculating.', 'info');
    } catch (error) {
      report(error instanceof Error ? error.message : 'Could not load the template.', 'error');
    }
  };

  const saveCurrent = (): void => {
    const name = nameInput.value.trim();
    if (!name) {
      report('Enter a name for this indicator template.', 'error');
      nameInput.focus();
      return;
    }

    const state = getWorkspace()?.getState();
    if (!state) {
      report('The chart workspace is no longer available.', 'error');
      return;
    }
    let indicators: IndicatorSnapshot;
    try {
      indicators = captureLiveIndicatorSnapshot(getWorkspace()!.active, activeChartIndicatorSnapshot(state));
    } catch (error) {
      report(error instanceof Error ? error.message : 'Could not capture indicator settings.', 'error');
      return;
    }
    if (indicatorCount(indicators) === 0) {
      report('Add at least one indicator before saving a template.', 'error');
      return;
    }

    const current = load();
    const existing = current.find((item) => item.name.toLowerCase() === name.toLowerCase());
    if (!existing && current.length >= MAX_TEMPLATES) {
      report('Template limit reached. Delete a template before adding another.', 'error');
      return;
    }
    if (existing) {
      const confirmed = doc.defaultView?.confirm('Replace the saved "' + existing.name + '" template with the current indicators and settings?') ?? false;
      if (!confirmed) return;
    }
    const template: IndicatorTemplate = {
      id: existing?.id ?? newTemplateId(doc),
      name,
      indicators,
      updatedAt: Date.now(),
      favorite: existing?.favorite ?? false,
      ...(existing?.lastUsedAt ? { lastUsedAt: existing.lastUsedAt } : {}),
    };
    const next = [template, ...current.filter((item) => item.id !== template.id)];
    if (!persist(next)) {
      report('Could not save the template. Check browser storage permissions.', 'error');
      return;
    }
    nameInput.value = '';
    report('Indicator template saved with its current settings.', 'success');
    renderList();
  };

  saveButton.addEventListener('click', saveCurrent);
  nameInput.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      saveCurrent();
    }
  });
  dialog.addEventListener('click', (event) => {
    if (event.target === dialog) dialog.close();
  });
  dialog.addEventListener('close', () => dialog.remove(), { once: true });

  renderList();
  if (typeof dialog.showModal === 'function') dialog.showModal();
  else dialog.setAttribute('open', '');
  nameInput.focus();
}

export function registerIndicatorTemplates(getWorkspace: () => VelaWorkspace | undefined): () => void {
  registerIcon(
    'qnext-indicator-templates',
    '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3.75h9l3 3v13.5H6z"/><path d="M15 3.75v4h3M9 12h6M9 16h6"/></svg>',
  );
  return registerWidgetAction({
    id: 'qnext.indicator.templates',
    target: 'topbar',
    label: 'Templates',
    icon: 'qnext-indicator-templates',
    iconOnly: true,
    align: 'left',
    mobile: 'menu',
    order: 1,
    run: (ctx) => showIndicatorTemplates(ctx, getWorkspace),
  });
}
