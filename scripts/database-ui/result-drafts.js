// Result rows keep their loaded values until an explicit commit succeeds.
const equal = (a,b) => JSON.stringify(a) === JSON.stringify(b);
const stable = value => JSON.stringify(Object.entries(value || {}).sort(([a],[b]) => a.localeCompare(b)));
export function resultRowChange(result,row) {
  const edit = result.sources[row.source]?.edit;
  if (!edit) return null;
  const key = {}, original = {}, values = {};
  for (const [label, meta] of Object.entries(edit.columns)) {
    const index = result.columns.findIndex(column => column.name === label);
    const before = row.values[index], after = (row.draft || row.values)[index];
    if (meta.primary) { if (before == null || typeof before === 'object') return null; key[meta.name] = before; }
    if (row.deleted || !equal(before,after)) {
      if (Object.hasOwn(original,meta.name) && !equal(original[meta.name],before)) throw new Error('conflicting drafts');
      original[meta.name] = before;
      if (!row.deleted && !meta.generated) {
        if (Object.hasOwn(values,meta.name) && !equal(values[meta.name],after)) throw new Error('conflicting drafts');
        values[meta.name] = after;
      }
    }
  }
  if (!Object.keys(key).length || !row.deleted && !Object.keys(values).length) return null;
  return { target:edit.target, change:{operation:row.deleted ? 'delete':'update', key, original, ...(row.deleted ? {} : {values})} };
}

// Merge compatible edits to the same physical row across multiple result tabs.
// Each group is one independent source database transaction, possibly many tables.
export function mergeChangeEntries(entries) {
  const groups = new Map();
  for (const entry of entries) {
    const {target,change} = entry, groupID = JSON.stringify([target.id,target.database]);
    if (!groups.has(groupID)) groups.set(groupID,{target:{id:target.id,database:target.database},batches:[],entries:[],rows:new Map(),tables:new Map()});
    const group = groups.get(groupID), tableID = JSON.stringify([target.schema || '',target.table]);
    group.entries.push(entry);
    if (!group.tables.has(tableID)) { const batch = {target,changes:[]}; group.tables.set(tableID,batch); group.batches.push(batch); }
    const rowID = tableID + '/' + stable(change.key), previous = change.operation === 'insert' ? null : group.rows.get(rowID);
    if (previous) {
      if (previous.operation !== change.operation) throw new Error('conflicting drafts');
      for (const name of ['original','values']) for (const [column,value] of Object.entries(change[name] || {})) {
        previous[name] ||= {};
        if (Object.hasOwn(previous[name],column) && !equal(previous[name][column],value)) throw new Error('conflicting drafts');
        previous[name][column] = value;
      }
    } else {
      const copy = structuredClone(change);
      group.tables.get(tableID).changes.push(copy);
      if (change.operation !== 'insert') group.rows.set(rowID,copy);
    }
  }
  return [...groups.values()];
}
