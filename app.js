const nodes = [];
const logPageSize = 50;
const initialLogPreviewSize = 50;
const logVirtualRowHeight = 74;
const logVirtualOverscan = 60;
const adaptiveLogRenderLimit = 500;
const streamBatchInterval = 100;
const logRenderInterval = 250;
const maxPendingStreamLogs = 2000;
let maxBufferedLogs = 100000;
const selectionStorageKey = 'log-agent-selection';
const aiProfilesStorageKey = 'log-agent-ai-profiles';
const aiActiveProfileStorageKey = 'log-agent-ai-active-profile';
const aiActiveModelStorageKey = 'log-agent-ai-active-model';
const assistantSessionsStorageKey = 'log-agent-ai-sessions';
const browserNodesStorageKey = 'log-agent-browser-nodes';
const maxAssistantSessions = 12;

const state = {
  selectedNodes: [],
  selectedContainers: [],
  expandedNodes: [],
  level: 'all',
  query: '',
  globalQuery: '',
  paused: false,
  selectedLog: 0,
  activeAnalysisLogId: '',
  range: '30m',
  historyLoading: false,
  visibleLogs: logPageSize,
  logs: [],
  processed: 0,
  ruleState: { mask: true, structure: true, noise: false },
  ruleOrder: ['mask', 'structure', 'noise'],
  containerLogCache: {},
  configInfo: null,
  storageMode: '',
  fullRangeSearchLogs: [],
  fullRangeSearchKey: '',
  fullRangeSearchLoading: false,
  assistantContext: [],
  assistantMessages: [],
  assistantAttachments: [],
  assistantBusy: false,
  assistantSessions: [],
  activeAssistantSessionId: '',
  aiProfiles: [],
  activeAIProfileId: '',
  activeAIModelId: '',
  settingsAIProfileId: '',
  aiEnvConfigured: false,
  aiModel: ''
};

let goServerConnected = false;
let eventStream;
let assistantRequestController = null;
let assistantRequestId = 0;
let loadedContainerSelectionKey = '';
let containerLogRetryTimer;
let fullRangeSearchTimer;
let fullRangeSearchRequest = 0;
let olderLogsLoading = false;
const olderLogExhausted = new Set();
let backendRefreshTimer;
let historySyncTimer;
let historyRequestVersion = 0;
let lastSyncedLogTimestamp = 0;
let initialLogPreviewRendered = false;
let initialLogPreviewActive = false;
let initialLogPreviewLimit = 0;
let initialLogPreviewTimer;
let aiAdminToken = '';
let aiAdminTokenResolver = null;
// Mirrors the server's hasAdminToken. When it is false the server accepts
// privileged writes without a token, so prompting for one would block a save
// that would otherwise succeed — and no answer could ever be correct. Set from
// /api/config/info at startup and from /api/settings when the panel opens.
let serverAdminTokenConfigured = false;
let logRenderTimer;
let logScrollFrame;
let streamBatchTimer;
let pendingStreamLogs = [];
let lastFilteredLogs = [];
let lastVirtualWindowKey = '';
let logTextSelectionActive = false;
let followLatestLogs = true;

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => Array.from(document.querySelectorAll(selector));

function bindBackdropDismissal(modal, close) {
  let startedOnBackdrop = false;
  let endedOnBackdrop = false;
  modal.addEventListener('pointerdown', (event) => {
    startedOnBackdrop = event.target === modal;
    endedOnBackdrop = false;
  });
  modal.addEventListener('pointerup', (event) => {
    endedOnBackdrop = event.target === modal;
  });
  modal.addEventListener('pointercancel', () => {
    startedOnBackdrop = false;
    endedOnBackdrop = false;
  });
  modal.addEventListener('click', (event) => {
    if (event.target === modal && startedOnBackdrop && endedOnBackdrop) close();
    startedOnBackdrop = false;
    endedOnBackdrop = false;
  });
}

function escapeHtml(value) {
  return String(value).replace(/[&<>'"]/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
}

function searchHighlightTerms() {
  return [...new Set([state.query, state.globalQuery].map((value) => String(value || '').trim()).filter(Boolean))];
}

function highlightEscapedText(value, terms = []) {
  if (!terms.length) return value;
  const pattern = terms.map((term) => term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|');
  if (!pattern) return value;
  const matcher = new RegExp(pattern, 'gi');
  return value.split(/(<[^>]+>)/g).map((part) => part.startsWith('<') ? part : part.replace(matcher, '<mark class="log-search-hit">$&</mark>')).join('');
}

function highlightSearchText(value) {
  return highlightEscapedText(escapeHtml(value), searchHighlightTerms());
}

function maskLogText(value, terms = []) {
  let display = escapeHtml(value);
  if (state.ruleState.mask) display = display.replace(/(\*{2,}|\b(?:\d{1,3}\.){3}\d{1,3}\b|usr_[a-z0-9]+|ord_[a-z0-9]+|file_[a-z0-9]+|s_[a-z0-9]+)/gi, '<em class="masked">$1</em>');
  return highlightEscapedText(display, terms);
}

function trimLogURL(value) {
  let url = String(value);
  let trailing = '';
  while (/[),.;!?，。；！？、》）】\]}]$/u.test(url)) {
    trailing = url.slice(-1) + trailing;
    url = url.slice(0, -1);
  }
  return { url, trailing };
}

