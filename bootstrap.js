// This file is loaded after the core and assistant modules. It is the sole
// application bootstrap point, which keeps cross-module initialization order explicit.
initializeLanguage();
initializeTheme();
restoreSelection();
// Containers dropped from 连接管理 are a display choice of their own, so they
// load separately from the log selection they sit next to in storage.
restoreHiddenContainers();
// Establishes whether this deployment has an admin token, which decides whether
// saving a model needs to prompt for one. Loaded here rather than only when the
// settings panel opens, because the model panel is reachable on its own.
loadConfigInfo();
// Providers are loaded from whichever backend serves this page, but that is not
// known until /api/bootstrap answers, so applyStorageMode() owns that load. The
// render calls below only need the empty initial state.
loadAssistantSessions();
initializeCustomSelects();
renderNodes();
// renderNodes() only paints the static switch markup; its listener and the
// initial checked state are bound here, after the node list exists.
bindMultiSelectToggle();
renderLogs();
updateDetailPanel();
updatePreview();
renderAssistant();
bindEvents();
syncAIStatus();
syncGoBackend({ connectStream: true }).then((connected) => {
  if (connected) {
    backendRefreshTimer = setInterval(() => {
      if (!state.paused) syncGoBackend({ incremental: true });
    }, 3000);
    if (state.historyLoading) scheduleHistorySync();
  }
});
