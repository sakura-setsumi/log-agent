import test from 'node:test';
import assert from 'node:assert/strict';
import { incrementalAt, applyIncrement } from './incremental.js';
import { recognizeSlots } from './slot-recognizer.js';

const choose = (marked, values) => {
  const head = marked.indexOf('|'), source = marked.replace('|','');
  const pending = incrementalAt(source,head,'mysql');
  assert.ok(pending, marked);
  return applyIncrement(source,pending,values);
};

test('WHERE, ORDER and GROUP join the current statement in clause order',()=> {
  assert.equal(choose('SELECT * FROM orders WHERE| LIMIT 100;', {field:'id'}), 'SELECT * FROM orders WHERE id IS NOT NULL LIMIT 100;');
  assert.equal(choose('SELECT * FROM orders WHERE id=1 LIMIT 100 order|;', {columns:'amount',direction:'DESC'}), 'SELECT * FROM orders WHERE id=1 ORDER BY amount DESC LIMIT 100;');
  assert.match(choose('SELECT id FROM orders GROUP BY| LIMIT 100;', {columns:'id, note'}), /GROUP BY id, note LIMIT 100;/);
  assert.match(choose('SELECT * FROM orders WHERE id=1 where| LIMIT 100;', {field:'amount'}), /WHERE id=1 AND amount IS NOT NULL LIMIT 100;/);
});

test('COUNT and DISTINCT modify only the projection and preserve existing predicates',()=> {
  assert.equal(choose('SELECT * FROM orders WHERE id=1 count|;', {field:'id',alias:'total'}), 'SELECT COUNT(id) AS total FROM orders WHERE id=1;');
  assert.equal(choose('SELECT count| FROM orders WHERE id=1;', {field:'*',alias:'row_count'}), 'SELECT COUNT(*) AS row_count FROM orders WHERE id=1;');
  assert.equal(choose('SELECT id FROM orders count|;', {field:'*',alias:'total'}), 'SELECT id, COUNT(*) AS total FROM orders;');
  assert.equal(choose('SELECT * FROM orders distinct| LIMIT 100;', {columns:'note'}), 'SELECT DISTINCT note FROM orders  LIMIT 100;');
});

test('keywords in literals/comments, identifier suffixes and nested queries never trigger',()=> {
  for(const marked of ["SELECT * FROM orders WHERE note='count|'", 'SELECT * FROM orders -- where|', 'SELECT * FROM orders /* group| */', 'SELECT * FROM orders order_id|', 'SELECT * FROM orders WHERE id IN (SELECT count| FROM items)', 'SELECT * FROM orders ORDER BY id|']) {
    assert.equal(incrementalAt(marked.replace('|',''),marked.indexOf('|'),'mysql'),null, marked);
  }
});

test('aggregate operand and alias get their own editable group',()=> {
  const sql='SELECT COUNT(*) AS row_count, COUNT(id) AS total FROM orders;';
  const slots=recognizeSlots(sql,'mysql'), fields=slots.filter(slot=>slot.kind==='countField'), aliases=slots.filter(slot=>slot.kind==='alias');
  assert.deepEqual(fields.map(slot=>sql.slice(slot.from,slot.to)),['*','id']);
  assert.deepEqual(aliases.map(slot=>sql.slice(slot.from,slot.to)),['row_count','total']);
  assert.equal(fields[0].template.id,aliases[0].template.id);
  assert.notEqual(fields[0].template.id,fields[1].template.id);
});

test('multi-column sort applies the direction to every field and keeps comment newlines',()=> {
  assert.equal(choose('SELECT * FROM orders order| LIMIT 10;', {columns:'id, note',direction:'DESC'}), 'SELECT * FROM orders ORDER BY id DESC, note DESC LIMIT 10;');
  assert.match(choose('SELECT * FROM orders -- keep comment\nwhere|;', {field:'id'}), /-- keep comment\n WHERE id IS NOT NULL;/);
});