function renderLogLink(url, label = url, terms = []) {
  const safeURL = String(url).trim();
  if (!/^https?:\/\//i.test(safeURL)) return maskLogText(label, terms);
  return `<a class="log-link" href="${escapeHtml(safeURL)}" target="_blank" rel="noopener noreferrer">${maskLogText(label || safeURL, terms)}</a>`;
}

function linkifyPlainLogText(value, terms = []) {
  const source = String(value ?? '');
  const urlPattern = /https?:\/\/[^\s<>"'`]+/gi;
  let output = '';
  let cursor = 0;
  source.replace(urlPattern, (match, offset) => {
    const { url, trailing } = trimLogURL(match);
    output += maskLogText(source.slice(cursor, offset), terms);
    output += renderLogLink(url, url, terms);
    if (trailing) output += maskLogText(trailing, terms);
    cursor = offset + match.length;
    return match;
  });
  return output + maskLogText(source.slice(cursor), terms);
}

function renderLogMessage(message, terms = []) {
  const source = String(message ?? '');
  const anchorPattern = /<a\b[^>]*\bhref\s*=\s*(['"])(https?:\/\/[^'"]+)\1[^>]*>([\s\S]*?)<\/a>/gi;
  let output = '';
  let cursor = 0;
  source.replace(anchorPattern, (match, quote, url, inner, offset) => {
    output += linkifyPlainLogText(source.slice(cursor, offset), terms);
    const label = inner.replace(/<[^>]*>/g, '').trim() || url;
    output += renderLogLink(url, label, terms);
    cursor = offset + match.length;
    return match;
  });
  return output + linkifyPlainLogText(source.slice(cursor), terms);
}

function restoreSelection() {
  try {
    const saved = JSON.parse(localStorage.getItem(selectionStorageKey) || 'null');
    if (!saved || typeof saved !== 'object') return;
    if (Array.isArray(saved.nodes)) state.selectedNodes = saved.nodes.filter((id) => typeof id === 'string' && id);
    if (Array.isArray(saved.containers)) state.selectedContainers = saved.containers.filter((key) => typeof key === 'string' && key);
  } catch (error) {
    // Ignore unavailable or invalid browser storage and use the default selection.
  }
}

function persistSelection() {
  try {
    localStorage.setItem(selectionStorageKey, JSON.stringify({
      nodes: state.selectedNodes,
      containers: state.selectedContainers
    }));
  } catch (error) {
    // The selection still works for the current page when storage is unavailable.
  }
}

function normalizeAIProvider(profile, index = 0) {
  if (!profile || typeof profile !== 'object') return null;
  const name = String(profile.name || '').trim();
  const baseURL = String(profile.baseURL || '').trim().replace(/\/+$/, '');
  if (!name || !baseURL) return null;
  const type = profile.type === 'anthropic' ? 'anthropic' : 'openai';
  let models = Array.isArray(profile.models) ? profile.models : [];
  if (!models.length && profile.model) models = [{ id: `model-${index}-legacy`, name: profile.model }];
  models = models.map((model, modelIndex) => ({
    id: String(model?.id || `model-${index}-${modelIndex}-${Date.now()}`),
    name: String(model?.name || '').trim()
  })).filter((model) => model.name);
  return {
    id: String(profile.id || `ai-profile-${index}-${Date.now()}`),
    name,
    baseURL,
    apiKey: String(profile.apiKey || '').trim(),
    type,
    enabled: profile.enabled !== false,
    models
  };
}

// loadAIProfiles restores this browser's own providers.
//
// It is deliberately a no-op in file mode. There the configuration belongs to
// the machine running the service, and /api/ai/profiles is the authority;
// starting from localStorage would make stale cached providers reappear on a
// page that is supposed to show only what is in models.json. bootstrap.js calls
// this before the storage mode is known, so applyStorageMode re-runs it once
// the mode resolves.
function loadAIProfiles() {
  if (state.storageMode === 'file') {
    state.aiProfiles = [];
    state.activeAIProfileId = '';
    state.activeAIModelId = '';
    return;
  }
  try {
    const saved = JSON.parse(localStorage.getItem(aiProfilesStorageKey) || '[]');
    if (Array.isArray(saved)) state.aiProfiles = saved.map(normalizeAIProvider).filter(Boolean);
    state.activeAIProfileId = localStorage.getItem(aiActiveProfileStorageKey) || '';
    state.activeAIModelId = localStorage.getItem(aiActiveModelStorageKey) || '';
    if (!state.aiProfiles.some((profile) => profile.id === state.activeAIProfileId)) {
      state.activeAIProfileId = '';
      state.activeAIModelId = '';
    }
  } catch (error) {
    state.aiProfiles = [];
    state.activeAIProfileId = '';
    state.activeAIModelId = '';
  }
}

function copyAssistantSessionData(session) {
  return {
    context: (session.context || []).slice(0, 20).map((log) => ({ id: log.id, date: log.date, time: log.time, timestamp: log.timestamp, level: log.level, node: log.node, container: log.container, message: log.message })),
    messages: (session.messages || []).slice(-12).map((message) => ({ role: message.role === 'assistant' ? 'assistant' : 'user', content: String(message.content || '').slice(0, 16000), error: Boolean(message.error) }))
  };
}

function persistAssistantSessions() {
  try { localStorage.setItem(assistantSessionsStorageKey, JSON.stringify(state.assistantSessions.slice(0, maxAssistantSessions))); } catch (error) { /* Local history is optional. */ }
}

function activeAssistantSession() {
  return state.assistantSessions.find((session) => session.id === state.activeAssistantSessionId) || null;
}

function syncActiveAssistantSession() {
  const session = activeAssistantSession();
  if (!session) return;
  Object.assign(session, copyAssistantSessionData({ context: state.assistantContext, messages: state.assistantMessages }), { activeLogId: state.activeAnalysisLogId, updatedAt: Date.now() });
  if (session.title === '新对话' && session.messages.length) session.title = String(session.messages.find((message) => message.role === 'user')?.content || '新对话').replace(/\s+/g, ' ').slice(0, 80);
  state.assistantSessions = [session, ...state.assistantSessions.filter((item) => item.id !== session.id)].slice(0, maxAssistantSessions);
  persistAssistantSessions();
}

function startAssistantSession({ context = [], activeLogId = '', title = '新对话' } = {}) {
  syncActiveAssistantSession();
  const now = Date.now();
  const session = { id: `assistant-session-${now}-${Math.random().toString(36).slice(2, 8)}`, title: String(title).slice(0, 80) || '新对话', createdAt: now, updatedAt: now, activeLogId: String(activeLogId || ''), ...copyAssistantSessionData({ context, messages: [] }) };
  state.assistantSessions = [session, ...state.assistantSessions].slice(0, maxAssistantSessions);
  state.activeAssistantSessionId = session.id;
  state.activeAnalysisLogId = session.activeLogId;
  state.assistantContext = session.context;
  state.assistantMessages = [];
  state.assistantAttachments = [];
  persistAssistantSessions();
}

function selectAssistantSession(id) {
  const session = state.assistantSessions.find((item) => item.id === id);
  if (!session) return;
  state.activeAssistantSessionId = session.id;
  state.activeAnalysisLogId = session.activeLogId;
  state.selectedLog = Number(session.activeLogId) || 0;
  const copied = copyAssistantSessionData(session);
  state.assistantContext = copied.context;
  state.assistantMessages = copied.messages;
  state.assistantAttachments = [];
  closeAssistantSessionMenu();
  renderLogs();
  renderAssistant();
}

function deleteAssistantSession(id) {
  const active = id === state.activeAssistantSessionId;
  state.assistantSessions = state.assistantSessions.filter((session) => session.id !== id);
  if (active) {
    if (state.assistantSessions[0]) selectAssistantSession(state.assistantSessions[0].id);
    else startAssistantSession();
  }
  persistAssistantSessions();
  renderAssistant();
}

function closeAssistantSessionMenu() {
  $('#assistant-session-menu')?.classList.add('hidden');
  $('#assistant-session-toggle')?.setAttribute('aria-expanded', 'false');
}

function renderAssistantSessions() {
  const list = $('#assistant-session-list');
  if (!list) return;
  list.innerHTML = state.assistantSessions.length ? state.assistantSessions.map((session) => `<div class="assistant-session-item${session.id === state.activeAssistantSessionId ? ' active' : ''}"><button class="assistant-session-select" type="button" data-select-assistant-session="${escapeHtml(session.id)}"><span>${escapeHtml(session.title)}</span><small>${new Date(session.updatedAt).toLocaleString('zh-CN', { hour12: false })}</small></button><button class="assistant-session-remove" type="button" data-delete-assistant-session="${escapeHtml(session.id)}" aria-label="删除会话">×</button></div>`).join('') : '<div class="assistant-session-empty">暂无历史会话</div>';
}

function loadAssistantSessions() {
  try {
    const saved = JSON.parse(localStorage.getItem(assistantSessionsStorageKey) || '[]');
    state.assistantSessions = Array.isArray(saved) ? saved.filter((session) => session && session.id).slice(0, maxAssistantSessions).map((session) => ({ id: String(session.id), title: String(session.title || '新对话').slice(0, 80), createdAt: Number(session.createdAt) || Date.now(), updatedAt: Number(session.updatedAt) || Date.now(), activeLogId: String(session.activeLogId || ''), ...copyAssistantSessionData(session) })) : [];
    if (state.assistantSessions[0]) selectAssistantSession(state.assistantSessions[0].id);
  } catch (error) { state.assistantSessions = []; }
}

async function loadAIProfilesFromFile() {
  try {
    const response = await fetch('/api/ai/profiles', { headers: { Accept: 'application/json' } });
    if (!response.ok) return;
    const payload = await response.json();
    const storedProfiles = Array.isArray(payload.profiles) ? payload.profiles.map(normalizeAIProvider).filter(Boolean) : [];
    // The file is the authority in file mode, so this is a replace rather than
    // a merge. An empty result is a legitimate state (nothing configured yet)
    // and must clear the list; merging here would keep resurrecting whatever
    // this browser happens to have cached, so a deleted provider would reappear
    // on the next load.
    state.aiProfiles = storedProfiles;
    if (!state.aiProfiles.some((profile) => profile.id === state.activeAIProfileId)) {
      state.activeAIProfileId = storedProfiles[0]?.id || '';
      state.activeAIModelId = storedProfiles[0]?.models[0]?.id || '';
    } else if (!activeAIModel()) {
      state.activeAIModelId = activeAIProfile()?.models[0]?.id || '';
    }
    // No persistAIProfiles() here on purpose: in file mode localStorage holds
    // only this browser's own providers, so writing the file's contents into it
    // would be exactly the cross-contamination this replace is fixing.
    renderAIProviderList();
    renderAIModelList();
    renderAssistant();
  } catch (error) {
    // Keep whatever is already loaded; a refresh will retry.
  }
}

function persistAIProfiles() {
  // In file mode the providers live in models.json, not here. Mirroring them
  // into localStorage would leave a stale copy that a later localStorage-backed
  // page would render as if it were current.
  if (state.storageMode === 'file') return;
  try {
    localStorage.setItem(aiProfilesStorageKey, JSON.stringify(state.aiProfiles));
    if (state.activeAIProfileId) localStorage.setItem(aiActiveProfileStorageKey, state.activeAIProfileId);
    else localStorage.removeItem(aiActiveProfileStorageKey);
    if (state.activeAIModelId) localStorage.setItem(aiActiveModelStorageKey, state.activeAIModelId);
    else localStorage.removeItem(aiActiveModelStorageKey);
  } catch (error) {
    // The current profile still works until the page is closed when storage is unavailable.
  }
}

function activeAIProfile() {
  return state.aiProfiles.find((profile) => profile.id === state.activeAIProfileId && profile.enabled !== false) || null;
}

function activeAIModel() {
  const profile = activeAIProfile();
  return profile?.models.find((model) => model.id === state.activeAIModelId) || profile?.models[0] || null;
}

function aiProfileConfigured(profile, model = activeAIModel()) {
  return Boolean(profile?.baseURL?.trim() && model?.name?.trim());
}

function currentAIConfigured() {
  const profile = activeAIProfile();
  return profile ? aiProfileConfigured(profile) : state.aiEnvConfigured;
}

function closeAIModelMenu() {
  const menu = $('#assistant-model-menu');
  const trigger = $('#assistant-model-trigger');
  if (!menu || !trigger) return;
  menu.classList.add('hidden');
  trigger.setAttribute('aria-expanded', 'false');
}

function renderAIModelPicker() {
  const triggerName = $('#assistant-model-name');
  const options = $('#assistant-model-options');
  if (!triggerName || !options) return;
  const profile = activeAIProfile();
  const model = activeAIModel();
  triggerName.textContent = profile && model ? model.name : state.aiEnvConfigured ? (state.aiModel || '服务端默认模型') : '选择模型';
  const envOption = state.aiEnvConfigured ? `<button class="assistant-model-option${!profile ? ' selected' : ''}" type="button" data-select-ai-model="" data-model-id=""><span>${escapeHtml(state.aiModel || '服务端默认模型')}</span><small>服务端默认配置</small></button>` : '';
  const profileOptions = state.aiProfiles.flatMap((provider) => provider.enabled === false ? [] : provider.models.map((item) => `<button class="assistant-model-option${provider.id === state.activeAIProfileId && item.id === state.activeAIModelId ? ' selected' : ''}" type="button" data-select-ai-model="${escapeHtml(provider.id)}" data-model-id="${escapeHtml(item.id)}"><span>${escapeHtml(item.name)}</span><small>${escapeHtml(provider.name)}</small></button>`)).join('');
  options.innerHTML = envOption + profileOptions || '<div class="assistant-model-empty">请先在“管理模型”中添加配置</div>';
  $$('[data-select-ai-model]').forEach((button) => button.addEventListener('click', () => selectAIModel(button.dataset.selectAiModel, button.dataset.modelId)));
}

function selectAIModel(providerId, modelId) {
  state.activeAIProfileId = providerId || '';
  state.activeAIModelId = modelId || '';
  persistAIProfiles();
  closeAIModelMenu();
  renderAssistant();
  const profile = activeAIProfile();
  const model = activeAIModel();
  showToast(profile && model ? `已切换至 ${profile.name} / ${model.name}` : '已切换至服务端默认模型');
}

function renderAIProviderList() {
  const list = $('#ai-provider-list');
  if (!list) return;
  list.innerHTML = `${state.aiProfiles.map((profile) => `
    <button class="ai-provider-item${profile.id === state.settingsAIProfileId ? ' active' : ''}" type="button" data-select-ai-provider="${escapeHtml(profile.id)}"><span class="ai-provider-icon">◈</span><span>${escapeHtml(profile.name)}</span><i class="ai-provider-status ${profile.enabled === false ? 'offline' : 'connected'}"></i></button>
  `).join('')}<button class="ai-add-provider" type="button" id="ai-add-provider">＋ 添加供应商</button>`;
  $$('[data-select-ai-provider]').forEach((button) => button.addEventListener('click', () => selectAISettingsProvider(button.dataset.selectAiProvider)));
  $('#ai-add-provider')?.addEventListener('click', () => selectAISettingsProvider(''));
}

function renderAIModelList() {
  const list = $('#ai-model-list');
  const profile = state.aiProfiles.find((item) => item.id === state.settingsAIProfileId);
  if (!list) return;
  list.innerHTML = profile?.models?.length ? profile.models.map((model) => `
    <div class="ai-model-item"><span>${escapeHtml(model.name)}</span><div><button class="text-button" type="button" data-edit-ai-model="${escapeHtml(model.id)}">编辑</button><button class="text-button danger-text" type="button" data-delete-ai-model="${escapeHtml(model.id)}">删除</button></div></div>
  `).join('') : '<div class="ai-model-empty">暂无模型，请添加一个模型</div>';
  $$('[data-edit-ai-model]').forEach((button) => button.addEventListener('click', () => editAIModel(button.dataset.editAiModel)));
  $$('[data-delete-ai-model]').forEach((button) => button.addEventListener('click', () => deleteAIModel(button.dataset.deleteAiModel)));
}

function fillAISettingsForm() {
  const form = $('#ai-profile-form');
  if (!form) return;
  const profile = state.aiProfiles.find((item) => item.id === state.settingsAIProfileId);
  form.elements.profileId.value = profile?.id || '';
  form.elements.profileName.value = profile?.name || '';
  form.elements.baseURL.value = profile?.baseURL || '';
  form.elements.apiKey.value = profile?.apiKey || '';
  form.elements.connectionType.value = profile?.type || 'openai';
  form.elements.enabled.checked = profile?.enabled !== false;
  $('#ai-editor-provider-name').textContent = profile?.name || '新供应商';
  $('#ai-editor-provider-status').textContent = profile?.enabled === false ? '未启用' : profile ? '已启用' : '新配置';
  $('#ai-new-model-name').value = '';
  renderAIModelList();
}

function renderAssistant() {
  const panel = $('#assistant-panel');
  if (!panel) return;
  renderAssistantSessions();
  renderAIModelPicker();
  renderAssistantAttachments();
  // Keep neighboring rows available to the model, while showing only the log
  // the user explicitly selected in the assistant header.
  const visibleContext = state.activeAnalysisLogId
    ? state.assistantContext.filter((log) => String(log.id) === state.activeAnalysisLogId).slice(0, 1)
    : state.assistantContext.slice(0, 1);
  $('#assistant-context-count').textContent = String(visibleContext.length);
  const contextList = $('#assistant-context-list');
  contextList.innerHTML = visibleContext.length ? visibleContext.map((log) => `
    <div class="assistant-context-item" data-assistant-context-id="${escapeHtml(log.id)}">
      <span class="assistant-context-level ${escapeHtml(log.level)}">${escapeHtml(log.level || 'log').toUpperCase()}</span>
      <span class="assistant-context-copy" title="${escapeHtml(`${log.node} / ${log.container}\n${log.message}`)}">${escapeHtml(log.node)} / ${escapeHtml(log.container)}: ${escapeHtml(log.message)}</span>
      <button class="assistant-context-remove" type="button" data-remove-assistant-log="${escapeHtml(log.id)}" aria-label="移除日志">×</button>
    </div>
  `).join('') : '<div class="assistant-context-empty">未选择日志</div>';

  const messages = $('#assistant-messages');
  const empty = $('#assistant-empty');
  empty.classList.toggle('hidden', state.assistantMessages.length > 0);
  messages.querySelectorAll('.assistant-message').forEach((item) => item.remove());
  state.assistantMessages.forEach((message) => {
    const item = document.createElement('div');
    item.className = `assistant-message ${message.role === 'user' ? 'user' : 'assistant'}${message.error ? ' error' : ''}`;
    item.innerHTML = message.role === 'assistant'
      ? renderAssistantMarkdown(message.content)
      : escapeHtml(message.content).replace(/\n/g, '<br>');
    messages.appendChild(item);
  });
  if (state.assistantBusy) {
    const loading = document.createElement('div');
    loading.className = 'assistant-message assistant assistant-loading';
    loading.innerHTML = '<img class="loading-animation" src="loading.gif" alt="" aria-hidden="true" /><span>正在分析日志…</span>';
    messages.appendChild(loading);
    $('#assistant-status').textContent = '正在分析中…';
  } else if (currentAIConfigured()) {
    const provider = activeAIProfile();
    const selectedModel = activeAIModel();
    $('#assistant-status').textContent = provider && selectedModel ? `AI 已就绪 · ${provider.name} / ${selectedModel.name}` : `AI 已就绪 · ${state.aiModel || '服务端默认模型'}`;
  } else {
    $('#assistant-status').textContent = 'AI 未配置 · 点击选择模型';
  }
  $('#assistant-send').disabled = state.assistantBusy;
  $('#assistant-send').textContent = state.assistantBusy ? '分析中…' : '发送';
  $('#assistant-input').disabled = state.assistantBusy;
  if (state.assistantMessages.length) messages.scrollTop = messages.scrollHeight;
}

const maxAssistantAttachmentCount = 5;
const maxAssistantAttachmentBytes = 2 * 1024 * 1024;
const maxAssistantAttachmentTotalBytes = 5 * 1024 * 1024;
const assistantAttachmentAccept = /^(image\/(png|jpeg|gif|webp)|text\/|application\/(json|xml)|application\/x-(yaml|yml))$/i;

function renderAssistantAttachments() {
  const list = $('#assistant-attachment-list');
  const attachButton = $('#assistant-attach-button');
  if (!list || !attachButton) return;
  list.innerHTML = state.assistantAttachments.map((attachment, index) => `
    <span class="assistant-attachment ${attachment.type.startsWith('image/') ? 'image' : 'file'}" title="${escapeHtml(`${attachment.name} · ${Math.ceil(attachment.size / 1024)} KB`)}">
      ${attachment.type.startsWith('image/')
        ? `<button class="assistant-attachment-preview" type="button" data-preview-assistant-attachment="${index}" aria-label="放大预览 ${escapeHtml(attachment.name)}"><img src="${escapeHtml(attachment.data)}" alt="${escapeHtml(attachment.name)}" /></button>`
        : '<span class="assistant-attachment-file-icon" aria-hidden="true">FILE</span>'}
      <span class="assistant-attachment-name">${escapeHtml(attachment.name)}</span>
      <button class="assistant-attachment-remove" type="button" data-remove-assistant-attachment="${index}" aria-label="移除附件 ${escapeHtml(attachment.name)}">×</button>
    </span>
  `).join('');
  list.classList.toggle('hidden', state.assistantAttachments.length === 0);
  attachButton.disabled = state.assistantBusy || state.assistantAttachments.length >= maxAssistantAttachmentCount;
  attachButton.setAttribute('aria-label', state.assistantAttachments.length >= maxAssistantAttachmentCount ? '最多添加 5 个附件' : '添加图片或文件');
}

function openAssistantAttachmentPreview(index) {
  const attachment = state.assistantAttachments[index];
  if (!attachment?.type.startsWith('image/')) return;
  $('#assistant-attachment-preview-image').src = attachment.data;
  $('#assistant-attachment-preview-image').alt = attachment.name;
  $('#assistant-attachment-preview-name').textContent = attachment.name;
  $('#assistant-attachment-preview-modal').classList.remove('hidden');
}

function closeAssistantAttachmentPreview() {
  const modal = $('#assistant-attachment-preview-modal');
  modal.classList.add('hidden');
  $('#assistant-attachment-preview-image').removeAttribute('src');
}

function readAssistantAttachment(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error(`无法读取附件：${file.name}`));
    reader.onload = () => resolve(String(reader.result || ''));
    reader.readAsDataURL(file);
  });
}

function assistantAttachmentType(file) {
  const declared = String(file.type || '').toLowerCase();
  if (declared) return declared;
  const extension = String(file.name || '').split('.').pop().toLowerCase();
  return ({ log: 'text/plain', txt: 'text/plain', md: 'text/markdown', csv: 'text/csv', json: 'application/json', xml: 'application/xml', yaml: 'application/x-yaml', yml: 'application/x-yaml' })[extension] || '';
}

async function addAssistantAttachments(files) {
  const slots = maxAssistantAttachmentCount - state.assistantAttachments.length;
  const candidates = Array.from(files || []).slice(0, Math.max(0, slots));
  if (!candidates.length) return;
  const accepted = [];
  let totalBytes = state.assistantAttachments.reduce((total, attachment) => total + attachment.size, 0);
  for (const file of candidates) {
    const type = assistantAttachmentType(file);
    if (!assistantAttachmentAccept.test(type)) {
      showToast(`暂支持图片和文本类文件：${file.name}`);
      continue;
    }
    if (file.size > maxAssistantAttachmentBytes) {
      showToast(`附件不能超过 2 MB：${file.name}`);
      continue;
    }
    if (totalBytes + file.size > maxAssistantAttachmentTotalBytes) {
      showToast('本次附件总大小不能超过 5 MB');
      continue;
    }
    try {
      const data = (await readAssistantAttachment(file)).replace(/^data:[^;]+;base64,/i, `data:${type};base64,`);
      accepted.push({ name: file.name || '未命名附件', type, size: file.size, data });
      totalBytes += file.size;
    } catch (error) {
      showToast(error.message || '读取附件失败');
    }
  }
  if (!accepted.length) return;
  state.assistantAttachments = [...state.assistantAttachments, ...accepted].slice(0, maxAssistantAttachmentCount);
  renderAssistant();
}

const assistantMarkdownMaxLength = 24000;
const assistantMarkdownMaxTableColumns = 12;
const assistantMarkdownMaxTableRows = 100;
const assistantMarkdownTags = new Set(['A', 'CODE', 'DIV', 'EM', 'H2', 'H3', 'H4', 'PRE', 'SPAN', 'STRONG', 'TABLE', 'TBODY', 'TD', 'TH', 'THEAD', 'TR']);
const assistantMarkdownClasses = new Set(['assistant-markdown-link', 'markdown-blockquote', 'markdown-code-block', 'markdown-divider', 'markdown-list-copy', 'markdown-list-item', 'markdown-spacer', 'markdown-table', 'markdown-table-wrap']);

function safeAssistantMarkdownURL(value) {
  const raw = String(value ?? '').trim();
  if (!raw || /[\u0000-\u001F\u007F]/.test(raw)) return '';
  try {
    const url = new URL(raw);
    return ['http:', 'https:', 'mailto:'].includes(url.protocol) ? url.href : '';
  } catch (error) {
    return '';
  }
}

function renderAssistantMarkdownText(value) {
  return escapeHtml(value)
    .replace(/\*\*([^*\r\n]{1,4096})\*\*/g, '<strong>$1</strong>')
    .replace(/(?<!\*)\*([^*\r\n]{1,4096})\*(?!\*)/g, '<em>$1</em>');
}

function renderAssistantMarkdownInline(value) {
  const source = String(value ?? '');
  const tokenPattern = /`([^`\r\n]{1,4096})`|\[([^\]\r\n]{1,4096})\]\(([^()\s]{1,2048})\)/g;
  const tokens = [];
  let output = '';
  let cursor = 0;
  source.replace(tokenPattern, (match, code, label, href, offset) => {
    output += source.slice(cursor, offset);
    let renderedToken = '';
    if (code !== undefined) {
      renderedToken = `<code>${escapeHtml(code)}</code>`;
    } else {
      const safeHref = safeAssistantMarkdownURL(href);
      renderedToken = safeHref
        ? `<a class="assistant-markdown-link" href="${escapeHtml(safeHref)}" target="_blank" rel="noopener noreferrer">${renderAssistantMarkdownText(label)}</a>`
        : renderAssistantMarkdownText(match);
    }
    output += `\uE000${tokens.push(renderedToken) - 1}\uE001`;
    cursor = offset + match.length;
    return match;
  });
  output += source.slice(cursor);
  return renderAssistantMarkdownText(output).replace(/\uE000(\d+)\uE001/g, (_, index) => tokens[Number(index)] || '');
}

function sanitizeAssistantMarkdownHTML(html) {
  const template = document.createElement('template');
  template.innerHTML = String(html ?? '');
  const elements = [];
  const walker = document.createTreeWalker(template.content, NodeFilter.SHOW_ELEMENT);
  while (walker.nextNode()) elements.push(walker.currentNode);
  elements.forEach((element) => {
    if (!assistantMarkdownTags.has(element.tagName)) {
      element.replaceWith(document.createTextNode(element.textContent || ''));
      return;
    }
    Array.from(element.attributes).forEach((attribute) => {
      const name = attribute.name.toLowerCase();
      const classNames = attribute.value.split(/\s+/).filter(Boolean);
      const allowedClass = name === 'class' && classNames.length && classNames.every((className) => assistantMarkdownClasses.has(className));
      const allowedLinkAttribute = element.tagName === 'A' && ['href', 'target', 'rel'].includes(name);
      if (!allowedClass && !allowedLinkAttribute) element.removeAttribute(attribute.name);
    });
    if (element.tagName === 'A') {
      const safeHref = safeAssistantMarkdownURL(element.getAttribute('href'));
      if (!safeHref) {
        element.replaceWith(...Array.from(element.childNodes));
        return;
      }
      element.setAttribute('href', safeHref);
      element.setAttribute('target', '_blank');
      element.setAttribute('rel', 'noopener noreferrer');
    }
  });
  return template.innerHTML;
}

function renderAssistantMarkdown(content) {
  const source = String(content ?? '');
  const truncated = source.length > assistantMarkdownMaxLength;
  const lines = source.slice(0, assistantMarkdownMaxLength).split(/\r?\n/);
  const tableCells = (line) => String(line).trim().replace(/^\||\|$/g, '').split('|').map((cell) => cell.trim());
  const isTableSeparator = (line) => /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$/.test(line);
  const rendered = [];

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    if (/^```/.test(line)) {
      const code = [];
      index += 1;
      while (index < lines.length && !/^```/.test(lines[index])) {
        code.push(lines[index]);
        index += 1;
      }
      rendered.push(`<pre class="markdown-code-block"><code>${escapeHtml(code.join('\n'))}</code></pre>`);
      continue;
    }
    if (line.includes('|') && isTableSeparator(lines[index + 1] || '')) {
      const headers = tableCells(line);
      if (headers.length <= assistantMarkdownMaxTableColumns) {
        const rows = [];
        index += 2;
        while (index < lines.length && rows.length < assistantMarkdownMaxTableRows && lines[index].includes('|') && lines[index].trim()) {
          const cells = tableCells(lines[index]);
          if (cells.length !== headers.length) break;
          rows.push(cells);
          index += 1;
        }
        index -= 1;
        rendered.push(`<div class="markdown-table-wrap"><table class="markdown-table"><thead><tr>${headers.map((cell) => `<th>${renderAssistantMarkdownInline(cell)}</th>`).join('')}</tr></thead><tbody>${rows.map((cells) => `<tr>${cells.map((cell) => `<td>${renderAssistantMarkdownInline(cell)}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`);
        continue;
      }
    }

    if (!line.trim()) {
      rendered.push('<div class="markdown-spacer"></div>');
      continue;
    }
    if (/^\s{0,3}(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line)) { rendered.push('<div class="markdown-divider"></div>'); continue; }
    if (/^>\s?/.test(line)) { rendered.push(`<div class="markdown-blockquote">${renderAssistantMarkdownInline(line.replace(/^>\s?/, ''))}</div>`); continue; }
    if (/^#{3,6}\s+/.test(line)) { rendered.push(`<h4>${renderAssistantMarkdownInline(line.replace(/^#{3,6}\s+/, ''))}</h4>`); continue; }
    if (/^##\s+/.test(line)) { rendered.push(`<h3>${renderAssistantMarkdownInline(line.slice(3))}</h3>`); continue; }
    if (/^#\s+/.test(line)) { rendered.push(`<h2>${renderAssistantMarkdownInline(line.slice(2))}</h2>`); continue; }
    if (/^[-*]\s+/.test(line)) { rendered.push(`<div class="markdown-list-item"><span>•</span><span class="markdown-list-copy">${renderAssistantMarkdownInline(line.slice(2))}</span></div>`); continue; }
    const ordered = line.match(/^(\d+)\.\s+(.*)$/);
    if (ordered) { rendered.push(`<div class="markdown-list-item"><span>${escapeHtml(ordered[1])}.</span><span class="markdown-list-copy">${renderAssistantMarkdownInline(ordered[2])}</span></div>`); continue; }
    rendered.push(`<div>${renderAssistantMarkdownInline(line)}</div>`);
  }
  if (truncated) rendered.push('<div class="markdown-spacer"></div><div>内容过长，已截断显示。</div>');
  return sanitizeAssistantMarkdownHTML(rendered.join(''));
}

function addAssistantContext(log) {
  if (!log) return;
  if (!activeAssistantSession()) startAssistantSession();
  if (state.assistantContext.some((item) => item.id === log.id)) {
    showToast('这条日志已在 AI 上下文中');
    return;
  }
  state.assistantContext = [log, ...state.assistantContext].slice(0, 20);
  syncActiveAssistantSession();
  renderAssistant();
  showToast('日志已加入 AI 分析上下文');
}

function removeAssistantContext(id) {
  state.assistantContext = state.assistantContext.filter((log) => String(log.id) !== String(id));
  if (state.activeAnalysisLogId === String(id)) {
    state.activeAnalysisLogId = '';
    state.assistantContext = [];
    renderLogs();
  }
  syncActiveAssistantSession();
  renderAssistant();
}

function clearAssistantContext() {
  assistantRequestId += 1;
  if (assistantRequestController) {
    assistantRequestController.abort();
    assistantRequestController = null;
  }
  state.assistantBusy = false;
  startAssistantSession();
  renderLogs();
  renderAssistant();
  showToast('已开始新聊天');
}

function beginLogAnalysis(log) {
  const searchKey = fullRangeSearchKey();
  const orderedLogs = searchKey && state.fullRangeSearchKey === searchKey
    ? state.fullRangeSearchLogs
    : logsForActiveContainer();
  const selectedIndex = orderedLogs.findIndex((item) => item.id === log.id);
  if (selectedIndex < 0) return;

  // The stream is newest-first. Keep the three rows visible above and below
  // the selected log, without reordering the surrounding sequence.
  const contextStart = Math.max(0, selectedIndex - 3);
  const contextEnd = Math.min(orderedLogs.length, selectedIndex + 4);
  if (assistantRequestController) assistantRequestController.abort();
  assistantRequestController = null;
  assistantRequestId += 1;
  state.assistantBusy = false;
  startAssistantSession({ context: orderedLogs.slice(contextStart, contextEnd), activeLogId: String(log.id), title: `${String(log.level || '日志').toUpperCase()} · ${String(log.container || log.node || '日志分析')}` });
  $('#assistant-dock').classList.add('open');
  $('#assistant-input').value = `请重点分析选中的这条日志有什么问题。请结合前后各 3 条日志，说明异常现象、可能原因和建议的排查步骤。\n\n选中日志：${log.time} · ${String(log.level || '').toUpperCase()} · ${log.node} / ${log.container}`;
  renderAssistant();
  sendAssistantMessage({ preventDefault() {} });
}

function analyzeLogFromButton(button) {
  const log = logByID(button.dataset.analyzeLog);
  if (!log) return;
  state.selectedLog = log.id;
  state.activeAnalysisLogId = String(log.id);
  // Update the clicked control before opening the assistant. Any later
  // virtualized render restores this same state from activeAnalysisLogId.
  $$('#log-stream .row-ai-button.active').forEach((item) => {
    item.classList.remove('active');
    item.setAttribute('aria-pressed', 'false');
  });
  button.classList.add('active');
  button.setAttribute('aria-pressed', 'true');
  updatePreview();
  beginLogAnalysis(log);
}

function openAISettings(profileId = '') {
  const modal = $('#ai-settings-modal');
  state.settingsAIProfileId = profileId || state.activeAIProfileId || state.aiProfiles[0]?.id || '';
  $('#ai-settings-title').textContent = '模型设置';
  modal.classList.remove('hidden');
  renderAIProviderList();
  fillAISettingsForm();
}

function closeAISettings() {
  $('#ai-settings-modal').classList.add('hidden');
  $('#ai-profile-form').reset();
}

function requestAIAdminToken() {
  return new Promise((resolve) => {
    aiAdminTokenResolver = resolve;
    const modal = $('#ai-admin-token-modal');
    const input = $('#ai-admin-token-input');
    input.value = '';
    modal.classList.remove('hidden');
    input.focus();
  });
}

// resolveAIAdminToken returns a token to send with a provider write.
//
// When the server has no admin token configured it accepts the write without
// one, so this resolves to an empty string rather than prompting: the prompt
// would demand a value that cannot possibly be verified, and the caller would
// be stuck in a loop it cannot exit. `serverAdminTokenConfigured` is refreshed
// at startup and whenever the settings panel loads, which is also where a
// token would be set.
async function resolveAIAdminToken() {
  if (aiAdminToken) return aiAdminToken;
  if (!serverAdminTokenConfigured) return '';
  aiAdminToken = await requestAIAdminToken();
  return aiAdminToken;
}

function closeAIAdminTokenPrompt(value = '') {
  $('#ai-admin-token-modal').classList.add('hidden');
  $('#ai-admin-token-input').value = '';
  const resolve = aiAdminTokenResolver;
  aiAdminTokenResolver = null;
  if (resolve) resolve(String(value).trim());
}

function selectAISettingsProvider(id) {
  state.settingsAIProfileId = id;
  renderAIProviderList();
  fillAISettingsForm();
}

async function saveAIProfile(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const name = String(form.get('profileName') || '').trim();
  const baseURL = String(form.get('baseURL') || '').trim().replace(/\/+$/, '');
  const apiKey = String(form.get('apiKey') || '').trim();
  const id = String(form.get('profileId') || '').trim();
  if (!name || !baseURL) {
    showToast('请填写配置名称和 API 地址');
    return;
  }
  try {
    const parsed = new URL(baseURL);
    if (!['http:', 'https:'].includes(parsed.protocol) || !parsed.host) throw new Error('invalid URL');
  } catch (error) {
    showToast('API 地址必须是 http 或 https 地址');
    return;
  }
  const existing = state.aiProfiles.find((item) => item.id === id);
  const newModelName = String(form.get('modelName') || '').trim();
  const models = existing?.models ? existing.models.map((model) => ({ ...model })) : [];
  if (newModelName && !models.some((model) => model.name === newModelName)) models.push({ id: `model-${Date.now()}-${Math.random().toString(36).slice(2)}`, name: newModelName });
  if (!models.length) {
    showToast('请至少添加一个模型');
    return;
  }
  const profile = { id: id || `ai-profile-${Date.now()}-${Math.random().toString(36).slice(2)}`, name, baseURL, apiKey, type: String(form.get('connectionType') || 'openai'), enabled: Boolean(form.get('enabled')), models };
  // A visitor on someone else's deployment keeps its providers — and therefore
  // its API keys — in this browser. Uploading them to that host would leak
  // credentials to a machine the visitor does not control.
  if (!isBrowserStorageMode()) {
    aiAdminToken = await resolveAIAdminToken();
    if (serverAdminTokenConfigured && !aiAdminToken) { showToast('未提供管理员令牌，模型未保存'); return; }
    try {
      const response = await fetch('/api/ai/profiles', { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-Log-Agent-Admin-Token': aiAdminToken }, body: JSON.stringify(profile) });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(payload.error || '模型保存失败');
      profile.id = payload.id || profile.id;
    } catch (error) {
      showToast(error.message || '模型保存失败');
      return;
    }
  }
  const index = state.aiProfiles.findIndex((item) => item.id === profile.id);
  if (index >= 0) state.aiProfiles[index] = profile; else state.aiProfiles.push(profile);
  state.activeAIProfileId = profile.id;
  if (!models.some((model) => model.id === state.activeAIModelId)) state.activeAIModelId = models[0].id;
  state.settingsAIProfileId = profile.id;
  persistAIProfiles();
  closeAISettings();
  renderAssistant();
  showToast(`${name} 已保存并切换`);
}

async function deleteAIProfile(id) {
  const profile = state.aiProfiles.find((item) => item.id === id);
  if (!profile || !window.confirm(`确定删除 AI 配置“${profile.name}”吗？`)) return;
  const dbID = profile.id.match(/^ai-profile-db-(\d+)$/)?.[1];
  if (dbID && !isBrowserStorageMode()) {
    aiAdminToken = await resolveAIAdminToken();
    if (serverAdminTokenConfigured && !aiAdminToken) { showToast('未提供管理员令牌，模型未删除'); return; }
    const response = await fetch(`/api/ai/profiles?id=${dbID}`, { method: 'DELETE', headers: { Accept: 'application/json', 'X-Log-Agent-Admin-Token': aiAdminToken } });
    if (!response.ok) { const payload = await response.json().catch(() => ({})); showToast(payload.error || '模型删除失败'); return; }
  }
  state.aiProfiles = state.aiProfiles.filter((item) => item.id !== id);
  if (state.activeAIProfileId === id) {
    state.activeAIProfileId = '';
    state.activeAIModelId = '';
  }
  state.settingsAIProfileId = state.aiProfiles[0]?.id || '';
  persistAIProfiles();
  renderAIProviderList();
  fillAISettingsForm();
  renderAssistant();
  showToast(`${profile.name} 已删除`);
}

function addAIModel() {
  const profile = state.aiProfiles.find((item) => item.id === state.settingsAIProfileId);
  const input = $('#ai-new-model-name');
  const name = input.value.trim();
  if (!profile) {
    showToast('请先保存供应商，再添加更多模型');
    return;
  }
  if (!name) {
    showToast('请输入模型名称');
    input.focus();
    return;
  }
  if (profile.models.some((model) => model.name === name)) {
    showToast('该模型已经存在');
    return;
  }
  profile.models.push({ id: `model-${Date.now()}-${Math.random().toString(36).slice(2)}`, name });
  persistAIProfiles();
  input.value = '';
  renderAIModelList();
  renderAIModelPicker();
  showToast(`${name} 已添加`);
}

function editAIModel(id) {
  const profile = state.aiProfiles.find((item) => item.id === state.settingsAIProfileId);
  const model = profile?.models.find((item) => item.id === id);
  if (!profile || !model) return;
  const name = window.prompt('修改模型名称', model.name)?.trim();
  if (!name || name === model.name) return;
  if (profile.models.some((item) => item.id !== id && item.name === name)) {
    showToast('该模型已经存在');
    return;
  }
  model.name = name;
  persistAIProfiles();
  renderAIModelList();
  renderAIModelPicker();
}

function deleteAIModel(id) {
  const profile = state.aiProfiles.find((item) => item.id === state.settingsAIProfileId);
  const model = profile?.models.find((item) => item.id === id);
  if (!profile || !model || !window.confirm(`确定删除模型“${model.name}”吗？`)) return;
  profile.models = profile.models.filter((item) => item.id !== id);
  if (state.activeAIProfileId === profile.id && state.activeAIModelId === id) state.activeAIModelId = profile.models[0]?.id || '';
  persistAIProfiles();
  renderAIModelList();
  renderAIModelPicker();
}

async function sendAssistantMessage(event) {
  event.preventDefault();
  if (state.assistantBusy) return;
  const input = $('#assistant-input');
  const typedContent = input.value.trim();
  const attachments = state.assistantAttachments.slice();
  if (!typedContent && !attachments.length) return;
  const content = typedContent || `请分析已附加的文件：${attachments.map((attachment) => attachment.name).join('、')}`;
  if (!activeAssistantSession()) startAssistantSession();
  const profile = activeAIProfile();
  const model = activeAIModel();
  const requestId = ++assistantRequestId;
  const requestController = new AbortController();
  assistantRequestController = requestController;
  state.assistantMessages.push({ role: 'user', content });
  state.assistantMessages = state.assistantMessages.slice(-12);
  input.value = '';
  state.assistantAttachments = [];
  state.assistantBusy = true;
  syncActiveAssistantSession();
  renderAssistant();
  try {
    const response = await fetch('/api/ai/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      signal: requestController.signal,
      body: JSON.stringify({
        messages: state.assistantMessages.slice(-12),
        logs: state.assistantContext.slice(0, 20),
        attachments,
        config: profile && model ? { profile_id: Number(profile.id.match(/^ai-profile-db-(\d+)$/)?.[1] || 0), name: profile.name, base_url: profile.baseURL, api_key: profile.apiKey, model: model.name, type: profile.type } : null
      })
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || 'AI 请求失败');
    state.assistantMessages.push({ role: 'assistant', content: payload.message || 'AI 未返回内容' });
    state.assistantMessages = state.assistantMessages.slice(-12);
  } catch (error) {
    if (error.name === 'AbortError' || requestId !== assistantRequestId) return;
    state.assistantMessages.push({ role: 'assistant', content: error.message || 'AI 请求失败', error: true });
    state.assistantMessages = state.assistantMessages.slice(-12);
    showToast(error.message || 'AI 请求失败');
  } finally {
    if (requestId !== assistantRequestId) return;
    assistantRequestController = null;
    state.assistantBusy = false;
    syncActiveAssistantSession();
    renderAssistant();
  }
}

async function syncAIStatus() {
  try {
    const response = await fetch('/api/ai/status', { headers: { Accept: 'application/json' } });
    if (!response.ok) throw new Error('AI status unavailable');
    const payload = await response.json();
    state.aiEnvConfigured = Boolean(payload.configured);
    state.aiModel = String(payload.model || '');
  } catch (error) {
    state.aiEnvConfigured = false;
    state.aiModel = '';
  }
  renderAssistant();
}

function fillAppSettingsForm(payload) {
  const form = $('#app-settings-form');
  if (!form) return;
  const environment = payload.environment || 'production';
  if (!Array.from(form.elements.environment.options).some((option) => option.value === environment)) {
    form.elements.environment.add(new Option(environment, environment));
  }
  form.elements.environment.value = environment;
  form.elements.currentAdminToken.value = '';
  form.elements.adminToken.value = '';
  serverAdminTokenConfigured = Boolean(payload.adminTokenConfigured);
  $('#settings-admin-status').textContent = serverAdminTokenConfigured ? '已配置' : '未配置';
}

async function openAppSettings() {
  const modal = $('#app-settings-modal');
  modal.classList.remove('hidden');
  updateStorageModeUI();
  loadConfigInfo();
  try {
    const response = await fetch('/api/settings', { headers: { Accept: 'application/json' } });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || '设置读取失败');
    fillAppSettingsForm(payload);
  } catch (error) {
    modal.classList.add('hidden');
    showToast(error.message || '设置读取失败');
  }
}

// Reads where the configuration lives so the settings panel can expose the
// file-manager link. The paths themselves are deliberately never rendered:
// the link is the only affordance, so its presence is what tells the user
// whether this page is backed by files or by browser storage.
async function loadConfigInfo() {
  try {
    const response = await fetch('/api/config/info', { headers: { Accept: 'application/json' } });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || '配置信息读取失败');
    state.configInfo = payload;
    // Keeps the provider-save path from prompting for a token the server is
    // not asking for, even if the settings panel has never been opened.
    serverAdminTokenConfigured = Boolean(payload.hasAdminToken);
    // openAppSettings() calls updateStorageModeUI() before this fetch settles,
    // so re-apply it now that localMode is known for sure.
    updateStorageModeUI();
  } catch (error) {
    showToast(error.message || '配置信息读取失败');
  }
}

function configAdminHeaders() {
  const token = String($('#app-settings-form')?.elements.currentAdminToken?.value || '').trim() || aiAdminToken;
  const headers = { Accept: 'application/json' };
  if (token) headers['X-Log-Agent-Admin-Token'] = token;
  return headers;
}

async function revealConfigDirectory(event) {
  if (event) event.preventDefault();
  const link = $('#settings-config-reveal');
  if (!link || link.classList.contains('hidden') || link.dataset.busy === '1') return;
  link.dataset.busy = '1';
  try {
    const response = await fetch('/api/config/reveal', { method: 'POST', headers: configAdminHeaders() });
    await readSettingsResponse(response, '打开配置目录失败');
    showToast('已在文件管理器中打开配置目录');
  } catch (error) {
    showToast(error.message || '打开配置目录失败');
  } finally {
    link.dataset.busy = '0';
  }
}

async function exportConfiguration() {
  const button = $('#settings-config-export');
  if (!button || button.disabled) return;
  button.disabled = true;
  try {
    const response = await fetch('/api/config/export', { headers: configAdminHeaders() });
    if (!response.ok) throw new Error((await response.json().catch(() => ({}))).error || '导出配置失败');
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `log-agent-config-${new Date().toISOString().slice(0, 10)}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    showToast('配置已导出');
  } catch (error) {
    showToast(error.message || '导出配置失败');
  } finally {
    button.disabled = false;
  }
}

async function importConfiguration(file) {
  if (!file) return;
  if (!window.confirm('导入将覆盖当前全部节点与模型配置，确定继续吗？')) return;
  try {
    const text = await file.text();
    let parsed;
    try {
      parsed = JSON.parse(text);
    } catch (error) {
      throw new Error('配置文件不是合法的 JSON');
    }
    const response = await fetch('/api/config/import', {
      method: 'POST',
      headers: { ...configAdminHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify(parsed),
    });
    const payload = await readSettingsResponse(response, '导入配置失败');
    showToast(`已导入 ${payload.nodes || 0} 个节点、${payload.providers || 0} 个模型配置`);
    // Nodes were rebuilt server-side; refresh so the sidebar reflects them.
    await syncGoBackend();
    loadConfigInfo();
  } catch (error) {
    showToast(error.message || '导入配置失败');
  } finally {
    const input = $('#settings-config-file');
    if (input) input.value = '';
  }
}

function closeAppSettings() {
  $('#app-settings-modal').classList.add('hidden');
  $('#app-settings-form')?.reset();
}

function appSettingsAuthHeaders(form) {
  const currentAdminToken = String(form.elements.currentAdminToken.value || '').trim() || aiAdminToken;
  const headers = { 'Content-Type': 'application/json', Accept: 'application/json' };
  if (currentAdminToken) headers['X-Log-Agent-Admin-Token'] = currentAdminToken;
  return { headers, currentAdminToken };
}

function ensureSettingsAdminToken(form) {
  const { currentAdminToken } = appSettingsAuthHeaders(form);
  if ($('#settings-admin-status').textContent === '已配置' && !currentAdminToken) {
    showToast('请先填写当前管理员 key');
    form.elements.currentAdminToken.focus();
    return false;
  }
  return true;
}

async function readSettingsResponse(response, fallbackMessage) {
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = payload.error === 'admin token required' ? '请填写正确的当前管理员 key' : payload.error;
    throw new Error(error || (response.status === 401 ? '当前管理员 key 不正确' : fallbackMessage));
  }
  return payload;
}

async function saveAdminSettings() {
  const form = $('#app-settings-form');
  if (!ensureSettingsAdminToken(form)) return;
  const button = $('#settings-admin-save');
  const adminToken = String(form.elements.adminToken.value || '').trim();
  const { headers, currentAdminToken } = appSettingsAuthHeaders(form);
  button.disabled = true;
  try {
    const response = await fetch('/api/settings/admin', { method: 'PUT', headers, body: JSON.stringify({ adminToken }) });
    const payload = await readSettingsResponse(response, '管理员 key 保存失败');
    if (adminToken) aiAdminToken = adminToken;
    else if (currentAdminToken) aiAdminToken = currentAdminToken;
    fillAppSettingsForm(payload);
    showToast('管理员 key 已保存');
  } catch (error) {
    showToast(error.message || '管理员 key 保存失败');
  } finally {
    button.disabled = false;
  }
}

async function saveAppSettings(event) {
  event.preventDefault();
  const form = $('#app-settings-form');
  if (!ensureSettingsAdminToken(form)) return;
  const submitButton = $('#app-settings-form button[type="submit"]');
  const { headers } = appSettingsAuthHeaders(form);
  submitButton.disabled = true;
  try {
    const response = await fetch('/api/settings/environment', {
      method: 'PUT', headers,
      body: JSON.stringify({ environment: String(form.elements.environment.value || '').trim() })
    });
    const payload = await readSettingsResponse(response, '运行环境保存失败');
    fillAppSettingsForm(payload);
    closeAppSettings();
    showToast('运行环境已保存');
  } catch (error) {
    showToast(error.message || '运行环境保存失败');
  } finally {
    submitButton.disabled = false;
  }
}

function applyTheme(theme) {
  const isLight = theme === 'light';
  document.documentElement.dataset.theme = isLight ? 'light' : 'dark';
  const button = $('#theme-toggle');
  if (button) {
    button.textContent = isLight ? '☾' : '☀';
    button.setAttribute('aria-label', isLight ? '切换到黑夜模式' : '切换到白天模式');
    button.title = isLight ? '切换到黑夜模式' : '切换到白天模式';
  }
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', isLight ? '#f4f6f8' : '#101214');
}

function initializeTheme() {
  let theme = 'light';
  try {
    const savedTheme = localStorage.getItem('dozzle-ops-theme');
    if (savedTheme === 'dark' || savedTheme === 'light') theme = savedTheme;
  } catch (error) {
    // Use the light default when browser storage is unavailable.
  }
  applyTheme(theme);
  $('#theme-toggle').addEventListener('click', () => {
    const nextTheme = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
    applyTheme(nextTheme);
    try {
      localStorage.setItem('dozzle-ops-theme', nextTheme);
    } catch (error) {
      // The theme still applies for the current page when storage is unavailable.
    }
  });
}

function closeCustomSelect(custom) {
  custom.classList.remove('open');
  custom.querySelector('.custom-select-trigger').setAttribute('aria-expanded', 'false');
}

function closeAllCustomSelects(except) {
  $$('.custom-select.open').forEach((custom) => {
    if (custom !== except) closeCustomSelect(custom);
  });
}

function openCustomSelect(custom) {
  closeAllCustomSelects(custom);
  custom.classList.add('open');
  custom.querySelector('.custom-select-trigger').setAttribute('aria-expanded', 'true');
  custom.querySelector('.custom-select-option[aria-selected="true"]')?.focus();
}

function chooseCustomSelectOption(select, value) {
  if (select.multiple) {
    const options = Array.from(select.options);
    const option = options.find((item) => item.value === value);
    if (!option) return;
    if (value === 'all') {
      options.forEach((item) => { item.selected = item.value === 'all'; });
    } else {
      const allOption = options.find((item) => item.value === 'all');
      if (allOption) allOption.selected = false;
      option.selected = !option.selected;
      if (!options.some((item) => item.value !== 'all' && item.selected) && allOption) allOption.selected = true;
    }
    refreshCustomSelect(select);
    select.dispatchEvent(new Event('change', { bubbles: true }));
    return;
  }
  if (select.value !== value) {
    select.value = value;
    refreshCustomSelect(select);
    select.dispatchEvent(new Event('change', { bubbles: true }));
  }
  closeCustomSelect(select._customSelect);
  select._customSelect.querySelector('.custom-select-trigger').focus();
}

function refreshCustomSelect(select) {
  const custom = select?._customSelect;
  if (!custom) return;
  const options = Array.from(select.options);
  const selectedOptions = select.multiple
    ? options.filter((option) => option.value !== 'all' && option.selected)
    : [];
  const selected = select.multiple
    ? (selectedOptions[0] || options.find((option) => option.value === 'all') || options[0])
    : (options.find((option) => option.value === select.value) || options[0]);
  const selectionLabel = select.multiple
    ? (selectedOptions.length === 0
      ? (select.id === 'node-filter' ? '全部节点' : '全部容器')
      : selectedOptions.length === 1
        ? selectedOptions[0].textContent
        : `已选 ${selectedOptions.length} 个${select.id === 'node-filter' ? '节点' : '容器'}`)
    : selected?.textContent || '';
  const menu = custom.querySelector('.custom-select-menu');
  menu.innerHTML = options.map((option) => `
    <button type="button" class="custom-select-option${select.multiple && option.value !== 'all' && option.selected ? ' multi-selected' : ''}" role="option" data-value="${escapeHtml(option.value)}" aria-selected="${select.multiple ? (option.value === 'all' ? selectedOptions.length === 0 : option.selected) : selected?.value === option.value}" title="${escapeHtml(option.textContent)}">${escapeHtml(option.textContent)}</button>
  `).join('');
  if (selected && !select.multiple) {
    select.value = selected.value;
  }
  custom.querySelector('.custom-select-value').textContent = selectionLabel;
  custom.querySelector('.custom-select-trigger').setAttribute('title', selectionLabel);
}

function initializeCustomSelects() {
  $$('.filter-select-wrap select').forEach((select) => {
    const custom = document.createElement('div');
    custom.className = `custom-select${select.multiple ? ' multi-select' : ''}`;
    custom.innerHTML = `
      <button type="button" class="custom-select-trigger" aria-haspopup="listbox" aria-expanded="false">
        <span class="custom-select-value"></span>
      </button>
      <div class="custom-select-menu" role="listbox"></div>
    `;
    select.classList.add('custom-select-native');
    select.tabIndex = -1;
    select.setAttribute('aria-hidden', 'true');
    select.parentNode.insertBefore(custom, select);
    select._customSelect = custom;

    const trigger = custom.querySelector('.custom-select-trigger');
    trigger.setAttribute('aria-label', select.getAttribute('aria-label') || 'Select option');
    trigger.addEventListener('click', (event) => {
      event.stopPropagation();
      if (custom.classList.contains('open')) closeCustomSelect(custom); else openCustomSelect(custom);
    });
    trigger.addEventListener('keydown', (event) => {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp' || event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        openCustomSelect(custom);
      }
      if (event.key === 'Escape') closeCustomSelect(custom);
    });
    custom.querySelector('.custom-select-menu').addEventListener('click', (event) => {
      event.stopPropagation();
      const option = event.target.closest('.custom-select-option');
      if (option) chooseCustomSelectOption(select, option.dataset.value);
    });
    custom.querySelector('.custom-select-menu').addEventListener('keydown', (event) => {
      const options = Array.from(custom.querySelectorAll('.custom-select-option'));
      const currentIndex = options.indexOf(document.activeElement);
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        event.preventDefault();
        const offset = event.key === 'ArrowDown' ? 1 : -1;
        options[Math.max(0, Math.min(options.length - 1, currentIndex + offset))]?.focus();
      } else if (event.key === 'Escape') {
        event.preventDefault();
        closeCustomSelect(custom);
        trigger.focus();
      }
    });
    refreshCustomSelect(select);
  });
  document.addEventListener('click', () => closeAllCustomSelects());
}

function getNode(id) { return nodes.find((node) => node.id === id); }

function nodeIdByName(name) { return nodes.find((node) => node.name === name)?.id; }

function containerKey(nodeId, containerId) { return `${nodeId}::${containerId}`; }

function selectedContainerTargets() {
  return state.selectedContainers.map((value) => {
    const separator = value.indexOf('::');
    if (separator < 0) return null;
    const nodeId = value.slice(0, separator);
    const containerId = value.slice(separator + 2);
    const node = getNode(nodeId);
    const container = node?.containers?.find((item) => (item.id || item.name) === containerId);
    if (!node || !container) return null;
    return { nodeId, containerId, name: container.name || container.id };
  }).filter(Boolean);
}

function nodeIdsForContainerKeys(values) {
  return [...new Set(values.map((value) => {
    const separator = value.indexOf('::');
    return separator >= 0 ? value.slice(0, separator) : '';
  }).filter((id) => id && getNode(id)))];
}

function logsForActiveContainer() {
  if (!state.selectedContainers.length) return state.logs;
  const selectedSources = new Set();
  selectedContainerTargets().forEach((target) => {
    const node = getNode(target.nodeId);
    if (!node) return;
    selectedSources.add(`${node.name}\u0000${target.name}`);
    selectedSources.add(`${node.name}\u0000${target.containerId}`);
  });
  const merged = new Map();
  state.logs.forEach((log) => {
    if (selectedSources.has(`${log.node}\u0000${log.container}`)) merged.set(log.id, log);
  });
  state.selectedContainers.forEach((key) => (state.containerLogCache[key] || []).forEach((log) => merged.set(log.id, log)));
  return sortLogsNewest(Array.from(merged.values()));
}

function normalizedLogSearch(value) {
  return String(value ?? '').replace(/\s+/g, ' ').trim().toLowerCase();
}

function logByID(id) {
  const target = Number(id);
  return [...state.fullRangeSearchLogs, ...logsForActiveContainer()].find((item) => item.id === target);
}

// The remote full-range scan is only driven by the in-panel search box, so it
// is only usable while the global search box is empty. Otherwise filteredLogs()
// would reuse a result set that was fetched for a different query and render
// an empty list.
function fullRangeSearchKey() {
  if (state.globalQuery.trim()) return '';
  const query = normalizedLogSearch(state.query);
  return query && state.selectedContainers.length
    ? `${state.range}|${state.selectedContainers.join('|')}|${query}`
    : '';
}

function clearFullRangeSearch() {
  if (fullRangeSearchTimer) clearTimeout(fullRangeSearchTimer);
  fullRangeSearchTimer = null;
  fullRangeSearchRequest += 1;
  state.fullRangeSearchLogs = [];
  state.fullRangeSearchKey = '';
  state.fullRangeSearchLoading = false;
}

function scheduleFullRangeSearch() {
  const key = fullRangeSearchKey();
  if (!key || !goServerConnected) return;
  const request = ++fullRangeSearchRequest;
  state.fullRangeSearchLoading = true;
  fullRangeSearchTimer = setTimeout(async () => {
    const targets = selectedContainerTargets();
    const query = normalizedLogSearch(state.query);
    try {
      const responses = await Promise.all(targets.map(async (target) => {
        const params = new URLSearchParams({ node: target.nodeId, container: target.containerId, range: state.range, q: query });
        const response = await fetch(`/api/logs/container/search?${params.toString()}`, { headers: { Accept: 'application/json' } });
        if (!response.ok) throw new Error((await response.json().catch(() => ({}))).error || '完整时间范围筛选失败');
        return response.json();
      }));
      if (request !== fullRangeSearchRequest || key !== fullRangeSearchKey()) return;
      state.fullRangeSearchLogs = sortLogsNewest(responses.flatMap((payload) => payload.logs || []));
      state.fullRangeSearchKey = key;
    } catch (error) {
      if (request === fullRangeSearchRequest) showToast(error.message || '完整时间范围筛选失败');
    } finally {
      if (request === fullRangeSearchRequest) {
        state.fullRangeSearchLoading = false;
        resetLogPagination();
        renderLogs();
      }
    }
  }, 320);
}

function compareLogsNewest(left, right) {
  return (Number(right.timestamp) || 0) - (Number(left.timestamp) || 0) || right.id - left.id;
}

function sortLogsNewest(logs) {
  return [...logs].sort(compareLogsNewest);
}

function mergeLogs(existing, incoming, { skipDuplicateCheck = false } = {}) {
  if (!incoming.length) return existing;
  const incomingById = new Map();
  incoming.forEach((log) => {
    if (log && log.id != null) incomingById.set(log.id, log);
  });
  if (!incomingById.size) return existing;
  if (!skipDuplicateCheck) {
    const existingIds = new Set(existing.map((log) => log?.id));
    const hasNewLog = Array.from(incomingById.keys()).some((id) => !existingIds.has(id));
    if (!hasNewLog) return existing;
  }
  const sortedIncoming = sortLogsNewest(Array.from(incomingById.values()));
  const merged = [];
  let existingIndex = 0;
  let incomingIndex = 0;
  while (merged.length < maxBufferedLogs && (existingIndex < existing.length || incomingIndex < sortedIncoming.length)) {
    while (existingIndex < existing.length && incomingById.has(existing[existingIndex]?.id)) existingIndex += 1;
    const existingLog = existing[existingIndex];
    const incomingLog = sortedIncoming[incomingIndex];
    if (!existingLog) {
      if (incomingLog) merged.push(incomingLog);
      incomingIndex += 1;
      continue;
    }
    if (!incomingLog) {
      merged.push(existingLog);
      existingIndex += 1;
      continue;
    }
    if (compareLogsNewest(incomingLog, existingLog) <= 0) {
      merged.push(incomingLog);
      incomingIndex += 1;
    } else {
      merged.push(existingLog);
      existingIndex += 1;
    }
  }
  return merged;
}

function logDate(log) {
  if (log.date) return String(log.date);
  const timestamp = Number(log.timestamp);
  if (!Number.isFinite(timestamp)) return '—';
  const date = new Date(timestamp);
  const pad = (value) => String(value).padStart(2, '0');
  return `${date.getFullYear()}/${pad(date.getMonth() + 1)}/${pad(date.getDate())}`;
}

function logTime(log) {
  if (log.time) return String(log.time).slice(0, 8);
  const timestamp = Number(log.timestamp);
  if (!Number.isFinite(timestamp)) return '—';
  return new Date(timestamp).toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

async function loadSelectedContainerLogs() {
  const targets = selectedContainerTargets();
  if (state.paused || !targets.length || !goServerConnected) {
    if (containerLogRetryTimer) clearTimeout(containerLogRetryTimer);
    containerLogRetryTimer = null;
    return;
  }
  const selectionKey = state.selectedContainers.join('|');
  if (loadedContainerSelectionKey === selectionKey && targets.every((target) => Object.prototype.hasOwnProperty.call(state.containerLogCache, containerKey(target.nodeId, target.containerId)))) return;
  let loading = false;
  try {
    await Promise.all(targets.map(async (target) => {
      const cacheKey = containerKey(target.nodeId, target.containerId);
      const query = new URLSearchParams({ node: target.nodeId, container: target.containerId, range: state.range });
      const response = await fetch(`/api/logs/container?${query.toString()}`, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error('容器日志加载失败');
      const payload = await response.json();
      if (state.paused || state.selectedContainers.join('|') !== selectionKey) return;
      state.containerLogCache[cacheKey] = payload.logs || [];
      loading = loading || Boolean(payload.loading);
    }));
    if (state.paused || state.selectedContainers.join('|') !== selectionKey) return;
    scheduleLogRender();
    updatePreview();
    if (loading) {
      if (containerLogRetryTimer) clearTimeout(containerLogRetryTimer);
      containerLogRetryTimer = setTimeout(() => {
        containerLogRetryTimer = null;
        if (state.selectedContainers.join('|') === selectionKey) loadSelectedContainerLogs();
      }, 1000);
    } else {
      loadedContainerSelectionKey = selectionKey;
    }
  } catch (error) {
    showToast(error.message || '容器日志加载失败');
  }
}

const rangeLabels = {
  '30m': '最近 30 分钟',
  '5h': '最近 5 小时',
  '1d': '最近 1 天',
  '1w': '最近一周'
};

const rangeDurations = {
  '30m': 30 * 60 * 1000,
  '5h': 5 * 60 * 60 * 1000,
  '1d': 24 * 60 * 60 * 1000,
  '1w': 7 * 24 * 60 * 60 * 1000
};

function isLogInSelectedRange(log, now = Date.now()) {
  const timestamp = Number(log.timestamp);
  const duration = rangeDurations[state.range];
  return !Number.isFinite(timestamp) || !duration || timestamp >= now - duration;
}

function nodeStatusLabel(node) {
  if (node.status === 'connected') return '已连接';
  if (node.status === 'error') return '连接失败';
  return '连接中';
}

function nodeStatusClass(node) {
  if (node.status === 'connected') return 'connected';
  if (node.status === 'error') return 'error';
  return 'offline';
}

function updateSyncFooter(label, state = 'connected') {
  $('#system-status-label').textContent = label;
  $('#system-status').dataset.state = state;
}

function updateLastSync(timestamp) {
  const value = Number(timestamp);
  if (!Number.isFinite(value) || value < lastSyncedLogTimestamp) return;
  lastSyncedLogTimestamp = value;
  $('#last-sync').textContent = new Date(value).toLocaleTimeString('zh-CN', { hour12: false });
}

function updateStorage(storage) {
  const used = Number(storage?.used);
  const capacity = Number(storage?.capacity);
  const percent = Number(storage?.percent);
  const evicted = Math.max(0, Number(storage?.evicted) || 0);
  const lastEvictedAt = Number(storage?.lastEvictedAt);
  if (!Number.isFinite(used) || !Number.isFinite(capacity) || capacity <= 0) {
    $('#metric-storage').textContent = '—';
    $('#metric-storage-foot').textContent = '等待数据';
    $('#metric-storage-policy').textContent = '满额后自动淘汰最早日志';
    return;
  }
  $('#metric-storage').innerHTML = `${Math.max(0, Math.min(100, percent))}<span class="unit">%</span>`;
  $('#metric-storage-foot').textContent = `当前 ${used.toLocaleString('en-US')} / ${capacity.toLocaleString('en-US')} 条`;
  if (!evicted) {
    $('#metric-storage-policy').textContent = '尚未发生淘汰；满额后新增一条，淘汰最早一条';
    return;
  }
  const lastEvicted = Number.isFinite(lastEvictedAt) && lastEvictedAt > 0
    ? new Date(lastEvictedAt).toLocaleTimeString('zh-CN', { hour12: false })
    : '时间未知';
  $('#metric-storage-policy').textContent = `累计淘汰 ${evicted.toLocaleString('en-US')} 条 · 最近 ${lastEvicted}`;
}

async function clearLogCache() {
  const button = $('#storage-clear-cache');
  if (!button || button.disabled) return;
  if (!window.confirm('确定清除全部已缓存的日志吗？此操作不可恢复，缓存将重新开始累积。')) return;
  button.disabled = true;
  try {
    const response = await fetch('/api/logs/cache/clear', { method: 'POST', headers: { Accept: 'application/json' } });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || '清除缓存失败');
    state.logs = [];
    state.containerLogCache = {};
    resetLogPagination();
    renderLogs();
    updateStorage(payload.storage);
    const cleared = Number(payload.cleared) || 0;
    showToast(cleared ? `已清除 ${cleared.toLocaleString('en-US')} 条缓存日志` : '缓存已是空的');
  } catch (error) {
    showToast(error.message || '清除缓存失败，请稍后重试');
  } finally {
    button.disabled = false;
  }
}

function syncRuleButtons() {
  $$('.pipeline-item').forEach((item) => {
    const enabled = Boolean(state.ruleState[item.dataset.rule]);
    item.classList.toggle('enabled', enabled);
    item.querySelector('.toggle')?.classList.toggle('active', enabled);
  });
  $$('[data-rule-toggle]').forEach((button) => {
    button.classList.toggle('active', Boolean(state.ruleState[button.dataset.ruleToggle]));
  });
  applyPipelineRuleOrder();
}

const ruleNames = {
  mask: '敏感信息脱敏',
  structure: '结构化字段提取',
  noise: '健康检查过滤'
};

function normalizeRuleOrder(order) {
  const defaults = ['mask', 'structure', 'noise'];
  if (!Array.isArray(order) || order.length !== defaults.length) return defaults;
  const unique = new Set(order);
  return unique.size === defaults.length && defaults.every((rule) => unique.has(rule)) ? [...order] : defaults;
}

function applyPipelineRuleOrder() {
  state.ruleOrder = normalizeRuleOrder(state.ruleOrder);
  const list = $('.pipeline-list');
  if (list) {
    state.ruleOrder.forEach((rule) => {
      const item = list.querySelector(`.pipeline-item[data-rule="${rule}"]`);
      if (item) list.append(item);
    });
  }
  const summary = $('#rule-order-summary');
  if (summary) summary.textContent = `日志接收 → ${state.ruleOrder.map((rule) => ruleNames[rule]).join(' → ')} → 日志缓存`;
}

async function persistRuleOrder(order, previousOrder) {
  state.ruleOrder = normalizeRuleOrder(order);
  applyPipelineRuleOrder();
  updatePreview();
  try {
    const response = await fetch('/api/rules', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ order: state.ruleOrder })
    });
    if (!response.ok) throw new Error('规则顺序保存失败');
    const payload = await response.json();
    state.ruleOrder = normalizeRuleOrder(payload.order);
    applyPipelineRuleOrder();
    showToast('加工规则顺序已保存');
  } catch (error) {
    state.ruleOrder = normalizeRuleOrder(previousOrder);
    applyPipelineRuleOrder();
    showToast(error.message || '规则顺序保存失败，已恢复原顺序');
  }
}

function bindPipelineDragAndDrop() {
  const list = $('.pipeline-list');
  if (!list) return;
  let draggingItem = null;
  Array.from(list.querySelectorAll('.pipeline-item')).forEach((item) => {
    const handle = item.querySelector('.drag-handle');
    item.draggable = false;
    handle?.addEventListener('pointerdown', () => {
      item.dataset.dragHandleActive = 'true';
      item.draggable = true;
    });
    item.addEventListener('dragstart', (event) => {
      if (item.dataset.dragHandleActive !== 'true') {
        event.preventDefault();
        return;
      }
      delete item.dataset.dragHandleActive;
      draggingItem = item;
      item.classList.add('dragging');
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', item.dataset.rule || '');
    });
    item.addEventListener('dragend', () => {
      item.classList.remove('dragging');
      item.draggable = false;
      draggingItem = null;
      Array.from(list.querySelectorAll('.pipeline-item.drag-over')).forEach((entry) => entry.classList.remove('drag-over'));
    });
  });
  list.addEventListener('dragover', (event) => {
    if (!draggingItem) return;
    event.preventDefault();
    const target = event.target.closest('.pipeline-item');
    if (!target || target === draggingItem) return;
    Array.from(list.querySelectorAll('.pipeline-item.drag-over')).forEach((entry) => entry.classList.remove('drag-over'));
    target.classList.add('drag-over');
    const afterTarget = event.clientY > target.getBoundingClientRect().top + target.offsetHeight / 2;
    list.insertBefore(draggingItem, afterTarget ? target.nextSibling : target);
  });
  list.addEventListener('drop', (event) => {
    if (!draggingItem) return;
    event.preventDefault();
    const nextOrder = Array.from(list.querySelectorAll('.pipeline-item')).map((item) => item.dataset.rule);
    const previousOrder = state.ruleOrder;
    if (nextOrder.join('\u0000') !== previousOrder.join('\u0000')) persistRuleOrder(nextOrder, previousOrder);
  });
}

async function persistRuleGroup(updates, successMessage) {
  const previous = { ...state.ruleState };
  state.ruleState = { ...state.ruleState, ...updates };
  syncRuleButtons();
  renderLogs();
  updatePreview();
  try {
    const response = await fetch('/api/rules', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ rules: updates })
    });
    if (!response.ok) throw new Error('规则保存失败');
    const payload = await response.json();
    state.ruleState = { ...state.ruleState, ...(payload.rules || {}) };
    syncRuleButtons();
    renderLogs();
    updatePreview();
    showToast(successMessage);
  } catch (error) {
    state.ruleState = previous;
    syncRuleButtons();
    renderLogs();
    updatePreview();
    showToast(error.message || '规则保存失败，已恢复原状态');
  }
}

function closePipelineMenu() {
  const menu = $('#pipeline-menu');
  const button = $('#pipeline-more-button');
  if (!menu || !button) return;
  menu.classList.add('hidden');
  button.setAttribute('aria-expanded', 'false');
}

function renderNodes() {
  const previousNodes = state.selectedNodes.join('\u0000');
  if (nodes.length) state.selectedNodes = state.selectedNodes.filter((id) => getNode(id));
  if (previousNodes !== state.selectedNodes.join('\u0000')) persistSelection();
  state.expandedNodes = state.expandedNodes.filter((id) => getNode(id));
  $('#node-total').textContent = String(nodes.length).padStart(2, '0');
  $('#metric-nodes').textContent = String(nodes.length).padStart(2, '0');
  const connectedNodes = nodes.filter((node) => node.status === 'connected').length;
  const activeContainers = nodes.reduce((total, node) => node.status === 'connected' ? total + (Number(node.count) || 0) : total, 0);
  $('#metric-nodes-foot').textContent = nodes.length
    ? `${connectedNodes} / ${nodes.length} 个节点已连接`
    : '尚未接入节点';
  $('#metric-containers').textContent = activeContainers.toLocaleString('en-US');
  $('#metric-containers-foot').textContent = activeContainers
    ? `${activeContainers} 个运行中容器`
    : '当前没有运行中的容器';
  $('#metric-processed-foot').textContent = state.paused
    ? '已暂停，当前视图不会接收新日志'
    : state.processed
      ? '服务启动后累计接收，不等于缓存条数'
      : '等待日志流';
  $('#node-list').innerHTML = nodes.length ? nodes.map((node) => {
    const containers = node.containers || [];
    const expanded = state.expandedNodes.includes(node.id);
    return `
      <div class="node-group ${expanded ? 'expanded' : ''}" data-node-group-id="${escapeHtml(node.id)}">
        <div class="node-row">
          <button class="node-item ${state.selectedNodes.includes(node.id) ? 'active' : ''}" data-node-id="${escapeHtml(node.id)}" title="${escapeHtml(node.error || nodeStatusLabel(node))}">
            <i class="node-dot ${nodeStatusClass(node)}"></i>
            <span class="node-copy"><strong>${escapeHtml(node.name)}</strong><span>${escapeHtml(node.url.replace(/^https?:\/\//, ''))}</span></span>
            <span class="node-live">${node.count}</span>
          </button>
          <button class="node-expand-button" type="button" data-expand-node-id="${escapeHtml(node.id)}" aria-expanded="${expanded}" aria-label="${expanded ? '收起容器' : '展开容器'}" title="${expanded ? '收起容器' : '展开容器'}"><span class="node-expand-icon${expanded ? ' expanded' : ''}" aria-hidden="true">›</span></button>
        </div>
        ${expanded ? `<div class="node-children" data-container-list-for="${escapeHtml(node.id)}">${containers.length ? containers.map((container) => {
          const containerId = container.id || container.name;
          const containerName = container.name || containerId || '未命名容器';
          const selected = state.selectedContainers.includes(containerKey(node.id, containerId));
          return `<button class="container-item ${selected ? 'active' : ''}" type="button" data-node-id="${escapeHtml(node.id)}" data-container-id="${escapeHtml(containerId)}" title="${escapeHtml(containerName)}"><i class="container-dot ${container.state === 'running' ? '' : 'stopped'}"></i><span>${escapeHtml(containerName)}</span></button>`;
        }).join('') : '<div class="container-empty">暂无容器数据</div>'}</div>` : ''}
      </div>
    `;
  }).join('') : '<div class="node-empty">暂无已连接节点</div>';
  renderConnectionsView();
  $('#stream-nav-count').textContent = String(state.logs.length).padStart(2, '0');
  $$('[data-expand-node-id]').forEach((button) => button.addEventListener('click', (event) => {
    event.stopPropagation();
    const nodeId = event.currentTarget.dataset.expandNodeId;
    state.expandedNodes = state.expandedNodes.includes(nodeId)
      ? state.expandedNodes.filter((id) => id !== nodeId)
      : [...state.expandedNodes, nodeId];
    renderNodes();
  }));
  $$('#node-list .node-item').forEach((item) => item.addEventListener('click', () => {
    const nodeId = item.dataset.nodeId;
    state.selectedNodes = [nodeId];
    state.selectedContainers = [];
    clearFullRangeSearch();
    loadedContainerSelectionKey = '';
    if (!state.expandedNodes.includes(nodeId)) state.expandedNodes.push(nodeId);
    state.containerLogCache = {};
    loadedContainerSelectionKey = '';
    resetLogPagination();
    renderNodes();
    updateDetailPanel();
    renderLogs();
    persistSelection();
    showToast(`已切换至 ${getNode(nodeId).name}`);
  }));
  $$('#node-list .container-item').forEach((item) => item.addEventListener('click', () => {
    const nodeId = item.dataset.nodeId;
    const containerId = item.dataset.containerId;
    const node = getNode(nodeId);
    const container = node?.containers?.find((entry) => (entry.id || entry.name) === containerId);
    const key = containerKey(nodeId, containerId);
    const alreadySelected = state.selectedContainers.includes(key);
    state.selectedContainers = alreadySelected
      ? state.selectedContainers.filter((value) => value !== key)
      : [...state.selectedContainers, key];
    clearFullRangeSearch();
    const selectedNodeIds = nodeIdsForContainerKeys(state.selectedContainers);
    state.selectedNodes = selectedNodeIds.length ? selectedNodeIds : [nodeId];
    if (!state.expandedNodes.includes(nodeId)) state.expandedNodes.push(nodeId);
    state.containerLogCache = {};
    loadedContainerSelectionKey = '';
    state.selectedLog = 0;
    resetLogPagination();
    renderNodes();
    updateDetailPanel();
    renderLogs();
    persistSelection();
    loadSelectedContainerLogs();
    showToast(`${alreadySelected ? '已取消选择' : '已选择'} ${node?.name || '节点'} / ${container?.name || containerId}`);
  }));
}

function renderConnectionsView() {
  const list = $('#connections-list');
  if (!list) return;
  if (!nodes.length) {
    list.innerHTML = '<div class="connection-empty panel"><strong>暂无 Dozzle 节点</strong><span>点击右上角添加节点开始同步日志。</span></div>';
    return;
  }
  list.innerHTML = nodes.map((node) => {
    const containers = (node.containers || []).slice(0, 8);
    return `
      <article class="connection-card panel">
        <div class="connection-card-heading"><div class="connection-node"><div class="node-avatar orange-bg">${escapeHtml(node.initial)}</div><div><strong>${escapeHtml(node.name)}</strong><span>${escapeHtml(node.url)}</span></div></div><span class="healthy-badge status-${nodeStatusClass(node)}" title="${escapeHtml(nodeStatusLabel(node))}" aria-label="${escapeHtml(nodeStatusLabel(node))}"><i></i></span></div>
        <div class="connection-stats"><div><span>延迟</span><strong>${node.latency > 0 ? `${node.latency} ms` : '—'}</strong></div><div><span>运行中容器</span><strong>${node.count ?? '—'}</strong></div><div><span>版本</span><strong>${escapeHtml(node.version || '—')}</strong></div></div>
        <div class="connection-containers"><div><strong>容器列表</strong><span>${node.containers?.length || 0} 个已发现</span></div><div class="connection-container-tags">${containers.length ? containers.map((container) => `<span>${escapeHtml(container.name || container.id)}</span>`).join('') : '<em>等待容器同步</em>'}</div></div>
        <div class="connection-card-actions"><span>${escapeHtml(node.error || (node.status === 'connected' ? '实时同步正常' : '等待连接结果'))}</span><div class="connection-card-buttons"><button class="secondary-button compact-button" type="button" data-edit-connection-id="${escapeHtml(node.id)}">编辑连接</button><button class="danger-button" type="button" data-unbind-connection-id="${escapeHtml(node.id)}">解绑节点</button></div></div>
      </article>
    `;
  }).join('');
  $$('#connections-list [data-unbind-connection-id]').forEach((button) => button.addEventListener('click', async (event) => {
    const node = getNode(event.currentTarget.dataset.unbindConnectionId);
    if (node) await unbindNode(node);
  }));
  $$('#connections-list [data-edit-connection-id]').forEach((button) => button.addEventListener('click', (event) => {
    const node = getNode(event.currentTarget.dataset.editConnectionId);
    if (node) openNodeModal(node);
  }));
}

function highlightMessage(message) {
  return renderLogMessage(message, searchHighlightTerms());
}

function filteredLogs() {
  const query = normalizedLogSearch(`${state.query} ${state.globalQuery}`);
  const selectedNodeIds = new Set(state.selectedNodes);
  const selectedTargets = selectedContainerTargets();
  const now = Date.now();
  const currentSearchKey = fullRangeSearchKey();
  const source = currentSearchKey && state.fullRangeSearchKey === currentSearchKey
    ? state.fullRangeSearchLogs
    : logsForActiveContainer();
  return source.filter((log) => {
    const logNodeId = nodeIdByName(log.node);
    const nodeMatch = !state.selectedNodes.length || selectedNodeIds.has(logNodeId);
    const containerMatch = !state.selectedContainers.length || selectedTargets.some((target) => target.nodeId === logNodeId && (log.container === target.name || log.container === target.containerId));
    const levelMatch = state.level === 'all' || log.level === state.level;
    const queryMatch = !query || normalizedLogSearch(`${log.node} ${log.container} ${log.message}`).includes(query);
    const noiseMatch = state.ruleState.noise ? !log.message.includes('/healthz') : true;
    return isLogInSelectedRange(log, now) && nodeMatch && containerMatch && levelMatch && queryMatch && noiseMatch;
  }).reverse();
}

async function loadOlderSelectedContainerLogs() {
  const targets = selectedContainerTargets();
  if (!targets.length || state.fullRangeSearchLoading || olderLogsLoading) return;
  olderLogsLoading = true;
  $('#older-log-loader')?.classList.remove('hidden');
  try {
    const pages = await Promise.all(targets.map(async (target) => {
      const key = containerKey(target.nodeId, target.containerId);
      if (olderLogExhausted.has(`${state.range}|${key}`)) return { key, logs: [], hasMore: false };
      const cached = state.containerLogCache[key] || [];
      const oldest = cached.reduce((value, log) => Math.min(value, Number(log.timestamp) || value), Number.POSITIVE_INFINITY);
      if (!Number.isFinite(oldest)) return { key, logs: [], hasMore: false };
      const params = new URLSearchParams({ node: target.nodeId, container: target.containerId, range: state.range, before: String(oldest) });
      const response = await fetch(`/api/logs/container/page?${params.toString()}`, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error((await response.json().catch(() => ({}))).error || '加载更早日志失败');
      return { key, ...(await response.json()) };
    }));
    let added = 0;
    pages.forEach((page) => {
      added += (page.logs || []).length;
      state.containerLogCache[page.key] = mergeLogs(state.containerLogCache[page.key] || [], page.logs || []);
      if (!page.hasMore) olderLogExhausted.add(`${state.range}|${page.key}`);
    });
    lastVirtualWindowKey = '';
    renderLogs({ preserveScroll: true });
    if (!added) showToast('已到所选时间范围的最早日志');
  } catch (error) {
    showToast(error.message || '加载更早日志失败');
  } finally {
    olderLogsLoading = false;
    $('#older-log-loader')?.classList.add('hidden');
  }
}

function maybeLoadOlderLogs() {
  if (olderLogsLoading || state.fullRangeSearchLoading || fullRangeSearchKey()) return;
  const stream = $('#log-stream');
  const reachedBrowseCap = state.selectedContainers.some((key) => (state.containerLogCache[key] || []).length >= 5000);
  const atOldestVisibleRow = Boolean(stream) && stream.scrollTop <= 2;
  if (reachedBrowseCap && atOldestVisibleRow) loadOlderSelectedContainerLogs();
}

function resetLogPagination() {
  state.visibleLogs = logPageSize;
  if (initialLogPreviewTimer) clearTimeout(initialLogPreviewTimer);
  initialLogPreviewActive = false;
  initialLogPreviewLimit = 0;
  lastVirtualWindowKey = '';
  followLatestLogs = true;
}

function scheduleInitialLogCompletion() {
  if (initialLogPreviewTimer || !initialLogPreviewActive || state.historyLoading) return;
  initialLogPreviewTimer = setTimeout(() => {
    initialLogPreviewTimer = null;
    if (state.historyLoading) return;
    initialLogPreviewActive = false;
    initialLogPreviewLimit = 0;
    renderLogs({ reuseFiltered: true });
  }, 80);
}

function logVirtualWindow(total, stream, stickToBottom = false) {
  if (!total) return { start: 0, end: 0 };
  // Adaptive row heights cannot use fixed spacer math; render normal-sized
  // result sets completely so every multi-line message can determine its row height.
  // Keep virtualization as a safeguard for unusually large log histories.
  if (total <= adaptiveLogRenderLimit) return { start: 0, end: total };
  const viewportRows = Math.max(12, Math.ceil(stream.clientHeight / logVirtualRowHeight));
  const windowSize = viewportRows + logVirtualOverscan * 2;
  if (stickToBottom) return { start: Math.max(0, total - windowSize), end: total };
  const anchor = Math.floor(stream.scrollTop / logVirtualRowHeight);
  const start = Math.max(0, Math.min(Math.max(0, total - 1), anchor - logVirtualOverscan));
  return { start, end: Math.min(total, start + windowSize) };
}

function scheduleLogRender() {
  if (logRenderTimer) return;
  logRenderTimer = setTimeout(() => {
    logRenderTimer = null;
    renderLogs({ preserveScroll: true });
  }, logRenderInterval);
}

function hasLogTextSelection() {
  const selection = window.getSelection();
  const stream = $('#log-stream');
  if (!selection || selection.isCollapsed || !selection.rangeCount || !stream) return false;
  return stream.contains(selection.getRangeAt(0).commonAncestorContainer);
}

function scheduleLogWindowRender() {
  const stream = $('#log-stream');
  followLatestLogs = isLogStreamNearBottom(stream);
  syncLogFlowVisibility(stream);
  if (logScrollFrame) return;
  logScrollFrame = requestAnimationFrame(() => {
    logScrollFrame = null;
    renderLogs({ reuseFiltered: true, preserveScroll: true });
  });
}

function isLogStreamNearBottom(stream) {
  return stream.scrollHeight - stream.scrollTop - stream.clientHeight < 24;
}

function syncLogFlowVisibility(stream) {
  const footer = $('.logs-panel .stream-footer');
  if (!footer) return;
  const distanceFromBottom = stream.scrollHeight - stream.scrollTop - stream.clientHeight;
  footer.classList.toggle('is-at-latest', distanceFromBottom <= 2);
}

function renderLogs({ reuseFiltered = false, preserveScroll = false, renderLimit = 0 } = {}) {
  const stream = $('#log-stream');
  const emptyState = $('#empty-state');
  // Replacing the log rows destroys the browser's native text selection.
  // Keep it intact while the user is dragging or copying a log excerpt.
  if (hasLogTextSelection()) return;
  const scrollTop = stream.scrollTop;
  const stickToBottom = followLatestLogs;
  const allResults = reuseFiltered ? lastFilteredLogs : filteredLogs();
  if (!reuseFiltered) lastFilteredLogs = allResults;
  const stagedResults = renderLimit > 0 ? allResults.slice(-renderLimit) : allResults;
  const { start, end } = logVirtualWindow(stagedResults.length, stream, stickToBottom);
  const results = stagedResults.slice(start, end);
  const loadedLabel = state.fullRangeSearchLoading
    ? `正在筛选完整${rangeLabels[state.range] || '时间范围'}日志…`
    : state.historyLoading
    ? `历史日志加载中 · 已发现 ${allResults.length} 条`
    : `显示 ${allResults.length} 条`;
  $('#all-count').textContent = allResults.length.toLocaleString('en-US');
  $('#row-count').textContent = loadedLabel;
  const selectedNodeNames = state.selectedNodes.map((id) => getNode(id)?.name).filter(Boolean);
  const rangeLabel = rangeLabels[state.range] || rangeLabels['30m'];
  const nodeCaption = selectedNodeNames.length === 1
    ? selectedNodeNames[0]
    : selectedNodeNames.length > 1
      ? `已选 ${selectedNodeNames.length} 个 Dozzle 节点`
      : `${nodes.length} 个 Dozzle 节点`;
  $('#stream-caption').textContent = `来自 ${nodeCaption} · ${rangeLabel}`;
  $('#stream-nav-count').textContent = String(allResults.length).padStart(2, '0');
  const firstId = results[0]?.id || 0;
  const lastId = results[results.length - 1]?.id || 0;
  const windowKey = `${allResults.length}:${stagedResults.length}:${start}:${end}:${firstId}:${lastId}:${state.selectedLog}:${state.activeAnalysisLogId}`;
  if (reuseFiltered && windowKey === lastVirtualWindowKey) {
    if (preserveScroll && stream.scrollTop !== scrollTop) stream.scrollTop = scrollTop;
    syncLogFlowVisibility(stream);
    return;
  }
  lastVirtualWindowKey = windowKey;
  const topSpacer = start > 0 ? `<div class="log-virtual-spacer" style="height:${start * logVirtualRowHeight}px" aria-hidden="true"></div>` : '';
  const bottomSpacer = end < stagedResults.length ? `<div class="log-virtual-spacer" style="height:${(stagedResults.length - end) * logVirtualRowHeight}px" aria-hidden="true"></div>` : '';
  stream.innerHTML = `${topSpacer}${results.map((log, index) => `
    <div class="log-row log-row-${escapeHtml(log.level || 'info')} log-row-tone-${(start + index) % 2 ? 'odd' : 'even'} ${log.id === state.selectedLog ? 'selected' : ''}" data-log-id="${log.id}">
      <span class="log-meta">
        <span class="log-date">${highlightSearchText(logDate(log))}</span>
        <span class="log-time">${highlightSearchText(logTime(log))}</span>
        <span class="log-container-origin" title="${escapeHtml(`${log.node || '未知节点'}/${log.container || '未知容器'}`)}">${highlightSearchText(`${log.node || '未知节点'}/${log.container || '未知容器'}`)}</span>
      </span>
      <span class="log-level ${log.level}">${highlightSearchText(log.level.toUpperCase())}</span>
      <span class="log-source"><span class="source-tag">${highlightSearchText(log.node)}</span><span class="container-tag">/${highlightSearchText(log.container)}</span></span>
      <span class="log-level-marker ${escapeHtml(log.level || 'info')}" aria-hidden="true"></span>
      <span class="log-message" title="${escapeHtml(log.message)}">${highlightMessage(log.message)}</span><span class="row-actions"><button class="row-ai-button${String(log.id) === state.activeAnalysisLogId ? ' active' : ''}" type="button" data-analyze-log="${log.id}" aria-label="分析这条日志" aria-pressed="${String(log.id) === state.activeAnalysisLogId}" title="让 AI 分析这条日志"><img src="ai-icon.png" alt="" aria-hidden="true" /></button></span>
    </div>
  `).join('')}${bottomSpacer}`;
  stream.append(emptyState);
  emptyState.classList.toggle('hidden', allResults.length > 0);
  if (stickToBottom) stream.scrollTop = stream.scrollHeight;
  else if (preserveScroll && stream.scrollTop !== scrollTop) stream.scrollTop = scrollTop;
  syncLogFlowVisibility(stream);
}

function updateDetailPanel() {
  if (!$('#node-detail-panel')) return;
  const detailNodeId = state.selectedNodes.length ? state.selectedNodes[0] : nodes[0]?.id;
  const node = detailNodeId ? getNode(detailNodeId) : null;
  const selectedNodeAvatar = $('.selected-node-row .node-avatar');
  const unbindButton = $('#unbind-node-button');
  const editButton = $('#edit-node-button');
  if (!node) {
    $('#detail-node-name').textContent = '未选择节点';
    $('#detail-node-url').textContent = '暂无连接';
    selectedNodeAvatar.classList.remove('orange-bg');
    selectedNodeAvatar.style.background = '';
    $('#detail-latency').textContent = '—';
    $('#detail-containers').textContent = '—';
    $('#detail-version').textContent = '—';
    $('#detail-heartbeat').textContent = '暂无心跳数据';
    $('#detail-node-status').className = 'healthy-badge status-offline';
    $('#detail-node-status').innerHTML = '<i></i>';
    $('#detail-node-status').title = '未连接';
    unbindButton.disabled = true;
    delete unbindButton.dataset.nodeId;
    editButton.disabled = true;
    delete editButton.dataset.nodeId;
    return;
  }
  $('#detail-node-name').textContent = node.name;
  $('#detail-node-url').textContent = node.url;
  selectedNodeAvatar.classList.remove('orange-bg');
  selectedNodeAvatar.style.background = '';
  $('#detail-node-status').className = `healthy-badge status-${nodeStatusClass(node)}`;
  $('#detail-node-status').innerHTML = '<i></i>';
  $('#detail-node-status').title = nodeStatusLabel(node);
  $('#detail-node-status').setAttribute('aria-label', nodeStatusLabel(node));
  unbindButton.disabled = false;
  unbindButton.dataset.nodeId = node.id;
  editButton.disabled = false;
  editButton.dataset.nodeId = node.id;
  $('#detail-latency').textContent = node.latency > 0 ? `${node.latency} ms` : '—';
  $('#detail-containers').textContent = node.count ?? '—';
  $('#detail-version').textContent = node.version || '—';
  $('#detail-heartbeat').textContent = node.error || (node.status === 'connected' ? '等待日志数据' : '等待连接结果');
}

function updatePreview() {
  if (!$('#preview-code')) return;
  const log = logByID(state.selectedLog) || state.logs[0];
  if (!log) {
    $('#preview-code').textContent = '暂无日志数据';
    return;
  }
  const level = log.level;
  const service = log.container;
  const preview = { level, service };
  if (state.ruleState.structure) {
    const trace = log.message.match(/trace[_-]?id[=:]\s*([^\s·]+)/i)?.[1];
    if (trace) preview.trace_id = state.ruleState.mask ? '***' : trace;
  }
  preview.processing_order = state.ruleOrder.filter((rule) => state.ruleState[rule]);
  $('#preview-code').textContent = JSON.stringify(preview);
}

function showToast(message) {
  $('#toast-message').textContent = message;
  $('#toast').classList.remove('hidden');
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => $('#toast').classList.add('hidden'), 2300);
}

async function syncGoBackend({ connectStream = false, incremental = false } = {}) {
  try {
    const processedBeforeRequest = state.processed;
    const knownLogId = incremental ? state.logs.reduce((maxId, log) => Math.max(maxId, Number(log.id) || 0), 0) : 0;
    const cursor = Math.max(0, knownLogId - 2000);
    const endpoint = knownLogId ? `/api/bootstrap?since=${encodeURIComponent(cursor)}` : '/api/bootstrap';
    let response = await fetch(endpoint, { headers: { Accept: 'application/json' } });
    if (!response.ok) throw new Error('Go backend is unavailable');
    let payload = await response.json();
    let serverRestarted = false;
    if (knownLogId && Number(payload.processed) < processedBeforeRequest) {
      response = await fetch('/api/bootstrap', { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error('Go backend is unavailable');
      payload = await response.json();
      serverRestarted = true;
    }
    const updateLogView = !state.paused;
    const previousRange = state.range;
    const previousHistoryLoading = state.historyLoading;
    const incomingLogs = payload.logs || [];
    const stageInitialLogs = updateLogView && !initialLogPreviewRendered && incomingLogs.length > 0;
    applyStorageMode(payload.storageMode);
    // In browser mode the server's node list is always empty by design: this
    // visitor's nodes live in localStorage and must survive the refresh.
    nodes.splice(0, nodes.length, ...(isBrowserStorageMode() ? loadBrowserNodes() : (payload.nodes || [])));
    state.selectedNodes = state.selectedNodes.filter((id) => getNode(id));
    state.expandedNodes = state.expandedNodes.filter((id) => getNode(id));
    const serverCapacity = Number(payload.storage?.capacity);
    if (Number.isInteger(serverCapacity) && serverCapacity > 0) maxBufferedLogs = serverCapacity;
    let logsChanged = false;
    if (updateLogView) {
      const previousLogs = state.logs;
      state.logs = incremental && !serverRestarted ? mergeLogs(state.logs, incomingLogs) : incomingLogs;
      logsChanged = state.logs !== previousLogs;
      state.processed = payload.processed;
      state.historyLoading = Boolean(payload.historyLoading);
      if (stageInitialLogs) {
        initialLogPreviewRendered = true;
        initialLogPreviewActive = true;
        initialLogPreviewLimit = initialLogPreviewSize;
      }
      updateLastSync(state.logs[0]?.timestamp);
      updateStorage(payload.storage);
    }
    if (rangeLabels[payload.range]) {
      state.range = payload.range;
      $('#time-range-filter').value = payload.range;
      refreshCustomSelect($('#time-range-filter'));
    }
    state.ruleState = { ...state.ruleState, ...payload.rules };
    state.ruleOrder = normalizeRuleOrder(payload.ruleOrder || state.ruleOrder);
    syncRuleButtons();
    goServerConnected = true;
    const syncLabel = eventStream?.readyState === EventSource.OPEN ? '实时同步中' : '服务端已连接';
    $('#sync-status').textContent = state.paused ? '接收已暂停' : syncLabel;
    if (!state.paused) $('#stream-status').textContent = state.historyLoading ? '正在加载历史日志' : '正在监听';
    updateSyncFooter(state.paused ? '已暂停接收' : syncLabel, state.paused ? 'paused' : 'connected');
    if (updateLogView) $('#metric-processed').textContent = state.processed.toLocaleString('en-US');
    $('#metric-processed-foot').textContent = state.paused
      ? '已暂停，当前视图不会接收新日志'
      : state.processed
        ? '服务启动后累计接收，不等于缓存条数'
        : '等待日志流';
    renderNodes();
    if (updateLogView && (!incremental || serverRestarted || logsChanged || previousRange !== state.range || previousHistoryLoading !== state.historyLoading)) {
      const renderLimit = initialLogPreviewActive ? initialLogPreviewLimit : 0;
      renderLogs({ renderLimit });
    }
    if (updateLogView) scheduleInitialLogCompletion();
    updateDetailPanel(); updatePreview();
    persistSelection();
    if (!state.paused && (connectStream || !eventStream || eventStream.readyState === EventSource.CLOSED)) connectGoStream();
    if (!state.paused && state.selectedContainers.length) loadSelectedContainerLogs();
    return true;
  } catch (error) {
    goServerConnected = false;
    if (state.paused) return false;
    $('#sync-status').textContent = '等待后端连接';
    updateSyncFooter('等待服务连接', 'offline');
    return false;
  }
}

function scheduleHistorySync() {
  if (historySyncTimer || state.paused || !goServerConnected || !state.historyLoading) return;
  historySyncTimer = setTimeout(async () => {
    historySyncTimer = null;
    if (state.paused || !goServerConnected || !state.historyLoading) return;
    if (await syncGoBackend({ incremental: true }) && state.historyLoading) scheduleHistorySync();
  }, 500);
}

function clearPendingStreamBatch() {
  if (streamBatchTimer) clearTimeout(streamBatchTimer);
  streamBatchTimer = null;
  pendingStreamLogs = [];
}

function flushPendingStreamLogs() {
  streamBatchTimer = null;
  if (state.paused || !pendingStreamLogs.length) {
    pendingStreamLogs = [];
    return;
  }
  const incomingLogs = pendingStreamLogs;
  pendingStreamLogs = [];
  state.logs = mergeLogs(state.logs, incomingLogs, { skipDuplicateCheck: true });

  const cacheKeysBySource = new Map();
  selectedContainerTargets().forEach((target) => {
    const activeNode = getNode(target.nodeId);
    if (!activeNode) return;
    const cacheKey = containerKey(target.nodeId, target.containerId);
    cacheKeysBySource.set(`${activeNode.name}\u0000${target.name}`, cacheKey);
    cacheKeysBySource.set(`${activeNode.name}\u0000${target.containerId}`, cacheKey);
  });
  const cacheUpdates = new Map();
  incomingLogs.forEach((incoming) => {
    const cacheKey = cacheKeysBySource.get(`${incoming.node}\u0000${incoming.container}`);
    if (!cacheKey) return;
    if (!cacheUpdates.has(cacheKey)) cacheUpdates.set(cacheKey, []);
    cacheUpdates.get(cacheKey).push(incoming);
  });
  cacheUpdates.forEach((updates, cacheKey) => {
    state.containerLogCache[cacheKey] = mergeLogs(state.containerLogCache[cacheKey] || [], updates, { skipDuplicateCheck: true });
  });

  state.processed += incomingLogs.length;
  $('#metric-processed').textContent = state.processed.toLocaleString('en-US');
  $('#metric-processed-foot').textContent = '服务启动后累计接收，不等于缓存条数';
  updateSyncFooter('实时同步中', 'connected');
  const newestTimestamp = incomingLogs.reduce((latest, log) => Math.max(latest, Number(log.timestamp) || 0), 0);
  updateLastSync(newestTimestamp);
  scheduleLogRender();
}

function enqueueStreamLog(incoming) {
  pendingStreamLogs.push(incoming);
  if (pendingStreamLogs.length > maxPendingStreamLogs) {
    pendingStreamLogs.splice(0, pendingStreamLogs.length - maxPendingStreamLogs);
  }
  if (!streamBatchTimer) streamBatchTimer = setTimeout(flushPendingStreamLogs, streamBatchInterval);
}

function connectGoStream() {
  if (state.paused) return;
  if (eventStream) eventStream.close();
  const stream = new EventSource('/api/stream');
  eventStream = stream;
  stream.onopen = () => {
    if (eventStream !== stream) {
      stream.close();
      return;
    }
    if (state.paused) {
      stream.close();
      return;
    }
    $('#sync-status').textContent = '实时同步中';
    $('#stream-status').textContent = '正在监听';
    updateSyncFooter('实时同步中', 'connected');
  };
  stream.onmessage = (event) => {
    if (state.paused || eventStream !== stream) return;
    enqueueStreamLog(JSON.parse(event.data));
  };
  stream.onerror = () => {
    stream.close();
    if (state.paused || eventStream !== stream) return;
    goServerConnected = false;
    $('#sync-status').textContent = '实时流未连接';
    $('#stream-status').textContent = '等待连接';
    updateSyncFooter('实时流未连接', 'offline');
  };
}

function openNodeModal(node = null) {
  const modal = $('#add-node-modal');
  const form = $('#add-node-form');
  modal.dataset.nodeId = node?.id || '';
  form.elements.name.value = node?.name || '';
  form.elements.url.value = node?.url || '';
  form.elements.protocol.value = node?.style || 'HTTP / WebSocket';
  $('#modal-kicker').textContent = node ? 'EDIT CONNECTION' : 'NEW CONNECTION';
  $('#modal-title').textContent = node ? '编辑 Dozzle 节点' : '添加 Dozzle 节点';
  $('#node-submit-button').textContent = node ? '保存修改' : '连接并添加';
  modal.classList.remove('hidden');
}

function closeNodeModal() {
  const modal = $('#add-node-modal');
  modal.classList.add('hidden');
  delete modal.dataset.nodeId;
  $('#add-node-form').reset();
  $('#modal-kicker').textContent = 'NEW CONNECTION';
  $('#modal-title').textContent = '添加 Dozzle 节点';
  $('#node-submit-button').textContent = '连接并添加';
}

// ---------------------------------------------------------------------------
// Storage mode
//
// The dashboard can be opened two ways and they keep separate data:
//
//   file    - the page is on the machine running the service, so nodes and
//             providers belong in the service's JSON files.
//   browser - the page is a visitor on someone else's deployment, so its nodes
//             and API keys stay in this browser and never reach that host.
//
// The server decides which applies (from the request's source address) and
// reports it via /api/bootstrap; the client never guesses, because a wrong
// guess would mean uploading credentials to a stranger or losing them.
// ---------------------------------------------------------------------------

function isBrowserStorageMode() {
  return state.storageMode === 'browser';
}

// applyStorageMode records which backend serves this page and loads the
// providers that belong to it.
//
// It must not touch `nodes` here: syncGoBackend replaces that array immediately
// after this call, so anything merged in would be discarded on the same tick.
// The replace site is what consults isBrowserStorageMode(). Providers have no
// such replace site, so the load happens here instead — and only on the first
// resolve, because a later switch already reloads the page.
function applyStorageMode(mode) {
  const next = mode === 'browser' ? 'browser' : 'file';
  if (state.storageMode === next) return;
  const isFirstResolve = state.storageMode === '';
  state.storageMode = next;
  if (!isFirstResolve) showToast('存储位置已切换，正在重新加载');
  if (isFirstResolve) {
    loadAIProfiles();
    if (next === 'file') loadAIProfilesFromFile();
  }
  updateStorageModeUI();
}

function loadBrowserNodes() {
  try {
    const saved = JSON.parse(localStorage.getItem(browserNodesStorageKey) || '[]');
    if (!Array.isArray(saved)) return [];
    return saved.map((node) => ({
      ...node,
      id: String(node.id || ''),
      status: node.status === 'online' ? 'online' : 'connecting',
    })).filter((node) => node.id && node.name && node.url);
  } catch (error) {
    return [];
  }
}

function persistBrowserNodes() {
  if (!isBrowserStorageMode()) return;
  try {
    localStorage.setItem(browserNodesStorageKey, JSON.stringify(nodes.map((node) => ({
      id: node.id,
      name: node.name,
      url: node.url,
      style: node.style,
      initial: node.initial,
    }))));
  } catch (error) {
    showToast('浏览器本地存储不可用，节点配置无法保存');
  }
}

// updateStorageModeUI is the single place that reflects the storage backend in
// the settings panel. The reveal link is the whole signal: present when this
// page is served from the machine that owns the files, absent otherwise. The
// note text explains the difference without naming a path.
function updateStorageModeUI() {
  const browser = isBrowserStorageMode();
  const note = $('#settings-config-note');
  const section = $('#settings-config-storage');
  const reveal = $('#settings-config-reveal');
  if (section) section.classList.toggle('hidden', false);
  if (note) {
    note.textContent = browser
      ? '当前页面未在本机打开，节点与模型配置保存在此浏览器中，不会上传到服务器。'
      : '节点与模型保存在程序目录下的 data 文件夹，整个目录拷到别的机器即可带走配置。保存模型密钥的文件含明文凭据，请勿分享或同步到公开位置。';
  }
  if (reveal) {
    // canReveal is false on a desktop-less host, where opening a file manager
    // is meaningless; treat that the same as browser mode and hide the link.
    const show = !browser && state.configInfo?.canReveal !== false;
    reveal.classList.toggle('hidden', !show);
  }
}

async function persistNodeToGo(name, url, style) {
  // A visitor on a remote deployment keeps its nodes in this browser.
  if (isBrowserStorageMode()) {
    const node = {
      id: `node-local-${Date.now()}`,
      name,
      url,
      style: style || 'HTTP / WebSocket',
      initial: String(name || '?').trim().charAt(0) || '?',
      status: 'connecting',
      latency: 0,
      version: '',
      containers: [],
    };
    nodes.push(node);
    persistBrowserNodes();
    renderNodes();
    updateDetailPanel();
    showToast('节点已添加到此浏览器');
    return node;
  }
  if (!goServerConnected) {
    showToast('服务端未连接，暂时无法添加节点');
    return null;
  }
  try {
    const response = await fetch('/api/nodes', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, url, style })
    });
    if (!response.ok) {
      const detail = await response.json().catch(() => ({}));
      throw new Error(detail.error || 'node create failed');
    }
    const node = await response.json();
    const index = nodes.findIndex((item) => item.id === node.id);
    if (index === -1) nodes.push(node); else nodes[index] = node;
    renderNodes();
    updateDetailPanel();
    return node;
  } catch (error) {
    showToast(error.message === 'node already exists' ? '该节点已经添加' : `节点添加失败：${error.message}`);
    return null;
  }
}

async function updateNodeToGo(id, name, url, style) {
  if (isBrowserStorageMode()) {
    const index = nodes.findIndex((item) => item.id === id);
    if (index < 0) return null;
    nodes[index] = { ...nodes[index], name, url, style: style || nodes[index].style, initial: String(name || '?').trim().charAt(0) || '?' };
    persistBrowserNodes();
    renderNodes();
    updateDetailPanel();
    showToast('节点已更新');
    return nodes[index];
  }
  if (!goServerConnected) {
    showToast('服务端未连接，暂时无法修改节点');
    return null;
  }
  try {
    const response = await fetch(`/api/nodes/${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ name, url, style })
    });
    if (!response.ok) {
      const detail = await response.json().catch(() => ({}));
      throw new Error(detail.error || 'node update failed');
    }
    const node = await response.json();
    const index = nodes.findIndex((item) => item.id === node.id);
    if (index >= 0) nodes[index] = node; else nodes.push(node);
    renderNodes();
    updateDetailPanel();
    showToast('节点连接信息已更新，正在重新连接');
    return node;
  } catch (error) {
    showToast(error.message === 'node already exists' ? '该节点已经添加' : `节点修改失败：${error.message}`);
    return null;
  }
}

async function unbindNode(node) {
  if (isBrowserStorageMode()) {
    if (!window.confirm(`确定解绑 Dozzle 节点“${node.name}”吗？`)) return;
    const index = nodes.findIndex((item) => item.id === node.id);
    if (index >= 0) nodes.splice(index, 1);
    state.selectedNodes = state.selectedNodes.filter((id) => id !== node.id);
    state.selectedContainers = state.selectedContainers.filter((value) => !value.startsWith(`${node.id}::`));
    state.expandedNodes = state.expandedNodes.filter((id) => id !== node.id);
    state.selectedLog = 0;
    resetLogPagination();
    persistBrowserNodes();
    renderNodes();
    renderLogs();
    updateDetailPanel();
    updatePreview();
    persistSelection();
    showToast('节点已从此浏览器移除');
    return;
  }
  if (!goServerConnected) {
    showToast('服务端未连接，暂时无法解绑节点');
    return;
  }
  if (!window.confirm(`确定解绑 Dozzle 节点“${node.name}”吗？`)) return;
  try {
    const response = await fetch(`/api/nodes/${encodeURIComponent(node.id)}`, { method: 'DELETE' });
    if (!response.ok) {
      const detail = await response.json().catch(() => ({}));
      throw new Error(detail.error || 'node delete failed');
    }
    const index = nodes.findIndex((item) => item.id === node.id);
    if (index >= 0) nodes.splice(index, 1);
    state.selectedNodes = state.selectedNodes.filter((id) => id !== node.id);
    state.selectedContainers = state.selectedContainers.filter((value) => !value.startsWith(`${node.id}::`));
    state.expandedNodes = state.expandedNodes.filter((id) => id !== node.id);
    state.selectedLog = 0;
    resetLogPagination();
    renderNodes();
    renderLogs();
    updateDetailPanel();
    updatePreview();
    persistSelection();
    showToast(`${node.name} 已解绑`);
  } catch (error) {
    showToast(`解绑失败：${error.message}`);
  }
}


const commandStorageKey = 'log-agent-command-flows';
const commandServerStorageKey = 'log-agent-command-servers';
const commandFavoritesStorageKey = 'log-agent-command-favorites';

function commandDefaults() {
  return [{ id: `flow-${Date.now()}`, name: '发布前检查', serverId: '', lines: [
    { text: 'echo "开始检查"', status: 'idle' },
    { text: 'docker ps --format "table {{.Names}}\\t{{.Status}}"', status: 'idle' },
    { text: 'df -h', status: 'idle' }
  ] }];
}

function loadCommandFlows() {
  state.commandServerKeys = state.commandServerKeys || {};
  try {
    const savedFlows = JSON.parse(localStorage.getItem(commandStorageKey) || 'null');
    const savedServers = JSON.parse(localStorage.getItem(commandServerStorageKey) || 'null');
    const savedFavorites = JSON.parse(localStorage.getItem(commandFavoritesStorageKey) || '[]');
    state.commandFlows = Array.isArray(savedFlows) && savedFlows.length ? savedFlows : commandDefaults();
    state.commandServers = Array.isArray(savedServers) ? savedServers : [];
    state.commandFavorites = Array.isArray(savedFavorites) ? savedFavorites.filter((item) => item && Array.isArray(item.flows)).slice(0, 50) : [];
  } catch {
    state.commandFlows = commandDefaults();
    state.commandServers = [];
    state.commandFavorites = [];
  }
  state.commandFlows.forEach((flow) => {
    if (!flow.serverId && flow.host) {
      const server = { id: `server-${Date.now()}-${Math.random().toString(16).slice(2)}`, name: flow.host, host: flow.host, port: flow.port || '22', user: flow.user || '', auth: flow.auth || 'key', secret: flow.secret || '' };
      state.commandServers.push(server);
      flow.serverId = server.id;
    }
  });
  state.activeCommandFlowId = state.commandFlows[0].id;
  saveCommandFlows();
}

function saveCommandFlows() {
  try {
    localStorage.setItem(commandStorageKey, JSON.stringify(state.commandFlows));
    localStorage.setItem(commandServerStorageKey, JSON.stringify(state.commandServers || []));
    localStorage.setItem(commandFavoritesStorageKey, JSON.stringify(state.commandFavorites || []));
  } catch {}
}

function commandFavoriteSnapshot() {
  return JSON.parse(JSON.stringify((state.commandFlows || []).map((flow) => ({
    id: flow.id,
    name: flow.name || '',
    serverId: flow.serverId || '',
    lines: (flow.lines || []).map((line) => ({
      type: line.type || 'command',
      text: line.text || '',
      fileId: line.fileId || '',
      fileName: (state.commandFiles || []).find((file) => file.id === line.fileId)?.name || line.fileName || '',
      destination: line.destination || '',
      status: 'idle',
      uploadProgress: 0
    }))
  }))));
}

function renderCommandFavorites() {
  const list = $('#command-favorites-list');
  if (!list) return;
  const favorites = state.commandFavorites || [];
  list.innerHTML = favorites.length ? favorites.map((item) => {
    const lineCount = item.flows.reduce((total, flow) => total + (flow.lines?.length || 0), 0);
    const date = new Date(item.createdAt || Date.now()).toLocaleString('zh-CN', { hour12: false });
    return `<article class="command-favorite-item"><div><strong>${escapeHtml(item.name || '未命名收藏')}</strong><small>${escapeHtml(date)} · ${item.flows.length} 组 · ${lineCount} 行</small></div><div class="command-favorite-item-actions"><button class="secondary-button compact-button" type="button" data-restore-command-favorite="${escapeHtml(item.id)}">还原</button><button class="danger-button compact-button" type="button" data-delete-command-favorite="${escapeHtml(item.id)}">删除</button></div></article>`;
  }).join('') : '<div class="command-favorites-empty">暂无收藏的指令集组</div>';
}

function openCommandFavorites() {
  const modal = $('#command-favorites-modal');
  if (!modal) return;
  const flow = activeCommandFlow();
  $('#command-favorite-name').value = flow?.name ? `${flow.name}收藏` : `指令集组收藏 ${new Date().toLocaleDateString('zh-CN')}`;
  renderCommandFavorites();
  modal.classList.remove('hidden');
  requestAnimationFrame(() => { $('#command-favorite-name').focus(); $('#command-favorite-name').select(); });
}

function closeCommandFavorites() { $('#command-favorites-modal')?.classList.add('hidden'); }

function saveCommandFavorite(event) {
  event?.preventDefault();
  const name = $('#command-favorite-name')?.value.trim();
  if (!name) { $('#command-favorite-name')?.focus(); return showToast('请输入收藏名称'); }
  state.commandFavorites = state.commandFavorites || [];
  state.commandFavorites.unshift({ id: `favorite-${Date.now()}-${Math.random().toString(16).slice(2)}`, name, createdAt: Date.now(), flows: commandFavoriteSnapshot() });
  state.commandFavorites = state.commandFavorites.slice(0, 50);
  saveCommandFlows();
  renderCommandFavorites();
  showToast(`已收藏整个指令集组：${name}`);
}

function restoreCommandFavorite(favoriteId) {
  const favorite = (state.commandFavorites || []).find((item) => item.id === favoriteId);
  if (!favorite?.flows?.length) return;
  const restoredAt = Date.now();
  state.commandFlows = JSON.parse(JSON.stringify(favorite.flows)).map((flow, flowIndex) => ({
    ...flow,
    id: `flow-${restoredAt}-${flowIndex}-${Math.random().toString(16).slice(2)}`,
    lines: (flow.lines || []).map((line) => ({ ...line, status: 'idle', uploadProgress: 0, error: '' }))
  }));
  state.activeCommandFlowId = state.commandFlows[0].id;
  saveCommandFlows();
  renderCommandFlows();
  renderCommandEditor();
  closeCommandFavorites();
  showToast(`已还原“${favorite.name}”的可编辑副本`);
}

function deleteCommandFavorite(favoriteId) {
  const favorite = (state.commandFavorites || []).find((item) => item.id === favoriteId);
  state.commandFavorites = (state.commandFavorites || []).filter((item) => item.id !== favoriteId);
  saveCommandFlows();
  renderCommandFavorites();
  if (favorite) showToast(`已删除收藏：${favorite.name}`);
}

function activeCommandFlow() { return state.commandFlows.find((flow) => flow.id === state.activeCommandFlowId) || state.commandFlows[0]; }
function commandServer(id) { return (state.commandServers || []).find((server) => server.id === id); }

function bindServerToFlow(flowId, serverId) {
  const flow = state.commandFlows.find((item) => item.id === flowId);
  if (!flow || !commandServer(serverId)) return;
  flow.serverId = serverId;
  flow.lines.forEach((line) => { if (line.status !== 'idle') line.status = 'idle'; });
  saveCommandFlows();
  renderCommandFlows();
  if (flow.id === state.activeCommandFlowId) renderCommandEditor();
  showToast(`已绑定服务器：${commandServer(serverId).name || commandServer(serverId).host}`);
}

function addCommandFiles(fileList) {
  const files = Array.from(fileList || []).filter((file) => file && file.name);
  if (!files.length) return;
  state.commandFiles = state.commandFiles || [];
  files.forEach((file) => {
    const existing = state.commandFiles.find((item) => item.name === file.name && item.size === file.size);
    if (!existing) { const imageFile = file.type?.startsWith('image/') || /\.(?:avif|gif|jpe?g|png|webp)$/i.test(file.name); state.commandFiles.push({ id: `file-${Date.now()}-${Math.random().toString(16).slice(2)}`, name: file.name, size: file.size, type: file.type || 'application/octet-stream', file, previewURL: imageFile ? URL.createObjectURL(file) : '', uploadProgress: 0, uploadStatus: '' }); }
  });
  renderCommandFileShelf();
  showToast(`已添加 ${files.length} 个文件，可拖入指令行`);
}
function formatFileSize(size) { if (size < 1024) return `${size} B`; if (size < 1024 * 1024) return `${Math.round(size / 1024)} KB`; return `${(size / (1024 * 1024)).toFixed(1)} MB`; }
function renderCommandFileShelf() {
  const list = $('#command-file-list');
  if (!list) return;
  const files = state.commandFiles || [];
  $('#command-file-count').textContent = `${files.length} 个文件`;
  list.innerHTML = files.map((item) => `<article class="command-file-card upload-${escapeHtml(item.uploadStatus || 'idle')}" draggable="true" data-command-file-id="${escapeHtml(item.id)}"><div class="command-file-thumb${item.previewURL ? ' image' : ''}">${item.previewURL ? `<img src="${escapeHtml(item.previewURL)}" alt="${escapeHtml(item.name)}" />` : `<span>${escapeHtml((item.name.split('.').pop() || 'FILE').slice(0, 4).toUpperCase())}</span>`}</div><div class="command-file-copy"><strong>${escapeHtml(item.name)}</strong><small><span>${formatFileSize(item.size || 0)}</span><span class="command-file-progress-label">${item.uploadStatus === 'uploading' ? `${item.uploadProgress || 0}%` : item.uploadStatus === 'success' ? '已上传' : item.uploadStatus === 'error' ? '上传失败' : ''}</span></small></div><div class="command-file-progress"><i style="width:${Math.max(0, Math.min(100, item.uploadProgress || 0))}%"></i></div><button type="button" class="command-file-remove" aria-label="移除文件">×</button></article>`).join('');
  list.querySelectorAll('.command-file-card').forEach((card) => {
    card.addEventListener('dragstart', (event) => { event.dataTransfer.setData('text/command-file-id', card.dataset.commandFileId); event.dataTransfer.effectAllowed = 'copy'; });
    card.querySelector('.command-file-remove').addEventListener('click', () => { const removed = state.commandFiles.find((item) => item.id === card.dataset.commandFileId); if (removed?.previewURL) URL.revokeObjectURL(removed.previewURL); state.commandFiles = state.commandFiles.filter((item) => item.id !== card.dataset.commandFileId); renderCommandFileShelf(); });
  });
}
function bindCommandFileShelf() {
  state.commandFiles = state.commandFiles || [];
  renderCommandFileShelf();
  const zone = $('#command-file-drop-zone');
  const input = $('#command-file-input-shelf');
  $('#command-file-select')?.addEventListener('click', () => input?.click());
  input?.addEventListener('change', (event) => { addCommandFiles(event.target.files); event.target.value = ''; });
  zone?.addEventListener('dragover', (event) => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); zone.classList.add('drag-active'); } });
  zone?.addEventListener('dragleave', () => zone.classList.remove('drag-active'));
  zone?.addEventListener('drop', (event) => { event.preventDefault(); zone.classList.remove('drag-active'); addCommandFiles(event.dataTransfer.files); });
  document.addEventListener('paste', (event) => { if ($('#commands-view')?.classList.contains('hidden')) return; const files = Array.from(event.clipboardData?.files || []); if (files.length) { event.preventDefault(); addCommandFiles(files); } });
}
function renderCommandServers() {
  const list = $('#server-card-list');
  if (!list) return;
  if (!state.commandServers.length) {
    list.innerHTML = '<div class="server-library-empty">暂无服务器，点击右上角新增后即可复用。</div>';
    return;
  }
  list.innerHTML = state.commandServers.map((server) => {
    const connectionStatus = ['success', 'failed'].includes(server.connectionStatus) ? server.connectionStatus : 'untested';
    return `
    <article class="reusable-server-card connection-${connectionStatus}" draggable="${connectionStatus === 'success'}" data-server-id="${escapeHtml(server.id)}" title="${escapeHtml(server.connectionError || '')}">
      <div class="reusable-server-heading"><span class="server-drag-handle">⁙</span><input data-server-field="name" value="${escapeHtml(server.name || '')}" placeholder="服务器名称" /><button type="button" class="server-test-button" data-test-server="${escapeHtml(server.id)}">${connectionStatus === 'success' ? '已连接' : connectionStatus === 'failed' ? '重试' : '测试连接'}</button><button type="button" class="server-remove-button" aria-label="删除服务器">×</button></div>
      <div class="reusable-server-fields">
        <label>地址<input data-server-field="host" value="${escapeHtml(server.host || '')}" placeholder="192.168.1.20" /></label>
        <label>端口<input data-server-field="port" type="number" min="1" max="65535" value="${escapeHtml(server.port || '22')}" /></label>
        <label>用户名<input data-server-field="user" value="${escapeHtml(server.user || '')}" placeholder="deploy" /></label>
        <label>认证<select data-server-field="auth"><option value="key" ${server.auth !== 'password' ? 'selected' : ''}>SSH Key</option><option value="password" ${server.auth === 'password' ? 'selected' : ''}>密码</option></select></label>
      </div>
      ${server.auth === 'password' ? `<input class="reusable-server-secret" data-server-field="secret" type="password" value="${escapeHtml(server.secret || '')}" placeholder="登录密码" />` : `<label class="server-key-picker"><span>选择密钥文件</span><small>${escapeHtml(server.keyName || '未选择文件')}</small><input data-server-key-file type="file" accept=".key,.pem,.ppk,application/x-pem-file" hidden /></label>`}
    </article>`;
  }).join('');
  list.querySelectorAll('.reusable-server-card').forEach((card) => {
    card.addEventListener('dragstart', (event) => { const server = commandServer(card.dataset.serverId); if (server?.connectionStatus !== 'success') { event.preventDefault(); showToast('请先测试连接，只有连接成功的服务器可以拖动'); return; } const preview = document.createElement('div'); preview.className = 'server-drag-preview'; preview.innerHTML = `<span>⁙</span><strong>${escapeHtml(server.name || server.host || '未命名服务器')}</strong>`; document.body.appendChild(preview); event.dataTransfer.setDragImage(preview, 18, 17); setTimeout(() => preview.remove(), 0); card.classList.add('dragging'); event.dataTransfer.setData('text/server-id', card.dataset.serverId); event.dataTransfer.effectAllowed = 'copy'; });
    card.addEventListener('dragend', () => card.classList.remove('dragging'));
    card.querySelectorAll('[data-server-field]').forEach((input) => {
      const updateServer = () => { const server = commandServer(card.dataset.serverId); const field = input.dataset.serverField; server[field] = input.value; server.connectionStatus = 'untested'; server.connectionError = ''; if (field === 'host' || field === 'port') server.hostFingerprint = ''; card.classList.remove('connection-success', 'connection-failed'); card.classList.add('connection-untested'); card.draggable = false; const testButton = card.querySelector('.server-test-button'); if (testButton) testButton.textContent = '测试连接'; saveCommandFlows(); renderCommandFlows(); renderCommandEditor(); if (field === 'auth' && input.matches('select')) renderCommandServers(); };
      input.addEventListener('input', updateServer);
      input.addEventListener('change', updateServer);
      input.addEventListener('pointerdown', (event) => event.stopPropagation());
    });
    card.querySelector('[data-server-key-file]')?.addEventListener('change', async (event) => { const file = event.target.files?.[0]; if (!file) return; const server = commandServer(card.dataset.serverId); try { state.commandServerKeys[server.id] = await file.text(); server.keyName = file.name; server.connectionStatus = 'untested'; server.connectionError = ''; saveCommandFlows(); renderCommandServers(); renderCommandFlows(); showToast(`已选择密钥：${file.name}`); } catch (error) { showToast(`读取密钥失败：${error.message}`); } });
    card.querySelector('.server-test-button').addEventListener('click', () => testCommandServerConnection(card.dataset.serverId));
    card.querySelector('.server-remove-button').addEventListener('click', () => {
      const id = card.dataset.serverId;
      state.commandServers = state.commandServers.filter((server) => server.id !== id);
      state.commandFlows.forEach((flow) => { if (flow.serverId === id) flow.serverId = ''; });
      saveCommandFlows(); renderCommandServers(); renderCommandFlows(); renderCommandEditor();
    });
  });
}

async function testCommandServerConnection(serverId) {
  const server = commandServer(serverId);
  const card = document.querySelector(`.reusable-server-card[data-server-id="${CSS.escape(serverId)}"]`);
  const button = card?.querySelector('.server-test-button');
  if (!server || !button) return;
  button.disabled = true; button.textContent = '测试中…';
  try {
    const requestConnection = () => fetch('/api/commands/test', { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json' }, body: JSON.stringify({ host: server.host || '', port: server.port || '22', user: server.user || '', auth: server.auth || 'key', secret: server.auth === 'password' ? (server.secret || '') : (state.commandServerKeys?.[server.id] || ''), fingerprint: server.hostFingerprint || '' }) });
    let response = await requestConnection();
    let payload = await response.json().catch(() => ({}));
    if (response.status === 428 && payload.fingerprint) {
      if (!window.confirm(`首次连接，请核对服务器指纹：\n\n${payload.fingerprint}\n\n确认信任此服务器吗？`)) { server.connectionStatus = 'untested'; server.connectionError = ''; return; }
      server.hostFingerprint = payload.fingerprint;
      response = await requestConnection();
      payload = await response.json().catch(() => ({}));
    }
    if (!response.ok) throw new Error(payload.error || '连接失败');
    server.connectionStatus = 'success'; server.connectionError = '';
    showToast(`连接成功：${server.name || server.host}`);
  } catch (error) {
    server.connectionStatus = 'failed'; server.connectionError = error.message || '连接失败';
    showToast(server.connectionError);
  } finally {
    saveCommandFlows(); renderCommandServers(); renderCommandFlows(); renderCommandEditor();
  }
}

function renderCommandFlows() {
  const list = $('#commands-flow-list');
  if (!list) return;
  list.innerHTML = state.commandFlows.map((flow) => {
    const server = commandServer(flow.serverId);
    const done = flow.lines.length && flow.lines.every((line) => line.status === 'success');
    return `<div class="command-flow-card ${flow.id === state.activeCommandFlowId ? 'active' : ''}" draggable="true" data-flow-id="${escapeHtml(flow.id)}"><span class="command-flow-handle">⁙</span><div class="command-flow-main"><strong>${escapeHtml(flow.name || '未命名指令集')}</strong><span>${flow.lines.length} 行指令</span></div><span class="flow-direction-arrow">→</span><button class="flow-bind-slot ${server ? 'bound' : ''}" type="button" data-bind-flow="${escapeHtml(flow.id)}"><i>⌘</i><span>${escapeHtml(server ? (server.name || server.host) : '拖入服务器')}</span></button><i class="command-flow-status ${done ? 'ok' : ''}"></i></div>`;
  }).join('');
  $('#commands-flow-count').textContent = `${state.commandFlows.length} 组`;
  list.querySelectorAll('.command-flow-card').forEach((card) => {
    card.addEventListener('click', (event) => { if (event.target.closest('.flow-bind-slot')) return; state.activeCommandFlowId = card.dataset.flowId; renderCommandFlows(); renderCommandEditor(); });
    card.addEventListener('dragstart', (event) => { if (event.target.closest('.flow-bind-slot')) return; card.classList.add('dragging'); event.dataTransfer.setData('text/flow-id', card.dataset.flowId); });
    card.addEventListener('dragend', () => card.classList.remove('dragging'));
    card.addEventListener('dragover', (event) => { if (event.dataTransfer.types.includes('text/flow-id')) { event.preventDefault(); card.classList.add('drag-over'); } });
    card.addEventListener('dragleave', () => card.classList.remove('drag-over'));
    card.addEventListener('drop', (event) => { const movedId = event.dataTransfer.getData('text/flow-id'); if (!movedId) return; event.preventDefault(); card.classList.remove('drag-over'); const from = state.commandFlows.findIndex((flow) => flow.id === movedId); const to = state.commandFlows.findIndex((flow) => flow.id === card.dataset.flowId); if (from >= 0 && to >= 0 && from !== to) { const [moved] = state.commandFlows.splice(from, 1); state.commandFlows.splice(to, 0, moved); saveCommandFlows(); renderCommandFlows(); } });
  });
  list.querySelectorAll('.flow-bind-slot').forEach((slot) => {
    slot.addEventListener('dragover', (event) => { if (event.dataTransfer.types.includes('text/server-id')) { event.preventDefault(); event.stopPropagation(); slot.classList.add('drop-ready'); } });
    slot.addEventListener('dragleave', () => slot.classList.remove('drop-ready'));
    slot.addEventListener('drop', (event) => { const serverId = event.dataTransfer.getData('text/server-id'); if (!serverId) return; event.preventDefault(); event.stopPropagation(); slot.classList.remove('drop-ready'); bindServerToFlow(slot.dataset.bindFlow, serverId); });
    slot.addEventListener('click', () => { const flow = state.commandFlows.find((item) => item.id === slot.dataset.bindFlow); if (!state.commandServers.length) return showToast('请先在上方新增服务器'); const current = Math.max(-1, state.commandServers.findIndex((server) => server.id === flow.serverId)); bindServerToFlow(flow.id, state.commandServers[(current + 1) % state.commandServers.length].id); });
  });
}

function commandLineMarkup(text) {
  const source = String(text || '');
  let html = '';
  let cursor = 0;
  const pattern = /\[\[file:([^\]]+)\]\]/g;
  source.replace(pattern, (match, id, offset) => {
    html += escapeHtml(source.slice(cursor, offset));
    const file = (state.commandFiles || []).find((item) => item.id === id);
    html += `<span class="command-file-token" contenteditable="false" draggable="true" data-file-id="${escapeHtml(id)}">${escapeHtml(file?.name || id)}</span>`;
    cursor = offset + match.length;
    return match;
  });
  return html + escapeHtml(source.slice(cursor));
}
function serializeCommandLine(element) {
  let value = '';
  element.childNodes.forEach((node) => {
    if (node.nodeType === Node.TEXT_NODE) value += node.nodeValue;
    else if (node.nodeType === Node.ELEMENT_NODE && node.matches('.command-file-token')) value += `[[file:${node.dataset.fileId}]]`;
    else value += serializeCommandLine(node);
  });
  return value;
}

function defaultCommandUploadDestination(flow) { const server = commandServer(flow?.serverId); if (!server?.user) return ''; return server.user === 'root' ? '/root' : `/home/${server.user}`; }
function commandUploadLineMarkup(line, index) { const file = (state.commandFiles || []).find((item) => item.id === line.fileId); const fileName = file?.name || line.fileName || ''; const progress = Math.max(0, Math.min(100, line.uploadProgress || 0)); const status = line.status === 'running' ? `${progress}%` : line.status === 'success' ? '上传完成' : line.status === 'error' ? '上传失败' : '等待上传'; return `<div class="command-line command-upload-line" data-index="${index}" data-status="${line.status || 'idle'}"><span class="command-line-index">${String(index + 1).padStart(2, '0')}</span><div class="command-upload-fields"><div class="command-upload-file-box">${file?.previewURL ? `<img src="${escapeHtml(file.previewURL)}" alt="" />` : '<span>FILE</span>'}<strong title="${escapeHtml(fileName)}">${escapeHtml(file ? fileName : fileName ? `${fileName}（需重新拖入）` : '文件已移除')}</strong></div><label class="command-upload-destination"><span>服务器位置</span><input value="${escapeHtml(line.destination || '')}" placeholder="例如：/home/opc" /></label><div class="command-line-upload-progress"><i style="width:${progress}%"></i><span>${status}</span></div></div><button class="command-line-remove" type="button" aria-label="删除第 ${index + 1} 行">×</button></div>`; }
function renderCommandEditor() {
  const flow = activeCommandFlow();
  if (!flow) return;
  $('#command-flow-name').value = flow.name || '';
  $('#commands-editor-title').textContent = flow.name || '运行指令';
  const lines = $('#command-lines');
  lines.innerHTML = flow.lines.map((line, index) => line.type === 'upload' ? commandUploadLineMarkup(line, index) : `<div class="command-line" data-index="${index}" data-status="${line.status || 'idle'}"><span class="command-line-index">${String(index + 1).padStart(2, '0')}</span><div class="command-editable" contenteditable="true" spellcheck="false" data-placeholder="输入服务器指令">${commandLineMarkup(line.text)}</div><button class="command-line-remove" type="button" aria-label="删除第 ${index + 1} 行">×</button></div>`).join('');
  lines.querySelectorAll('.command-editable').forEach((input) => {
    input.addEventListener('input', () => { const line = flow.lines[Number(input.closest('.command-line').dataset.index)]; line.text = serializeCommandLine(input); line.status = 'idle'; delete line.error; saveCommandFlows(); renderCommandFlows(); });
    input.addEventListener('dragover', (event) => { if (event.dataTransfer.types.includes('text/command-file-id')) { event.preventDefault(); input.classList.add('file-drop-target'); } });
    input.addEventListener('dragleave', () => input.classList.remove('file-drop-target'));
    input.addEventListener('drop', (event) => { const fileId = event.dataTransfer.getData('text/command-file-id'); if (!fileId) return; event.preventDefault(); const line = flow.lines[Number(input.closest('.command-line').dataset.index)]; line.type = 'upload'; line.fileId = fileId; line.destination = defaultCommandUploadDestination(flow); line.text = ''; line.status = 'idle'; line.uploadProgress = 0; delete line.error; saveCommandFlows(); renderCommandEditor(); renderCommandFlows(); });
    input.addEventListener('keydown', (event) => { if (event.key === 'Enter') { event.preventDefault(); document.execCommand('insertText', false, '\n'); } });
  });
  lines.querySelectorAll('.command-upload-destination input').forEach((input) => input.addEventListener('input', () => { const line = flow.lines[Number(input.closest('.command-line').dataset.index)]; line.destination = input.value; line.status = 'idle'; line.uploadProgress = 0; delete line.error; saveCommandFlows(); renderCommandFlows(); }));
  lines.querySelectorAll('.command-line-remove').forEach((button) => button.addEventListener('click', () => { flow.lines.splice(Number(button.closest('.command-line').dataset.index), 1); if (!flow.lines.length) flow.lines.push({ text: '', status: 'idle' }); saveCommandFlows(); renderCommandEditor(); renderCommandFlows(); }));
  const firstError = flow.lines.findIndex((line) => line.status === 'error');
  $('#command-progress').textContent = firstError >= 0 ? `第 ${firstError + 1} 行失败，可修改后继续` : `${flow.lines.filter((line) => line.status === 'success').length} / ${flow.lines.length} 行已完成`;
  $('#command-error').classList.toggle('hidden', firstError < 0);
  if (firstError >= 0) $('#command-error-text').textContent = flow.lines[firstError].error || '服务器返回了错误';
}
function updateCommandUploadProgress(line, item, index, progress, status = 'uploading') { line.uploadProgress = progress; item.uploadProgress = progress; item.uploadStatus = status; const commandLine = document.querySelector(`.command-line[data-index="${index}"]`); if (commandLine) { const bar = commandLine.querySelector('.command-line-upload-progress i'); const label = commandLine.querySelector('.command-line-upload-progress span'); if (bar) bar.style.width = `${progress}%`; if (label) label.textContent = status === 'success' ? '上传完成' : status === 'error' ? '上传失败' : `${progress}%`; } const card = document.querySelector(`[data-command-file-id="${CSS.escape(item.id)}"]`); if (card) { card.classList.remove('upload-idle', 'upload-uploading', 'upload-success', 'upload-error'); card.classList.add(`upload-${status}`); const bar = card.querySelector('.command-file-progress i'); const label = card.querySelector('.command-file-progress-label'); if (bar) bar.style.width = `${progress}%`; if (label) label.textContent = status === 'success' ? '已上传' : status === 'error' ? '上传失败' : `${progress}%`; } }
async function uploadCommandFile(server, line, item, index) { const body = new FormData(); body.append('host', server.host || ''); body.append('port', server.port || '22'); body.append('user', server.user || ''); body.append('auth', server.auth || 'key'); body.append('secret', server.auth === 'password' ? (server.secret || '') : (state.commandServerKeys?.[server.id] || '')); body.append('fingerprint', server.hostFingerprint || ''); body.append('destination', line.destination || defaultCommandUploadDestination({ serverId: server.id })); body.append('file', item.file, item.name); return new Promise((resolve, reject) => { const xhr = new XMLHttpRequest(); updateCommandUploadProgress(line, item, index, 0); xhr.open('POST', '/api/commands/upload'); xhr.upload.addEventListener('progress', (event) => { if (event.lengthComputable) updateCommandUploadProgress(line, item, index, Math.min(99, Math.round((event.loaded / event.total) * 100))); }); xhr.addEventListener('load', () => { let payload = {}; try { payload = JSON.parse(xhr.responseText || '{}'); } catch {} if (xhr.status >= 200 && xhr.status < 300) { updateCommandUploadProgress(line, item, index, 100, 'success'); resolve(payload); } else { updateCommandUploadProgress(line, item, index, line.uploadProgress || 0, 'error'); reject(new Error(payload.error || `文件上传失败：${item.name}`)); } }); xhr.addEventListener('error', () => { updateCommandUploadProgress(line, item, index, line.uploadProgress || 0, 'error'); reject(new Error(`文件上传失败：${item.name}`)); }); xhr.send(body); }); }
async function uploadCommandFiles(server, line, index) {
  const ids = line.type === 'upload' ? [line.fileId] : [...String(line.text || '').matchAll(/\[\[file:([^\]]+)\]\]/g)].map((match) => match[1]);
  const files = ids.map((id) => (state.commandFiles || []).find((item) => item.id === id)).filter((item) => item?.file);
  if (line.type === 'upload' && !files.length) throw new Error('上传文件已被移除，请重新拖入文件');
  if (!files.length || !goServerConnected) return 0;
  for (const item of files) {
    await uploadCommandFile(server, line, item, index);
  }
  return files.length;
}
function commandTextForExecution(text) { return String(text || '').replace(/\[\[file:([^\]]+)\]\]/g, (_, id) => (state.commandFiles || []).find((item) => item.id === id)?.name || id); }
async function executeCommandLine(flow, line, index) {
  const server = commandServer(flow.serverId);
  if (line.type === 'upload') line.uploadProgress = 0;
  line.status = 'running'; renderCommandEditor();
  $('#commands-run-status').textContent = `正在运行第 ${index + 1} 行`;
  await new Promise((resolve) => setTimeout(resolve, 380));
  try {
    if (!server) throw new Error('当前指令集尚未绑定服务器');
    const uploadedFiles = await uploadCommandFiles(server, line, index);
    const executableText = commandTextForExecution(line.text);
    if (line.type === 'upload' || (uploadedFiles && /^\s*scp\s+/i.test(executableText))) { line.status = 'success'; line.error = ''; return true; }
    if (goServerConnected) {
      const response = await fetch('/api/commands/execute', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ host: server.host || '', port: server.port || '22', user: server.user || '', auth: server.auth || 'key', secret: server.auth === 'password' ? (server.secret || '') : (state.commandServerKeys?.[server.id] || ''), fingerprint: server.hostFingerprint || '', command: executableText }) });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error([payload.error || '服务器执行失败', payload.output].filter(Boolean).join('\n'));
    } else if (/fail|error|错误/i.test(executableText)) throw new Error('演示执行失败：检测到 fail/error 关键字');
    line.status = 'success'; line.error = ''; return true;
  } catch (error) { line.status = 'error'; line.error = error.message; return false; }
}

