import { StateEffect, StateField } from '@codemirror/state';
import { Decoration, EditorView } from '@codemirror/view';

export const setSlots = StateEffect.define();
export const inlineSlots = StateField.define({
  create: () => [],
  update(slots, transaction) {
    let next = slots;
    if (transaction.docChanged) {
      next = slots.flatMap(slot => {
        let replaced = false;
        transaction.changes.iterChangedRanges((from, to) => { if (from <= slot.from && to >= slot.to && to > from) replaced = true; });
        if (replaced && slot.kind !== 'filterValue') return [];
        const from = transaction.changes.mapPos(slot.from, -1), to = transaction.changes.mapPos(slot.to, 1);
        return to > from ? [{ ...slot, from, to }] : [];
      });
    }
    for (const effect of transaction.effects) if (effect.is(setSlots)) next = effect.value;
    return next;
  },
  provide: field => EditorView.decorations.from(field, slots => Decoration.set(slots.map(slot => Decoration.mark({ class: 'db-inline-slot', attributes: { 'data-db-slot': slot.id, 'data-db-slot-kind': slot.kind, title: slot.label } }).range(slot.from, slot.to)), true)),
});

export function expandTemplate(template, values, labels) {
  let sql = '', slots = [], end = 0;
  for (const match of template.matchAll(/\{\{(\w+)\}\}/g)) {
    sql += template.slice(end, match.index);
    const from = sql.length, kind = match[1];
    sql += values[kind] ?? match[0];
    slots.push({ id: `slot-${slots.length}`, kind, from, to: sql.length, label: labels[kind] || kind });
    end = match.index + match[0].length;
  }
  return { sql: sql + template.slice(end), slots };
}

export function replaceSlotValues(state, values) {
  const slots = state.field(inlineSlots), edits = slots.filter(slot => values[slot.kind] !== undefined).map(slot => ({ from: slot.from, to: slot.to, insert: values[slot.kind] }));
  if (!edits.length) return null;
  const changes = state.changes(edits), next = slots.map(slot => ({ ...slot, from: changes.mapPos(slot.from, -1), to: changes.mapPos(slot.to, 1) }));
  return { changes, effects: setSlots.of(next) };
}

// Keep the operator selectable while its argument remains ordinary editable SQL.
export function replacePredicate(state, operator, operand, label) {
  const slots = state.field(inlineSlots), current = slots.find(slot => slot.kind === 'operator');
  if (!current) return null;
  const argument = slots.find(slot => slot.kind === 'filterValue');
  const to = argument && /^\s*$/.test(state.sliceDoc(current.to, argument.from)) ? argument.to : current.to;
  const insert = operator + (operand ? ' ' + operand : '');
  const changes = state.changes({ from: current.from, to, insert });
  const next = slots.filter(slot => slot !== current && !(argument && slot === argument && to === argument.to)).map(slot => ({ ...slot, from: changes.mapPos(slot.from, -1), to: changes.mapPos(slot.to, 1) }));
  next.push({ ...current, to: current.from + operator.length });
  if (operand) next.push({ id: current.id + '-value', kind: 'filterValue', from: current.from + operator.length + 1, to: current.from + insert.length, label });
  return { changes, effects: setSlots.of(next.sort((a, b) => a.from - b.from)) };
}
