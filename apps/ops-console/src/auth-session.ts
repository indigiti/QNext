import { OPS_AUTH_REJECTED_EVENT } from './api';

let toastTimer = 0;

window.addEventListener(OPS_AUTH_REJECTED_EVENT, () => {
  const tokenInput = document.querySelector<HTMLInputElement>('#token');
  const tokenLabel = document.querySelector<HTMLLabelElement>('label[for="token"]');
  const toast = document.querySelector<HTMLDivElement>('#toast');

  if (tokenInput) {
    tokenInput.value = '';
    tokenInput.placeholder = 'Re-enter admin token';
  }
  if (tokenLabel) {
    tokenLabel.textContent = 'Admin token rejected — re-authenticate';
  }
  if (toast) {
    window.clearTimeout(toastTimer);
    toast.textContent = 'Admin token rejected. Re-enter the token and click Use token.';
    toast.className = 'show error';
    toastTimer = window.setTimeout(() => {
      toast.className = '';
    }, 8000);
  }
});
