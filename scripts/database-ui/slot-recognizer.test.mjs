import test from 'node:test';
import assert from 'node:assert/strict';
import { recognizeSlots } from './slot-recognizer.js';

const pieces = sql => recognizeSlots(sql, 'mysql').map(slot => `${slot.kind}:${sql.slice(slot.from, slot.to)}`);

test('select with a comparison keeps table, field, operator and value selectable', () => {
  assert.deepEqual(pieces("SELECT * FROM `sys_user` WHERE `phonenumber` = '1830405' LIMIT 100;"), ['columns:*', 'table:`sys_user`', 'field:`phonenumber`', 'operator:=', "filterValue:'1830405'"]);
});

test('every statement of a script is recognized on its own', () => {
  const sql = "SELECT * FROM `a` WHERE `b` = 'x;y' LIMIT 100;\n\nDELETE FROM `a` WHERE `b` = '1';";
  assert.deepEqual(pieces(sql), ['columns:*', 'table:`a`', 'field:`b`', 'operator:=', "filterValue:'x;y'", 'table:`a`', 'field:`b`', 'operator:=', "filterValue:'1'"]);
});

test('IS NULL has no value and AND adds another predicate', () => {
  assert.deepEqual(pieces('SELECT id, name FROM t WHERE a IS NOT NULL AND b IN (1, 2) ORDER BY id DESC;'), ['columns:id, name', 'table:t', 'field:a', 'operator:IS NOT NULL', 'field:b', 'operator:IN', 'filterValue:(1, 2)', 'field:id']);
  assert.deepEqual(pieces('SELECT * FROM t WHERE a BETWEEN 1 AND 5 AND b = 2;'), ['columns:*', 'table:t', 'field:a', 'operator:BETWEEN', 'filterValue:1 AND 5', 'field:b', 'operator:=', 'filterValue:2']);
});

test('update and insert expose their table, fields and values', () => {
  assert.deepEqual(pieces("UPDATE `t` SET `n` = 'x' WHERE `id` = 3;"), ['table:`t`', 'field:`n`', "value:'x'", 'field:`id`', 'operator:=', 'filterValue:3']);
  assert.deepEqual(pieces("INSERT INTO s.t (a, b) VALUES (1, 'two');"), ['table:s.t', 'columns:a, b', "values:1, 'two'"]);
});

test('unrecognized text gets no slots', () => {
  assert.deepEqual(pieces('SHOW TABLES;'), []);
  assert.deepEqual(pieces('SELECT 1;'), []);
});