async function runCommandFlow(startAt = 0, flow = activeCommandFlow()) {
  if (!flow || state.commandRunning) return false;
  state.commandRunning = true; $('#commands-run-all').disabled = true; $('#commands-run-current').disabled = true;
  const firstError = flow.lines.findIndex((line) => line.status === 'error');
  const firstPending = flow.lines.findIndex((line) => line.status !== 'success');
  const begin = startAt > 0 ? startAt : (firstError >= 0 ? firstError : Math.max(0, firstPending));
  let failed = -1;
  for (let i = begin; i < flow.lines.length; i += 1) { if (flow.lines[i].type !== 'upload' && !String(flow.lines[i].text || '').trim()) continue; const ok = await executeCommandLine(flow, flow.lines[i], i); saveCommandFlows(); renderCommandFlows(); if (!ok) { failed = i; break; } }
  state.commandRunning = false; $('#commands-run-all').disabled = false; $('#commands-run-current').disabled = false; renderCommandEditor();
  $('#commands-run-status').textContent = failed >= 0 ? `第 ${failed + 1} 行执行失败` : '当前指令集执行完成';
  if (failed >= 0) showToast('指令执行失败，可修改后从错误处继续');
  return failed < 0;
}

async function runAllCommandFlows() {
  if (state.commandRunning) return;
  for (const flow of state.commandFlows) {
    state.activeCommandFlowId = flow.id; renderCommandFlows(); renderCommandEditor();
    const ok = await runCommandFlow(0, flow); if (!ok) return;
  }
  $('#commands-run-status').textContent = '全部指令集执行完成'; showToast('全部指令集已按顺序执行完成');
}

