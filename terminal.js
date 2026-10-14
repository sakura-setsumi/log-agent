// Server terminal page: an SSH terminal (xterm.js) with an SFTP file browser and a
// command-set launcher underneath, in the style of FinalShell. The Go side owns
// the connections (term_api.go): a session is opened once and addressed by id, the
// shell's output arrives as a byte stream and keystrokes go back as small POSTs.
// This file only draws the page; it relies on the globals app.js and i18n.js
// define (state, t, showToast, escapeHtml, formatFileSize, apiErrorMessage...).
(() => {
  const view = document.getElementById('terminal-view');
  if (!view || typeof Terminal === 'undefined') return;

  const $v = (selector) => view.querySelector(selector);
  const sessions = [];
  let active = null;
  let pane = 'files';
  let sequence = 0;

  const api = (path, init) => fetch(`/api/term/${path}`, init);
  const query = (params) => new URLSearchParams(params).toString();
  const joinPath = (dir, name) => (dir === '/' ? '' : dir.replace(/\/+$/, '')) + '/' + name;
  const parentPath = (path) => path.replace(/\/[^/]*\/?$/, '') || '/';
  const errorText = async (response, fallback) => apiErrorMessage(await response.json().catch(() => ({})), fallback);

  const ICON_FOLDER = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2.2h8.5A1.5 1.5 0 0 1 21 8.7v8.8a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5z"/></svg>';
  const ICON_FILE = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 3h8l4 4v13a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z"/><path d="M14 3v4h4"/></svg>';

  // ---------------------------------------------------------------- terminals

  function createSession(server) {
    const host = document.createElement('div');
    host.className = 'term-session';
    $v('#term-screen').appendChild(host);
    const term = new Terminal({
      cursorBlink: true,
      scrollback: 5000,
      fontSize: 13,
      fontFamily: 'Consolas, "Cascadia Mono", "Courier New", monospace',
      theme: { background: '#0f1b27', foreground: '#d7e3ec', cursor: '#37d0bf', selectionBackground: '#2c5d8a' },
    });
    const fit = new FitAddon.FitAddon();
    term.loadAddon(fit);
    term.open(host);
    const session = {
      key: ++sequence,
      server,
      name: server.name || server.host,
      host,
      term,
      fit,
      sid: '',
      offset: 0,
      status: 'connecting',
      closed: false,
      pending: '',
      sending: false,
      abort: null,
      cwd: '',
      entries: [],
      selected: new Set(),
      tree: new Map(),
      open: new Set(['/']),
      resizeTimer: 0,
    };
    term.onData((data) => sendInput(session, data));
    term.onResize(({ cols, rows }) => {
      clearTimeout(session.resizeTimer);
      session.resizeTimer = setTimeout(() => {
        if (session.sid && !session.closed) api(`resize?${query({ id: session.sid, cols, rows })}`, { method: 'POST' }).catch(() => {});
      }, 150);
    });
    sessions.push(session);
    activate(session);
    return session;
  }

  // xterm only knows its cell size once it has rendered, and a freshly opened
  // terminal can be asked to fit before that. fit() then does nothing and the
  // shell stays at the default 80x24, so a failed attempt is retried shortly.
  function fitActive(retries = 0) {
    const screen = $v('#term-screen');
    const session = active;
    if (!session || !screen.clientWidth || !screen.clientHeight) return;
    let size;
    try { size = session.fit.proposeDimensions(); } catch { size = null; }
    if (!size || !Number.isFinite(size.cols) || !Number.isFinite(size.rows)) {
      if (retries < 20) setTimeout(() => { if (active === session) fitActive(retries + 1); }, 50);
      return;
    }
    try { session.fit.fit(); } catch { /* the container is not laid out yet */ }
  }

  function activate(session) {
    active = session;
    sessions.forEach((item) => item.host.classList.toggle('active', item === session));
    $v('#term-empty').classList.toggle('hidden', sessions.length > 0);
    renderTabs();
    renderFiles();
    requestAnimationFrame(() => { fitActive(); if (session && !session.closed) session.term.focus(); });
  }

  async function openServerSession(server, session) {
    const request = () => api('open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        host: server.host || '',
        port: server.port || '22',
        user: server.user || '',
        auth: server.auth || 'key',
        secret: server.auth === 'password' ? (server.secret || '') : (state.commandServerKeys?.[server.id] || ''),
        fingerprint: server.hostFingerprint || '',
        cols: session.term.cols,
        rows: session.term.rows,
      }),
    });
    let response = await request();
    let payload = await response.json().catch(() => ({}));
    if (response.status === 428 && payload.fingerprint) {
      if (!window.confirm(t('commands.confirmFingerprint', { fingerprint: payload.fingerprint }))) return false;
      server.hostFingerprint = payload.fingerprint;
      saveCommandFlows();
      response = await request();
      payload = await response.json().catch(() => ({}));
    }
    if (!response.ok) throw new Error(apiErrorMessage(payload, t('terminal.connectFailed')));
    session.sid = payload.id;
    return true;
  }

  async function connect() {
    const server = (state.commandServers || []).find((item) => item.id === $v('#term-server-select').value);
    if (!server) { showToast(t('terminal.pickServer')); return; }
    const button = $v('#term-connect');
    button.disabled = true;
    button.textContent = t('terminal.connecting');
    const session = createSession(server);
    try {
      if (!(await openServerSession(server, session))) { disposeSession(session); return; }
      session.status = 'open';
      renderTabs();
      fitActive();
      pump(session);
      sendInput(session, '');
      loadDir(session, '');
    } catch (error) {
      disposeSession(session);
      showToast(error.message || t('terminal.connectFailed'));
    } finally {
      button.disabled = false;
      button.textContent = t('terminal.connect');
    }
  }

  // The shell's output is one long response; if it drops while the shell is
  // still alive the page asks again from the offset it already has.
  async function pump(session) {
    for (let attempt = 0; attempt < 4 && !session.closed; attempt += 1) {
      session.abort = new AbortController();
      try {
        const response = await fetch(`/api/term/output?${query({ id: session.sid, from: session.offset })}`, { signal: session.abort.signal });
        if (!response.ok || !response.body) break;
        const start = Number(response.headers.get('X-Term-Offset'));
        if (Number.isFinite(start)) session.offset = start;
        const reader = response.body.getReader();
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          session.term.write(value);
          session.offset += value.length;
        }
        break; // the shell ended and everything was delivered
      } catch {
        if (session.closed) return;
        await new Promise((resolve) => setTimeout(resolve, 400));
      }
    }
    if (!session.closed) markEnded(session);
  }

  function markEnded(session) {
    session.status = 'ended';
    session.term.write(`\r\n\x1b[31m[${t('terminal.closed')}]\x1b[0m\r\n`);
    renderTabs();
  }

  // Keystrokes are batched: while one request is in flight everything typed is
  // collected and sent as the next one, which keeps them in order.
  function sendInput(session, data) {
    session.pending += data;
    if (session.sending || !session.sid) return;
    session.sending = true;
    (async () => {
      while (session.pending && !session.closed) {
        const chunk = session.pending;
        session.pending = '';
        try { await api(`input?id=${encodeURIComponent(session.sid)}`, { method: 'POST', body: chunk }); } catch { /* shown by the output stream ending */ }
      }
      session.sending = false;
    })();
  }

  function disposeSession(session) {
    session.closed = true;
    clearTimeout(session.resizeTimer);
    session.abort?.abort();
    if (session.sid) api(`close?id=${encodeURIComponent(session.sid)}`, { method: 'POST' }).catch(() => {});
    session.term.dispose();
    session.host.remove();
    const index = sessions.indexOf(session);
    if (index >= 0) sessions.splice(index, 1);
    if (active === session) {
      active = sessions[Math.max(0, index - 1)] || null;
      activate(active);
    } else {
      renderTabs();
    }
    $v('#term-empty').classList.toggle('hidden', sessions.length > 0);
  }

  function renderTabs() {
    const tabs = $v('#term-tabs');
    tabs.innerHTML = sessions.map((session) => `<div class="term-tab ${session === active ? 'active' : ''} ${session.status}" role="tab" data-key="${session.key}"><i class="term-tab-dot"></i><span>${escapeHtml(session.name)}</span><button type="button" class="term-tab-close" data-close="${session.key}" title="${escapeHtml(t('terminal.closeTab'))}" aria-label="${escapeHtml(t('terminal.closeTab'))}">×</button></div>`).join('');
  }

  // -------------------------------------------------------------------- files

  function setStatus(message) { $v('#term-status').textContent = message || ''; }

  function usable() {
    if (active && active.sid && !active.closed && active.status !== 'ended') return true;
    showToast(t('terminal.needSession'));
    return false;
  }

  async function loadDir(session, path) {
    if (!session.sid) return false;
    const response = await api(`fs/list?${query({ id: session.sid, path })}`).catch(() => null);
    if (!response || !response.ok) {
      if (session === active) showToast(response ? await errorText(response, t('terminal.file.loadFailed')) : t('terminal.file.loadFailed'));
      return false;
    }
    const data = await response.json();
    session.cwd = data.path;
    session.entries = data.entries;
    session.selected = new Set();
    session.tree.set(data.path, data.entries.filter((entry) => entry.isDir).map((entry) => entry.name));
    // The tree shows the whole way down to the current folder, so every
    // ancestor's listing is needed once.
    const parts = data.path.split('/').filter(Boolean);
    let walk = '/';
    session.open.add('/');
    const missing = [];
    for (let i = 0; i <= parts.length; i += 1) {
      session.open.add(walk);
      if (!session.tree.has(walk)) missing.push(walk);
      if (i < parts.length) walk = joinPath(walk, parts[i]);
    }
    await Promise.all(missing.map((dir) => loadTreeNode(session, dir)));
    if (session === active) renderFiles();
    return true;
  }

  async function loadTreeNode(session, path) {
    const response = await api(`fs/list?${query({ id: session.sid, path })}`).catch(() => null);
    if (!response || !response.ok) { session.tree.set(path, []); return; }
    const data = await response.json();
    session.tree.set(path, data.entries.filter((entry) => entry.isDir).map((entry) => entry.name));
  }

  function formatTime(seconds) {
    const date = new Date(seconds * 1000);
    const pad = (value) => String(value).padStart(2, '0');
    return `${date.getFullYear()}/${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
  }

  function typeLabel(entry) {
    if (entry.isDir) return t('terminal.file.folder');
    const dot = entry.name.lastIndexOf('.');
    return dot > 0 ? t('terminal.file.kind', { ext: entry.name.slice(dot + 1).toUpperCase() }) : t('terminal.file.plain');
  }

  function treeMarkup(session, path, depth) {
    const dirs = session.tree.get(path);
    const isOpen = session.open.has(path);
    const label = path === '/' ? '/' : path.slice(path.lastIndexOf('/') + 1);
    const expandable = dirs === undefined || dirs.length > 0;
    let html = `<div class="term-tree-row ${path === session.cwd ? 'active' : ''}" data-path="${escapeHtml(path)}" style="padding-left:${6 + depth * 14}px"><button type="button" class="term-tree-arrow ${expandable ? '' : 'leaf'}" data-toggle="${escapeHtml(path)}" tabindex="-1">${expandable ? (isOpen ? '▾' : '▸') : ''}</button>${ICON_FOLDER}<span>${escapeHtml(label)}</span></div>`;
    if (isOpen && dirs) html += dirs.map((name) => treeMarkup(session, joinPath(path, name), depth + 1)).join('');
    return html;
  }

  function renderFiles() {
    const rows = $v('#term-file-rows');
    const tree = $v('#term-tree');
    const path = $v('#term-path');
    if (!active) {
      rows.innerHTML = '';
      tree.innerHTML = '';
      path.value = '';
      setStatus('');
      return;
    }
    if (document.activeElement !== path) path.value = active.cwd;
    tree.innerHTML = active.cwd ? treeMarkup(active, '/', 0) : '';
    rows.innerHTML = active.entries.length
      ? active.entries.map((entry) => `<tr data-name="${escapeHtml(entry.name)}" class="${active.selected.has(entry.name) ? 'selected' : ''}"><td class="term-name">${entry.isDir ? ICON_FOLDER : ICON_FILE}<span>${escapeHtml(entry.name)}</span>${entry.isLink ? '<em>→</em>' : ''}</td><td>${entry.isDir ? '' : escapeHtml(formatFileSize(entry.size))}</td><td>${escapeHtml(typeLabel(entry))}</td><td>${escapeHtml(formatTime(entry.modTime))}</td><td>${escapeHtml(entry.mode)}</td><td>${entry.uid === 0 ? 'root' : entry.uid}/${entry.gid === 0 ? 'root' : entry.gid}</td></tr>`).join('')
      : `<tr class="term-empty-row"><td colspan="6">${escapeHtml(active.cwd ? t('terminal.file.empty') : '')}</td></tr>`;
  }

  const selectedEntries = () => (active ? active.entries.filter((entry) => active.selected.has(entry.name)) : []);

  async function runFileAction(request, failure) {
    const response = await request().catch(() => null);
    if (!response || !response.ok) {
      showToast(response ? await errorText(response, failure) : failure);
      return false;
    }
    return true;
  }

  async function makeDir() {
    if (!usable()) return;
    const name = (window.prompt(t('terminal.file.promptFolder')) || '').trim();
    if (!name || /[\\/]/.test(name)) return;
    if (await runFileAction(() => api(`fs/mkdir?${query({ id: active.sid, path: joinPath(active.cwd, name) })}`, { method: 'POST' }), t('terminal.file.loadFailed'))) loadDir(active, active.cwd);
  }

  async function renameSelected() {
    if (!usable()) return;
    const [entry, extra] = selectedEntries();
    if (!entry || extra) { showToast(t('terminal.file.selectOne')); return; }
    const name = (window.prompt(t('terminal.file.promptRename'), entry.name) || '').trim();
    if (!name || name === entry.name || /[\\/]/.test(name)) return;
    const from = joinPath(active.cwd, entry.name);
    if (await runFileAction(() => api(`fs/rename?${query({ id: active.sid, from, to: joinPath(active.cwd, name) })}`, { method: 'POST' }), t('terminal.file.loadFailed'))) loadDir(active, active.cwd);
  }

  async function deleteSelected() {
    if (!usable()) return;
    const entries = selectedEntries();
    if (!entries.length) { showToast(t('terminal.file.selectOne')); return; }
    const message = entries.length === 1 ? t('terminal.file.confirmDelete', { name: entries[0].name }) : t('terminal.file.confirmDeleteMany', { count: entries.length });
    if (!window.confirm(message)) return;
    for (const entry of entries) {
      const path = joinPath(active.cwd, entry.name);
      if (!(await runFileAction(() => api(`fs/remove?${query({ id: active.sid, path })}`, { method: 'POST' }), t('terminal.file.loadFailed')))) break;
      active.tree.delete(path);
    }
    loadDir(active, active.cwd);
  }

  async function downloadEntry(entry) {
    const session = active;
    const path = joinPath(session.cwd, entry.name);
    setStatus(t('terminal.file.downloading', { name: entry.name }));
    const response = await api(`fs/download?${query({ id: session.sid, path })}`).catch(() => null);
    if (!response || !response.ok) {
      showToast(response ? await errorText(response, t('terminal.file.loadFailed')) : t('terminal.file.loadFailed'));
      setStatus('');
      return;
    }
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = entry.name;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 10000);
    setStatus(t('terminal.file.downloaded', { name: entry.name }));
  }

  async function downloadSelected() {
    if (!usable()) return;
    const files = selectedEntries().filter((entry) => !entry.isDir);
    if (!files.length) { showToast(t('terminal.file.selectFiles')); return; }
    for (const entry of files) await downloadEntry(entry);
  }

  function uploadTarget() {
    if (!usable() || !active.cwd) return null;
    return { session: active, directory: active.cwd };
  }

  // Both picker paths and dropped folders include the root folder name.
  function uploadEntries(items) {
    const entries = items.flatMap((item) => item.folder
      ? [{ path: item.name, directory: true }, ...item.entries.map((entry) => ({ ...entry, path: `${item.name}/${entry.path}` }))]
      : [{ path: item.webkitRelativePath || item.name, file: item }]);
    const directories = new Set();
    for (const entry of entries) {
      const parts = entry.path.split('/');
      if (parts.some((part) => !part || part === '.' || part === '..' || /[\\\0]/.test(part))) throw new Error(t('api.invalid_path'));
      const count = entry.directory ? parts.length : parts.length - 1;
      for (let i = 1; i <= count; i += 1) directories.add(parts.slice(0, i).join('/'));
    }
    return { directories: [...directories], files: entries.filter((entry) => !entry.directory) };
  }

  function uploadFiles(source, target) {
    if (!target) return;
    // Read drag entries immediately, even when another upload is still queued.
    const collected = Promise.resolve(source).then((items) => ({ items }), (error) => ({ error }));
    const { session, directory } = target;
    const status = (message) => { if (session === active) setStatus(message); };
    session.uploadQueue = (session.uploadQueue || Promise.resolve()).then(async () => {
      const result = await collected;
      if (result.error) throw new Error(t('terminal.file.readFailed', { error: result.error.message || String(result.error) }));
      const plan = uploadEntries(result.items);
      if (!plan.directories.length && !plan.files.length) return;
      const request = async (action, relativePath, body) => {
        if (session.closed || session.status === 'ended') throw new Error(t('terminal.needSession'));
        const name = relativePath;
        const response = await api(`fs/${action}?${query({ id: session.sid, path: joinPath(directory, relativePath) })}`, { method: 'POST', body }).catch(() => null);
        if (!response || !response.ok) throw new Error(response ? await errorText(response, t('terminal.file.uploadFailed', { name })) : t('terminal.file.uploadFailed', { name }));
      };
      try {
        for (const path of plan.directories) {
          status(t('terminal.file.creatingFolder', { name: path }));
          await request('mkdir', path);
        }
        for (const { path, file } of plan.files) {
          status(t('terminal.file.uploading', { name: path }));
          await request('upload', path, file);
        }
        const message = plan.directories.length
          ? t('terminal.file.uploadedItems', { files: plan.files.length, folders: plan.directories.length })
          : t('terminal.file.uploaded', { count: plan.files.length });
        status(message);
        showToast(message);
      } finally {
        if (!session.closed && session.status !== 'ended') {
          session.tree.clear();
          await loadDir(session, session.cwd);
        }
      }
    }).catch((error) => {
      const message = error.message || t('terminal.file.loadFailed');
      status(message);
      showToast(message);
    });
  }

  // ----------------------------------------------------------------- commands

  function renderCommands() {
    const list = $v('#term-cmd-list');
    const flows = state.commandFlows || [];
    if (!flows.length) { list.innerHTML = `<div class="term-cmd-empty">${escapeHtml(t('terminal.commands.empty'))}</div>`; return; }
    list.innerHTML = flows.map((flow) => {
      const lines = (flow.lines || []).filter((line) => line.type !== 'upload' && String(line.text || '').trim());
      return `<div class="term-cmd-item"><div><strong>${escapeHtml(flow.name || t('commands.unnamedFlow'))}</strong><small>${escapeHtml(t('terminal.commands.lines', { count: lines.length }))}</small><code>${escapeHtml(lines.map((line) => line.text).join('  ›  ').slice(0, 160))}</code></div><button type="button" class="secondary-button compact-button" data-send-flow="${escapeHtml(flow.id)}" ${lines.length ? '' : 'disabled'}>${escapeHtml(t('terminal.commands.send'))}</button></div>`;
    }).join('');
  }

  function sendFlow(flowId) {
    if (!usable()) return;
    const flow = (state.commandFlows || []).find((item) => item.id === flowId);
    const lines = (flow?.lines || []).filter((line) => line.type !== 'upload' && String(line.text || '').trim());
    if (!lines.length) return;
    sendInput(active, `${lines.map((line) => line.text).join('\r')}\r`);
    showToast(t('terminal.commands.sent', { name: flow.name || '' }));
    active.term.focus();
  }

  // --------------------------------------------------------------- page chrome

  function renderServers() {
    const select = $v('#term-server-select');
    const previous = select.value;
    const servers = state.commandServers || [];
    select.innerHTML = servers.length
      ? servers.map((server) => `<option value="${escapeHtml(server.id)}">${escapeHtml(`${server.name || server.host} · ${server.user || ''}@${server.host || ''}`)}</option>`).join('')
      : `<option value="">${escapeHtml(t('terminal.noServers'))}</option>`;
    if (servers.some((server) => server.id === previous)) select.value = previous;
    $v('#term-connect').disabled = !servers.length;
  }

  function setPane(name) {
    pane = name;
    view.querySelectorAll('[data-term-pane]').forEach((button) => button.classList.toggle('active', button.dataset.termPane === name));
    $v('#term-pane-files').classList.toggle('hidden', name !== 'files');
    $v('#term-pane-commands').classList.toggle('hidden', name !== 'commands');
    if (name === 'commands') renderCommands();
  }

  window.renderTerminalView = ({ shown = false } = {}) => {
    renderServers();
    renderTabs();
    renderFiles();
    if (pane === 'commands') renderCommands();
    if (shown) requestAnimationFrame(() => { fitActive(); if (active && !active.closed) active.term.focus(); });
  };

  function bind() {
    $v('#term-connect').addEventListener('click', connect);
    $v('#term-manage-servers').addEventListener('click', () => openServerManager());
    $v('#term-tabs').addEventListener('click', (event) => {
      const close = event.target.closest('[data-close]');
      if (close) {
        const session = sessions.find((item) => item.key === Number(close.dataset.close));
        if (session) disposeSession(session);
        return;
      }
      const tab = event.target.closest('[data-key]');
      const session = tab && sessions.find((item) => item.key === Number(tab.dataset.key));
      if (session && session !== active) activate(session);
    });
    view.querySelectorAll('[data-term-pane]').forEach((button) => button.addEventListener('click', () => setPane(button.dataset.termPane)));

    $v('#term-refresh').addEventListener('click', () => { if (usable()) { active.tree.clear(); loadDir(active, active.cwd); } });
    $v('#term-up').addEventListener('click', () => { if (usable() && active.cwd) loadDir(active, parentPath(active.cwd)); });
    $v('#term-mkdir').addEventListener('click', makeDir);
    $v('#term-rename').addEventListener('click', renameSelected);
    $v('#term-delete').addEventListener('click', deleteSelected);
    $v('#term-download').addEventListener('click', downloadSelected);
    [['#term-upload', '#term-upload-input'], ['#term-upload-folder', '#term-upload-folder-input']].forEach(([button, selector]) => {
      const input = $v(selector);
      let target = null;
      $v(button).addEventListener('click', () => {
        target = uploadTarget();
        if (target) input.click();
      });
      input.addEventListener('change', () => {
        const files = [...input.files];
        const destination = target || uploadTarget();
        target = null;
        input.value = '';
        if (files.length) uploadFiles(files, destination);
      });
      input.addEventListener('cancel', () => { target = null; });
    });
    $v('#term-path').addEventListener('keydown', (event) => {
      if (event.key !== 'Enter' || !usable()) return;
      const value = event.target.value.trim();
      if (value.startsWith('/')) loadDir(active, value).then((ok) => { if (!ok) event.target.value = active.cwd; });
    });

    $v('#term-tree').addEventListener('click', (event) => {
      if (!active) return;
      const toggle = event.target.closest('[data-toggle]');
      const row = event.target.closest('[data-path]');
      if (!row) return;
      const path = row.dataset.path;
      if (toggle) {
        if (active.open.has(path)) { active.open.delete(path); renderFiles(); return; }
        active.open.add(path);
        (active.tree.has(path) ? Promise.resolve() : loadTreeNode(active, path)).then(renderFiles);
        return;
      }
      loadDir(active, path);
    });

    const rows = $v('#term-file-rows');
    rows.addEventListener('click', (event) => {
      const row = event.target.closest('tr[data-name]');
      if (!row || !active) return;
      const name = row.dataset.name;
      if (event.ctrlKey || event.metaKey) {
        if (active.selected.has(name)) active.selected.delete(name); else active.selected.add(name);
      } else {
        active.selected = new Set([name]);
      }
      // Only the highlight changes: rebuilding the rows here would swap the node
      // out between the two clicks of a double-click, which then never fires.
      rows.querySelectorAll('tr[data-name]').forEach((item) => item.classList.toggle('selected', active.selected.has(item.dataset.name)));
    });
    rows.addEventListener('dblclick', (event) => {
      const row = event.target.closest('tr[data-name]');
      if (!row || !active) return;
      const entry = active.entries.find((item) => item.name === row.dataset.name);
      if (!entry) return;
      if (entry.isDir) loadDir(active, joinPath(active.cwd, entry.name)); else downloadEntry(entry);
    });

    const wrap = $v('#term-table-wrap');
    const hasFiles = (event) => [...(event.dataTransfer?.types || [])].includes('Files');
    wrap.addEventListener('dragover', (event) => { if (!hasFiles(event)) return; event.preventDefault(); wrap.classList.add('drag-active'); });
    wrap.addEventListener('dragleave', (event) => { if (!wrap.contains(event.relatedTarget)) wrap.classList.remove('drag-active'); });
    wrap.addEventListener('drop', (event) => {
      if (!hasFiles(event)) return;
      event.preventDefault();
      wrap.classList.remove('drag-active');
      const target = uploadTarget();
      if (target) uploadFiles(collectCommandTransferFiles(event.dataTransfer), target);
    });

    // The divider between the terminal and the lower panel.
    const splitter = $v('#term-splitter');
    const workspace = $v('#term-workspace');
    splitter.addEventListener('pointerdown', (event) => {
      event.preventDefault();
      splitter.setPointerCapture(event.pointerId);
      splitter.classList.add('dragging');
    });
    splitter.addEventListener('pointermove', (event) => {
      if (!splitter.classList.contains('dragging')) return;
      const box = workspace.getBoundingClientRect();
      const lower = Math.min(Math.max(box.bottom - event.clientY, 120), box.height - 160);
      workspace.style.setProperty('--term-lower-h', `${lower}px`);
      fitActive();
    });
    const endDrag = () => splitter.classList.remove('dragging');
    splitter.addEventListener('pointerup', endDrag);
    splitter.addEventListener('pointercancel', endDrag);

    view.addEventListener('click', (event) => {
      const send = event.target.closest('[data-send-flow]');
      if (send) sendFlow(send.dataset.sendFlow);
    });

    if (typeof ResizeObserver === 'function') {
      let frame = 0;
      new ResizeObserver(() => { cancelAnimationFrame(frame); frame = requestAnimationFrame(fitActive); }).observe($v('#term-screen'));
    }
  }

  bind();
})();
