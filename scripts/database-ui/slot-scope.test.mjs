import test from 'node:test';
import assert from 'node:assert/strict';
import { EditorState } from '@codemirror/state';
import { inlineSlots, setSlots, replaceSlotValues, replacePredicate, applyTemplate } from './inline-slots.js';
import { recognizeSlots } from './slot-recognizer.js';
import { slotScope } from './slot-scope.js';

function restored(source) {
  // Reproduce old persisted templates whose group IDs collided after restart.
  const slots = recognizeSlots(source, 'sqlite').map((slot, index) => ({ ...slot, id: 'saved-' + index, template: { id: 'template-1' } }));
  return EditorState.create({ doc: source, extensions: [inlineSlots] }).update({ effects: setSlots.of(slots) }).state;
}

test('editing a lower saved statement reads and changes only its selected predicate', () => {
  let state = restored('SELECT * FROM orders WHERE id = 220 LIMIT 100;\nSELECT * FROM orders WHERE id = 197;');
  const selected = state.field(inlineSlots).filter(slot => slot.kind === 'operator')[1];
  const scope = slotScope(state.doc.toString(), state.field(inlineSlots), selected.id, 'sqlite');
  const value = scope.own.find(slot => slot.kind === 'filterValue');
  assert.equal(state.sliceDoc(value.from, value.to), '197');
  assert.equal(scope.related.find(slot => slot.kind === 'table').from, state.doc.line(2).from + 14);
  state = state.update(replacePredicate(state, '=', '330', 'value', selected.template.id, selected.id)).state;
  assert.equal(state.doc.toString(), 'SELECT * FROM orders WHERE id = 220 LIMIT 100;\nSELECT * FROM orders WHERE id = 330;');
});

test('AND and OR predicates in one restored group have independent fields and values', () => {
  let state = restored('SELECT * FROM orders WHERE id = 1 AND amount >= 2 OR note LIKE \'old\';');
  const selected = state.field(inlineSlots).filter(slot => slot.kind === 'field')[1];
  let scope = slotScope(state.doc.toString(), state.field(inlineSlots), selected.id, 'sqlite');
  state = state.update(replaceSlotValues(state, { field: 'total' }, 'template-1', new Set(scope.own.map(slot => slot.id)))).state;
  scope = slotScope(state.doc.toString(), state.field(inlineSlots), selected.id, 'sqlite');
  const operator = scope.own.find(slot => slot.kind === 'operator');
  state = state.update(replacePredicate(state, 'IN', '(3, 4)', 'value', 'template-1', operator.id)).state;
  assert.equal(state.doc.toString(), 'SELECT * FROM orders WHERE id = 1 AND total IN (3, 4) OR note LIKE \'old\';');
});

test('table and insert value replacements are bounded by the clicked statement', () => {
  let state = restored('INSERT INTO orders (id) VALUES (1);\nINSERT INTO orders (id) VALUES (2);');
  const selected = state.field(inlineSlots).filter(slot => slot.kind === 'values')[1];
  const scope = slotScope(state.doc.toString(), state.field(inlineSlots), selected.id, 'sqlite');
  state = state.update(replaceSlotValues(state, { values: '7' }, 'template-1', new Set(scope.own.map(slot => slot.id)))).state;
  assert.equal(state.doc.toString(), 'INSERT INTO orders (id) VALUES (1);\nINSERT INTO orders (id) VALUES (7);');
});

test('new templates never reuse persisted sequential group IDs', () => {
  const state = restored('SELECT * FROM orders WHERE id = 220;\n');
  const positioned = state.update({ selection: { anchor: state.doc.length } }).state;
  const result = applyTemplate(positioned, 'SELECT * FROM {{table}};', { table: 'orders' }, {});
  assert.ok(!state.field(inlineSlots).some(slot => slot.template.id === result.groupId));
});

test('editing an ORDER BY field does not rewrite WHERE fields in the same group', () => {
  let state = restored('SELECT * FROM orders WHERE id = 1 ORDER BY amount;');
  const selected = state.field(inlineSlots).filter(slot => slot.kind === 'field').at(-1);
  const scope = slotScope(state.doc.toString(), state.field(inlineSlots), selected.id, 'sqlite');
  state = state.update(replaceSlotValues(state, {field:'note'}, 'template-1', new Set(scope.own.map(slot=>slot.id)))).state;
  assert.equal(state.doc.toString(), 'SELECT * FROM orders WHERE id = 1 ORDER BY note;');
});
