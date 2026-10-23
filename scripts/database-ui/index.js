import { closeCompletion } from '@codemirror/autocomplete';
import { incrementalAt, applyIncrement, statementTables, joinBoundary } from './incremental.js';
import { slotScope, uniqueSlotGroup } from './slot-scope.js';
import * as templateInput from './template-input.js';
import { resultRowChange, mergeChangeEntries } from './result-drafts.js';
import { createRemoteTextEditor } from './remote-text-editor.js';
import { EditorView, basicSetup } from 'codemirror';
import { Compartment, EditorState, StateEffect, StateField, RangeSet, Prec } from '@codemirror/state';
import { keymap, gutter, GutterMarker } from '@codemirror/view';
import { sql } from '@codemirror/lang-sql';
import { oneDark } from '@codemirror/theme-one-dark';
import { TabulatorFull } from 'tabulator-tables';
import { recognizeStatements, queryAtCursor, sqlDialect } from './statements.js';
import { recognizeSlots } from './slot-recognizer.js';
import { inlineSlots, setSlots, applyTemplate, replaceSlotValues, replacePredicate } from './inline-slots.js';

const queryContext = StateEffect.define();
const openLogicalCondition = StateEffect.define();
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

let copiedSlots = null;
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
  templateInput,
  resultRowChange, mergeChangeEntries,
  createRemoteTextEditor,
  Tabulator: TabulatorFull,
  createEditor(parent, run, executeStatement, onChange, onSlot, labels = {}) {
    const language = new Compartment(), theme = new Compartment();
    let activeTemplate, activeSlotId, logicalTimer;
    const scope = () => slotScope(editor.state.doc.toString(), editor.state.field(inlineSlots), activeSlotId, editor.state.field(actions).context.engine);
    const activeSlots = () => scope().related;
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
    const logicalInput = EditorState.transactionExtender.of(transaction => {
      if (!transaction.docChanged || !transaction.isUserEvent('input') || transaction.effects.some(effect => effect.is(setSlots))) return null;
      const state = transaction.state, source = state.doc.toString(), head = state.selection.main.head;
      const found = recognizeSlots(source, state.field(actions).context.engine, labels);
      const raw = state.field(inlineSlots);
      const current = raw.flatMap(slot => {
        if (!slot.logical) return [slot];
        const live = found.find(item => item.logical === slot.logical && item.from === slot.from);
        return live ? [{ ...slot, to: live.to }] : [];
      });
      const cleaned = current.length !== raw.length || current.some((slot, i) => slot.to !== raw[i]?.to);
      const pending = found.find(slot => slot.logical && head >= slot.to && /^\s*$/.test(source.slice(slot.to, head)) && head <= slot.to + 8);
      if (!pending) return cleaned ? { effects: setSlots.of(current) } : null;
      const existing = current.find(slot => slot.logical && slot.from === pending.from && slot.to === pending.to);
      if (existing) return { effects: [...(cleaned ? [setSlots.of(current)] : []), openLogicalCondition.of(existing.id)] };
      const table = found.find(slot => slot.kind === 'table' && slot.template.id === pending.template.statementId);
      if (!table) return null;
      const existingTable = current.find(slot => slot.kind === 'table' && slot.from === table.from && slot.to === table.to);
      const root = existingTable?.template || table.template;
      const context = root.context || current.find(slot => slot.template?.statementId === root.id && slot.template.context)?.template.context || null;
      const group = { ...root, context, id: uniqueSlotGroup('logical-condition'), statementId: root.statementId || root.id };
      const next = current.flatMap(slot => {
        if (slot.from >= pending.to || slot.to <= pending.from) return [slot];
        if (slot.kind !== 'filterValue' || slot.from >= pending.from) return [];
        const to = source.slice(slot.from, pending.from).trimEnd().length + slot.from;
        return to > slot.from ? [{ ...slot, to }] : [];
      });
      if (!existingTable) next.push(table);
      const added = { ...pending, id: group.id + '-condition', template: group };
      next.push(added);
      return { effects: [setSlots.of(next.sort((a,b) => a.from-b.from)), openLogicalCondition.of(added.id)] };
    });
    const incrementalInput = EditorState.transactionExtender.of(transaction => {
      if (!transaction.docChanged || !transaction.isUserEvent('input') || transaction.effects.some(effect => effect.is(setSlots))) return null;
      const state = transaction.state, source = state.doc.toString(), engine = state.field(actions).context.engine;
      const pending = incrementalAt(source, state.selection.main.head, engine);
      const raw = state.field(inlineSlots);
      const current = raw.filter(slot => !slot.incremental || (() => { const live = incrementalAt(source, slot.to, engine); return live?.from === slot.from && live.kind === slot.incremental; })());
      if (!pending) return current.length !== raw.length ? {effects: setSlots.of(current)} : null;
      const existing = current.find(slot => slot.incremental === pending.kind && slot.from === pending.from);
      if (existing) {
        const updated = {...existing, to: pending.to};
        return {effects: [setSlots.of(current.map(slot => slot === existing ? updated : slot)), openLogicalCondition.of(updated.id)]};
      }
      const table = current.find(slot => slot.kind === 'table' && slot.from === pending.table.from && slot.to === pending.table.to);
      const root = table?.template || {id: uniqueSlotGroup('statement'), templateId: 'select', context: null};
      const group = {...root, id: uniqueSlotGroup('incremental'), statementId: root.statementId || root.id};
      const next = current.flatMap(slot => {
        if (slot.to <= pending.from || slot.from >= pending.to) return [slot];
        if (slot.kind !== 'filterValue' || slot.from >= pending.from) return [];
        const to = source.slice(slot.from, pending.from).trimEnd().length + slot.from;
        return to > slot.from ? [{...slot, to}] : [];
      });
      if (!table) next.push({...pending.table, id: root.id + '-table', template: root, label: labels.table});
      const added = {id: group.id + '-keyword', kind: 'incremental', incremental: pending.kind, from: pending.from, to: pending.to, template: group};
      next.push(added);
      return {effects: [setSlots.of(next.sort((a,b)=>a.from-b.from)), openLogicalCondition.of(added.id)]};
    });
    const editor = new EditorView({
      parent,
      extensions: [basicSetup, EditorView.lineWrapping, language.of(sql()), theme.of([]),
        actions, inlineSlots, logicalInput, incrementalInput, gutter({ class: 'cm-query-actions', side: 'after', markers: view => view.state.field(actions).markers }),
        EditorView.updateListener.of(update => {
          if (update.docChanged) { clearTimeout(logicalTimer); onChange?.(); }
          const effect = update.transactions.flatMap(transaction => transaction.effects).find(effect => effect.is(openLogicalCondition));
          // Let the user finish a word (notably ORDER) before opening a picker.
          if (effect) logicalTimer = setTimeout(() => {
            const slot = editor.state.field(inlineSlots).find(slot => slot.id === effect.value);
            const head = editor.state.selection.main.head;
            if (slot && head >= slot.to && /^\s*$/.test(editor.state.sliceDoc(slot.to, head))) { activeTemplate = slot.template.id; activeSlotId = slot.id; closeCompletion(editor); onSlot?.(slot); }
          }, 250);
        }),
        EditorView.domEventHandlers({ click(event, view) { const target = event.target.closest('[data-db-slot]'); if (target) { const slot = view.state.field(inlineSlots).find(item => item.id === target.dataset.dbSlot); if (slot) { activeTemplate = slot.template?.id; activeSlotId = slot.id; closeCompletion(editor); onSlot?.(slot); return true; } } return false; } }),
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
            const group = { ...copied.slots[0]?.template, id: uniqueSlotGroup('template-paste') };
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
      setValue(value) { activeTemplate = undefined; activeSlotId = undefined; editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value }, effects: setSlots.of([]), selection: { anchor: 0 }, scrollIntoView: true }); },
      setTemplate(template, values, templateLabels, metadata) {
        labels = { ...labels, ...templateLabels };
        const state = editor.state, statement = queryAtCursor(state.field(actions).statements, state.selection.main.head, state.doc.toString());
        const result = applyTemplate(state, template, values, labels, metadata, statement);
        activeTemplate = result.groupId; editor.dispatch(result.transaction); activeSlotId = editor.state.field(inlineSlots).find(slot => slot.template?.id === activeTemplate)?.id;
      },
      templateContext: () => activeSlots().find(slot => slot.template?.context)?.template.context || null,
      setTemplateContext(context) {
        const active = activeSlots()[0]?.template, root = active?.statementId || active?.id;
        if (!root) return;
        const ids = new Set(scope().statementSlots.map(slot => slot.id));
        editor.dispatch({ effects: setSlots.of(editor.state.field(inlineSlots).map(slot => ids.has(slot.id) && (slot.template?.statementId || slot.template?.id) === root ? { ...slot, template: { ...slot.template, context } } : slot)) });
      },
      templateAtCursor() {
        const state = editor.state, line = state.doc.lineAt(state.selection.main.head);
        if (!line.text.trim()) return null;
        const range = queryAtCursor(state.field(actions).statements, state.selection.main.head, state.doc.toString()) || line;
        return state.field(inlineSlots).find(slot => slot.from >= range.from && slot.to <= range.to)?.template || null;
      },
      slots: () => editor.state.field(inlineSlots),
      slotText: kind => { const slot = activeSlots().find(slot => slot.kind === kind); return slot ? editor.state.sliceDoc(slot.from, slot.to) : null; },
      selectSlot(id) { const slot = editor.state.field(inlineSlots).find(slot => slot.id === id); if (slot) { activeTemplate = slot.template?.id; activeSlotId = slot.id; editor.dispatch({ selection: { anchor: slot.from, head: slot.to }, scrollIntoView: true }); editor.focus(); } },
      slotRect(id) { const slot = editor.state.field(inlineSlots).find(slot => slot.id === id); return slot ? editor.coordsAtPos(slot.from) : null; },
      replaceSlots(values) {
        const table = activeSlots().find(slot => slot.kind === 'table');
        if (values.table !== undefined && table && table.template?.id !== activeTemplate) {
          const change = replaceSlotValues(editor.state, { table: values.table }, table.template.id, new Set([table.id]));
          if (change) editor.dispatch(change);
        }
        const transaction = replaceSlotValues(editor.state, values, activeTemplate, new Set(activeSlots().map(slot => slot.id))); if (transaction) editor.dispatch(transaction);
      },
      // Turn a bare condition slot into "<field> IS NOT NULL" with selectable field and operator slots.
      replaceCondition(id, field, labels) {
        const state = editor.state, slots = state.field(inlineSlots), slot = slots.find(item => item.id === id); if (!slot) return;
        const group = { ...slot.template, id: uniqueSlotGroup('condition'), statementId: slot.template?.statementId || slot.template?.id }, prefix = slot.logical ? slot.logical + ' ' : '', insert = prefix + field + ' IS NOT NULL';
        const changes = state.changes({ from: slot.from, to: slot.to, insert });
        const others = slots.filter(item => item !== slot).map(item => ({ ...item, from: changes.mapPos(item.from, -1), to: changes.mapPos(item.to, 1) }));
        const created = [{ id: group.id + '-field', kind: 'field', template: group, from: slot.from + prefix.length, to: slot.from + prefix.length + field.length, label: labels.field }, { id: group.id + '-operator', kind: 'operator', template: group, from: slot.from + prefix.length + field.length + 1, to: slot.from + insert.length, label: labels.operator }];
        activeTemplate = group.id; activeSlotId = created.find(item => item.kind === 'operator').id;
        editor.dispatch({ changes, effects: setSlots.of([...others, ...created].sort((a, b) => a.from - b.from)) });
        return created;
      },
      replacePredicate(operator, operand, label) { const transaction = replacePredicate(editor.state, operator, operand, label, activeTemplate, activeSlots().find(slot => slot.kind === 'operator')?.id || '__missing_operator__'); if (transaction) editor.dispatch(transaction); },
      incrementalTables(id) {
        const state=editor.state, slot=state.field(inlineSlots).find(item=>item.id===id);
        if(!slot?.incremental) return [];
        const source=state.doc.toString(), pending=incrementalAt(source,slot.to,state.field(actions).context.engine);
        return pending ? statementTables(source.slice(pending.statement.from,pending.from)+source.slice(pending.to,pending.statement.to)) : [];
      },
      applyIncremental(id, values, nextLabels) {
        const state = editor.state, slots = state.field(inlineSlots), slot = slots.find(item => item.id === id);
        if (!slot?.incremental) return null;
        const source = state.doc.toString(), engine = state.field(actions).context.engine;
        const pending = incrementalAt(source, slot.to, engine);
        if (!pending || pending.from !== slot.from || pending.kind !== slot.incremental) return null;
        let sql = applyIncrement(source, pending, values);
        let cursor;
        if (sql && ['inner','left','right'].includes(slot.incremental)) {
          const boundary=joinBoundary(sql), left=sql.slice(0,boundary).replace(/[ \t]+$/, '');
          sql=left+'  '+sql.slice(boundary); cursor=left.length+1;
        }
        if (!sql) return null;
        labels = {...labels, ...nextLabels};
        const context = activeSlots().find(item => item.template?.context)?.template.context || null;
        const changes = state.changes({from: pending.statement.from, to: pending.statement.to, insert: sql});
        const remaining = slots.filter(item => item.to <= pending.statement.from || item.from >= pending.statement.to)
          .map(item => ({...item, from: changes.mapPos(item.from,1), to: changes.mapPos(item.to,-1)}));
        const namespace = uniqueSlotGroup('increment-applied');
        const created = recognizeSlots(sql, engine, labels).map(item => ({...item, from: item.from + pending.statement.from, to: item.to + pending.statement.from,
          id: namespace + '-' + item.id, template: {...item.template, id: namespace + '-' + item.template.id,
            statementId: item.template.statementId ? namespace + '-' + item.template.statementId : undefined, context}}));
        const operator = slot.incremental === 'where' ? created.filter(item => item.kind === 'operator').at(-1) : null;
        activeSlotId = (operator || created[0])?.id; activeTemplate = (operator || created[0])?.template.id;
        editor.dispatch({changes, effects: setSlots.of([...remaining,...created].sort((a,b)=>a.from-b.from)), selection: {anchor: pending.statement.from + (cursor ?? sql.length)}, scrollIntoView: true});
        return {operator};
      },
      // Give plain SQL (no slots yet) picker slots wherever its structure is recognizable.
      recognizeSlots(nextLabels) {
        labels = { ...labels, ...nextLabels };
        if (editor.state.field(inlineSlots).length) return false;
        const slots = recognizeSlots(editor.state.doc.toString(), editor.state.field(actions).context.engine, labels);
        if (slots.length) editor.dispatch({ effects: setSlots.of(slots) });
        return slots.length > 0;
      },
      // Slots are plain objects, so they survive JSON; used to keep the picker slots across a restart.
      slotState: () => ({ slots: editor.state.field(inlineSlots), activeTemplate, activeSlotId }),
      setSlotState(saved) {
        const length = editor.state.doc.length, slots = (saved?.slots || []).filter(slot => Number.isInteger(slot?.from) && Number.isInteger(slot?.to) && slot.from >= 0 && slot.from <= slot.to && slot.to <= length);
        activeTemplate = saved?.activeTemplate; activeSlotId = undefined;
        // Upgrade COUNT statements saved before aggregate fields/aliases had slots.
        const detected = recognizeSlots(editor.state.doc.toString(), editor.state.field(actions).context.engine, labels), aggregates = new Map();
        for (const item of detected.filter(item => ['countField','alias'].includes(item.kind))) {
          if (slots.some(slot => slot.kind === item.kind && slot.from === item.from && slot.to === item.to)) continue;
          let group = aggregates.get(item.template.id);
          if (!group) {
            const recognizedTable = detected.find(slot => slot.kind === 'table' && slot.template.id === item.template.statementId);
            const table = recognizedTable && slots.find(slot => slot.kind === 'table' && slot.from === recognizedTable.from && slot.to === recognizedTable.to);
            const root = table?.template;
            group = {...item.template, id: uniqueSlotGroup('restored-count'), statementId: root?.id || item.template.statementId, context: root?.context || null};
            aggregates.set(item.template.id, group);
          }
          slots.push({...item, template: group});
        }
        slots.sort((a,b)=>a.from-b.from);
        const restored = uniqueSlotGroup('restored-slot');
        const selected = slots.find(slot => slot.id === saved?.activeSlotId);
        slots.forEach((slot, index) => { slots[index] = { ...slot, id: restored + '-' + index }; if (slot === selected) activeSlotId = slots[index].id; });
        if (slots.length) editor.dispatch({ effects: setSlots.of(slots) });
      },
      snapshot: () => ({ state: editor.state, activeTemplate, activeSlotId, top: editor.scrollDOM.scrollTop, left: editor.scrollDOM.scrollLeft }),
      restore(snapshot) { configuration = ''; activeTemplate = snapshot.activeTemplate; activeSlotId = snapshot.activeSlotId; editor.setState(snapshot.state); requestAnimationFrame(() => { editor.scrollDOM.scrollTop = snapshot.top; editor.scrollDOM.scrollLeft = snapshot.left; }); },
      configure(engine, schema = {}) { const next = JSON.stringify([engine, schema]); if (configuration === next) return; configuration = next; editor.dispatch({ effects: [language.reconfigure(sql({ dialect: sqlDialect(engine), schema, upperCaseKeywords: true })), queryContext.of({ engine })] }); },
      queryActions(context) { const current = editor.state.field(actions).context; if (Object.entries(context).some(([key, value]) => current[key] !== value)) editor.dispatch({ effects: queryContext.of(context) }); },
      currentStatement: () => queryAtCursor(editor.state.field(actions).statements, editor.state.selection.main.head, editor.state.doc.toString())?.sql || '',
      theme: dark => editor.dispatch({ effects: theme.reconfigure(dark ? oneDark : []) }),
      focus: () => editor.focus(),
      resize: () => editor.requestMeasure(),
    };
  },
};
