import test from 'node:test';
import assert from 'node:assert/strict';
import { EditorState } from '@codemirror/state';
import { expandTemplate, inlineSlots, setSlots, replaceSlotValues, replacePredicate } from './inline-slots.js';

function templateState(template, values) {
  const result = expandTemplate(template, values, { table: 'Choose table', field: 'Choose field' });
  return EditorState.create({ doc: result.sql, extensions: [inlineSlots] }).update({ effects: setSlots.of(result.slots) }).state;
}
test('replaces repeated field slots and keeps all ranges aligned after lengths change', () => {
  let state = templateState('SELECT {{field}} FROM {{table}} GROUP BY {{field}};', { field: 'id', table: '{{table}}' });
  state = state.update(replaceSlotValues(state, { table: '"odd""table"', field: '"long field"' })).state;
  assert.equal(state.doc.toString(), 'SELECT "long field" FROM "odd""table" GROUP BY "long field";');
  for (const slot of state.field(inlineSlots)) assert.equal(state.sliceDoc(slot.from, slot.to), slot.kind === 'table' ? '"odd""table"' : '"long field"');
});
test('ordinary typing remaps slots and overwriting a slot releases its editable text', () => {
  let state = templateState('SELECT {{columns}} FROM {{table}};', { columns: '*', table: 'orders' });
  state = state.update({ changes: { from: 0, insert: '-- comment\n' } }).state;
  const table = state.field(inlineSlots).find(slot => slot.kind === 'table');
  assert.equal(state.sliceDoc(table.from, table.to), 'orders');
  state = state.update({ changes: { from: table.from, to: table.to, insert: 'manually_edited' } }).state;
  assert.equal(state.field(inlineSlots).some(slot => slot.kind === 'table'), false);
  assert.equal(state.field(inlineSlots).length, 1);
  assert.match(state.doc.toString(), /FROM manually_edited;/);
});
test('independent editor states retain their own text, selection and inline slots', () => {
  const first = templateState('SELECT {{columns}} FROM {{table}};', { columns: '*', table: 'orders' });
  const second = first.update({ changes: { from: 0, to: first.doc.length, insert: 'SELECT 2;' } }).state;
  assert.equal(first.doc.toString(), 'SELECT * FROM orders;');
  assert.equal(first.field(inlineSlots).length, 2);
  assert.equal(second.doc.toString(), 'SELECT 2;');
  assert.equal(second.field(inlineSlots).length, 0);
});

test('operator changes add and remove editable arguments without swallowing following SQL', () => {
  let state = templateState('WHERE {{field}} {{operator}}\nLIMIT 100;', { field: 'id', operator: 'IS NOT NULL' });
  state = state.update(replacePredicate(state, 'IN', '(1, 2)', 'Edit value')).state;
  assert.equal(state.doc.toString(), 'WHERE id IN (1, 2)\nLIMIT 100;');
  state = state.update(replaceSlotValues(state, { field: '"long field"' })).state;
  const argument = state.field(inlineSlots).find(slot => slot.kind === 'filterValue');
  assert.equal(state.sliceDoc(argument.from, argument.to), '(1, 2)');
  state = state.update({ changes: { from: argument.from, to: argument.to, insert: '(3, 4, 5)' } }).state;
  const edited = state.field(inlineSlots).find(slot => slot.kind === 'filterValue');
  assert.equal(state.sliceDoc(edited.from, edited.to), '(3, 4, 5)');
  state = state.update(replacePredicate(state, 'IS NULL', '', 'Edit value')).state;
  assert.equal(state.doc.toString(), 'WHERE "long field" IS NULL\nLIMIT 100;');
  assert.equal(state.field(inlineSlots).some(slot => slot.kind === 'filterValue'), false);
  state = state.update(replacePredicate(state, 'BETWEEN', '1 AND 9', 'Edit value')).state;
  assert.equal(state.doc.toString(), 'WHERE "long field" BETWEEN 1 AND 9\nLIMIT 100;');
});
