// Application menu bar. Items do not carry logic of their own: each one clicks
// the control that already does the job (data-click), or calls a global
// (data-call), so the menu and the page can never disagree. Loaded after
// bootstrap.js, once every handler it points at is bound.
(() => {
  const bar = document.getElementById('menubar');
  if (!bar) return;
  const menus = [...bar.querySelectorAll('.menu')];

  const openMenu = (menu) => {
    menus.forEach((other) => {
      const open = other === menu;
      other.classList.toggle('open', open);
      other.querySelector('.menu-title').setAttribute('aria-expanded', String(open));
    });
    if (menu) refresh(menu);
  };
  const closeAll = () => openMenu(null);
  const isOpen = () => menus.some((menu) => menu.classList.contains('open'));

  // Labels and check marks mirror the live state of the controls behind them.
  function refresh(menu) {
    menu.querySelectorAll('[data-label-from]').forEach((item) => {
      const source = document.querySelector(item.dataset.labelFrom);
      const label = source && (source.title || source.getAttribute('aria-label'));
      if (label) item.textContent = label;
    });
    menu.querySelectorAll('[data-checked-from]').forEach((item) => {
      const source = document.querySelector(item.dataset.checkedFrom);
      item.setAttribute('aria-checked', source?.getAttribute('aria-checked') === 'true' ? 'true' : 'false');
    });
  }

  menus.forEach((menu) => {
    const title = menu.querySelector('.menu-title');
    title.addEventListener('click', () => (menu.classList.contains('open') ? closeAll() : openMenu(menu)));
    // Once a menu is open, moving along the bar switches menus, as in a desktop app.
    title.addEventListener('mouseenter', () => { if (isOpen()) openMenu(menu); });
  });

  bar.addEventListener('click', (event) => {
    const item = event.target instanceof Element ? event.target.closest('.menu-item') : null;
    if (!item) return;
    closeAll();
    if (item.hasAttribute('data-quit')) {
      if (typeof window.__lbQuit === 'function') window.__lbQuit();
      else window.close();
    } else if (item.dataset.click) {
      document.querySelector(item.dataset.click)?.click();
    } else if (item.dataset.call && typeof window[item.dataset.call] === 'function') {
      window[item.dataset.call]();
    }
  });

  document.addEventListener('mousedown', (event) => {
    if (isOpen() && !bar.contains(event.target)) closeAll();
  }, true);
  window.addEventListener('blur', closeAll);
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && isOpen()) {
      event.stopPropagation();
      closeAll();
    }
  }, true);
})();
