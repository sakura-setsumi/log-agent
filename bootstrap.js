// This file is loaded after the core and assistant modules. It is the sole
// application bootstrap point, which keeps cross-module initialization order explicit.
initializeTheme();
restoreSelection();
// Providers are loaded from whichever backend serves this page, but that is not
// known until /api/bootstrap answers, so applyStorageMode() owns that load. The
// render calls below only need the empty initial state.
loadAssistantSessions();
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
    backendRefreshTimer = setInterval(() => {
      if (!state.paused) syncGoBackend({ incremental: true });
    }, 3000);
    if (state.historyLoading) scheduleHistorySync();
  }
});
