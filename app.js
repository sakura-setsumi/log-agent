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
let appSettingsDatabaseDraft = null;
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

function loadAIProfiles() {
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

async function loadAIProfilesFromDatabase() {
  try {
    const response = await fetch('/api/ai/profiles', { headers: { Accept: 'application/json' } });
    if (!response.ok) return;
    const payload = await response.json();
    const databaseProfiles = Array.isArray(payload.profiles) ? payload.profiles.map(normalizeAIProvider).filter(Boolean) : [];
    if (!databaseProfiles.length) return;
    const localProfiles = state.aiProfiles.filter((profile) => !/^ai-profile-db-\d+$/.test(profile.id));
    state.aiProfiles = [...localProfiles, ...databaseProfiles];
    if (!state.aiProfiles.some((profile) => profile.id === state.activeAIProfileId)) {
      state.activeAIProfileId = databaseProfiles[0]?.id || '';
      state.activeAIModelId = databaseProfiles[0]?.models[0]?.id || '';
    } else if (!activeAIModel()) {
      state.activeAIModelId = activeAIProfile()?.models[0]?.id || '';
    }
    persistAIProfiles();
    renderAIProviderList();
    renderAIModelList();
    renderAssistant();
  } catch (error) {
    // Database-backed models are optional; keep local and environment models usable.
  }
}

function persistAIProfiles() {
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
const assistantMarkdownClasses = new Set(['assistant-markdown-link', 'markdown-code-block', 'markdown-list-copy', 'markdown-list-item', 'markdown-spacer', 'markdown-table', 'markdown-table-wrap']);

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
  let output = '';
  let cursor = 0;
  source.replace(tokenPattern, (match, code, label, href, offset) => {
    output += renderAssistantMarkdownText(source.slice(cursor, offset));
    if (code !== undefined) {
      output += `<code>${escapeHtml(code)}</code>`;
    } else {
      const safeHref = safeAssistantMarkdownURL(href);
      output += safeHref
        ? `<a class="assistant-markdown-link" href="${escapeHtml(safeHref)}" target="_blank" rel="noopener noreferrer">${renderAssistantMarkdownText(label)}</a>`
        : renderAssistantMarkdownText(match);
    }
    cursor = offset + match.length;
    return match;
  });
  return output + renderAssistantMarkdownText(source.slice(cursor));
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
    if (/^###\s+/.test(line)) { rendered.push(`<h4>${renderAssistantMarkdownInline(line.slice(4))}</h4>`); continue; }
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
  const orderedLogs = logsForActiveContainer();
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
  const log = logsForActiveContainer().find((item) => item.id === Number(button.dataset.analyzeLog));
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
  if (!aiAdminToken) aiAdminToken = await requestAIAdminToken();
  if (!aiAdminToken) { showToast('未提供管理员令牌，模型未写入数据库'); return; }
  try {
    const response = await fetch('/api/ai/profiles', { method: 'POST', headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-Log-Agent-Admin-Token': aiAdminToken }, body: JSON.stringify(profile) });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || '模型保存失败');
    profile.id = payload.id || profile.id;
  } catch (error) {
    showToast(error.message || '模型保存失败');
    return;
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
  if (dbID) {
    if (!aiAdminToken) aiAdminToken = await requestAIAdminToken();
    if (!aiAdminToken) { showToast('未提供管理员令牌，模型未删除'); return; }
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

function fillAppSettingsForm(payload, { databaseDraft = null } = {}) {
  const form = $('#app-settings-form');
  if (!form) return;
  const draft = databaseDraft || (!payload.database?.configured ? appSettingsDatabaseDraft : null);
  const environment = payload.environment || 'production';
  if (!Array.from(form.elements.environment.options).some((option) => option.value === environment)) {
    form.elements.environment.add(new Option(environment, environment));
  }
  form.elements.environment.value = environment;
  form.elements.dbEnabled.checked = Boolean(payload.database?.enabled);
  form.elements.dbDsn.value = '';
  form.elements.dbDsn.placeholder = payload.database?.dsnConfigured ? '已配置，留空保持不变' : '例如：user:password@tcp(127.0.0.1:3306)/log_agent';
  form.elements.dbHost.value = payload.database?.host || '';
  form.elements.dbPort.value = payload.database?.port || '';
  form.elements.dbUser.value = payload.database?.user || '';
  form.elements.dbPassword.value = '';
  form.elements.currentAdminToken.value = '';
  form.elements.adminToken.value = '';
  $('#settings-db-status').textContent = payload.database?.configured ? '已连接' : '未配置';
  $('#settings-admin-status').textContent = payload.adminTokenConfigured ? '已配置' : '未配置';
  if (draft) {
    form.elements.dbEnabled.checked = Boolean(draft.enabled);
    form.elements.dbDsn.value = draft.dsn || '';
    form.elements.dbHost.value = draft.host || '';
    form.elements.dbPort.value = draft.port || '';
    form.elements.dbUser.value = draft.user || '';
    form.elements.dbPassword.value = draft.password || '';
    if (!payload.database?.configured && draft.enabled) $('#settings-db-status').textContent = '未保存';
  }
}

async function openAppSettings() {
  const modal = $('#app-settings-modal');
  modal.classList.remove('hidden');
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

function closeAppSettings() {
  $('#app-settings-modal').classList.add('hidden');
  $('#app-settings-form')?.reset();
}

function appSettingsDatabasePayload(form) {
  return {
    enabled: form.elements.dbEnabled.checked,
    dsn: String(form.elements.dbDsn.value || '').trim(),
    host: String(form.elements.dbHost.value || '').trim(),
    port: String(form.elements.dbPort.value || '').trim(),
    user: String(form.elements.dbUser.value || '').trim(),
    password: String(form.elements.dbPassword.value || ''),
  };
}

function rememberDatabaseDraft(form) {
  appSettingsDatabaseDraft = appSettingsDatabasePayload(form);
  return appSettingsDatabaseDraft;
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

async function testDatabaseSettings() {
  const form = $('#app-settings-form');
  if (!ensureSettingsAdminToken(form)) return;
  const button = $('#settings-db-test');
  const { headers } = appSettingsAuthHeaders(form);
  button.disabled = true;
  try {
    const response = await fetch('/api/settings/database/test', { method: 'POST', headers, body: JSON.stringify({ database: appSettingsDatabasePayload(form) }) });
    const payload = await readSettingsResponse(response, '数据库连接测试失败');
    $('#settings-db-status').textContent = '连接成功';
    showToast(payload.message || '数据库连接测试成功');
  } catch (error) {
    $('#settings-db-status').textContent = '连接失败';
    showToast(error.message || '数据库连接测试失败');
  } finally {
    button.disabled = false;
  }
}

async function saveDatabaseSettings() {
  const form = $('#app-settings-form');
  if (!ensureSettingsAdminToken(form)) return;
  const button = $('#settings-db-save');
  const { headers, currentAdminToken } = appSettingsAuthHeaders(form);
  button.disabled = true;
  try {
    const response = await fetch('/api/settings/database', { method: 'PUT', headers, body: JSON.stringify({ database: appSettingsDatabasePayload(form) }) });
    const payload = await readSettingsResponse(response, '数据库保存失败');
    appSettingsDatabaseDraft = null;
    if (currentAdminToken) aiAdminToken = currentAdminToken;
    fillAppSettingsForm(payload);
    $('#settings-db-status').textContent = payload.database?.configured ? '已连接' : '未配置';
    showToast('数据库设置已保存');
    await syncGoBackend({ incremental: true });
  } catch (error) {
    showToast(error.message || '数据库保存失败');
  } finally {
    button.disabled = false;
  }
}

async function saveAdminSettings() {
  const form = $('#app-settings-form');
  if (!ensureSettingsAdminToken(form)) return;
  const button = $('#settings-admin-save');
  const adminToken = String(form.elements.adminToken.value || '').trim();
  const databaseDraft = rememberDatabaseDraft(form);
  const { headers, currentAdminToken } = appSettingsAuthHeaders(form);
  button.disabled = true;
  try {
    const response = await fetch('/api/settings/admin', { method: 'PUT', headers, body: JSON.stringify({ adminToken }) });
    const payload = await readSettingsResponse(response, '管理员 key 保存失败');
    if (adminToken) aiAdminToken = adminToken;
    else if (currentAdminToken) aiAdminToken = currentAdminToken;
    fillAppSettingsForm(payload, { databaseDraft });
    const databasePending = databaseDraft.enabled && (databaseDraft.dsn || databaseDraft.host || databaseDraft.user);
    showToast(databasePending && !payload.database?.configured ? '管理员 key 已保存；数据库信息尚未保存，请点击“保存数据库”' : '管理员 key 已保存');
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
  const databaseDraft = rememberDatabaseDraft(form);
  const submitButton = $('#app-settings-form button[type="submit"]');
  const { headers } = appSettingsAuthHeaders(form);
  submitButton.disabled = true;
  try {
    const response = await fetch('/api/settings/environment', {
      method: 'PUT', headers,
      body: JSON.stringify({ environment: String(form.elements.environment.value || '').trim() })
    });
    const payload = await readSettingsResponse(response, '运行环境保存失败');
    fillAppSettingsForm(payload, { databaseDraft });
    closeAppSettings();
    const databasePending = databaseDraft.enabled && (databaseDraft.dsn || databaseDraft.host || databaseDraft.user);
    showToast(databasePending && !payload.database?.configured ? '运行环境已保存；数据库信息尚未保存' : '运行环境已保存');
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
      const query = new URLSearchParams({ node: target.nodeId, container: target.containerId });
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
  const normalizeSearchText = (value) => String(value ?? '').replace(/\s+/g, ' ').trim().toLowerCase();
  const query = normalizeSearchText(`${state.query} ${state.globalQuery}`);
  const selectedNodeIds = new Set(state.selectedNodes);
  const selectedTargets = selectedContainerTargets();
  const now = Date.now();
  return logsForActiveContainer().filter((log) => {
    const logNodeId = nodeIdByName(log.node);
    const nodeMatch = !state.selectedNodes.length || selectedNodeIds.has(logNodeId);
    const containerMatch = !state.selectedContainers.length || selectedTargets.some((target) => target.nodeId === logNodeId && (log.container === target.name || log.container === target.containerId));
    const levelMatch = state.level === 'all' || log.level === state.level;
    const queryMatch = !query || normalizeSearchText(`${log.node} ${log.container} ${log.message}`).includes(query);
    const noiseMatch = state.ruleState.noise ? !log.message.includes('/healthz') : true;
    return isLogInSelectedRange(log, now) && nodeMatch && containerMatch && levelMatch && queryMatch && noiseMatch;
  }).reverse();
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
  const loadedLabel = state.historyLoading
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
  const log = state.logs.find((item) => item.id === state.selectedLog) || state.logs[0];
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
    nodes.splice(0, nodes.length, ...(payload.nodes || []));
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

async function persistNodeToGo(name, url, style) {
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

function setView(view) {
  const labels = { overview: '实时日志流', rules: '加工规则', connections: '连接管理' };
  $$('.nav-item').forEach((button) => button.classList.toggle('active', button.dataset.view === view));
  $('#view-breadcrumb').textContent = labels[view];
  Object.entries({ overview: '#overview-view', rules: '#rules-view', connections: '#connections-view' }).forEach(([name, selector]) => {
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
  logSearch.addEventListener('input', (event) => { state.query = event.target.value; resetLogPagination(); renderLogs(); });
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
  $('#log-stream').addEventListener('scroll', scheduleLogWindowRender);
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
  bindAssistantEvents();
  $('#settings-button').addEventListener('click', openAppSettings);
  $('#app-settings-form').addEventListener('submit', saveAppSettings);
  $('#settings-db-test').addEventListener('click', testDatabaseSettings);
  $('#settings-db-save').addEventListener('click', saveDatabaseSettings);
  $('#settings-admin-save').addEventListener('click', saveAdminSettings);
  ['dbEnabled', 'dbDsn', 'dbHost', 'dbPort', 'dbUser', 'dbPassword'].forEach((name) => {
    $('#app-settings-form').elements[name].addEventListener('input', () => rememberDatabaseDraft($('#app-settings-form')));
    $('#app-settings-form').elements[name].addEventListener('change', () => rememberDatabaseDraft($('#app-settings-form')));
  });
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
  document.addEventListener('click', (event) => {
    if (!event.target.closest('#pipeline-actions')) closePipelineMenu();
    if (!event.target.closest('#assistant-model-picker')) closeAIModelMenu();
    if (!event.target.closest('#assistant-session-menu') && !event.target.closest('#assistant-session-toggle')) closeAssistantSessionMenu();
    if (!event.target.closest('#assistant-dock') && !event.target.closest('#assistant-attachment-preview-modal')) $('#assistant-dock').classList.remove('open');
  });
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
    if (event.key === ' ' && document.activeElement.tagName !== 'INPUT') { event.preventDefault(); $('#pause-button').click(); }
    if (event.key === 'Escape') {
      if (!$('#ai-admin-token-modal').classList.contains('hidden')) closeAIAdminTokenPrompt();
      else if (!$('#app-settings-modal').classList.contains('hidden')) closeAppSettings();
      else if (!$('#ai-settings-modal').classList.contains('hidden')) closeAISettings();
      else closeNodeModal();
    }
    if (event.key === '/' && document.activeElement.tagName !== 'INPUT') { event.preventDefault(); $('#global-search').focus(); }
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
