import test from 'node:test';
import assert from 'node:assert/strict';
import { incrementalAt, applyIncrement, statementTables } from './incremental.js';

function pending(marked) {
  const head=marked.indexOf('|'), source=marked.replace('|','');
  const result=incrementalAt(source,head,'mysql');assert.ok(result,marked);
  return {source,result};
}
const values={table:'`users`',alias:'`t2`',conditions:[{left:'o.`user_id`',right:'`t2`.`id`'}]};
for(const keyword of ['inner','LEFT','right','left join','right outer join','INNER JOIN']) {
  test(`${keyword} triggers and joins before the existing WHERE / ORDER / LIMIT`,()=>{
    const {source,result}=pending(`SELECT o.* FROM orders o WHERE o.id=1 ORDER BY o.id LIMIT 100 ${keyword}|;`);
    const sql=applyIncrement(source,result,values);
    assert.equal(sql,`SELECT o.* FROM orders o ${result.kind.toUpperCase()} JOIN \`users\` AS \`t2\` ON o.\`user_id\` = \`t2\`.\`id\` WHERE o.id=1 ORDER BY o.id LIMIT 100;`);
  });
}

test('table occurrences preserve aliases, schema names and self joins',()=>{
  const tables=statementTables('SELECT * FROM `biz`.`users` AS u LEFT JOIN orders o ON u.id=o.user_id INNER JOIN `biz`.`users` t3 ON t3.id=u.parent_id WHERE u.id=1;');
  assert.deepEqual(tables.map(({table,alias,qualifier})=>({table,alias,qualifier})),[
    {table:'`biz`.`users`',alias:'u',qualifier:'u'}, {table:'orders',alias:'o',qualifier:'o'}, {table:'`biz`.`users`',alias:'t3',qualifier:'t3'}]);
});

test('multiple joins can use the preceding joined table and compound keys',()=>{
  const {source,result}=pending('SELECT o.* FROM orders o LEFT JOIN users t2 ON o.user_id=t2.id right|;');
  const sql=applyIncrement(source,result,{table:'teams',alias:'t3',conditions:[{left:'t2.team_id',right:'t3.id'},{left:'t2.tenant_id',right:'t3.tenant_id'}]});
  assert.equal(sql,'SELECT o.* FROM orders o LEFT JOIN users t2 ON o.user_id=t2.id RIGHT JOIN teams AS t3 ON t2.team_id = t3.id AND t2.tenant_id = t3.tenant_id;');
  assert.equal(statementTables(sql).length,3);
});

test('keywords in literals, comments, column names and SELECT functions do not trigger joins',()=>{
  for(const marked of ["SELECT * FROM orders WHERE note='left|'", 'SELECT * FROM orders -- inner|', 'SELECT * FROM orders right_id|', 'SELECT left| FROM orders']) {
    assert.equal(incrementalAt(marked.replace('|',''),marked.indexOf('|'),'mysql'),null,marked);
  }
});
