// This file is loaded after the core and assistant modules. It is the sole
// application bootstrap point, which keeps cross-module initialization order explicit.
initializeTheme();
restoreSelection();
loadAIProfiles();
loadAssistantSessions();
initializeCustomSelects();
renderNodes();
renderLogs();
updateDetailPanel();
updatePreview();
renderAssistant();
bindEvents();
syncAIStatus();
loadAIProfilesFromDatabase();
syncGoBackend({ connectStream: true }).then((connected) => {
  if (connected) {
    backendRefreshTimer = setInterval(() => {
      if (!state.paused) syncGoBackend({ incremental: true });
    }, 3000);
    if (state.historyLoading) scheduleHistorySync();
  }
});
