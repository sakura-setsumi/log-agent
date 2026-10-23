import test from 'node:test';
import assert from 'node:assert/strict';
import { recognizeStatements, queryAtCursor } from './statements.js';

test('splits query blocks, preserving literals, comments and nested selects', () => {
  for (const engine of ['mysql', 'postgres', 'sqlite']) {
    const source = "-- leading ; comment\nSELECT 'a;b' AS marker;\n/* another ; */\nWITH c AS (SELECT 2) SELECT * FROM c;\nSELECT 3";
    const statements = recognizeStatements(source, engine);
    assert.equal(statements.length, 3, engine);
    assert.equal(statements[0].sql, "SELECT 'a;b' AS marker;");
    assert.equal(statements[1].sql, 'WITH c AS (SELECT 2) SELECT * FROM c;');
    assert.equal(statements[2].sql, 'SELECT 3');
    for (const statement of statements) assert.equal(source.slice(statement.from, statement.to), statement.sql);
  }
});

test('supports same-line queries, quoted names and dialect-specific strings', () => {
  assert.equal(recognizeStatements('SELECT 1; SELECT 2;', 'sqlite').length, 2);
  assert.equal(recognizeStatements('SELECT `semi;name` FROM `orders`;', 'mysql').length, 1);
  assert.equal(recognizeStatements('SELECT "semi;name" FROM "orders";', 'postgres').length, 1);
  assert.equal(recognizeStatements('SELECT $tag$a;b$tag$ AS value; SELECT $$c;d$$;', 'postgres').length, 2);
  assert.equal(recognizeStatements("SELECT E'a\\';b' AS value; SELECT 2;", 'postgres').length, 2);
  assert.equal(recognizeStatements("SELECT 'a'';b'; # ignored ;\nSELECT 2;", 'mysql').length, 2);
  assert.equal(recognizeStatements('SELECT 1 /* outer ; /* inner ; */ done */; SELECT 2;', 'postgres').length, 2);
});

test('does not offer execution actions for unfinished blocks', () => {
  for (const engine of ['mysql', 'postgres', 'sqlite']) {
    for (const source of ['', '-- comment only', '/* comment */', 'SELECT', 'SELECT;', 'SELECT (1 + 2', "SELECT 'not finished", 'SELECT 1 /* not finished', 'SELECT * FROM', 'SELECT 1 WHERE', 'UPDATE', 'UPDATE orders SET', 'INSERT INTO', 'CREATE TABLE']) {
      assert.equal(recognizeStatements(source, engine).length, 0, `${engine}: ${source}`);
    }
  }
  assert.equal(recognizeStatements("SELECT 'escaped\\'", 'mysql').length, 0);
  assert.equal(recognizeStatements('SELECT $tag$not finished', 'postgres').length, 0);
});

test('finds the current statement across mixed queries and writes', () => {
  const source = 'SELECT 1;\nSELECT 2;\nUPDATE orders SET id=3;\n';
  const statements = recognizeStatements(source, 'sqlite');
  assert.equal(queryAtCursor(statements, 5, source)?.sql, 'SELECT 1;');
  assert.equal(queryAtCursor(statements, 15, source)?.sql, 'SELECT 2;');
  assert.equal(queryAtCursor(statements, source.length, source)?.sql, 'UPDATE orders SET id=3;');
  const tail = 'SELECT 1;\n\n';
  assert.equal(queryAtCursor(recognizeStatements(tail), tail.length, tail)?.sql, 'SELECT 1;');
  const adjacent = 'SELECT 1;SELECT 2;';
  assert.equal(queryAtCursor(recognizeStatements(adjacent), 9, adjacent)?.sql, 'SELECT 2;');
});

test('offers execution actions for DML, DDL and CTE writes in every supported dialect', () => {
  for (const engine of ['mysql', 'postgres', 'sqlite']) {
    const source = "SELECT 1; UPDATE orders SET amount=1 WHERE id=1; INSERT INTO orders(id) VALUES(2); DELETE FROM orders WHERE id=2; CREATE TABLE sample(id INT); ALTER TABLE sample ADD name TEXT; DROP TABLE sample; WITH c AS (SELECT 1) DELETE FROM orders WHERE id IN (SELECT * FROM c);";
    const statements = recognizeStatements(source, engine);
    assert.equal(statements.length, 8, engine);
    assert.match(statements[1].sql, /^UPDATE/);
    assert.match(statements[7].sql, /^WITH.*DELETE/);
  }
});

test('qualified wildcard projections remain valid executable statements',()=>{
  const source='SELECT o.*, u.name FROM orders o LEFT JOIN users u ON o.user_id=u.id;';
  assert.equal(recognizeStatements(source,'mysql').length,1);
});
