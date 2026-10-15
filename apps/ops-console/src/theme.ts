export type ThemePreference = 'dark' | 'light' | 'system';
const STORAGE_KEY = 'qnext-theme-preference';
const CHOICES: ThemePreference[] = ['dark', 'light', 'system'];
export function resolveTheme(preference: ThemePreference, systemDark: boolean): 'dark' | 'light' {
  return preference === 'system' ? (systemDark ? 'dark' : 'light') : preference;
}
export function getThemePreference(): ThemePreference {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'dark' || stored === 'light' || stored === 'system') return stored;
  } catch { /* storage can be disabled */ }
  return 'dark';
}
function prefersDark(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches;
}
export function currentTheme(): 'dark' | 'light' {
  return resolveTheme(getThemePreference(), prefersDark());
}
export function applyTheme(): void {
  const theme = currentTheme();
  document.documentElement.dataset.qnextTheme = theme;
  document.documentElement.style.colorScheme = theme;
  window.dispatchEvent(new CustomEvent('qnext-theme-change', { detail: { theme } }));
}
function choose(preference: ThemePreference): void {
  if (!CHOICES.includes(preference)) return;
  try {
    localStorage.setItem(STORAGE_KEY, preference);
    applyTheme();
  } catch {
    const theme = resolveTheme(preference, prefersDark());
    document.documentElement.dataset.qnextTheme = theme;
    document.documentElement.style.colorScheme = theme;
    window.dispatchEvent(new CustomEvent('qnext-theme-change', { detail: { theme } }));
  }
}
export function mountThemeControl(options: { reloadOnChange?: boolean } = {}): void {
  if (document.querySelector('#qnext-theme-preference')) return;
  const label = document.createElement('label');
  label.className = 'qnext-theme-control';
  label.htmlFor = 'qnext-theme-preference';
  const caption = document.createElement('span');
  caption.textContent = 'Theme';
  const select = document.createElement('select');
  select.id = 'qnext-theme-preference';
  select.setAttribute('aria-label', 'Appearance preference');
  for (const choice of CHOICES) {
    const option = document.createElement('option');
    option.value = choice;
    option.textContent = choice[0].toUpperCase() + choice.slice(1);
    select.append(option);
  }
  select.value = getThemePreference();
  select.addEventListener('change', () => {
    choose(select.value as ThemePreference);
    // Vela is configured at construction; avoid undocumented runtime methods.
    if (options.reloadOnChange) window.location.reload();
  });
  label.append(caption, select);
  document.body.append(label);
  if (typeof matchMedia === 'function') {
    matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (getThemePreference() !== 'system') return;
      applyTheme();
      if (options.reloadOnChange) window.location.reload();
    });
  }
}
