const nodes = [];
const logPageSize = 50;
const initialLogPreviewSize = 50;
const logVirtualRowHeight = 74;
const logVirtualOverscan = 60;
let maxBufferedLogs = 100000;
const selectionStorageKey = 'log-agent-selection';
const aiProfilesStorageKey = 'log-agent-ai-profiles';
const aiActiveProfileStorageKey = 'log-agent-ai-active-profile';
const aiActiveModelStorageKey = 'log-agent-ai-active-model';

const state = {
  selectedNodes: [],
  selectedContainers: [],
  expandedNodes: [],
  level: 'all',
  query: '',
  globalQuery: '',
  paused: false,
  selectedLog: 0,
  range: '30m',
  historyLoading: false,
  visibleLogs: logPageSize,
  logs: [],
  processed: 0,
  ruleState: { mask: true, structure: true, noise: false },
  containerLogCache: {},
  assistantContext: [],
  assistantMessages: [],
  assistantBusy: false,
  aiProfiles: [],
  activeAIProfileId: '',
  activeAIModelId: '',
  settingsAIProfileId: '',
  aiEnvConfigured: false,
  aiModel: ''
};

let goServerConnected = false;
let eventStream;
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
let logRenderTimer;
let logScrollFrame;
let lastFilteredLogs = [];
let lastVirtualWindowKey = '';

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => Array.from(document.querySelectorAll(selector));

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
  renderAIModelPicker();
  $('#assistant-context-count').textContent = String(state.assistantContext.length);
  const contextList = $('#assistant-context-list');
  contextList.innerHTML = state.assistantContext.length ? state.assistantContext.map((log) => `
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
    $('#assistant-status').textContent = 'AI 请求中…';
  } else if (currentAIConfigured()) {
    const provider = activeAIProfile();
    const selectedModel = activeAIModel();
    $('#assistant-status').textContent = provider && selectedModel ? `AI 已就绪 · ${provider.name} / ${selectedModel.name}` : `AI 已就绪 · ${state.aiModel || '服务端默认模型'}`;
  } else {
    $('#assistant-status').textContent = 'AI 未配置 · 点击选择模型';
  }
  $('#assistant-send').disabled = state.assistantBusy;
  $('#assistant-input').disabled = state.assistantBusy;
  if (state.assistantMessages.length) messages.scrollTop = messages.scrollHeight;
}

function renderAssistantMarkdown(content) {
  return String(content).split(/\r?\n/).map((line) => {
    const escaped = escapeHtml(line);
    if (!escaped.trim()) return '<div class="markdown-spacer"></div>';
    let rendered = escaped
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/\*([^*]+)\*/g, '<em>$1</em>');
    if (/^###\s+/.test(rendered)) return `<h4>${rendered.slice(4)}</h4>`;
    if (/^##\s+/.test(rendered)) return `<h3>${rendered.slice(3)}</h3>`;
    if (/^#\s+/.test(rendered)) return `<h2>${rendered.slice(2)}</h2>`;
    if (/^[-*]\s+/.test(rendered)) return `<div class="markdown-list-item"><span>•</span><span class="markdown-list-copy">${rendered.slice(2)}</span></div>`;
    if (/^\d+\.\s+/.test(rendered)) return `<div class="markdown-list-item"><span>${rendered.match(/^\d+/)[0]}.</span><span class="markdown-list-copy">${rendered.replace(/^\d+\.\s+/, '')}</span></div>`;
    return `<div>${rendered}</div>`;
  }).join('');
}

function addAssistantContext(log) {
  if (!log) return;
  if (state.assistantContext.some((item) => item.id === log.id)) {
    showToast('这条日志已在 AI 上下文中');
    return;
  }
  state.assistantContext = [log, ...state.assistantContext].slice(0, 20);
  renderAssistant();
  showToast('日志已加入 AI 分析上下文');
}

function removeAssistantContext(id) {
  state.assistantContext = state.assistantContext.filter((log) => String(log.id) !== String(id));
  renderAssistant();
}

function clearAssistantContext() {
  state.assistantContext = [];
  state.assistantMessages = [];
  renderAssistant();
  showToast('已开始新聊天');
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
  const content = input.value.trim();
  if (!content) return;
  const profile = activeAIProfile();
  const model = activeAIModel();
  state.assistantMessages.push({ role: 'user', content });
  state.assistantMessages = state.assistantMessages.slice(-12);
  input.value = '';
  state.assistantBusy = true;
  renderAssistant();
  try {
    const response = await fetch('/api/ai/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({
        messages: state.assistantMessages.slice(-12),
        logs: state.assistantContext.slice(0, 20),
        config: profile && model ? { name: profile.name, base_url: profile.baseURL, api_key: profile.apiKey, model: model.name, type: profile.type } : null
      })
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || 'AI 请求失败');
    state.assistantMessages.push({ role: 'assistant', content: payload.message || 'AI 未返回内容' });
    state.assistantMessages = state.assistantMessages.slice(-12);
  } catch (error) {
    state.assistantMessages.push({ role: 'assistant', content: error.message || 'AI 请求失败', error: true });
    state.assistantMessages = state.assistantMessages.slice(-12);
    showToast(error.message || 'AI 请求失败');
  } finally {
    state.assistantBusy = false;
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
  const merged = new Map();
  state.logs.forEach((log) => merged.set(log.id, log));
  state.selectedContainers.forEach((key) => (state.containerLogCache[key] || []).forEach((log) => merged.set(log.id, log)));
  return sortLogsNewest(Array.from(merged.values()));
}

function sortLogsNewest(logs) {
  return [...logs].sort((left, right) => (Number(right.timestamp) || 0) - (Number(left.timestamp) || 0) || right.id - left.id);
}

function mergeLogs(existing, incoming) {
  if (!incoming.length) return existing;
  const merged = new Map();
  [...existing, ...incoming].forEach((log) => {
    if (log && log.id != null) merged.set(log.id, log);
  });
  return sortLogsNewest(Array.from(merged.values())).slice(0, maxBufferedLogs);
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
  if (!targets.length || !goServerConnected) return;
  const selectionKey = state.selectedContainers.join('|');
  try {
    await Promise.all(targets.map(async (target) => {
      const cacheKey = containerKey(target.nodeId, target.containerId);
      const query = new URLSearchParams({ node: target.nodeId, container: target.containerId });
      const response = await fetch(`/api/logs/container?${query.toString()}`, { headers: { Accept: 'application/json' } });
      if (!response.ok) throw new Error('容器日志加载失败');
      const payload = await response.json();
      if (state.selectedContainers.join('|') !== selectionKey) return;
      state.containerLogCache[cacheKey] = payload.logs || [];
    }));
    if (state.selectedContainers.join('|') !== selectionKey) return;
    scheduleLogRender();
    updatePreview();
  } catch (error) {
    showToast(error.message || '容器日志加载失败');
  }
}

const rangeLabels = {
  '30m': '最近 30 分钟',
  '2h': '最近 2 小时',
  '1d': '最近 1 天',
  '1w': '最近一周'
};

const rangeDurations = {
  '30m': 30 * 60 * 1000,
  '2h': 2 * 60 * 60 * 1000,
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
  if (!Number.isFinite(used) || !Number.isFinite(capacity) || capacity <= 0) {
    $('#metric-storage').textContent = '—';
    $('#metric-storage-foot').textContent = '等待数据';
    return;
  }
  $('#metric-storage').innerHTML = `${Math.max(0, Math.min(100, percent))}<span class="unit">%</span>`;
  $('#metric-storage-foot').textContent = `日志缓存 ${used.toLocaleString('en-US')} / ${capacity.toLocaleString('en-US')} 条`;
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
  const activeContainers = nodes.reduce((total, node) => node.status === 'connected' ? total + (Number(node.count) || 0) : total, 0);
  $('#metric-containers').textContent = activeContainers ? activeContainers.toLocaleString('en-US') : '—';
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
    if (!state.expandedNodes.includes(nodeId)) state.expandedNodes.push(nodeId);
    state.containerLogCache = {};
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
  });
}

function resetLogPagination() {
  state.visibleLogs = logPageSize;
  if (initialLogPreviewTimer) clearTimeout(initialLogPreviewTimer);
  initialLogPreviewActive = false;
  initialLogPreviewLimit = 0;
  lastVirtualWindowKey = '';
  const stream = $('#log-stream');
  if (stream) stream.scrollTop = 0;
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

function logVirtualWindow(total, stream) {
  if (!total) return { start: 0, end: 0 };
  // Adaptive row heights cannot use fixed spacer math; render normal-sized
  // result sets completely so every multi-line message can determine its row height.
  if (total <= 2000) return { start: 0, end: total };
  const viewportRows = Math.max(12, Math.ceil(stream.clientHeight / logVirtualRowHeight));
  const windowSize = viewportRows + logVirtualOverscan * 2;
  const anchor = Math.floor(stream.scrollTop / logVirtualRowHeight);
  const start = Math.max(0, Math.min(Math.max(0, total - 1), anchor - logVirtualOverscan));
  return { start, end: Math.min(total, start + windowSize) };
}

function scheduleLogRender() {
  if (logRenderTimer) return;
  logRenderTimer = setTimeout(() => {
    logRenderTimer = null;
    renderLogs();
  }, 80);
}

function scheduleLogWindowRender() {
  if (logScrollFrame) return;
  logScrollFrame = requestAnimationFrame(() => {
    logScrollFrame = null;
    renderLogs({ reuseFiltered: true, preserveScroll: true });
  });
}

function renderLogs({ reuseFiltered = false, preserveScroll = false, renderLimit = 0 } = {}) {
  const stream = $('#log-stream');
  const emptyState = $('#empty-state');
  const scrollTop = stream.scrollTop;
  const allResults = reuseFiltered ? lastFilteredLogs : filteredLogs();
  if (!reuseFiltered) lastFilteredLogs = allResults;
  const stagedResults = renderLimit > 0 ? allResults.slice(0, renderLimit) : allResults;
  const { start, end } = logVirtualWindow(stagedResults.length, stream);
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
  const windowKey = `${allResults.length}:${stagedResults.length}:${start}:${end}:${firstId}:${lastId}:${state.selectedLog}`;
  if (reuseFiltered && windowKey === lastVirtualWindowKey) {
    if (preserveScroll && stream.scrollTop !== scrollTop) stream.scrollTop = scrollTop;
    return;
  }
  lastVirtualWindowKey = windowKey;
  const topSpacer = start > 0 ? `<div class="log-virtual-spacer" style="height:${start * logVirtualRowHeight}px" aria-hidden="true"></div>` : '';
  const bottomSpacer = end < stagedResults.length ? `<div class="log-virtual-spacer" style="height:${(stagedResults.length - end) * logVirtualRowHeight}px" aria-hidden="true"></div>` : '';
  stream.innerHTML = `${topSpacer}${results.map((log, index) => `
    <div class="log-row log-row-${escapeHtml(log.level || 'info')} log-row-tone-${(start + index) % 2 ? 'odd' : 'even'} ${log.id === state.selectedLog ? 'selected' : ''}" data-log-id="${log.id}">
      <span class="log-date">${highlightSearchText(logDate(log))}</span>
      <span class="log-time">${highlightSearchText(logTime(log))}</span>
      <span class="log-level ${log.level}">${highlightSearchText(log.level.toUpperCase())}</span>
      <span class="log-source"><span class="source-tag">${highlightSearchText(log.node)}</span><span class="container-tag">/${highlightSearchText(log.container)}</span></span>
      <span class="log-level-marker ${escapeHtml(log.level || 'info')}" aria-hidden="true"></span>
      <span class="log-message" title="${escapeHtml(log.message)}">${highlightMessage(log.message)}</span><span class="row-actions"><button class="row-ai-button" type="button" data-analyze-log="${log.id}" aria-label="分析这条日志" title="让 AI 分析这条日志">◔</button></span>
    </div>
  `).join('')}${bottomSpacer}`;
  stream.append(emptyState);
  emptyState.classList.toggle('hidden', allResults.length > 0);
  if (preserveScroll && stream.scrollTop !== scrollTop) stream.scrollTop = scrollTop;
  $$('#log-stream .row-ai-button').forEach((button) => button.addEventListener('click', (event) => {
    event.stopPropagation();
    const log = logsForActiveContainer().find((item) => item.id === Number(button.dataset.analyzeLog));
    if (!log) return;
    if (!state.assistantContext.some((item) => item.id === log.id)) state.assistantContext = [log, ...state.assistantContext].slice(0, 20);
    $('#assistant-dock').classList.add('open');
    $('#assistant-input').value = '请分析这条日志：说明问题、可能原因和建议的排查步骤。';
    renderAssistant();
    sendAssistantMessage({ preventDefault() {} });
  }));
  $$('#log-stream .log-row').forEach((row) => row.addEventListener('click', () => {
    const logId = Number(row.dataset.logId);
    state.selectedLog = state.selectedLog === logId ? 0 : logId;
    renderLogs();
    updatePreview();
  }));
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
    const knownLogId = incremental ? state.logs.reduce((maxId, log) => Math.max(maxId, Number(log.id) || 0), 0) : 0;
    const cursor = Math.max(0, knownLogId - 2000);
    const endpoint = knownLogId ? `/api/bootstrap?since=${encodeURIComponent(cursor)}` : '/api/bootstrap';
    const response = await fetch(endpoint, { headers: { Accept: 'application/json' } });
    if (!response.ok) throw new Error('Go backend is unavailable');
    const payload = await response.json();
    const previousRange = state.range;
    const previousHistoryLoading = state.historyLoading;
    const incomingLogs = payload.logs || [];
    const stageInitialLogs = !initialLogPreviewRendered && incomingLogs.length > 0;
    nodes.splice(0, nodes.length, ...(payload.nodes || []));
    state.selectedNodes = state.selectedNodes.filter((id) => getNode(id));
    state.expandedNodes = state.expandedNodes.filter((id) => getNode(id));
    const serverCapacity = Number(payload.storage?.capacity);
    if (Number.isInteger(serverCapacity) && serverCapacity > 0) maxBufferedLogs = serverCapacity;
    state.logs = incremental ? mergeLogs(state.logs, incomingLogs) : incomingLogs;
    state.processed = payload.processed;
    state.historyLoading = Boolean(payload.historyLoading);
    if (stageInitialLogs) {
      initialLogPreviewRendered = true;
      initialLogPreviewActive = true;
      initialLogPreviewLimit = initialLogPreviewSize;
    }
    updateLastSync(state.logs[0]?.timestamp);
    updateStorage(payload.storage);
    if (rangeLabels[payload.range]) {
      state.range = payload.range;
      $('#time-range-filter').value = payload.range;
      refreshCustomSelect($('#time-range-filter'));
    }
    state.ruleState = { ...state.ruleState, ...payload.rules };
    syncRuleButtons();
    goServerConnected = true;
    const syncLabel = eventStream?.readyState === EventSource.OPEN ? '实时同步中' : '服务端已连接';
    $('#sync-status').textContent = syncLabel;
    if (!state.paused) $('#stream-status').textContent = state.historyLoading ? '正在加载历史日志' : '正在监听';
    updateSyncFooter(syncLabel, 'connected');
    $('#metric-processed').textContent = state.processed.toLocaleString('en-US');
    renderNodes();
    if (!incremental || incomingLogs.length || previousRange !== state.range || previousHistoryLoading !== state.historyLoading) {
      const renderLimit = initialLogPreviewActive ? initialLogPreviewLimit : 0;
      renderLogs({ renderLimit });
    }
    scheduleInitialLogCompletion();
    updateDetailPanel(); updatePreview();
    persistSelection();
    if (connectStream || !eventStream || eventStream.readyState === EventSource.CLOSED) connectGoStream();
    return true;
  } catch (error) {
    goServerConnected = false;
    $('#sync-status').textContent = '等待后端连接';
    updateSyncFooter('等待服务连接', 'offline');
    return false;
  }
}

function scheduleHistorySync() {
  if (historySyncTimer || !goServerConnected || !state.historyLoading) return;
  historySyncTimer = setTimeout(async () => {
    historySyncTimer = null;
    if (!goServerConnected || !state.historyLoading) return;
    if (await syncGoBackend({ incremental: true }) && state.historyLoading) scheduleHistorySync();
  }, 500);
}

function connectGoStream() {
  if (eventStream) eventStream.close();
  eventStream = new EventSource('/api/stream');
  eventStream.onopen = () => {
    $('#sync-status').textContent = '实时同步中';
    $('#stream-status').textContent = '正在监听';
    updateSyncFooter('实时同步中', 'connected');
  };
  eventStream.onmessage = (event) => {
    if (state.paused) return;
    const incoming = JSON.parse(event.data);
    state.logs.unshift(incoming);
    state.logs = sortLogsNewest(state.logs).slice(0, maxBufferedLogs);
    const activeTargets = selectedContainerTargets();
    activeTargets.forEach((target) => {
      const activeNode = getNode(target.nodeId);
      if (!activeNode || incoming.node !== activeNode.name || (incoming.container !== target.name && incoming.container !== target.containerId)) return;
      const cacheKey = containerKey(target.nodeId, target.containerId);
      const cached = state.containerLogCache[cacheKey] || [];
      state.containerLogCache[cacheKey] = [incoming, ...cached.filter((log) => log.id !== incoming.id)].slice(0, maxBufferedLogs);
    });
    state.processed += 1;
    $('#metric-processed').textContent = state.processed.toLocaleString('en-US');
    updateSyncFooter('实时同步中', 'connected');
    updateLastSync(incoming.timestamp);
    scheduleLogRender();
  };
  eventStream.onerror = () => {
    eventStream.close();
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
      const response = await fetch(`/api/logs/range?range=${encodeURIComponent(state.range)}`, { method: 'POST' });
      if (!response.ok) throw new Error('时间范围加载失败');
      await syncGoBackend({ incremental: true });
      if (requestVersion !== historyRequestVersion) return;
      if (state.historyLoading) scheduleHistorySync();
      showToast(state.historyLoading ? `正在增量加载${rangeLabels[state.range]}日志` : `已切换至${rangeLabels[state.range]}`);
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
  $('#log-stream').addEventListener('scroll', scheduleLogWindowRender);
  $('#pause-button').addEventListener('click', () => {
    state.paused = !state.paused;
    $('#pause-button').classList.toggle('paused', state.paused);
    $('#pause-button').innerHTML = state.paused ? '<span>▶</span> 继续接收' : '<span>Ⅱ</span> 暂停接收';
    $('#stream-status').textContent = state.paused ? '已暂停接收' : '正在监听';
    $('.stream-indicator').style.background = state.paused ? 'var(--orange)' : 'var(--teal)';
    showToast(state.paused ? '日志接收已暂停' : '日志接收已恢复');
  });
  $('#clear-button').addEventListener('click', () => { state.query = ''; $('#log-search').value = ''; resetLogPagination(); renderLogs(); showToast('已清空当前过滤条件'); });
  $('#assistant-model-trigger').addEventListener('click', (event) => {
    event.stopPropagation();
    const menu = $('#assistant-model-menu');
    const isOpen = !menu.classList.contains('hidden');
    closeAIModelMenu();
    if (!isOpen) {
      menu.classList.remove('hidden');
      $('#assistant-model-trigger').setAttribute('aria-expanded', 'true');
    }
  });
  $('#assistant-manage-models').addEventListener('click', () => { closeAIModelMenu(); openAISettings(); });
  $('#assistant-clear-context').addEventListener('click', clearAssistantContext);
  $('#assistant-context-list').addEventListener('click', (event) => {
    const button = event.target.closest('[data-remove-assistant-log]');
    if (button) removeAssistantContext(button.dataset.removeAssistantLog);
  });
  $('#assistant-form').addEventListener('submit', sendAssistantMessage);
  $('#assistant-input').addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      $('#assistant-form').requestSubmit();
    }
  });
  $('#assistant-launcher').addEventListener('click', () => {
    const dock = $('#assistant-dock');
    dock.classList.toggle('open');
    if (dock.classList.contains('open')) $('#assistant-input').focus();
  });
  $('#ai-profile-form').addEventListener('submit', saveAIProfile);
  $('#new-ai-profile')?.addEventListener('click', () => openAISettings());
  $('#delete-ai-profile').addEventListener('click', () => deleteAIProfile(state.settingsAIProfileId));
  $('#ai-add-model-button').addEventListener('click', addAIModel);
  $$('[data-close-ai-modal]').forEach((button) => button.addEventListener('click', closeAISettings));
  $('#ai-settings-modal').addEventListener('click', (event) => { if (event.target.id === 'ai-settings-modal') closeAISettings(); });
  $('#ai-admin-token-form').addEventListener('submit', (event) => { event.preventDefault(); closeAIAdminTokenPrompt($('#ai-admin-token-input').value); });
  $$('[data-close-ai-token]').forEach((button) => button.addEventListener('click', () => closeAIAdminTokenPrompt()));
  $('#ai-admin-token-modal').addEventListener('click', (event) => { if (event.target.id === 'ai-admin-token-modal') closeAIAdminTokenPrompt(); });
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
    if (!event.target.closest('#assistant-dock')) $('#assistant-dock').classList.remove('open');
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
  $('#add-node-modal').addEventListener('click', (event) => { if (event.target.id === 'add-node-modal') closeNodeModal(); });
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

initializeTheme();
restoreSelection();
loadAIProfiles();
initializeCustomSelects();
renderNodes();
renderLogs();
updateDetailPanel();
updatePreview();
renderAssistant();
bindEvents();
syncAIStatus();
syncGoBackend({ connectStream: true }).then((connected) => {
  if (connected) {
    backendRefreshTimer = setInterval(() => syncGoBackend({ incremental: true }), 3000);
    if (state.historyLoading) scheduleHistorySync();
  }
});