function bindCommandEvents() {
  loadCommandFlows(); bindCommandFileShelf(); renderCommandServers(); renderCommandFlows(); renderCommandEditor();
  $('#command-flow-name')?.addEventListener('input', (event) => { const flow = activeCommandFlow(); flow.name = event.target.value; saveCommandFlows(); $('#commands-editor-title').textContent = flow.name || '运行指令'; renderCommandFlows(); });
  $('#commands-add-server')?.addEventListener('click', () => { state.commandServers.push({ id: `server-${Date.now()}`, name: `服务器 ${state.commandServers.length + 1}`, host: '', port: '22', user: '', auth: 'key', secret: '', connectionStatus: 'untested', connectionError: '' }); saveCommandFlows(); renderCommandServers(); });
  $('#commands-add-flow')?.addEventListener('click', () => { const flow = { id: `flow-${Date.now()}`, name: `新指令集 ${state.commandFlows.length + 1}`, serverId: '', lines: [{ text: '', status: 'idle' }] }; state.commandFlows.push(flow); state.activeCommandFlowId = flow.id; saveCommandFlows(); renderCommandFlows(); renderCommandEditor(); });
  $('#commands-duplicate')?.addEventListener('click', () => { const source = activeCommandFlow(); const copy = JSON.parse(JSON.stringify(source)); copy.id = `flow-${Date.now()}`; copy.name = `${source.name || '指令集'} 副本`; copy.lines.forEach((line) => { line.status = 'idle'; delete line.error; }); state.commandFlows.push(copy); state.activeCommandFlowId = copy.id; saveCommandFlows(); renderCommandFlows(); renderCommandEditor(); showToast('指令集已复制，服务器绑定已保留'); });
  $('#commands-delete')?.addEventListener('click', () => { if (state.commandFlows.length <= 1) return showToast('至少保留一组指令集'); const index = state.commandFlows.findIndex((flow) => flow.id === state.activeCommandFlowId); state.commandFlows.splice(index, 1); state.activeCommandFlowId = state.commandFlows[Math.max(0, index - 1)].id; saveCommandFlows(); renderCommandFlows(); renderCommandEditor(); });
  $('#commands-favorite')?.addEventListener('click', openCommandFavorites);
  $('#command-favorite-form')?.addEventListener('submit', saveCommandFavorite);
  $$('[data-close-command-favorites]').forEach((button) => button.addEventListener('click', closeCommandFavorites));
  bindBackdropDismissal($('#command-favorites-modal'), closeCommandFavorites);
  $('#command-favorites-list')?.addEventListener('click', (event) => {
    const restoreButton = event.target.closest('[data-restore-command-favorite]');
    const deleteButton = event.target.closest('[data-delete-command-favorite]');
    if (restoreButton) restoreCommandFavorite(restoreButton.dataset.restoreCommandFavorite);
    if (deleteButton) deleteCommandFavorite(deleteButton.dataset.deleteCommandFavorite);
  });
  $('#commands-add-line')?.addEventListener('click', () => { activeCommandFlow().lines.push({ text: '', status: 'idle' }); saveCommandFlows(); renderCommandEditor(); });
  $('#commands-run-all')?.addEventListener('click', runAllCommandFlows);
  $('#commands-run-current')?.addEventListener('click', () => { const flow = activeCommandFlow(); const index = flow.lines.findIndex((line) => line.status !== 'success'); runCommandFlow(index >= 0 ? index : 0, flow); });
  $('#commands-import-button')?.addEventListener('click', () => $('#commands-file-input').click());
  $('#commands-file-input')?.addEventListener('change', async (event) => { const file = event.target.files?.[0]; if (!file) return; const flow = activeCommandFlow(); const text = await file.text(); flow.lines = text.split(/\r?\n/).filter((line) => line.trim()).map((line) => ({ text: line, status: 'idle' })); if (!flow.lines.length) flow.lines = [{ text: '', status: 'idle' }]; saveCommandFlows(); renderCommandEditor(); renderCommandFlows(); showToast(`已导入 ${flow.lines.length} 行指令`); event.target.value = ''; });
}
function setView(view) {
  const labels = { overview: '实时日志流', rules: '加工规则', connections: '连接管理', commands: '服务器指令' };
  $$('.nav-item').forEach((button) => button.classList.toggle('active', button.dataset.view === view));
  $('#view-breadcrumb').textContent = labels[view];
  Object.entries({ overview: '#overview-view', rules: '#rules-view', connections: '#connections-view', commands: '#commands-view' }).forEach(([name, selector]) => {
    $(selector).classList.toggle('hidden', name !== view);
  });
  if (view === 'rules') syncRuleButtons();
  if (view === 'connections') renderConnectionsView();
  if (view === 'rules') showToast('规则中心已展开，当前规则可在右侧直接编辑');
  if (view === 'connections') showToast('连接管理已就绪，可从左侧添加 Dozzle 节点');
  if (view === 'overview') showToast('已回到实时日志流');
}

