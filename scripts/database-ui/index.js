import { createRemoteTextEditor } from './remote-text-editor.js';
import { EditorView, basicSetup } from 'codemirror';
import { Compartment, StateEffect, StateField, RangeSet, Prec } from '@codemirror/state';
import { keymap, gutter, GutterMarker } from '@codemirror/view';
import { sql } from '@codemirror/lang-sql';
import { oneDark } from '@codemirror/theme-one-dark';
import { TabulatorFull } from 'tabulator-tables';
import { recognizeStatements, queryAtCursor, sqlDialect } from './statements.js';
import { inlineSlots, setSlots, applyTemplate, replaceSlotValues, replacePredicate } from './inline-slots.js';

const queryContext = StateEffect.define();
class QueryActions extends GutterMarker {
  constructor(statement, context, execute) { super(); this.statement = statement; this.context = context; this.execute = execute; }
  eq(other) { return this.statement.from === other.statement.from && this.statement.to === other.statement.to && this.statement.sql === other.statement.sql && JSON.stringify(this.context) === JSON.stringify(other.context); }
  toDOM() {
    const group = document.createElement('span'); group.className = 'db-statement-actions';
    for (const scope of ['single', 'multi']) {
      const button = document.createElement('button'); button.type = 'button'; button.className = 'db-statement-action'; button.dataset.scope = scope; button.dataset.statementFrom = this.statement.from;
      button.disabled = !this.context[scope + 'Enabled'];
      button.title = `${this.context[scope + 'Label']}\n${this.statement.sql.slice(0, 100)}`; button.setAttribute('aria-label', this.context[scope + 'Label']);
      button.innerHTML = scope === 'single'
        ? '<svg viewBox="0 0 20 20" aria-hidden="true"><ellipse cx="7" cy="4" rx="4.5" ry="2"/><path d="M2.5 4v9c0 2.7 9 2.7 9 0V4M2.5 8c0 2.7 9 2.7 9 0"/><path class="db-statement-play" d="m14 10 4 3-4 3z"/></svg>'
        : '<svg viewBox="0 0 20 20" aria-hidden="true"><ellipse cx="6" cy="4" rx="4" ry="2"/><path d="M2 4v8c0 2.6 8 2.6 8 0V4M2 8c0 2.6 8 2.6 8 0M12 4c5-1 7 1 6 2v7c0 1-2 2-5 1"/><path class="db-statement-play" d="m12 11 4 3-4 3z"/></svg>';
      button.addEventListener('mousedown', event => event.preventDefault());
      button.addEventListener('click', event => { event.preventDefault(); event.stopPropagation(); if (!button.disabled) this.execute(scope, this.statement.sql); });
      group.appendChild(button);
    }
    return group;
  }
}

function actionMarkers(state, statements, context, execute) {
  return RangeSet.of(statements.map(statement => new QueryActions(statement, context, execute).range(state.doc.lineAt(Math.max(statement.from, statement.to - 1)).from)), true);
}

let copiedSlots = null, pasteSequence = 0;
function copySlots(event, view) {
  const state = view.state, range = state.selection.main;
  let { from, to } = range;
  if (range.empty) { const line = state.doc.lineAt(range.head); from = line.from; to = Math.min(line.to + 1, state.doc.length); }
  const slots = state.field(inlineSlots).filter(slot => slot.from >= from && slot.to <= to).map(slot => ({ ...slot, from: slot.from - from, to: slot.to - from }));
  const text = state.sliceDoc(from, to);
  copiedSlots = slots.length ? { text, slots } : null;
  return false;
}

