// Assistant-only browser interactions live here so their propagation and
// scrolling behavior can evolve independently from the log stream.
function bindAssistantEvents() {
  $('#assistant-session-toggle').addEventListener('click', (event) => {
    event.stopPropagation();
    const menu = $('#assistant-session-menu');
    const open = !menu.classList.contains('hidden');
    closeAssistantSessionMenu();
    if (!open) {
      menu.classList.remove('hidden');
      $('#assistant-session-toggle').setAttribute('aria-expanded', 'true');
    }
  });
  $('#assistant-session-list').addEventListener('click', (event) => {
    const selectButton = event.target.closest('[data-select-assistant-session]');
    if (selectButton) return selectAssistantSession(selectButton.dataset.selectAssistantSession);
    const deleteButton = event.target.closest('[data-delete-assistant-session]');
    if (deleteButton) deleteAssistantSession(deleteButton.dataset.deleteAssistantSession);
  });
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
  $('#assistant-clear-context').addEventListener('click', () => {
    clearAssistantContext();
    $('#assistant-input').focus();
  });
  $('#assistant-context-list').addEventListener('click', (event) => {
    const button = event.target.closest('[data-remove-assistant-log]');
    if (button) removeAssistantContext(button.dataset.removeAssistantLog);
  });
  $('#assistant-attach-button').addEventListener('click', () => $('#assistant-file-input').click());
  $('#assistant-file-input').addEventListener('change', (event) => {
    void addAssistantAttachments(event.target.files);
    event.target.value = '';
  });
  $('#assistant-attachment-list').addEventListener('click', (event) => {
    const previewButton = event.target.closest('[data-preview-assistant-attachment]');
    if (previewButton) {
      event.stopPropagation();
      openAssistantAttachmentPreview(Number(previewButton.dataset.previewAssistantAttachment));
      return;
    }
    const button = event.target.closest('[data-remove-assistant-attachment]');
    if (!button) return;
    // Rendering removes the clicked button; do not let the global outside-click
    // listener mistake this removal for a request to close the assistant.
    event.stopPropagation();
    state.assistantAttachments.splice(Number(button.dataset.removeAssistantAttachment), 1);
    renderAssistant();
  });
  $('#assistant-attachment-preview-close').addEventListener('click', (event) => {
    event.stopPropagation();
    closeAssistantAttachmentPreview();
  });
  $('#assistant-attachment-preview-modal').addEventListener('click', (event) => {
    if (event.target === event.currentTarget) closeAssistantAttachmentPreview();
  });
  $('#assistant-panel').addEventListener('wheel', (event) => {
    const scroller = event.target.closest('.assistant-context-list, .assistant-messages');
    if (!scroller) {
      event.preventDefault();
      return;
    }
    const atTop = scroller.scrollTop <= 0;
    const atBottom = scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 1;
    if ((event.deltaY < 0 && atTop) || (event.deltaY > 0 && atBottom)) event.preventDefault();
  }, { passive: false });
  $('#assistant-form').addEventListener('submit', sendAssistantMessage);
  $('#assistant-input').addEventListener('paste', (event) => {
    const pastedImages = Array.from(event.clipboardData?.files || []).filter((file) => file.type.startsWith('image/'));
    if (!pastedImages.length) return;
    event.preventDefault();
    void addAssistantAttachments(pastedImages);
  });
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
}