function bindEvents() {
  document.addEventListener('selectionchange', () => {
    const isSelectingLogText = hasLogTextSelection();
    if (logTextSelectionActive && !isSelectingLogText) scheduleLogRender();
    logTextSelectionActive = isSelectingLogText;
  });
  $$('.nav-item').forEach((button) => button.addEventListener('click', () => setView(button.dataset.view)));
  $('#time-range-filter').addEventListener('change', async (event) => {
    const requestVersion = ++historyRequestVersion;
    if (historySyncTimer) {
      clearTimeout(historySyncTimer);
      historySyncTimer = null;
    }
    const nextRange = event.target.value;
    state.range = rangeLabels[nextRange] ? nextRange : '30m';
    clearFullRangeSearch();
    // Keep the collected logs and selected container caches. Narrowing a
    // range is only a view-level slice; widening it asks the backend for the
    // older interval that is not cached yet.
    resetLogPagination();
    renderLogs();
    if (!goServerConnected) return;
    $('#stream-status').textContent = '正在加载';
    try {
      const rangeQuery = new URLSearchParams({ range: state.range });
      state.selectedNodes.forEach((nodeId) => rangeQuery.append('node', nodeId));
      // When a container is selected, request its history directly. Asking for
      // every container on the node can fill the shared cache before the
      // selected container's older pages are reached.
      state.selectedContainers.forEach((scope) => rangeQuery.append('container', scope));
      const response = await fetch(`/api/logs/range?${rangeQuery.toString()}`, { method: 'POST' });
      if (!response.ok) throw new Error('时间范围加载失败');
      const rangePayload = await response.json().catch(() => ({}));
      if (requestVersion !== historyRequestVersion) return;
      const strictReload = rangePayload.status === 'reloading';
      if (strictReload) {
        state.logs = [];
        state.containerLogCache = {};
        state.historyLoading = true;
        resetLogPagination();
        renderLogs();
      }
      await syncGoBackend({ incremental: !strictReload });
      if (requestVersion !== historyRequestVersion) return;
      if (state.historyLoading) scheduleHistorySync();
      showToast(state.historyLoading
        ? `${strictReload ? '正在重新加载' : '正在增量加载'}${rangeLabels[state.range]}日志`
        : `已切换至${rangeLabels[state.range]}`);
    } catch (error) {
      showToast(error.message);
    }
  });
  $$('.level-chip').forEach((button) => button.addEventListener('click', () => {
    state.level = button.dataset.level;
    resetLogPagination();
    $$('.level-chip').forEach((item) => item.classList.toggle('active', item === button));
    renderLogs();
  }));
  const logSearch = $('#log-search');
  logSearch.addEventListener('input', (event) => {
    state.query = event.target.value;
    clearFullRangeSearch();
    resetLogPagination();
    renderLogs();
    scheduleFullRangeSearch();
  });
  logSearch.addEventListener('paste', (event) => {
    const text = event.clipboardData?.getData('text/plain');
    if (typeof text !== 'string') return;
    event.preventDefault();
    const input = event.currentTarget;
    const start = input.selectionStart ?? input.value.length;
    const end = input.selectionEnd ?? start;
    input.setRangeText(text, start, end, 'end');
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
  $('#global-search').addEventListener('input', (event) => { state.globalQuery = event.target.value; resetLogPagination(); renderLogs(); });
  $('#log-stream').addEventListener('click', (event) => {
    const button = event.target.closest('[data-analyze-log]');
    if (button) {
      event.preventDefault();
      event.stopPropagation();
      analyzeLogFromButton(button);
      return;
    }
    const row = event.target.closest('.log-row');
    if (!row || hasLogTextSelection()) return;
    const logId = Number(row.dataset.logId);
    state.selectedLog = state.selectedLog === logId ? 0 : logId;
    renderLogs();
    updatePreview();
  });
  $('#log-stream').addEventListener('scroll', () => {
    maybeLoadOlderLogs();
    scheduleLogWindowRender();
  });
  $('#pause-button').addEventListener('click', async () => {
    state.paused = !state.paused;
    $('#pause-button').classList.toggle('paused', state.paused);
    $('#pause-button').innerHTML = state.paused ? '<span>▶</span> 继续接收' : '<span>Ⅱ</span> 暂停接收';
    $('.stream-indicator').style.background = state.paused ? 'var(--orange)' : 'var(--teal)';
    if (state.paused) {
      if (eventStream) eventStream.close();
      clearPendingStreamBatch();
      if (historySyncTimer) {
        clearTimeout(historySyncTimer);
        historySyncTimer = null;
      }
      if (containerLogRetryTimer) {
        clearTimeout(containerLogRetryTimer);
        containerLogRetryTimer = null;
      }
      $('#sync-status').textContent = '接收已暂停';
      $('#stream-status').textContent = '已暂停接收';
      $('#metric-processed-foot').textContent = '已暂停，当前视图不会接收新日志';
      updateSyncFooter('已暂停接收', 'paused');
      showToast('日志接收已暂停');
      return;
    }
    $('#stream-status').textContent = '正在恢复';
    showToast('正在恢复日志接收');
    const connected = await syncGoBackend({ incremental: true });
    if (!state.paused && connected && state.historyLoading) scheduleHistorySync();
  });
  $('#clear-button').addEventListener('click', () => { state.query = ''; $('#log-search').value = ''; resetLogPagination(); renderLogs(); showToast('已清空当前过滤条件'); });
  $('#storage-clear-cache')?.addEventListener('click', clearLogCache);
  bindAssistantEvents();
  $('#settings-button').addEventListener('click', openAppSettings);
  $('#app-settings-form').addEventListener('submit', saveAppSettings);
  $('#settings-admin-save').addEventListener('click', saveAdminSettings);
  $('#settings-config-reveal')?.addEventListener('click', revealConfigDirectory);
  $('#settings-config-export')?.addEventListener('click', exportConfiguration);
  $('#settings-config-file')?.addEventListener('change', (event) => importConfiguration(event.target.files?.[0]));
  $$('[data-close-app-settings]').forEach((button) => button.addEventListener('click', closeAppSettings));
  bindBackdropDismissal($('#app-settings-modal'), closeAppSettings);
  $('#ai-profile-form').addEventListener('submit', saveAIProfile);
  $('#new-ai-profile')?.addEventListener('click', () => openAISettings());
  $('#delete-ai-profile').addEventListener('click', () => deleteAIProfile(state.settingsAIProfileId));
  $('#ai-add-model-button').addEventListener('click', addAIModel);
  $$('[data-close-ai-modal]').forEach((button) => button.addEventListener('click', closeAISettings));
  bindBackdropDismissal($('#ai-settings-modal'), closeAISettings);
  $('#ai-admin-token-form').addEventListener('submit', (event) => { event.preventDefault(); closeAIAdminTokenPrompt($('#ai-admin-token-input').value); });
  $$('[data-close-ai-token]').forEach((button) => button.addEventListener('click', () => closeAIAdminTokenPrompt()));
  bindBackdropDismissal($('#ai-admin-token-modal'), () => closeAIAdminTokenPrompt());
  $('#refresh-button').addEventListener('click', () => { $('#refresh-button').style.transform = 'rotate(360deg)'; setTimeout(() => $('#refresh-button').style.transform = '', 350); showToast('节点状态已刷新'); });
  $('#export-button').addEventListener('click', exportLogs);
  const pipelineMoreButton = $('#pipeline-more-button');
  if (pipelineMoreButton) pipelineMoreButton.addEventListener('click', (event) => {
    event.stopPropagation();
    const menu = $('#pipeline-menu');
    const isOpen = !menu.classList.contains('hidden');
    closePipelineMenu();
    if (!isOpen) {
      menu.classList.remove('hidden');
      pipelineMoreButton.setAttribute('aria-expanded', 'true');
    }
  });
  $$('[data-pipeline-action]').forEach((button) => button.addEventListener('click', async (event) => {
    const action = event.currentTarget.dataset.pipelineAction;
    closePipelineMenu();
    if (action === 'enable-all') await persistRuleGroup({ mask: true, structure: true, noise: true }, '全部规则已启用');
    if (action === 'disable-all') await persistRuleGroup({ mask: false, structure: false, noise: false }, '全部规则已停用');
    if (action === 'reset') await persistRuleGroup({ mask: true, structure: true, noise: false }, '规则已恢复默认设置');
  }));
  // Resolve outside-click ancestry during the capture phase. Several click
  // handlers re-render the DOM (for example selecting an assistant session
  // rebuilds the session list), which detaches event.target. A detached node
  // has no ancestors, so calling closest() later would wrongly report an
  // outside click and collapse the assistant dock.
  document.addEventListener('click', (event) => {
    const target = event.target;
    const inside = (selector) => Boolean(target && target.closest && target.closest(selector));
    const outside = {
      pipeline: !inside('#pipeline-actions'),
      modelPicker: !inside('#assistant-model-picker'),
      sessionMenu: !inside('#assistant-session-menu') && !inside('#assistant-session-toggle'),
      assistantDock: !inside('#assistant-dock') && !inside('#assistant-attachment-preview-modal'),
    };
    // Apply on the bubble phase so inner handlers can still suppress the
    // default behavior first, but always using the pre-render ancestry.
    queueMicrotask(() => {
      if (outside.pipeline) closePipelineMenu();
      if (outside.modelPicker) closeAIModelMenu();
      if (outside.sessionMenu) closeAssistantSessionMenu();
      if (outside.assistantDock) $('#assistant-dock').classList.remove('open');
    });
  }, true);
  $$('.toggle').forEach((button) => button.addEventListener('click', async () => {
    const rule = button.dataset.ruleToggle || button.closest('.pipeline-item')?.dataset.rule;
    if (!rule) return;
    const previous = Boolean(state.ruleState[rule]);
    const enabled = !previous;
    button.disabled = true;
    state.ruleState[rule] = enabled;
    syncRuleButtons();
    renderLogs(); updatePreview();
    try {
      const response = await fetch('/api/rules', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ rule, enabled })
      });
      if (!response.ok) throw new Error('规则保存失败');
      const payload = await response.json();
      state.ruleState = { ...state.ruleState, ...(payload.rules || {}) };
      syncRuleButtons();
      renderLogs(); updatePreview();
      showToast(state.ruleState[rule] ? '规则已启用' : '规则已停用');
    } catch (error) {
      state.ruleState[rule] = previous;
      syncRuleButtons();
      renderLogs(); updatePreview();
      showToast(error.message || '规则保存失败，已恢复原状态');
    } finally {
      button.disabled = false;
    }
  }));
  bindPipelineDragAndDrop();
  bindCommandEvents();
  if ($('#add-rule-button')) $('#add-rule-button').addEventListener('click', () => showToast('规则模板面板即将开放')); 
  $('#rules-add-rule').addEventListener('click', () => showToast('规则模板面板即将开放'));
  $('#open-add-node').addEventListener('click', () => openNodeModal());
  $('#connections-add-node').addEventListener('click', () => openNodeModal());
  if ($('#edit-node-button')) $('#edit-node-button').addEventListener('click', (event) => {
    const node = getNode(event.currentTarget.dataset.nodeId);
    if (node) openNodeModal(node);
  });
  if ($('#unbind-node-button')) $('#unbind-node-button').addEventListener('click', async (event) => {
    const node = getNode(event.currentTarget.dataset.nodeId);
    if (node) await unbindNode(node);
  });
  $$('[data-close-modal]').forEach((button) => button.addEventListener('click', closeNodeModal));
  bindBackdropDismissal($('#add-node-modal'), closeNodeModal);
  $('#add-node-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = new FormData(event.target);
    const name = String(form.get('name') || '').trim();
    const url = String(form.get('url') || '').trim();
    const style = String(form.get('protocol') || 'HTTP / WebSocket').trim();
    const nodeId = $('#add-node-modal').dataset.nodeId;
    const node = nodeId ? await updateNodeToGo(nodeId, name, url, style) : await persistNodeToGo(name, url, style);
    if (!node) return;
    closeNodeModal();
    showToast(nodeId ? '节点连接信息已保存' : `${name} 已添加，正在建立连接`);
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === ' ' && document.activeElement.tagName !== 'INPUT' && document.activeElement.tagName !== 'TEXTAREA' && !document.activeElement.isContentEditable) { event.preventDefault(); $('#pause-button').click(); }
    if (event.key === 'Escape') {
      if (!$('#ai-admin-token-modal').classList.contains('hidden')) closeAIAdminTokenPrompt();
      else if (!$('#app-settings-modal').classList.contains('hidden')) closeAppSettings();
      else if (!$('#ai-settings-modal').classList.contains('hidden')) closeAISettings();
      else closeNodeModal();
    }
    if (event.key === '/' && document.activeElement.tagName !== 'INPUT' && document.activeElement.tagName !== 'TEXTAREA' && !document.activeElement.isContentEditable) { event.preventDefault(); $('#global-search').focus(); }
  });
}

function exportLogs() {
  const rows = filteredLogs();
  const csv = ['time,level,node,container,message', ...rows.map((log) => [log.time, log.level, log.node, log.container, log.message].map((value) => `"${String(value).replace(/"/g, '""')}"`).join(','))].join('\n');
  const blob = new Blob([`\ufeff${csv}`], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a'); link.href = url; link.download = `dozzle-logs-${new Date().toISOString().slice(0, 10)}.csv`; link.click(); URL.revokeObjectURL(url);
  showToast(`已导出 ${rows.length} 条日志`);
}