window.DatabaseUI = {
  createRemoteTextEditor,
  Tabulator: TabulatorFull,
  createEditor(parent, run, executeStatement, onChange, onSlot) {
    const language = new Compartment(), theme = new Compartment();
    let activeTemplate;
    const activeSlots = () => editor.state.field(inlineSlots).filter(slot => slot.template?.id === activeTemplate);
    const actions = StateField.define({
      create(state) { const context = { engine: 'mysql', singleLabel: 'Run on one database', multiLabel: 'Run on checked databases' }, statements = recognizeStatements(state.doc.toString()); return { context, statements, markers: actionMarkers(state, statements, context, executeStatement) }; },
      update(value, transaction) {
        let context = value.context;
        for (const effect of transaction.effects) if (effect.is(queryContext)) context = { ...context, ...effect.value };
        if (!transaction.docChanged && context === value.context) return value;
        const statements = transaction.docChanged || context.engine !== value.context.engine ? recognizeStatements(transaction.state.doc.toString(), context.engine) : value.statements;
        return { context, statements, markers: actionMarkers(transaction.state, statements, context, executeStatement) };
      },
    });
    const editor = new EditorView({
      parent,
      extensions: [basicSetup, EditorView.lineWrapping, language.of(sql()), theme.of([]),
        actions, inlineSlots, gutter({ class: 'cm-query-actions', side: 'after', markers: view => view.state.field(actions).markers }),
        EditorView.updateListener.of(update => { if (update.docChanged) onChange?.(); }),
        EditorView.domEventHandlers({ click(event, view) { const target = event.target.closest('[data-db-slot]'); if (target) { const slot = view.state.field(inlineSlots).find(item => item.id === target.dataset.dbSlot); if (slot) { activeTemplate = slot.template?.id; onSlot?.(slot); return true; } } return false; } }),
        // Copying a statement keeps its selectable slots: the clipboard text is
        // matched against what was last copied here, and the slots are re-created
        // (as their own template group) at the paste position.
        EditorView.domEventHandlers({
          copy: copySlots, cut: copySlots,
          paste(event, view) {
            const pasted = event.clipboardData?.getData('text/plain'), copied = copiedSlots;
            if (!copied || !pasted || pasted !== copied.text) return false;
            event.preventDefault();
            const state = view.state, { from, to } = state.selection.main, changes = state.changes({ from, to, insert: pasted });
            const group = { ...copied.slots[0]?.template, id: 'template-paste-' + ++pasteSequence };
            const remaining = state.field(inlineSlots).filter(slot => slot.to <= from || slot.from >= to).map(slot => ({ ...slot, from: changes.mapPos(slot.from, 1), to: changes.mapPos(slot.to, -1) }));
            const inserted = copied.slots.map((slot, index) => ({ ...slot, id: group.id + '-' + index, template: group, from: from + slot.from, to: from + slot.to }));
            view.dispatch({ changes, effects: setSlots.of([...remaining, ...inserted].sort((a, b) => a.from - b.from)), selection: { anchor: from + pasted.length }, scrollIntoView: true, userEvent: 'input.paste' });
            return true;
          },
        }),
        Prec.highest(keymap.of([{ key: 'Mod-Enter', run: () => { run(); return true; } }])),
        EditorView.theme({ '&': { height: '100%', fontSize: '13px' }, '.cm-scroller': { fontFamily: 'Consolas, monospace', overflow: 'auto' }, '.cm-content': { minHeight: '120px' } })],
    });
    let configuration = '';
    return {
      value: () => editor.state.doc.toString(),
      selected: () => editor.state.sliceDoc(editor.state.selection.main.from, editor.state.selection.main.to),
      setValue: value => editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value }, effects: setSlots.of([]), selection: { anchor: 0 }, scrollIntoView: true }),
      setTemplate(template, values, labels, metadata) {
        const state = editor.state, statement = queryAtCursor(state.field(actions).statements, state.selection.main.head, state.doc.toString());
        const result = applyTemplate(state, template, values, labels, metadata, statement);
        activeTemplate = result.groupId; editor.dispatch(result.transaction);
      },
      setTemplateContext(context) { editor.dispatch({ effects: setSlots.of(editor.state.field(inlineSlots).map(slot => slot.template?.id === activeTemplate ? { ...slot, template: { ...slot.template, context } } : slot)) }); },
      templateAtCursor() {
        const state = editor.state, line = state.doc.lineAt(state.selection.main.head);
        if (!line.text.trim()) return null;
        const range = queryAtCursor(state.field(actions).statements, state.selection.main.head, state.doc.toString()) || line;
        return state.field(inlineSlots).find(slot => slot.from >= range.from && slot.to <= range.to)?.template || null;
      },
      slots: () => editor.state.field(inlineSlots),
      slotText: kind => { const slot = activeSlots().find(slot => slot.kind === kind); return slot ? editor.state.sliceDoc(slot.from, slot.to) : null; },
      selectSlot(id) { const slot = editor.state.field(inlineSlots).find(slot => slot.id === id); if (slot) { editor.dispatch({ selection: { anchor: slot.from, head: slot.to }, scrollIntoView: true }); editor.focus(); } },
      slotRect(id) { const slot = editor.state.field(inlineSlots).find(slot => slot.id === id); return slot ? editor.coordsAtPos(slot.from) : null; },
      replaceSlots(values) {
        const transaction = replaceSlotValues(editor.state, values, activeTemplate); if (transaction) editor.dispatch(transaction);
      },
      replacePredicate(operator, operand, label) { const transaction = replacePredicate(editor.state, operator, operand, label, activeTemplate); if (transaction) editor.dispatch(transaction); },
      snapshot: () => ({ state: editor.state, activeTemplate, top: editor.scrollDOM.scrollTop, left: editor.scrollDOM.scrollLeft }),
      restore(snapshot) { configuration = ''; activeTemplate = snapshot.activeTemplate; editor.setState(snapshot.state); requestAnimationFrame(() => { editor.scrollDOM.scrollTop = snapshot.top; editor.scrollDOM.scrollLeft = snapshot.left; }); },
      configure(engine, schema = {}) { const next = JSON.stringify([engine, schema]); if (configuration === next) return; configuration = next; editor.dispatch({ effects: [language.reconfigure(sql({ dialect: sqlDialect(engine), schema, upperCaseKeywords: true })), queryContext.of({ engine })] }); },
      queryActions(context) { const current = editor.state.field(actions).context; if (Object.entries(context).some(([key, value]) => current[key] !== value)) editor.dispatch({ effects: queryContext.of(context) }); },
      currentStatement: () => queryAtCursor(editor.state.field(actions).statements, editor.state.selection.main.head, editor.state.doc.toString())?.sql || '',
      theme: dark => editor.dispatch({ effects: theme.reconfigure(dark ? oneDark : []) }),
      focus: () => editor.focus(),
      resize: () => editor.requestMeasure(),
    };
  },
};
