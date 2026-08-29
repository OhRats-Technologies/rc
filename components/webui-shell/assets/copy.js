for (const container of document.querySelectorAll('[data-copy-value]')) {
  const button = container.querySelector('.copy-value');
  if (!button) continue;
  button.addEventListener('click', async () => {
    await navigator.clipboard.writeText(container.dataset.copyValue || '');
    const label = button.getAttribute('aria-label') || 'Copy';
    button.dataset.copied = 'true';
    button.setAttribute('aria-label', 'Copied');
    setTimeout(() => {
      delete button.dataset.copied;
      button.setAttribute('aria-label', label);
    }, 1000);
  });
}
