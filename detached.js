// Navigation pages can open in another native window while the shared backend
// keeps running in this process. This is separate from SQL tab reordering.
(() => {
  const buttons = [...document.querySelectorAll('.rail .nav-item[data-view]')];
  const views = new Set(buttons.map(button => button.dataset.view));
  let drag = null, suppressClick = null;
  i18n.en['nav.detachFailed'] = 'Could not open the window: {error}';
  i18n.zh['nav.detachFailed'] = '无法打开窗口：{error}';
  const cleanup = () => {
    drag?.button.classList.remove('nav-detaching');
    document.body.classList.remove('nav-detaching');
    document.getElementById('nav-detach-preview')?.remove();
    drag = null;
  };
  for (const button of buttons) {
    button.style.touchAction = 'none';
    button.addEventListener('dragstart', event => event.preventDefault());
    button.addEventListener('pointerdown', event => {
      if (event.button !== 0 || drag) return;
      suppressClick = null;
      drag = { button, id: event.pointerId, x: event.clientX, y: event.clientY, moved: false };
      button.setPointerCapture(event.pointerId);
    });
    button.addEventListener('pointermove', event => {
      if (drag?.id !== event.pointerId) return;
      if (!drag.moved && Math.hypot(event.clientX - drag.x, event.clientY - drag.y) < 8) return;
      drag.moved = true;
      button.classList.add('nav-detaching'); document.body.classList.add('nav-detaching');
      let preview = document.getElementById('nav-detach-preview');
      if (!preview) {
        preview = document.createElement('div'); preview.id = 'nav-detach-preview';
        preview.textContent = t(`nav.${button.dataset.view}`) + ' ↗'; document.body.append(preview);
      }
      preview.style.left = `${Math.max(0, Math.min(innerWidth - preview.offsetWidth, event.clientX + 16))}px`;
      preview.style.top = `${Math.max(0, Math.min(innerHeight - preview.offsetHeight, event.clientY + 12))}px`;
    });
    button.addEventListener('pointerup', async event => {
      if (drag?.id !== event.pointerId) return;
      const rail = button.closest('.rail').getBoundingClientRect();
      const detach = drag.moved && (event.clientX > rail.right + 20 || event.clientX < rail.left - 20 || event.clientY < rail.top - 20 || event.clientY > rail.bottom + 20);
      if (drag.moved) suppressClick = button;
      cleanup();
      if (button.hasPointerCapture(event.pointerId)) button.releasePointerCapture(event.pointerId);
      if (!detach) return;
      try {
        if (typeof window.__lbOpenView === 'function') {
          await window.__lbOpenView(button.dataset.view, Math.round(event.screenX), Math.round(event.screenY));
        } else {
          const url = new URL(location.href); url.search = ''; url.searchParams.set('detached', button.dataset.view);
          const child = window.open(url.href, '_blank', `popup,width=1280,height=850,left=${Math.round(event.screenX)},top=${Math.round(event.screenY)}`);
          if (!child) throw new Error('Window blocked');
        }
      } catch (error) { showToast(t('nav.detachFailed', { error: error.message || String(error) })); }
    });
    button.addEventListener('click', event => {
      if (suppressClick === button) { suppressClick = null; event.preventDefault(); event.stopImmediatePropagation(); }
    }, true);
    for (const name of ['pointercancel', 'lostpointercapture']) button.addEventListener(name, event => {
      if (drag?.id === event.pointerId) cleanup();
    });
  }
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && drag) {
      const {button, id} = drag; if (drag.moved) suppressClick = button;
      cleanup(); if (button.hasPointerCapture(id)) button.releasePointerCapture(id);
    }
  });
  const detached = new URLSearchParams(location.search).get('detached');
  if (views.has(detached)) {
    setView(detached);
    document.title = `${t(`nav.${detached}`)} · 栈桥`;
  }
})();
