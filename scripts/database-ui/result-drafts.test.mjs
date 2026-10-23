import test from 'node:test';
import assert from 'node:assert/strict';
import { resultRowChange, mergeChangeEntries } from './result-drafts.js';
const target = {id:'east',database:'main',schema:'main',table:'orders'};
const result = {columns:[{name:'id'},{name:'note'},{name:'amount'}],sources:[{edit:{target,columns:{id:{name:'id',primary:true},note:{name:'note'},amount:{name:'amount'}}}}]};
test('edits keep original keys and values until commit; deletes use the original row',()=>{
 const row={source:0,values:['1',null,'12.5'],draft:['2','new','12.5']};
 assert.deepEqual(resultRowChange(result,row),{target,change:{operation:'update',key:{id:'1'},original:{id:'1',note:null},values:{id:'2',note:'new'}}});
 assert.deepEqual(row.values,['1',null,'12.5']);
 const deletion=resultRowChange(result,{...row,deleted:true});
 assert.equal(deletion.change.operation,'delete');
 assert.deepEqual(deletion.change.original,{id:'1',note:null,amount:'12.5'});
 assert.equal(resultRowChange(result,{source:0,values:['1',null,'12.5']}),null);
 assert.equal(resultRowChange(result,{source:0,values:[null,null,'12.5'],deleted:true}),null);
});
test('compatible edits across pages merge once per physical row and group by database',()=>{
 const a={target,change:{operation:'update',key:{id:'1'},original:{note:null},values:{note:'x'}}};
 const b={target,change:{operation:'update',key:{id:'1'},original:{amount:'12.5'},values:{amount:'25'}}};
 const c={target:{...target,table:'other'},change:{operation:'delete',key:{id:'2'},original:{note:'x'}}};
 const groups=mergeChangeEntries([a,b,c,{...a,target:{...target,id:'west'}}]);
 assert.equal(groups.length,2); assert.equal(groups[0].batches.length,2);
 assert.equal(groups[0].batches[0].changes.length,1);
 assert.deepEqual(groups[0].batches[0].changes[0].values,{note:'x',amount:'25'});
 assert.deepEqual(a.change.values,{note:'x'});
});
test('conflicting edits and update/delete conflicts are rejected before any write',()=>{
 const a={target,change:{operation:'update',key:{id:'1'},original:{note:null},values:{note:'x'}}};
 assert.throws(()=>mergeChangeEntries([a,{...a,change:{...a.change,values:{note:'y'}}}]),/conflicting/);
 assert.throws(()=>mergeChangeEntries([a,{...a,change:{operation:'delete',key:{id:'1'},original:{note:null}}}]),/conflicting/);
});
