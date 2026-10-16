import test from 'node:test';
import assert from 'node:assert/strict';
import { EditorState } from '@codemirror/state';
import { expandTemplate, inlineSlots, setSlots, applyTemplate, replaceSlotValues, replacePredicate } from './inline-slots.js';

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

function fill(state, template, values = {}, statement = null) {
  const result = applyTemplate(state, template, values, {}, { templateId: template }, statement);
  return { state: state.update(result.transaction).state, group: result.groupId };
}

test('blank lines insert new templates and occupied lines replace only their statement', () => {
  let state = EditorState.create({ doc: 'SELECT 1;\n  \nSELECT 3;', selection: { anchor: 12 }, extensions: [inlineSlots] });
  let result = fill(state, 'SELECT {{columns}} FROM {{table}};', { columns: '*', table: 'orders' });
  state = result.state;
  assert.equal(state.doc.toString(), 'SELECT 1;\nSELECT * FROM orders;\nSELECT 3;');
  const firstGroup = result.group;
  state = state.update({ selection: { anchor: state.doc.line(2).from + 8 } }).state;
  state = fill(state, 'DELETE FROM {{table}};', { table: 'orders' }).state;
  assert.equal(state.doc.toString(), 'SELECT 1;\nDELETE FROM orders;\nSELECT 3;');
  assert.ok(state.field(inlineSlots).every(slot => slot.template.id !== firstGroup));
});

test('replacing a multiline statement preserves adjacent SQL', () => {
  let state = EditorState.create({ doc: 'SELECT 1;\nSELECT *\nFROM orders;\nSELECT 3;', selection: { anchor: 23 }, extensions: [inlineSlots] });
  state = fill(state, 'SELECT COUNT(*) FROM {{table}};', { table: 'orders' }, { from: 10, to: 31 }).state;
  assert.equal(state.doc.toString(), 'SELECT 1;\nSELECT COUNT(*) FROM orders;\nSELECT 3;');
});

test('tables, repeated fields and predicates stay independent between inserted templates', () => {
  let result = fill(EditorState.create({ extensions: [inlineSlots] }), 'SELECT {{field}} FROM {{table}} WHERE {{field}} {{operator}};', { field: 'id', table: 'first', operator: 'IS NOT NULL' });
  const first = result.group;
  let state = result.state;
  state = state.update({ changes: { from: state.doc.length, insert: '\n' }, selection: { anchor: state.doc.length + 1 } }).state;
  result = fill(state, 'SELECT {{field}} FROM {{table}} WHERE {{field}} {{operator}};', { field: 'name', table: 'second', operator: 'IS NOT NULL' });
  state = result.state;
  state = state.update(replaceSlotValues(state, { field: 'title', table: 'another' }, result.group)).state;
  state = state.update(replacePredicate(state, 'IN', '(1, 2)', 'Edit value', result.group)).state;
  assert.equal(state.doc.line(1).text, 'SELECT id FROM first WHERE id IS NOT NULL;');
  assert.equal(state.doc.line(2).text, 'SELECT title FROM another WHERE title IN (1, 2);');
  state = state.update(replaceSlotValues(state, { table: 'first_changed' }, first)).state;
  assert.equal(state.doc.line(2).text, 'SELECT title FROM another WHERE title IN (1, 2);');
  const ids = state.field(inlineSlots).map(slot => slot.id);
  assert.equal(new Set(ids).size, ids.length);
  for (const slot of state.field(inlineSlots)) assert.ok(state.sliceDoc(slot.from, slot.to).length);
});
