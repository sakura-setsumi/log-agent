import test from 'node:test';
import assert from 'node:assert/strict';
import {identifierParts,splitSQLList,insertRows,bulkRows,templateLiteral} from './template-input.js';
test('identifier matching accepts qualifications and quoted names without changing them',()=>{
 assert.deepEqual(identifierParts('`biz_db`.`sys_user`'),['biz_db','sys_user']);
 assert.deepEqual(identifierParts('"odd.table"'),['odd.table']);
 assert.deepEqual(identifierParts('"odd""name"'),['odd"name']);
 assert.deepEqual(identifierParts('{{table}}'),[]);
 assert.deepEqual(splitSQLList('"a,b", "c"'),['"a,b"','"c"']);
});
test('insert rows preserve quoted commas, functions and multiple tuples',()=>{
 assert.deepEqual(insertRows("1, 'a,b'), (2, 'O''Brien'), (3, COALESCE(NULL, 'x'))".slice(0,-1),'sqlite'),[['1',"'a,b'"],['2',"'O''Brien'"],['3',"COALESCE(NULL, 'x')"]]);
 assert.deepEqual(insertRows('NULL, NULL','mysql'),[['NULL','NULL']]);
 assert.throws(()=>insertRows("1, 'unfinished",'sqlite'));
});
test('bulk CSV and spreadsheet pastes preserve NULL, quotes and embedded lines',()=>{
 assert.deepEqual(bulkRows('1\tAlice\r\n2\tNULL\r\n3\t"NULL"'),[['1','Alice'],['2',null],['3','NULL']]);
 assert.deepEqual(bulkRows('1,"a,b"\n2,"a""b"\n3,"two\nlines"\n'),[['1','a,b'],['2','a"b'],['3','two\nlines']]);
 assert.deepEqual(bulkRows('1,\n2,hello'),[['1',''],['2','hello']]);
 assert.throws(()=>bulkRows('1,"unfinished'));
});
test('literal formatting preserves numeric precision and escapes textual values',()=>{
 assert.equal(templateLiteral('9223372036854775807','sqlite','bigint'),'9223372036854775807');
 assert.equal(templateLiteral('1e5','postgres','numeric'),'1e5');
 assert.equal(templateLiteral("O'Brien",'sqlite','text'),"'O''Brien'");
 assert.equal(templateLiteral('NULL','sqlite','text'),"'NULL'");
 assert.equal(templateLiteral(null,'mysql','text'),'NULL');
 assert.equal(templateLiteral('C:\\data','mysql','text'),"'C:\\\\data'");
});
