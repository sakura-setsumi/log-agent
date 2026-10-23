// Database workspace. Mature editor/grid components are bundled locally; all
// database traffic goes through the existing in-process Go bridge.
const databaseLabels = {
  insertValues: ['Edit inserted values', '编辑插入值'], addValueRow: ['Add row', '添加一行'], bulkValues: ['Paste multiple rows (CSV / spreadsheet)', '粘贴批量值（CSV / 表格）'], bulkValuesHint: ['Column order matches the selected fields. Paste tab-separated or comma-separated rows. Unquoted NULL means database NULL.', '列顺序与已选字段一致；支持粘贴表格或逗号分隔的多行数据，未加引号的 NULL 表示数据库空值。'], importValues: ['Add pasted rows', '添加批量值'], invalidValues: ['Check the value format and number of selected columns.', '请检查值的格式，每行列数须与已选字段数一致。'],

  commitAll: ['Commit all pending changes', '提交全部待提交修改'], resultPending: ['{count} pending row change(s) · Commit to save', '{count} 行待提交 · 点击提交后生效'], pendingPages: ['{count} page(s) with pending changes', '{count} 个页面有待提交修改'], transactionDone: ['Committed {count} row change(s)', '已提交 {count} 行修改'], transactionPartial: ['Some source databases failed; their pending changes are retained. Successful transactions remain committed.', '部分来源数据库提交失败，其修改仍保留待提交；成功事务已提交。'], conflictingDrafts: ['The same row has conflicting changes across pages. Resolve them or roll back one page before committing.', '同一行在多个页面有冲突的修改，请先调整或回滚其中一页。'],

  manageConnections: ['Database manager…', '数据库管理…'], searchConnections: ['Search connection, host or user', '搜索连接名称、主机或用户名'], noConnectionMatches: ['No matching connections', '没有匹配的连接'], close: ['Close', '关闭'], showPassword: ['Show', '显示'], hidePassword: ['Hide', '隐藏'], savedPassword: ['Saved password (blank keeps it)', '已保存密码（留空保留）'],
  resultEditHint: ['Click a cell to edit; Edits and deletions are staged until you click Commit. Right-click to set NULL.', '编辑和删除先暂存，点击提交后写入数据库；支持多选删除，右键可设为 NULL。'], resultReadOnly: ['Editing requires a direct single-table query with its full primary key. Joins, aggregates and expressions are read-only.', '仅可修改包含完整主键的单表直接查询结果；关联、聚合和表达式结果只读。'], resultSaving: ['Saving to the source database…', '正在保存到来源数据库…'], resultSaved: ['Saved: {source} / {column}', '已保存：{source} / {column}'],
  subtitle: ['Query several databases with one SQL statement and one combined result.', '连接多个数据库，用一条 SQL 汇总查询结果。'],
  newConnection: ['New connection…', '新建连接…'], newItem: ['New', '新建'], newQuery: ['New SQL tab', '新建 SQL 页签'], connections: ['Connections & objects', '连接与结构'], refresh: ['Refresh', '刷新'], chooseObject: ['Choose a table or check databases to query', '选择数据表，或勾选多个库查询'],
  chooseTargets: ['Choose target databases', '选择目标数据库'], searchTargets: ['Search connections or databases', '搜索连接或数据库'], targetsLimit: ['{count} selected · maximum 16 databases', '已选 {count} 个库 · 最多 16 个'], noTargetMatches: ['No matching databases', '没有匹配的数据库'], emptyDatabases: ['No databases', '暂无数据库'], expandConnection: ['Expand or collapse {name}', '展开或收起 {name}'],
  data: ['Data', '数据'], structure: ['Structure', '结构'], sql: ['SQL query', 'SQL 查询'], addRow: ['Add row', '新增行'], deleteRows: ['Delete selected rows', '删除选中行'], saveRows: ['Save changes', '保存修改'], reload: ['Reload', '重新加载'], editHint: ['Select rows: Ctrl+C; add row: Ctrl+V; click to edit; Ctrl+S to save. Right-click for NULL.', '选中行 Ctrl+C，新增行 Ctrl+V，单击编辑，Ctrl+S 保存；右键可设为 NULL。'],
  invalidRowPaste: ['Cannot paste: the clipboard columns do not match this table or the row format is invalid.', '无法粘贴：剪贴板字段与当前表不匹配，或行数据格式无效。'],
  rowActions: ['Row actions', '行操作'], commitRows: ['Commit changes (Ctrl+S)', '提交修改（Ctrl+S）'], rollbackRows: ['Roll back pending changes', '回滚未提交的修改'], stopLoading: ['Stop loading', '停止加载'], loadingStopped: ['Loading stopped', '已停止加载'],
  previous: ['Previous', '上一页'], next: ['Next', '下一页'], last: ['Last', '尾页'], createTable: ['Create table', '新建表'], addColumn: ['Add column', '添加字段'], alterColumn: ['Edit column', '修改字段'], renameColumn: ['Rename column', '重命名字段'], dropColumn: ['Drop column', '删除字段'], createIndex: ['Create index', '新增索引'], dropIndex: ['Drop index', '删除索引'], renameTable: ['Rename table', '重命名表'], dropTable: ['Drop table', '删除表'], columns: ['Columns — select one to edit', '字段（选中后可修改）'], indexes: ['Indexes', '索引'],
  runQuery: ['Run SQL · Ctrl+Enter', '执行 SQL · Ctrl+Enter'], execute: ['Execute statement (one DB)', '执行语句（单库）'], cancel: ['Cancel', '取消'], clearTargets: ['Clear selection', '清空选择'], exportCsv: ['Export CSV', '导出 CSV'], queryHint: ['Choose databases in the target dropdown. SQL runs on each; result columns are aligned and combined.', '在目标数据库下拉框中多选，同一条 SQL 分别执行，返回结果按列名对齐合并。'],
  connection: ['Database connection', '数据库连接'], connectionName: ['Connection name', '连接名称'], engine: ['Database type', '数据库类型'], host: ['Host', '主机'], port: ['Port', '端口'], user: ['User', '用户名'], password: ['Password (blank keeps the saved password)', '密码（留空保留已保存密码）'], defaultDatabase: ['Default database (optional)', '默认数据库（可留空）'], tlsDisable: ['Disabled', '不启用'], tlsRequire: ['Encrypted connection', '加密连接'], tlsVerify: ['Encrypted, verify certificate', '加密并验证证书'], sqliteFile: ['Absolute SQLite file path (created if missing)', 'SQLite 文件绝对路径（不存在时创建）'], deleteConnection: ['Delete connection', '删除连接'], testConnection: ['Test connection', '测试连接'], save: ['Save', '保存'],
  designer: ['Schema designer', '结构设计器'], operation: ['Operation', '操作'], currentObject: ['Current object', '当前对象'], previewSql: ['Preview SQL', '预览 SQL'], applySchema: ['Apply schema changes', '应用结构变更'],
  emptyConnections: ['No connections yet. Add MySQL, PostgreSQL or SQLite.', '暂无连接，可新增 MySQL、PostgreSQL 或 SQLite。'], fetching: ['Fetching', '正在获取数据'], fetchedRecords: ['Fetched records', '已获取记录'], stopFetch: ['Stop', '停止'], loading: ['Loading…', '正在加载…'], emptyObjects: ['No tables or views', '暂无表或视图'], table: ['Table', '表'], view: ['View', '视图'], editConnection: ['Edit connection', '编辑连接'], confirmDeleteConnection: ['Delete this connection configuration?', '确定删除该连接配置吗？'], connected: ['Connection successful', '连接成功'], saved: ['Saved', '已保存'], name: ['Name', '名称'], type: ['Type', '类型'], nullable: ['Nullable', '允许 NULL'], primary: ['Primary key', '主键'], default: ['Default SQL expression', '默认 SQL 表达式'], generated: ['Generated', '生成列'], unique: ['Unique', '唯一'], indexColumns: ['Columns (comma-separated, in index order)', '字段（逗号分隔，按索引顺序）'], auto: ['Auto increment / identity', '自增 / 标识列'], remove: ['Remove', '移除'], addField: ['Add a column', '添加一列'], column: ['Column', '字段'], newName: ['New name', '新名称'], tableName: ['Table name', '表名'], indexName: ['Index name', '索引名'],
  chooseDatabase: ['Choose a database in the tree first', '请先点击左侧的数据库'], chooseTable: ['Open a table first', '请先打开一个数据表'], chooseColumn: ['Select a column in the structure grid first', '请先在结构列表中选中字段'], chooseIndex: ['Select a non-primary index first', '请先选中一个非主键索引'], readOnly: ['No primary key or view: existing rows are read-only; use SQL for custom changes.', '无主键表或视图的现有记录只读，可通过 SQL 自行修改。'], setNull: ['Set to NULL', '设为 NULL'], discard: ['Discard unsaved row changes?', '放弃未保存的数据修改吗？'], dirty: ['{count} pending row change(s)', '{count} 行待保存'], rows: ['{count} rows loaded', '已加载 {count} 行'], page: ['Page {page}', '第 {page} 页'], savedRows: ['Committed {count} row change(s)', '已提交 {count} 行修改'],
  targets: ['{count} database target(s)', '目标数据库：{count} 个'], noTargets: ['Choose databases in the target dropdown first.', '请先在目标数据库下拉框中选择数据库。'], maxTargets: ['At most 16 databases per execution', '单次最多对 16 个数据库执行'], running: ['Running SQL on {count} database(s)…', '正在对 {count} 个数据库执行 SQL…'], emptySql: ['Enter or select one SQL statement', '请输入或选中一条 SQL 语句'], queryDone: ['{rows} rows · {ok}/{total} databases succeeded · {ms} ms', '{rows} 行 · {ok}/{total} 个库成功 · {ms} ms'], sourceConnection: ['Source connection', '来源连接'], sourceDatabase: ['Source database', '来源数据库'], sourceEngine: ['Engine', '数据库类型'], truncated: ['Result limited by the row/size cap', '结果已达到行数或大小上限'], partial: ['Some databases failed; successful results remain in the grid.', '部分数据库查询失败，表格保留已成功数据库的结果。'], cancelled: ['Query cancelled', '已取消查询'], executed: ['Statement executed; {count} affected row(s)', '语句执行完成，影响 {count} 行'], details: ['Per-database results', '各数据库结果'], crossHint: ['SQL runs unchanged on every selected database. Use syntax and table names supported by those targets.', 'SQL 原文分别在各库执行，请使用各目标都支持的语法和表名。'],
  previewFirst: ['Generate and review the SQL preview before applying.', '请先生成并检查 SQL 预览，再应用变更。'], transaction: ['These schema changes run in a transaction.', '结构变更在事务中执行。'], autoCommit: ['This database may auto-commit DDL; each statement is shown above.', '该数据库可能自动提交 DDL，上方列出了将执行的语句。'], applied: ['Schema changes applied', '结构变更已应用'], nothingToChange: ['No schema changes', '结构未发生变化'],
  noResults: ['No data', '暂无数据'], executionDone: ['{ok}/{total} databases succeeded · {count} affected row(s) reported · {ms} ms', '{ok}/{total} 个库成功 · 已报告影响 {count} 行 · {ms} ms'], affectedRows: ['{count} affected row(s)', '影响 {count} 行'], affectedTitle: ['Affected rows', '影响行数'], executionStatus: ['Status', '执行状态'], completed: ['Completed', '已完成'], failed: ['Failed', '失败'], duration: ['Duration (ms)', '耗时（毫秒）'], errorDetail: ['Error', '错误详情'], unknownAffected: ['Not reported', '未报告'], unknownAffectedSources: ['{count} database(s) did not report affected rows', '{count} 个库未报告影响行数'], independentExecution: ['Each database executes independently. Successful changes remain committed if another database fails.', '各库独立执行；部分失败时，已成功库的修改不会回滚。'], executionCancelled: ['Execution cancelled. Completed changes remain; verify the outcome of databases that were still running.', '执行已取消。已完成的修改仍保留；正在执行的库请核实最终结果。'], executionPartial: ['Some databases failed. Review each database result before retrying; successful changes remain applied.', '部分数据库执行失败，请逐库检查后再重试；已成功的修改仍然生效。'], inlineOperator: ['Choose a comparison operator', '选择条件运算符'], operatorHint: ['Choose an operator and enter the value to compare; leave it empty to edit it in the SQL text later.', '选择运算符并输入比较值；留空则稍后直接在 SQL 文本中修改。'], operandValue: ['Comparison value', '比较值'], operandList: ['Values (comma separated)', '值列表（逗号分隔）'], operandFrom: ['From', '起始值'], operandTo: ['To', '结束值'],
  joinPicker: ['Join tables', '联表选择器'], joinExisting: ['Existing table', '已在语句中的表'], joinTable: ['Table to join', '关联新表'], joinAlias: ['New table alias', '新表别名'], joinLeftField: ['Existing table column', '已有表字段'], joinRightField: ['New table column', '新表字段'], joinAddCondition: ['Add join condition', '添加关联条件'], joinAliasError: ['Use a nonempty alias different from existing tables', '请填写与已有表不同的非空别名'], joinMissingTable: ['Table schema could not be resolved', '无法读取该表的字段结构'], joinHint: ['Add the join to this statement. Keep typing INNER / LEFT / RIGHT to join another table.', '加入当前语句后，可继续输入 INNER / LEFT / RIGHT 关联下一张表。'], countColumn: ['Count column', '统计字段'], resultColumnName: ['Result column name', '结果列名（行名）'], sortDirection: ['Sort direction', '排序方向'], allRows: ['All rows (*)', '所有行（*）'], aliasRequired: ['Enter a result column name', '请输入结果列名'], incrementalHint: ['Apply to the current statement; existing clauses are preserved.', '增量加入当前语句，保留已有条件和子句。'], templates: ['SQL templates', '模板语句'], templateHint: ['Choose a base statement, or type WHERE / ORDER / COUNT / DISTINCT / GROUP / INNER / LEFT / RIGHT in SQL to add a fragment.', '选择基础语句；在 SQL 中输入 WHERE / ORDER / COUNT / DISTINCT / GROUP / INNER / LEFT / RIGHT，弹出选择器增量补充当前语句。'], templateSource: ['Schema source', '结构来源'], templateColumns: ['Select columns', '选择字段'], selectAll: ['All', '全选'], selectNone: ['Clear', '清空'], templateField: ['Filter / sort / group column', '条件 / 排序 / 分组字段'], templateWriteHint: ['Templates update the current statement, or insert a new statement on an empty line. Edit values and conditions before execution.', '当前行有语句时修改该语句，空白行新增语句；值和条件可直接在文本中修改。'], templateFilled: ['Filled template: {name}', '已填入模板：{name}'],
  singleTarget: ['Single-database target', '单库目标'], singleQueryTitle: ['Run SQL on one database: {target}', '单库执行：{target}'], multiQueryTitle: ['Run SQL on {count} checked databases', '多库执行：已勾选的 {count} 个数据库'], chooseMultiple: ['Check at least two databases for multi-database execution.', '多库执行请至少勾选两个数据库。'],
  templateCreate: ['Create table', '创建表'], templateDrop: ['Drop table', '删除表'], templateAlter: ['Alter table', '修改表结构'], schemaName: ['Schema', '模式 / 数据库'], templateSchemaHint: ['Preview the generated SQL, then fill it into this SQL tab for editing and execution.', '预览生成的 SQL 后填入当前页签，可继续编辑并手动执行。'], templateSqlChanged: ['The SQL tab changed. Open this template again.', 'SQL 页签已变化，请重新打开模板。'], templateSelect: ['Select columns', '查询所选字段'], templateWhere: ['Filter non-empty values', '条件查询'], templateOrder: ['Sort results', '排序查询'], templateCount: ['Count rows', '统计行数'], templateDistinct: ['Distinct values', '去重查询'], templateGroup: ['Group and count', '分组统计'], templateInsert: ['Insert row', '插入数据'], templateUpdate: ['Update rows', '更新数据'], templateDelete: ['Delete rows', '删除数据'],
  addSqlTab: ['New SQL tab', '新建 SQL 页签'], tableTabs: ['Tables / results', '表 / 结果'], results: ['Results', '结果'], closeTab: ['Close tab', '关闭页签'], closeSqlConfirm: ['Close this SQL tab? Its text and any running query will be discarded.', '关闭此 SQL 页签？其中的文本将丢失，运行中的查询会取消。'], emptyWorkspace: ['Open a table from the tree or execute SQL above.', '打开左侧数据表，或在上方执行 SQL 查询。'], inlineTable: ['Click to choose a table', '点击选择表名'], inlineColumns: ['Click to choose columns', '点击选择字段'], inlineField: ['Click to choose a column', '点击选择条件 / 排序 / 分组字段'], inlineValue: ['Click and type a value', '点击后输入值'], inlineCondition: ['Click and edit the condition', '点击后编辑条件'], applyInline: ['Fill SQL', '填入 SQL'],
};
for (const [key, values] of Object.entries(databaseLabels)) { i18n.en[`db.${key}`] = values[0]; i18n.zh[`db.${key}`] = values[1]; }
i18n.en['nav.database'] = 'Database'; i18n.zh['nav.database'] = '数据库';

(() => {
  const view = document.getElementById('database-view');
  if (!view) return;
  const el = id => document.getElementById(id);
  const text = (key, values) => t(`db.${key}`, values);
  const escaped = value => escapeHtml(String(value ?? ''));
  const ui = { initialized: false, profiles: [], catalogs: new Map(), objects: new Map(), checked: new Map(), targetsExplicit: false, focus: null, table: null, tableResponse: null, original: new Map(), dirty: 0, page: 0, tab: 'data', grids: {}, query: null, controller: null, sequence: 0, schemaPlan: null, sqlTabs: [], tableTabs: [], bottomTabs: [], activeSQL: null, activeBottom: null, sqlSequence: 0, tableSequence: 0 };
  const key = target => JSON.stringify([target.id, target.database]);
  const profile = id => ui.profiles.find(item => item.id === id);
  const sourceName = target => `${profile(target.id)?.name || target.id} / ${target.database || '—'}`;
  const params = target => new URLSearchParams(Object.entries(target || {}).filter(([, value]) => value !== undefined && value !== null)).toString();
  const endpoint = (action, target) => `/api/database/${action}${target ? `?${params(target)}` : ''}`;
  const clone = value => structuredClone(value);
  async function api(action, { target, body, method, signal, progress } = {}) {
    const response = await fetch(endpoint(action, progress ? {...target,request_id:progress.id} : target), { method: method || (body === undefined ? 'GET' : 'POST'), headers: { Accept: 'application/json', ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) }, body: body === undefined ? undefined : JSON.stringify(body), signal });
    const result = await response.json();
    if (!response.ok) throw new Error(result.detail || result.error || `HTTP ${response.status}`);
    return result;
  }
  let databaseLoadTimer=null;
  function syncDatabaseLoads() {
    const item=ui.activeBottom;
    for(const name of ['data','query']) {
      const owner=name==='data' && item?.kind==='table'?tableSession():name==='query' && item?.kind==='result'?ui.sqlTabs.find(tab=>tab.id===item.id):null;
      const progress=owner?.fetchProgress, busy=Boolean(progress?.active);
      const overlay=el(`db-${name}-loading`),panel=el(name==='data'?'db-panel-data':'db-panel-result');
      overlay.classList.toggle('hidden',!busy);panel.setAttribute('aria-busy',String(busy));
      if(busy) {
        el(`db-${name}-load-time`).textContent=((performance.now()-progress.started)/1000).toFixed(1)+' s';
        const records=el(`db-${name}-load-records`),value=progress.records.toLocaleString();
        if(records.textContent!==value)records.textContent=value;
      }
    }
  }
  function beginDatabaseLoad(owner,controller) {
    const progress={id:crypto.randomUUID(),started:performance.now(),records:0,active:true,controller,pollController:new AbortController()};
    owner.fetchProgress=progress;
    const poll=async()=>{
      if(!progress.active) return;
      try {
        const result=await api('progress',{target:{request_id:progress.id},signal:progress.pollController.signal});
        if(progress.active){progress.records=Math.max(progress.records,Number(result.records)||0);syncDatabaseLoads();}
        if(result.done) return;
      } catch(_) {if(!progress.active) return;}
      if(progress.active) progress.poll=setTimeout(poll,250);
    };
    progress.poll=setTimeout(poll,200);
    if(databaseLoadTimer===null)databaseLoadTimer=setInterval(syncDatabaseLoads,100);
    syncDatabaseLoads();return progress;
  }
  function finishDatabaseLoad(owner,progress) {
    progress.active=false;clearTimeout(progress.poll);progress.pollController.abort();
    if(owner.fetchProgress===progress)delete owner.fetchProgress;
    syncDatabaseLoads();
    if(![...ui.sqlTabs,...ui.tableTabs].some(tab=>tab.fetchProgress?.active)) {clearInterval(databaseLoadTimer);databaseLoadTimer=null;}
  }
  function status(message, error = false) { el('db-status').textContent = message; el('db-status').classList.toggle('error', error); }
  function dialogStatus(name, message, error = false) { const output = el(`db-${name}-status`); output.textContent = message; output.classList.toggle('error', error); }
  const dirtyGuard = () => !ui.saving && (!ui.dirty || window.confirm(text('discard')));
  const quote = (engine, name) => { const mark = engine === 'mysql' ? '`' : '"'; return mark + name.replaceAll(mark, mark + mark) + mark; };
  const templates = { open: false };
  const templateDefinitions = [
    ['select', 'templateSelect', 'SELECT'], ['insert', 'templateInsert', 'INSERT'], ['update', 'templateUpdate', 'UPDATE'], ['delete', 'templateDelete', 'DELETE'],
    ['create', 'templateCreate', 'CREATE TABLE'], ['drop', 'templateDrop', 'DROP TABLE'], ['alter', 'templateAlter', 'ALTER TABLE'],
  ];
  const templateSQL = {
    select: 'SELECT {{columns}} FROM {{table}} LIMIT 100;',
    where: 'SELECT {{columns}} FROM {{table}} WHERE {{field}} {{operator}} LIMIT 100;',
    order: 'SELECT {{columns}} FROM {{table}} ORDER BY {{field}} ASC LIMIT 100;',
    count: 'SELECT COUNT({{countField}}) AS {{alias}} FROM {{table}};',
    distinct: 'SELECT DISTINCT {{columns}} FROM {{table}} LIMIT 100;',
    group: 'SELECT {{field}}, COUNT(*) AS row_count FROM {{table}} GROUP BY {{field}} LIMIT 100;',
    insert: 'INSERT INTO {{table}} ({{columns}}) VALUES ({{values}});',
    update: 'UPDATE {{table}} SET {{field}} = {{value}} WHERE {{condition}};',
    delete: 'DELETE FROM {{table}} WHERE {{condition}};',
  };
  const inline = { nonce: 0, open: false };
  function syncTemplates() {
    el('db-template-list').innerHTML = templateDefinitions.map(([id, label, sql]) => `<button type="button" data-db-template="${id}"><span>${text(label)}</span><code>${sql}</code></button>${id === 'delete' ? '<hr class="db-template-divider"/>' : ''}`).join('');
  }
  function toggleTemplates(open) {
    if (open) toggleTargetPicker(false);
    templates.open = open; el('db-template-panel').classList.toggle('hidden', !open); el('db-toggle-templates').setAttribute('aria-expanded', String(open));
    syncTemplates(); if (open) positionTemplates(); requestAnimationFrame(resizeGrids);
  }
  function positionTemplates() {
    if (!templates.open) return;
    const panel = el('db-template-panel'), anchor = el('db-toggle-templates').getBoundingClientRect();
    panel.style.left = Math.max(8, Math.min(anchor.right - panel.offsetWidth, innerWidth - panel.offsetWidth - 8)) + 'px';
    const top = Math.max(8, Math.min(anchor.bottom + 6, innerHeight - 160));
    panel.style.top = top + 'px'; panel.style.maxHeight = Math.max(120, innerHeight - top - 12) + 'px';
  }
  function inlineTableName(context) {
    const engine = profile(context.target.id)?.engine;
    return (engine === 'postgres' ? quote(engine, context.object.schema) + '.' : '') + quote(engine, context.object.name);
  }
  function templateValues(context, id) {
    const engine = context ? profile(context.target.id)?.engine : null, write = id === 'insert';
    const columns = context?.columns.filter(c => !write || !c.generated) || [];
    const chosen = columns.filter(c => context?.selectedFields?.includes(c.name));
    const names = chosen.length ? chosen : write ? columns : [];
    const first = columns.find(c => c.primary) || columns[0];
    return { table: context ? inlineTableName(context) : '{{table}}', columns: names.length ? names.map(c => quote(engine, c.name)).join(', ') : write ? '{{columns}}' : '*', field: first ? quote(engine, first.name) : '{{field}}', countField: '*', alias: 'row_count', values: Array(Math.max(1, names.length)).fill('NULL').join(', '), value: 'NULL', condition: '1 = 0' };
  }
  function slotLabels() { return { table: text('inlineTable'), columns: text('inlineColumns'), field: text('inlineField'), operator: text('inlineOperator'), values: text('inlineValue'), value: text('inlineValue'), filterValue: text('inlineValue'), condition: text('inlineCondition'), countField: text('countColumn'), alias: text('resultColumnName') }; }
  function fillTemplate(id) {
    const tab = sqlSession(); if (!tab) return;
    if (['create','drop','alter'].includes(id)) { openTemplateDesigner(id); return; }
    if (!templateSQL[id]) return;
    closeInlinePicker();
    const currentTemplate = ui.editor.templateAtCursor();
    if (currentTemplate) tab.templateContext = currentTemplate.context ? clone(currentTemplate.context) : null;
    if (!tab.templateContext && ui.tableResponse && singleQueryTarget() && key(singleQueryTarget()) === key(ui.table)) tab.templateContext = { target: { id: ui.table.id, database: ui.table.database }, object: { schema: ui.table.schema, name: ui.table.table, kind: ui.table.kind }, columns: clone(ui.tableResponse.structure.columns), selectedFields: [] };
    tab.templateId = id;
    ui.editor.setTemplate(templateSQL[id], { ...templateValues(tab.templateContext, id), operator: 'IS NOT NULL' }, { table: text('inlineTable'), columns: text('inlineColumns'), field: text('inlineField'), operator: text('inlineOperator'), values: text('inlineValue'), value: text('inlineValue'), condition: text('inlineCondition') }, { templateId: id, context: tab.templateContext ? clone(tab.templateContext) : null });
    ui.editor.focus(); status(text('templateFilled', { name: text(templateDefinitions.find(item => item[0] === id)[1]) }));
  }
  function closeInlinePicker() { el('db-inline-picker').classList.remove('db-inline-picker-values', 'db-inline-picker-join'); inline.open = false; inline.nonce++; el('db-inline-picker').classList.add('hidden'); }
  function inlineMessage(message, error = false) { el('db-inline-status').textContent = message; el('db-inline-status').classList.toggle('error', error); }
  function positionInlinePicker(id) {
    const rect = ui.editor.slotRect(id), picker = el('db-inline-picker');
    const anchor = rect || el('db-sql-editor').getBoundingClientRect();
    picker.style.left = Math.max(8, Math.min(anchor.left, innerWidth - (picker.offsetWidth || 350) - 8)) + 'px';
    picker.style.top = Math.max(8, Math.min(anchor.bottom + 5, innerHeight - (picker.offsetHeight || Math.min(390, picker.scrollHeight)) - 12)) + 'px';
  }
  function contextMatchesTable(context, raw) {
    if (!context) return false;
    const parts = DatabaseUI.templateInput.identifierParts(raw || '');
    return parts.at(-1) === context.object.name && (parts.length === 1 || parts.length === 2 && [context.object.schema, context.target.database].includes(parts[0]));
  }
  async function resolveTemplateContext(tab) {
    const raw = ui.editor.slotText('table'), saved = ui.editor.templateContext();
    if (contextMatchesTable(saved, raw)) return clone(saved);
    const target = saved?.target || singleQueryTarget(), parts = DatabaseUI.templateInput.identifierParts(raw || '');
    if (!target || !parts.length) return null;
    const objects = ui.objects.get(key(target))?.items || await api('objects',{target});
    const object = objects.find(object => contextMatchesTable({target,object},raw));
    if (!object) return null;
    const structure = await api('structure',{target:{...target,schema:object.schema,table:object.name}});
    return {target:clone(target),object:clone(object),columns:structure.columns,selectedFields:[]};
  }
  async function openInlinePicker(slot, requestedKind = slot.kind) {
    if (requestedKind === 'countField') requestedKind = 'count';
    // A bare condition (DELETE/UPDATE ... WHERE) is picked as field + operator, like the WHERE template.
    const isCondition = slot.kind === 'condition'; if (isCondition && requestedKind === 'condition') requestedKind = 'field';
    if (!isCondition && ['value', 'values', 'filterValue'].includes(slot.kind) && !(slot.kind === 'values' && slot.template?.templateId === 'insert')) { closeInlinePicker(); ui.editor.selectSlot(slot.id); return; }
    const tab = sqlSession(); if (!tab) return;
    if (slot.template) { tab.templateId = slot.template.templateId; tab.templateContext = slot.template.context ? clone(slot.template.context) : null; }
    closeInlinePicker(); inline.open = true; inline.sqlId = tab.id; inline.slotId = slot.id; inline.condition = isCondition; inline.kind = requestedKind; inline.pendingKind = null; inline.incremental = slot.incremental || null; inline.context = tab.templateContext ? clone(tab.templateContext) : null;
    if (requestedKind === 'alias') { renderResultAlias(); return; }
    if (requestedKind !== 'table') {
      const saved = ui.editor.templateContext();
      if (contextMatchesTable(saved, ui.editor.slotText('table'))) tab.templateContext = clone(saved);
      else {
        const nonce = inline.nonce;
        el('db-inline-title').textContent = text('loading'); el('db-inline-body').innerHTML = ''; el('db-inline-apply').disabled = true;
        el('db-inline-picker').classList.remove('hidden'); positionInlinePicker(slot.id);
        let context = null;
        try { context = await resolveTemplateContext(tab); } catch (_) { /* An explicit table picker remains available. */ }
        if (!inline.open || inline.nonce !== nonce || inline.sqlId !== ui.activeSQL) return;
        tab.templateContext = context;
        if (context) ui.editor.setTemplateContext(clone(context));
      }
    }
    if (requestedKind === 'operator') {
      el('db-inline-title').textContent = text('inlineOperator');
      const operators = ['IS NOT NULL', 'IS NULL', '=', '<>', '>', '>=', '<', '<=', 'LIKE', 'NOT LIKE', 'IN', 'NOT IN', 'BETWEEN', 'NOT BETWEEN'];
      el('db-inline-body').innerHTML = `<label class="db-template-label"><span>${text('inlineOperator')}</span><select id="db-inline-operator">${operators.map(value => `<option value="${escaped(value)}">${escaped(value)}</option>`).join('')}</select></label>${[['single', 'db-inline-value', 'operandValue'], ['list', 'db-inline-list', 'operandList'], ['range', 'db-inline-from', 'operandFrom'], ['range', 'db-inline-to', 'operandTo']].map(([group, id, label]) => `<label class="db-template-label" data-operand="${group}"><span>${text(label)}</span>${group === 'list' ? `<textarea id="${id}" rows="3" spellcheck="false"></textarea>` : `<input type="text" id="${id}" autocomplete="off" spellcheck="false"/>`}</label>`).join('')}<p class="db-template-hint">${text('operatorHint')}</p>`;
      const current = ui.editor.slotText('operator'); if (operators.includes(current)) el('db-inline-operator').value = current;
      const group = operandGroup(current), shown = group === 'none' ? [] : operandDisplay(group, ui.editor.slotText('filterValue'), operandEngine());
      inline.operandGroup = group; inline.operandShown = shown;
      operandInputs(group).forEach((input, index) => { input.value = shown[index] || ''; });
      el('db-inline-picker').classList.remove('hidden'); el('db-inline-apply').disabled = false; inlineMessage(''); syncOperandInputs(); positionInlinePicker(slot.id); return;
    }
    if (requestedKind !== 'table' && !contextMatchesTable(tab.templateContext, ui.editor.slotText('table'))) { inline.kind = 'table'; inline.pendingKind = requestedKind; }
    inline.context = tab.templateContext ? clone(tab.templateContext) : null;
    el('db-inline-title').textContent = text(inline.kind === 'table' ? 'inlineTable' : inline.kind === 'columns' ? 'inlineColumns' : 'inlineField');
    el('db-inline-picker').classList.remove('hidden'); el('db-inline-apply').disabled = true; inlineMessage('');
    if (inline.kind === 'table') {
      const choices = singleTargetChoices();
      el('db-inline-body').innerHTML = `<label class="db-template-label"><span>${text('templateSource')}</span><select id="db-inline-source">${choices.map(target => `<option value="${escaped(key(target))}">${escaped(sourceName(target))}</option>`).join('')}</select></label><label class="db-template-label"><span>${text('tableName')}</span><select id="db-inline-table"></select></label>`;
      const preferred = tab.templateContext?.target || singleQueryTarget() || choices[0];
      if (preferred && choices.some(target => key(target) === key(preferred))) el('db-inline-source').value = key(preferred);
      positionInlinePicker(slot.id);
      if (!choices.length) return inlineMessage(text('noTargets'));
      await loadInlineObjects();
    } else if (['inner','left','right'].includes(slot.incremental)) await renderJoinPicker(slot.incremental); else if (slot.incremental || inline.kind === 'count') renderIncremental(slot.incremental || 'count'); else if (inline.kind === 'values') renderInsertValues(); else renderInlineColumns();
    positionInlinePicker(slot.id);
  }
  async function renderJoinPicker(kind) {
    inline.kind = 'join'; inline.fragment = kind;
    const context = inline.context, nonce = ++inline.nonce;
    el('db-inline-picker').classList.add('db-inline-picker-join');
    el('db-inline-title').textContent = kind.toUpperCase() + ' JOIN · ' + text('joinPicker');
    el('db-inline-body').innerHTML = ''; el('db-inline-apply').disabled = true; inlineMessage(text('loading'));
    try {
      const tables = ui.editor.incrementalTables(inline.slotId), target = context.target;
      const objects = ui.objects.get(key(target))?.items || await api('objects',{target});
      if (!inline.open || inline.nonce !== nonce || inline.sqlId !== ui.activeSQL) return;
      if (!tables.length || !objects.length) throw new Error(text('joinMissingTable'));
      const used = new Set(tables.map(table=>DatabaseUI.templateInput.identifierParts(table.qualifier).at(-1)?.toLowerCase()));
      let suffix=tables.length+1; while(used.has('t'+suffix)) suffix++;
      inline.join = {tables, objects, cache:new Map(), conditions:[{left:'',right:''}], leftColumns:[], rightColumns:[], loading:true};
      inline.join.cache.set(JSON.stringify([context.object.schema,context.object.name]), context.columns);
      const selected = objects.findIndex(object=>!tables.some(table=>contextMatchesTable({target,object},table.table)));
      el('db-inline-body').innerHTML = `<div class="db-inline-object">${escaped(sourceName(target))}</div><label class="db-template-label"><span>${text('joinExisting')}</span><select id="db-join-left-table">${tables.map((table,index)=>`<option value="${index}" ${index===tables.length-1?'selected':''}>${escaped(table.table+(table.alias?' AS '+table.alias:''))}</option>`).join('')}</select></label><label class="db-template-label"><span>${text('joinTable')}</span><select id="db-join-table">${objects.map((object,index)=>`<option value="${index}" ${index===Math.max(0,selected)?'selected':''}>${escaped(object.schema+'.'+object.name)}</option>`).join('')}</select></label><label class="db-template-label"><span>${text('joinAlias')}</span><input type="text" id="db-join-alias" value="t${suffix}" autocomplete="off" spellcheck="false"/></label><div id="db-join-fields"></div><button type="button" class="text-button" id="db-join-add-condition">+ ${text('joinAddCondition')}</button><p class="db-template-hint">${text('joinHint')}</p>`;
      positionInlinePicker(inline.slotId);
      await loadJoinColumns();
    } catch(error) {if(inline.open && inline.nonce===nonce) inlineMessage(error.message,true);}
  }
  function joinConditions() {
    return [...el('db-inline-body').querySelectorAll('[data-join-condition]')].map(row=>({left:row.querySelector('[data-join-left-field]').value,right:row.querySelector('[data-join-right-field]').value}));
  }
  function renderJoinConditions() {
    const join=inline.join, options=(columns,value)=>columns.map(column=>`<option value="${escaped(column.name)}" ${column.name===value?'selected':''}>${escaped(column.name)} · ${escaped(column.type)}</option>`).join('');
    el('db-join-fields').innerHTML=join.conditions.map((condition,index)=>`<div class="db-join-condition" data-join-condition><label class="db-template-label"><span>${text('joinLeftField')}</span><select data-join-left-field>${options(join.leftColumns,condition.left)}</select></label><span class="db-join-equals">=</span><label class="db-template-label"><span>${text('joinRightField')}</span><select data-join-right-field>${options(join.rightColumns,condition.right)}</select></label><button type="button" class="text-button" data-join-remove="${index}" ${join.conditions.length===1?'disabled':''} aria-label="${text('remove')}">×</button></div>`).join('');
    validateJoin(); positionInlinePicker(inline.slotId);
  }
  async function loadJoinColumns() {
    const join=inline.join, nonce=++inline.nonce;
    if (!join) return;
    join.conditions=joinConditions().length?joinConditions():join.conditions;
    join.loading=true; el('db-inline-apply').disabled=true; inlineMessage(text('loading'));
    // Disable stale field choices while a different table's metadata is loading.
    el('db-join-fields').querySelectorAll('select').forEach(select=>select.disabled=true);
    const target=inline.context.target, table=join.tables[Number(el('db-join-left-table').value)];
    const left=join.objects.find(object=>contextMatchesTable({target,object},table.table)), right=join.objects[Number(el('db-join-table').value)];
    async function columns(object) {
      if(!object) throw new Error(text('joinMissingTable'));
      const id=JSON.stringify([object.schema,object.name]);
      if(join.cache.has(id)) return join.cache.get(id);
      const structure=await api('structure',{target:{...target,schema:object.schema,table:object.name}});
      join.cache.set(id,structure.columns);return structure.columns;
    }
    try {
      const [leftColumns,rightColumns]=await Promise.all([columns(left),columns(right)]);
      if(!inline.open || inline.nonce!==nonce || inline.sqlId!==ui.activeSQL) return;
      join.leftColumns=leftColumns;join.rightColumns=rightColumns;join.loading=false;
      join.conditions=join.conditions.map(pair=>{
        const l=leftColumns.find(c=>c.name===pair.left)||leftColumns.find(c=>c.primary)||leftColumns[0];
        const r=rightColumns.find(c=>c.name===pair.right)||rightColumns.find(c=>c.name===l?.name)||rightColumns.find(c=>c.primary)||rightColumns[0];
        return {left:l?.name||'',right:r?.name||''};
      });
      renderJoinConditions();
    } catch(error) {if(inline.open && inline.nonce===nonce){join.loading=true;inlineMessage(error.message,true);}}
  }
  function validateJoin() {
    const join=inline.join;
    if(inline.kind!=='join' || !join || join.loading){el('db-inline-apply').disabled=true;return false;}
    const alias=el('db-join-alias').value.trim(), used=join.tables.map(table=>DatabaseUI.templateInput.identifierParts(table.qualifier).at(-1)?.toLowerCase());
    const valid=alias && !used.includes(alias.toLowerCase()) && joinConditions().every(pair=>join.leftColumns.some(c=>c.name===pair.left)&&join.rightColumns.some(c=>c.name===pair.right));
    el('db-inline-apply').disabled=!valid;inlineMessage(!alias||used.includes(alias.toLowerCase())?text('joinAliasError'):'',!valid);
    return !!valid;
  }
  function aliasInput(value) {
    return `<label class="db-template-label"><span>${text('resultColumnName')}</span><input id="db-result-alias" type="text" list="db-result-alias-options" autocomplete="off" spellcheck="false" value="${escaped(value)}"/><datalist id="db-result-alias-options"><option value="row_count"><option value="total"><option value="count"></datalist></label>`;
  }
  function renderResultAlias() {
    inline.kind = 'alias';
    const current = DatabaseUI.templateInput.identifierParts(ui.editor.slotText('alias') || '').at(-1) || 'row_count';
    el('db-inline-title').textContent = text('resultColumnName'); el('db-inline-body').innerHTML = aliasInput(current);
    el('db-inline-picker').classList.remove('hidden'); el('db-inline-apply').disabled = false; inlineMessage(''); positionInlinePicker(inline.slotId); el('db-result-alias').focus();
  }
  function renderIncremental(kind) {
    inline.kind = inline.incremental ? 'incremental' : 'count'; inline.fragment = kind;
    const columns = inline.context.columns, single = kind === 'where';
    el('db-inline-title').textContent = text({where:'templateWhere',order:'templateOrder',count:'templateCount',distinct:'templateDistinct',group:'templateGroup'}[kind]);
    const current = DatabaseUI.templateInput.identifierParts(ui.editor.slotText('countField') || '*').at(-1) || '*';
    const alias = DatabaseUI.templateInput.identifierParts(ui.editor.slotText('alias') || '').at(-1) || 'row_count';
    const body = kind === 'count'
      ? `<label class="db-template-label"><span>${text('countColumn')}</span><select id="db-count-column"><option value="*">${text('allRows')}</option>${columns.map(c=>`<option value="${escaped(c.name)}" ${c.name===current?'selected':''}>${escaped(c.name)} · ${escaped(c.type)}</option>`).join('')}</select></label>${aliasInput(alias)}`
      : `<div class="db-template-columns">${columns.map((c,index)=>`<label><input type="${single?'radio':'checkbox'}" name="inline-column" value="${escaped(c.name)}" ${index===0?'checked':''}/><span>${escaped(c.name)}</span><small>${escaped(c.type)}</small></label>`).join('')}</div>${kind==='order'?`<label class="db-template-label"><span>${text('sortDirection')}</span><select id="db-sort-direction"><option value="ASC">ASC ↑</option><option value="DESC">DESC ↓</option></select></label>`:''}`;
    el('db-inline-body').innerHTML = `<div class="db-inline-object">${escaped(sourceName(inline.context.target))} / ${escaped(inline.context.object.name)}</div>${body}<p class="db-template-hint">${text('incrementalHint')}</p>`;
    el('db-inline-apply').disabled = kind !== 'count' && !columns.length; inlineMessage(''); positionInlinePicker(inline.slotId);
  }
  async function loadInlineObjects() {
    const nonce = ++inline.nonce, source = el('db-inline-source').value, [id, database] = JSON.parse(source);
    inline.target = { id, database }; inline.context = null; el('db-inline-apply').disabled = true; inlineMessage(text('loading'));
    el('db-inline-table').innerHTML = ''; el('db-inline-table').disabled = true;
    try {
      const objects = await api('objects', { target: inline.target });
      if (!inline.open || nonce !== inline.nonce || inline.sqlId !== ui.activeSQL) return;
      inline.objects = objects;
      const cache = ui.objects.get(source); ui.objects.set(source, { ...cache, items: objects });
      el('db-inline-table').innerHTML = objects.map((object, index) => `<option value="${index}">${escaped(object.schema + '.' + object.name)}</option>`).join(''); el('db-inline-table').disabled = !objects.length;
      const previous = sqlSession().templateContext?.object;
      const index = objects.findIndex(object => object.name === (previous?.name || ui.table?.table) && object.schema === (previous?.schema || ui.table?.schema));
      el('db-inline-table').value = String(index < 0 ? 0 : index);
      if (objects.length) await loadInlineMetadata(); else inlineMessage(text('emptyObjects'));
    } catch (error) { if (nonce === inline.nonce) inlineMessage(error.message, true); }
  }
  async function loadInlineMetadata() {
    const nonce = ++inline.nonce, object = inline.objects[Number(el('db-inline-table').value)];
    inline.context = null; el('db-inline-apply').disabled = true; inlineMessage(text('loading'));
    try {
      const structure = await api('structure', { target: { ...inline.target, schema: object.schema, table: object.name } });
      if (!inline.open || nonce !== inline.nonce || inline.sqlId !== ui.activeSQL) return;
      inline.context = { target: clone(inline.target), object, columns: structure.columns, selectedFields: [] };
      el('db-inline-apply').disabled = false; inlineMessage(inline.pendingKind ? text('chooseTable') : '');
    } catch (error) { if (nonce === inline.nonce) inlineMessage(error.message, true); }
  }
  function renderInlineColumns() {
    const context = inline.context, write = ['insert', 'update'].includes(sqlSession().templateId);
    const columns = context.columns.filter(c => !write || !c.generated), selected = inline.kind === 'columns' ? selectedInsertColumns().map(c => c.name) : context.selectedFields?.length ? context.selectedFields : columns.map(c => c.name);
    inlineMessage('');
    el('db-inline-body').innerHTML = `<div class="db-inline-object">${escaped(sourceName(context.target))} / ${escaped(context.object.name)}</div>${inline.kind === 'columns' ? `<div class="db-actions"><button class="text-button" type="button" id="db-inline-all">${text('selectAll')}</button><button class="text-button" type="button" id="db-inline-none">${text('selectNone')}</button>${!write ? '<button class="text-button" type="button" id="db-inline-star">*</button>' : ''}</div>` : ''}<div class="db-template-columns">${columns.map((c, index) => `<label><input type="${inline.kind === 'field' ? 'radio' : 'checkbox'}" name="inline-column" value="${escaped(c.name)}" ${inline.kind === 'field' ? index === 0 ? 'checked' : '' : selected.includes(c.name) ? 'checked' : ''}/><span>${escaped(c.name)}</span><small>${escaped(c.type)}</small></label>`).join('')}</div>`;
    el('db-inline-apply').disabled = !columns.length;
  }
  function selectedInsertColumns() {
    const context = inline.context;
    if (!context) return [];
    const raw = ui.editor.slotText('columns');
    if (!raw || raw.trim() === '*' || raw.includes('{{')) return context.columns.filter(c => sqlSession().templateId !== 'insert' || !c.generated);
    try { return DatabaseUI.templateInput.splitSQLList(raw).map(name => DatabaseUI.templateInput.identifierParts(name).at(-1)).map(name => context.columns.find(c => c.name === name)).filter(Boolean); } catch (_) { return []; }
  }
  function captureInsertValues() {
    return [...el('db-inline-body').querySelectorAll('[data-insert-row]')].map((row,r) => [...row.querySelectorAll('[data-insert-cell]')].map((cell,c) => ({...inline.insertRows[r][c],value:cell.querySelector('input[type="text"]').value,isNull:cell.querySelector('input[type="checkbox"]').checked})));
  }
  function renderInsertValues(rows) {
    const engine = profile(inline.context.target.id)?.engine;
    inline.insertColumns = selectedInsertColumns();
    el('db-inline-title').textContent = text('insertValues'); el('db-inline-picker').classList.add('db-inline-picker-values');
    if (!inline.insertColumns.length) { el('db-inline-body').innerHTML = ''; inlineMessage(text('chooseColumn'),true); el('db-inline-apply').disabled = true; return; }
    if (!rows) {
      try {
        rows = DatabaseUI.templateInput.insertRows(ui.editor.slotText('values') || 'NULL',engine).map(row => {
          if (row.length !== inline.insertColumns.length) throw new Error('columns');
          return row.map(raw => ({raw,original:operandUnquote(raw,engine),originalNull:/^NULL$/i.test(raw),value:operandUnquote(raw,engine),isNull:/^NULL$/i.test(raw)}));
        });
      } catch (_) { inlineMessage(text('invalidValues'),true); el('db-inline-apply').disabled = true; return; }
    }
    inline.insertRows = rows;
    el('db-inline-body').innerHTML = `<div class="db-inline-object">${escaped(sourceName(inline.context.target))} / ${escaped(inline.context.object.name)}</div><div class="db-insert-scroll"><table class="db-insert-grid"><thead><tr><th>#</th>${inline.insertColumns.map(c=>`<th>${escaped(c.name)}</th>`).join('')}<th></th></tr></thead><tbody>${rows.map((row,r)=>`<tr data-insert-row="${r}"><td>${r+1}</td>${row.map((cell,c)=>`<td data-insert-cell="${c}"><input type="text" value="${escaped(cell.value)}" ${cell.isNull?'disabled':''} aria-label="${escaped(inline.insertColumns[c].name)}"/><label><input type="checkbox" data-insert-null ${cell.isNull?'checked':''}/>NULL</label></td>`).join('')}<td><button type="button" class="text-button" data-insert-remove="${r}" ${rows.length===1?'disabled':''} aria-label="${text('remove')}">×</button></td></tr>`).join('')}</tbody></table></div><button type="button" class="text-button" id="db-insert-add">+ ${text('addValueRow')}</button><label class="db-template-label"><span>${text('bulkValues')}</span><textarea id="db-insert-bulk" rows="4" spellcheck="false"></textarea></label><p class="db-template-hint">${text('bulkValuesHint')}</p><button type="button" class="secondary-button compact-button" id="db-insert-import">${text('importValues')}</button>`;
    el('db-inline-apply').disabled = false; inlineMessage(''); positionInlinePicker(inline.slotId);
  }
  function importInsertValues() {
    try {
      const imported = DatabaseUI.templateInput.bulkRows(el('db-insert-bulk').value);
      if (!imported.length || imported.some(row => row.length !== inline.insertColumns.length)) throw new Error('columns');
      let rows = captureInsertValues();
      if (rows.length === 1 && rows[0].every(cell => cell.isNull)) rows = [];
      renderInsertValues([...rows,...imported.map(row=>row.map(value=>({value:value===null?'':value,isNull:value===null})))]);
    } catch (_) { inlineMessage(text('invalidValues'),true); }
  }
  const operandGroup = operator => !operator || operator.startsWith('IS ') ? 'none' : operator.includes('BETWEEN') ? 'range' : /^(NOT )?IN$/.test(operator) ? 'list' : 'single';
  const operandInputs = group => ({ single: ['db-inline-value'], list: ['db-inline-list'], range: ['db-inline-from', 'db-inline-to'] }[group] || []).map(id => el(id));
  const splitOperandList = value => (value.match(/'(?:[^']|'')*'|[^,\r\n\t]+/g) || []).map(item => item.trim()).filter(Boolean);
  function operandEngine() { const target = sqlSession()?.templateContext?.target || singleQueryTarget(); return profile(target?.id)?.engine; }
  function operandUnquote(value, engine) {
    value = value.trim(); if (/^null$/i.test(value)) return '';
    const match = /^'([\s\S]*)'$/.exec(value); if (!match) return value;
    const body = match[1].replaceAll("''", "'"); return engine === 'mysql' ? body.replaceAll('\\\\', '\\') : body;
  }
  function operandDisplay(group, raw, engine) {
    raw = (raw || '').trim();
    if (group === 'single') return [operandUnquote(raw, engine)];
    if (group === 'list') return [splitOperandList(raw.replace(/^\(|\)$/g, '')).map(item => operandUnquote(item, engine)).filter(Boolean).join(', ')];
    const parts = raw.split(/\s+AND\s+/i).map(part => operandUnquote(part, engine)); return [parts[0] || '', parts[1] || ''];
  }
  function operandLiteral(value, engine, operator) {
    value = value.trim();
    // A LIKE pattern without any wildcard would only match exactly, so it becomes a contains-match.
    if (operator.includes('LIKE') && !/[%_]/.test(value)) value = `%${value}%`;
    const field = ui.editor.slotText('field'), column = sqlSession()?.templateContext?.columns?.find(item => quote(engine, item.name) === field);
    const numeric = !operator.includes('LIKE') && /^(?:(?:tiny|small|medium|big)?int(?:eger|\d+)?|serial\d*|decimal|numeric|number|float\d*|double|real|money)\b/i.test(String(column?.type || '').trim());
    if (numeric && /^[+-]?\d+(?:\.\d+)?$/.test(value)) return value;
    const body = engine === 'mysql' ? value.replaceAll('\\', '\\\\') : value;
    return `'${body.replaceAll("'", "''")}'`;
  }
  function syncOperandInputs() {
    const group = operandGroup(el('db-inline-operator').value);
    el('db-inline-body').querySelectorAll('[data-operand]').forEach(label => { label.hidden = label.dataset.operand !== group; });
    operandInputs(group)[0]?.focus();
  }
  function applyInlinePicker(star = false) {
    const tab = sqlSession(); if (!inline.open || tab?.id !== inline.sqlId) return;
    if (inline.kind === 'join') {
      if(!validateJoin()) return;
      const join=inline.join, engine=profile(inline.context.target.id)?.engine;
      const source=join.tables[Number(el('db-join-left-table').value)], object=join.objects[Number(el('db-join-table').value)], alias=quote(engine,el('db-join-alias').value.trim());
      const values={table:inlineTableName({target:inline.context.target,object}),alias,conditions:joinConditions().map(pair=>({left:source.qualifier+'.'+quote(engine,pair.left),right:alias+'.'+quote(engine,pair.right)}))};
      if(!ui.editor.applyIncremental(inline.slotId,values,slotLabels())) return;
      closeInlinePicker();ui.editor.focus();return;
    }
    if (['incremental','count','alias'].includes(inline.kind)) {
      const engine = profile(inline.context?.target?.id || tab.templateContext?.target?.id || singleQueryTarget()?.id)?.engine;
      const name = el('db-result-alias')?.value.trim();
      if (['count','alias'].includes(inline.kind) || inline.fragment === 'count') {
        if (!name) { inlineMessage(text('aliasRequired'),true); return; }
      }
      if (inline.kind === 'alias') ui.editor.replaceSlots({alias:quote(engine,name)});
      else {
        const selected = [...el('db-inline-body').querySelectorAll('[name="inline-column"]:checked')].map(input=>input.value);
        const field = el('db-count-column')?.value;
        if (inline.fragment !== 'count' && !selected.length) return;
        const values = {field:inline.fragment==='count' ? field==='*'?'*':quote(engine,field) : quote(engine,selected[0]), columns:selected.map(name=>quote(engine,name)).join(', '), alias:name?quote(engine,name):undefined, direction:el('db-sort-direction')?.value};
        if (inline.kind === 'count') ui.editor.replaceSlots({countField:values.field,alias:values.alias});
        else {
          const result = ui.editor.applyIncremental(inline.slotId,values,slotLabels());
          if (!result) return;
          const operator = result.operator;
          closeInlinePicker(); ui.editor.focus(); if (operator) openInlinePicker(operator); return;
        }
      }
      closeInlinePicker(); ui.editor.focus(); return;
    }
    if (inline.kind === 'operator') {
      const operator = el('db-inline-operator').value, current = ui.editor.slotText('operator'), previousValue = ui.editor.slotText('filterValue');
      const group = operandGroup(operator), engine = operandEngine(), inputs = operandInputs(group).map(input => input.value);
      const untouched = group === inline.operandGroup && inputs.join('\u0000') === inline.operandShown.join('\u0000'), literal = value => operandLiteral(value, engine, operator);
      let operand;
      if (group === 'none') operand = '';
      else if (untouched && previousValue && operandGroup(current) === group && !(operator.includes('LIKE') && !/[%_]/.test(inputs[0]))) operand = previousValue;
      else if (group === 'range') operand = inputs.map(value => value.trim() ? literal(value) : 'NULL').join(' AND ');
      else if (group === 'list') { const items = splitOperandList(inputs[0]); operand = `(${items.length ? items.map(literal).join(', ') : 'NULL'})`; }
      else operand = inputs[0].trim() ? literal(inputs[0]) : operator.includes('LIKE') ? "''" : 'NULL';
      ui.editor.replacePredicate(operator, operand, text('inlineValue')); closeInlinePicker(); ui.editor.focus(); return;
    }
    if (!inline.context) return;
    if (inline.kind === 'values') {
      const engine = profile(inline.context.target.id)?.engine, rows = captureInsertValues();
      const values = rows.map(row=>row.map((cell,c)=>cell.raw !== undefined && cell.value === cell.original && cell.isNull === cell.originalNull ? cell.raw : DatabaseUI.templateInput.templateLiteral(cell.isNull?null:cell.value,engine,inline.insertColumns[c].type)).join(', ')).join('), (');
      ui.editor.replaceSlots({values}); ui.editor.setTemplateContext(clone(inline.context)); closeInlinePicker(); ui.editor.focus(); return;
    }
    let followupOperator;
    const context = inline.context, kind = inline.kind, pending = inline.pendingKind, slotId = inline.slotId, engine = profile(context.target.id)?.engine;
    if (kind === 'table') { tab.templateContext = context; ui.editor.replaceSlots(templateValues(context, tab.templateId)); }
    else {
      const selected = [...el('db-inline-body').querySelectorAll('[name="inline-column"]:checked')].map(input => input.value);
      if (!selected.length && !star) return;
      if (kind === 'field' && inline.condition) {
        const created = ui.editor.replaceCondition(slotId, quote(engine, selected[0]), { field: text('inlineField'), operator: text('inlineOperator') });
        followupOperator = created?.find(slot => slot.kind === 'operator');
      }
      else if (kind === 'field') ui.editor.replaceSlots({ field: quote(engine, selected[0]) });
      else {
        const useStar = star || (tab.templateId !== 'insert' && selected.length === context.columns.length);
        let values = Array(Math.max(1,selected.length)).fill('NULL').join(', ');
        if (tab.templateId === 'insert') {
          try {
            const oldColumns = selectedInsertColumns().map(c=>c.name), rows = DatabaseUI.templateInput.insertRows(ui.editor.slotText('values'),engine);
            values = rows.map(row=>selected.map(name=>oldColumns.includes(name)?row[oldColumns.indexOf(name)]:'NULL').join(', ')).join('), (');
          } catch (_) { inlineMessage(text('invalidValues'),true); return; }
        }
        context.selectedFields = useStar ? [] : selected; tab.templateContext = context;
        ui.editor.replaceSlots({columns:useStar?'*':selected.map(name=>quote(engine,name)).join(', '),values});
      }
    }
    ui.editor.setTemplateContext(tab.templateContext ? clone(tab.templateContext) : null);
    closeInlinePicker(); renderTargets(); ui.editor.focus();
    if (pending) { const slot = ui.editor.slots().find(slot => slot.id === slotId); if (slot) openInlinePicker(slot, pending); }
    else if (followupOperator) openInlinePicker(ui.editor.slots().find(slot => slot.id === followupOperator.id));
  }

  const sqlSession = () => ui.sqlTabs?.find(tab => tab.id === ui.activeSQL);
  const tableSession = () => ui.tableTabs?.find(tab => tab.id === ui.currentTableId);
  const tableKey = target => JSON.stringify([target.id, target.database, target.schema, target.table]);
  function renderWorkspaceTabs() { persistSQL();
    el('db-sql-tabs').innerHTML = ui.sqlTabs.map(tab => `<div class="db-session-tab ${tab.id === ui.activeSQL ? 'active' : ''}" draggable="true" data-reorder="${tab.id}"><button type="button" role="tab" aria-selected="${tab.id === ui.activeSQL}" data-db-sql="${tab.id}">${escaped(tab.name)}${tab.controller ? ' ◌' : tab.text?.trim() ? ' •' : ''}</button><button type="button" class="db-session-close" data-db-close-sql="${tab.id}" aria-label="${escaped(text('closeTab'))}">×</button></div>`).join('');
    el('db-table-tabs').innerHTML = ui.bottomTabs.map(item => {
      const tab = item.kind === 'table' ? ui.tableTabs.find(tab => tab.id === item.id) : ui.sqlTabs.find(tab => tab.id === item.id);
      if (!tab) return '';
      const label = item.kind === 'table' ? `▦ ${tab.target.table}${tab.dirty ? ' •' : ''}` : `${tab.name} · ${text('results')}${queryDirtyCount(tab.query) ? ' *' : ''}`;
      const title = item.kind === 'table' ? `${sourceName(tab.target)} / ${tab.target.schema}.${tab.target.table}` : label;
      return `<div class="db-session-tab ${ui.activeBottom?.id === item.id && ui.activeBottom?.kind === item.kind ? 'active' : ''}" draggable="true" data-reorder="${item.kind}:${item.id}"><button type="button" role="tab" aria-selected="${ui.activeBottom?.id === item.id && ui.activeBottom?.kind === item.kind}" data-db-bottom="${item.id}" data-kind="${item.kind}" title="${escaped(title)}">${escaped(label)}</button><button type="button" class="db-session-close" data-db-close-bottom="${item.id}" data-kind="${item.kind}" aria-label="${escaped(text('closeTab'))}">×</button></div>`;
    }).join('');
  }
  // Drag-and-drop ordering for rows that are rebuilt via innerHTML, so listeners live on the container.
  function enableReorder(container, itemSelector, idOf, onMove, horizontal = true) {
    let dragged = null;
    const clear = () => container.querySelectorAll('.drop-before, .drop-after, .dragging').forEach(node => node.classList.remove('drop-before', 'drop-after', 'dragging'));
    const after = (event, item) => { const rect = item.getBoundingClientRect(); return horizontal ? event.clientX > rect.left + rect.width / 2 : event.clientY > rect.top + rect.height / 2; };
    container.addEventListener('dragstart', event => {
      const item = event.target.closest?.(itemSelector); if (!item || !container.contains(item)) return;
      dragged = idOf(item); event.dataTransfer.effectAllowed = 'move'; event.dataTransfer.setData('text/plain', dragged); item.classList.add('dragging');
    });
    container.addEventListener('dragover', event => {
      const item = event.target.closest?.(itemSelector); if (dragged === null || !item) return;
      event.preventDefault(); event.dataTransfer.dropEffect = 'move';
      container.querySelectorAll('.drop-before, .drop-after').forEach(node => node.classList.remove('drop-before', 'drop-after'));
      if (idOf(item) !== dragged) item.classList.add(after(event, item) ? 'drop-after' : 'drop-before');
    });
    container.addEventListener('drop', event => {
      const item = event.target.closest?.(itemSelector); if (dragged === null || !item) return;
      event.preventDefault(); const from = dragged, to = idOf(item), placeAfter = after(event, item); dragged = null; clear();
      if (from !== to) onMove(from, to, placeAfter);
    });
    container.addEventListener('dragend', () => { dragged = null; clear(); });
  }
  function moveItem(list, from, to, placeAfter, idOf) {
    const fromIndex = list.findIndex(item => idOf(item) === from); if (fromIndex < 0 || !list.some(item => idOf(item) === to)) return false;
    const [moved] = list.splice(fromIndex, 1), toIndex = list.findIndex(item => idOf(item) === to);
    list.splice(toIndex + (placeAfter ? 1 : 0), 0, moved); return true;
  }
  function captureSQL() {
    const tab = sqlSession(); if (!tab || !ui.editor || ui.restoringSQL) return;
    tab.editor = ui.editor.snapshot(); tab.slotState = ui.editor.slotState(); tab.text = ui.editor.value(); tab.checked = new Map(ui.checked); tab.focus = ui.focus ? { ...ui.focus } : null;
    tab.targetsExplicit = ui.targetsExplicit; tab.singleTarget = ui.singleTarget; tab.singleTargetManual = ui.singleTargetManual; tab.limit = el('db-query-limit').value;
  }
  function editorChanged() { if (!ui.restoringSQL && sqlSession()) { const tab = sqlSession(), wasFilled = Boolean(tab.text.trim()); tab.text = ui.editor.value(); persistSQL(); if (inline.open) closeInlinePicker(); if (wasFilled !== Boolean(tab.text.trim())) renderWorkspaceTabs(); } }
  // SQL editor text lives in this browser's localStorage so a refresh or
  // restart does not lose unsaved queries. Only id/name/text are kept; targets
  // and results are session state.
  const sqlStorageKey = 'log-agent-database-sql-tabs';
  let sqlPersistTimer = null;
  function persistSQLNow() {
    clearTimeout(sqlPersistTimer); sqlPersistTimer = null;
    if (!ui.initialized || !ui.sqlTabs?.length) return;
    try {
      const tabs = ui.sqlTabs.map(tab => {
        const live = tab.id === ui.activeSQL && ui.editor && !ui.restoringSQL;
        return { id: tab.id, name: tab.name, text: live ? ui.editor.value() : tab.text, slotState: live ? ui.editor.slotState() : tab.slotState, templateId: tab.templateId, templateContext: tab.templateContext };
      });
      localStorage.setItem(sqlStorageKey, JSON.stringify({ tabs, active: ui.activeSQL, sequence: ui.sqlSequence }));
    } catch {}
  }
  function persistSQL() { clearTimeout(sqlPersistTimer); sqlPersistTimer = setTimeout(persistSQLNow, 300); }
  window.addEventListener('beforeunload', persistSQLNow);
  function restoreSQLTabs() {
    let saved = null; try { saved = JSON.parse(localStorage.getItem(sqlStorageKey) || 'null'); } catch {}
    const tabs = Array.isArray(saved?.tabs) ? saved.tabs.filter(tab => tab && typeof tab.id === 'string' && typeof tab.text === 'string') : [];
    if (!tabs.length) return false;
    ui.sqlSequence = Math.max(Number(saved.sequence) || 0, ...tabs.map(tab => Number(String(tab.id).replace(/\D/g, '')) || 0));
    for (const item of tabs) ui.sqlTabs.push({ id: item.id, name: String(item.name || item.id), text: item.text, slotState: item.slotState, templateId: item.templateId, templateContext: item.templateContext, checked: new Map(), targetsExplicit: false, focus: null, singleTarget: undefined, singleTargetManual: false, limit: el('db-query-limit').value || '500', controller: null, query: null });
    activateSQLTab(ui.sqlTabs.some(tab => tab.id === saved.active) ? saved.active : ui.sqlTabs[0].id);
    return true;
  }
  function createSQLTab() {
    captureSQL();
    const number = ++ui.sqlSequence, tab = { id: `sql-${number}`, name: `SQL ${number}`, text: '', checked: new Map(ui.checked), targetsExplicit: ui.targetsExplicit, focus: ui.focus ? { ...ui.focus } : null, singleTarget: ui.singleTarget, singleTargetManual: ui.singleTargetManual, limit: el('db-query-limit').value || '500', controller: null, query: null };
    ui.sqlTabs.push(tab); activateSQLTab(tab.id); return tab;
  }
  function activateSQLTab(id) {
    if (ui.activeSQL === id) { ui.editor.focus(); return; }
    captureSQL(); closeInlinePicker(); toggleTargetPicker(false); targetPicker.selectionEpoch++;
    const tab = ui.sqlTabs.find(tab => tab.id === id); if (!tab) return;
    ui.activeSQL = id; ui.checked = new Map([...tab.checked].filter(([, target]) => profile(target.id))); ui.focus = tab.focus && profile(tab.focus.id) ? { ...tab.focus } : null;
    ui.targetsExplicit = Boolean(tab.targetsExplicit); ui.singleTarget = tab.singleTarget; ui.singleTargetManual = tab.singleTargetManual; ui.controller = tab.controller;
    ui.restoringSQL = true;
    if (tab.editor) ui.editor.restore(tab.editor); else { ui.editor.setValue(tab.text); if (tab.slotState?.slots?.length) ui.editor.setSlotState(tab.slotState); else ui.editor.recognizeSlots(slotLabels()); }
    ui.editor.theme(document.documentElement.dataset.theme === 'dark'); ui.restoringSQL = false;
    el('db-query-limit').value = tab.limit;
    el('db-run-query').disabled = Boolean(tab.controller); el('db-cancel-query').disabled = !tab.controller;
    renderTree(); renderTargets();
    if (tab.query || tab.message || tab.controller) activateBottom({ kind: 'result', id });
    else renderWorkspaceTabs();
    ui.editor.focus(); requestAnimationFrame(resizeGrids);
  }
  function closeSQLTab(id) {
    captureSQL(); const tab = ui.sqlTabs.find(tab => tab.id === id); if (!tab) return;
    if (tab.query?.saving || ui.committing) return status(text('resultSaving'));
    finishResultEdit();
    if ((tab.text.trim() || tab.controller || queryDirtyCount(tab.query)) && !window.confirm(text(queryDirtyCount(tab.query) ? 'discard':'closeSqlConfirm'))) return;
    tab.controller?.abort(); ui.sqlTabs = ui.sqlTabs.filter(item => item !== tab); ui.bottomTabs = ui.bottomTabs.filter(item => !(item.kind === 'result' && item.id === id));
    if (ui.activeSQL === id) { ui.activeSQL = null; if (ui.sqlTabs.length) activateSQLTab(ui.sqlTabs.at(-1).id); else createSQLTab(); }
    if (ui.activeBottom?.kind === 'result' && ui.activeBottom.id === id) { ui.activeBottom = null; if (ui.bottomTabs.length) activateBottom(ui.bottomTabs.at(-1)); else paintBottom(); }
    renderWorkspaceTabs();
  }
  function captureTableState() {
    const tab = tableSession(); if (!tab || !ui.tableResponse) return;
    tab.response = ui.tableResponse; tab.page = ui.page; tab.pageSize = el('db-page-size').value; tab.dirty = ui.dirty; tab.view = ui.tab === 'structure' ? 'structure' : 'data';
    if (ui.grids.data?._databaseReady) { tab.rows = clone(ui.grids.data.getData()); tab.original = new Map(ui.original); tab.selected = ui.grids.data.getSelectedData().map(row => row.__rowKey); tab.sort = ui.grids.data.getSorters().map(sort => ({ column: sort.field, dir: sort.dir })); tab.scroll = el('db-data-grid').querySelector('.tabulator-tableholder')?.scrollTop || 0; }
  }
  function restoreTableState(tab) {
    ui.currentTableId = tab.id; ui.table = tab.target; ui.tableResponse = tab.response; ui.page = tab.page || 0; ui.dirty = tab.dirty || 0; ui.saving = Boolean(tab.saving);
    el('db-page-size').value = tab.pageSize || '100';
    if (tab.response) { renderData(tab); if(tab.view==='structure')renderStructure(); }
    el('db-data-grid').inert = Boolean(tab.saving || tab.loading); syncDataActions();
  }
  function activateBottom(item) {
    captureTableState(); captureResultState(); closeInlinePicker();
    if (!ui.bottomTabs.some(tab => tab.kind === item.kind && tab.id === item.id)) ui.bottomTabs.push({ ...item });
    ui.activeBottom = { ...item };
    if (item.kind === 'table') {
      const tab = ui.tableTabs.find(tab => tab.id === item.id); if (!tab) return;
      restoreTableState(tab); ui.focus = { id: tab.target.id, database: tab.target.database }; ui.tab = tab.view || 'data'; renderTree(); renderTargets();
    }
    paintBottom();
  }
  function paintBottom() {
    const item = ui.activeBottom, table = item?.kind === 'table' ? ui.tableTabs.find(tab => tab.id === item.id) : null, result = item?.kind === 'result' ? ui.sqlTabs.find(tab => tab.id === item.id) : null;
    const hasTable = Boolean(table?.response);
    el('db-panel-data').classList.toggle('hidden', !table || hasTable && ui.tab === 'structure'); el('db-panel-structure').classList.toggle('hidden', !hasTable || ui.tab !== 'structure');
    el('db-data-grid').classList.toggle('hidden', !hasTable);
    el('db-data-placeholder').classList.toggle('hidden', !table || hasTable);
    el('db-data-placeholder').textContent = table?.loading ? text('loading') : table?.error || table?.message || '';
    if (table && !hasTable) { el('db-data-count').textContent = ''; el('db-page-label').textContent = ''; }
    syncDataActions();
    el('db-table-subtabs').classList.toggle('hidden', !hasTable); el('db-panel-result').classList.toggle('hidden', !result); el('db-table-empty').classList.toggle('hidden', Boolean(table || result));
    el('db-table-empty').textContent = table?.loading ? text('loading') : table?.error || text('emptyWorkspace');
    view.querySelectorAll('[data-db-tab]').forEach(button => button.classList.toggle('active', button.dataset.dbTab === ui.tab));
    el('db-object-title').textContent = table ? `${sourceName(table.target)} / ${table.target.schema}.${table.target.table}` : result ? `${result.name} / ${text('results')}` : text('chooseObject');
    if (result) {
      ui.query = result.query;
      if (result.query) renderQuery(result.query); else { if (ui.grids.query?._databaseReady) ui.grids.query.clearData(); el('db-query-summary').textContent = result.message || text('queryHint'); status(result.message || '', result.error); }
    } else if (table) status(table.error || table.message || (table.dirty ? text('dirty', { count: table.dirty }) : (table.response?.editable === false ? text('readOnly') : '')), Boolean(table.error));
    else status('');
    syncResultActions(); syncDatabaseLoads(); renderWorkspaceTabs(); requestAnimationFrame(resizeGrids);
  }
  function closeBottomTab(item, force = false) {
    captureTableState();
    if (ui.committing) return false;
    if (item.kind === 'result') {
      const result = ui.sqlTabs.find(tab => tab.id === item.id)?.query;
      if (result?.saving) return false;
      if (queryDirtyCount(result)) { if (!window.confirm(text('discard'))) return false; for (const row of result.rows) {delete row.draft;delete row.deleted;} }
    }
    const tab = item.kind === 'table' ? ui.tableTabs.find(tab => tab.id === item.id) : null;
    if (tab?.saving || tab?.commentBusy || (!force && (tab?.dirty || commentDirty(tab)) && !window.confirm(text('discard')))) return false;
    if (tab) { tab.loadController?.abort(); ui.tableTabs = ui.tableTabs.filter(value => value !== tab); }
    ui.bottomTabs = ui.bottomTabs.filter(value => !(value.kind === item.kind && value.id === item.id));
    if (ui.currentTableId === item.id) { ui.currentTableId = null; ui.table = null; ui.tableResponse = null; ui.dirty = 0; ui.original = new Map(); ui.saving = false; }
    if (ui.activeBottom?.kind === item.kind && ui.activeBottom.id === item.id) { ui.activeBottom = null; if (ui.bottomTabs.length) activateBottom(ui.bottomTabs.at(-1)); else paintBottom(); }
    renderTree(); renderWorkspaceTabs(); return true;
  }
  function clearTable() { if (ui.currentTableId) closeBottomTab({ kind: 'table', id: ui.currentTableId }, true); }
  function connectionTabsGuard(id) {
    captureTableState(); captureResultState(); const tabs = ui.tableTabs.filter(tab => tab.target.id === id);
    const results = ui.sqlTabs.filter(tab => tab.query?.sources.some(source => source.target.id === id || source.edit?.target.id === id));
    return !ui.committing && !tabs.some(tab => tab.saving || tab.commentBusy) && !results.some(tab => tab.query.saving) && (!(tabs.some(tab => tab.dirty || commentDirty(tab)) || results.some(tab => queryDirtyCount(tab.query))) || window.confirm(text('discard')));
  }
  function clearConnectionTabs(id) {
    for (const tab of [...ui.tableTabs]) if (tab.target.id === id) closeBottomTab({ kind: 'table', id: tab.id }, true);
    for (const tab of ui.sqlTabs) { if (tab.query?.sources.some(source => source.target.id === id || source.edit?.target.id === id)) {tab.query = null;tab.message = '';} for (const [key, target] of tab.checked) if (target.id === id) tab.checked.delete(key); if (tab.focus?.id === id) tab.focus = null; if (tab.templateContext?.target.id === id) tab.templateContext = null; }
    if (ui.activeBottom?.kind === 'result') paintBottom();
  }

  const targetPicker = { open: false, expanded: new Set(), search: '', error: '', searchTimer: null, selectionEpoch: 0 };
  function positionTargetPicker() {
    if (!targetPicker.open) return;
    const panel = el('db-target-picker'), anchor = el('db-target-select').getBoundingClientRect();
    panel.style.left = Math.max(8, Math.min(anchor.left, innerWidth - panel.offsetWidth - 8)) + 'px';
    const top = Math.max(8, Math.min(anchor.bottom + 6, innerHeight - 180));
    panel.style.top = top + 'px'; panel.style.maxHeight = Math.min(440, innerHeight - top - 12) + 'px';
  }
  function toggleTargetPicker(open, focus = false) {
    targetPicker.open = open; el('db-target-picker').classList.toggle('hidden', !open); el('db-target-select').setAttribute('aria-expanded', String(open));
    if (!open) { if (focus) el('db-target-select').focus(); return; }
    closeInlinePicker(); if (templates.open) toggleTemplates(false);
    targetPicker.error = ''; targetPicker.search = ''; el('db-target-search').value = '';
    for (const target of targetList()) targetPicker.expanded.add(target.id);
    renderTargetTree(); positionTargetPicker();
    for (const id of targetPicker.expanded) if (profile(id)) loadCatalog(id);
    if (focus) el('db-target-search').focus();
  }
  function commitTargets(targets) {
    const checked = new Map(targets.map(target => [key(target), { id: target.id, database: target.database }]));
    if (checked.size > 16) { targetPicker.error = text('maxTargets'); renderTargetTree(); status(targetPicker.error, true); return false; }
    ui.checked = checked; ui.targetsExplicit = true; targetPicker.error = ''; renderTargets(); return true;
  }
  function clearTargets() { targetPicker.selectionEpoch++; commitTargets([]); }
  function renderTargetTree() {
    if (!targetPicker.open) return;
    const tree = el('db-target-tree'), previousScroll = tree.scrollTop, active = document.activeElement;
    const focus = active?.closest('#db-target-tree') ? { check: active.dataset.dbCheck, group: active.dataset.dbTargetGroup, expand: active.dataset.dbTargetExpand } : null;
    const selected = new Set(targetList().map(key)), search = targetPicker.search.trim().toLocaleLowerCase(), output = [];
    for (const p of ui.profiles) {
      const catalog = ui.catalogs.get(p.id), all = catalog?.databases || [], connectionMatch = p.name.toLocaleLowerCase().includes(search);
      const databases = !search || connectionMatch ? all : all.filter(database => database.toLocaleLowerCase().includes(search));
      if (search && !connectionMatch && (catalog?.error || catalog?.databases && !databases.length)) continue;
      const expanded = Boolean(search) || targetPicker.expanded.has(p.id), count = all.filter(database => selected.has(key({ id: p.id, database }))).length;
      const checked = all.length > 0 && count === all.length, partial = count > 0 && !checked;
      output.push(`<div class="db-target-branch"><div class="db-target-row"><button type="button" data-db-target-expand="${escaped(p.id)}" aria-expanded="${expanded}" aria-label="${escaped(text('expandConnection', { name: p.name }))}">${expanded ? '▾' : '▸'}</button><label><input type="checkbox" data-db-target-group="${escaped(p.id)}" data-partial="${partial}" ${checked ? 'checked' : ''} ${ui.controller || catalog?.loading ? 'disabled' : ''}/><span>◉ ${escaped(p.name)}</span></label><small>${escaped(p.engine)}</small></div>`);
      if (expanded) {
        if (catalog?.loading || !catalog) output.push(`<div class="db-tree-note db-target-child">${text('loading')}</div>`);
        else if (catalog.error) output.push(`<div class="db-tree-note db-target-child error">${escaped(catalog.error)} <button class="text-button" type="button" data-db-target-retry="${escaped(p.id)}">${text('refresh')}</button></div>`);
        else if (!databases.length) output.push(`<div class="db-tree-note db-target-child">${text(search ? 'noTargetMatches' : 'emptyDatabases')}</div>`);
        else for (const database of databases) {
          const target = { id: p.id, database }, id = key(target);
          output.push(`<div class="db-target-row db-target-child"><label><input type="checkbox" data-db-check="${escaped(id)}" ${selected.has(id) ? 'checked' : ''} ${ui.controller ? 'disabled' : ''}/><span>◫ ${escaped(database)}</span></label></div>`);
        }
      }
      output.push('</div>');
    }
    tree.innerHTML = output.join('') || `<div class="db-tree-note">${text(ui.profiles.length ? 'noTargetMatches' : 'emptyConnections')}</div>`;
    tree.querySelectorAll('[data-partial="true"]').forEach(input => { input.indeterminate = true; });
    tree.scrollTop = previousScroll;
    if (focus) [...tree.querySelectorAll('button, input')].find(node => focus.check ? node.dataset.dbCheck === focus.check : focus.group ? node.dataset.dbTargetGroup === focus.group : node.dataset.dbTargetExpand === focus.expand)?.focus({ preventScroll: true });
    const footer = el('db-target-picker-status'); footer.textContent = targetPicker.error || text('targetsLimit', { count: selected.size }); footer.classList.toggle('error', Boolean(targetPicker.error));
    el('db-target-picker-clear').disabled = Boolean(ui.controller);
  }
  async function changePickedTargets(input) {
    if (ui.controller) return renderTargetTree();
    if (input.dataset.dbCheck) {
      const [id, database] = JSON.parse(input.dataset.dbCheck), targets = new Map(targetList().map(target => [key(target), target]));
      if (input.checked) targets.set(input.dataset.dbCheck, { id, database }); else targets.delete(input.dataset.dbCheck);
      commitTargets([...targets.values()]); return;
    }
    if (!input.dataset.dbTargetGroup) return;
    const id = input.dataset.dbTargetGroup, checked = input.checked, sqlId = ui.activeSQL, epoch = targetPicker.selectionEpoch;
    targetPicker.expanded.add(id);
    const catalog = await loadCatalog(id);
    if (sqlId !== ui.activeSQL || ui.controller || epoch !== targetPicker.selectionEpoch) return;
    if (catalog.error) { targetPicker.error = catalog.error; renderTargetTree(); return; }
    const targets = new Map(targetList().map(target => [key(target), target]));
    for (const database of catalog.databases || []) { const target = { id, database }; if (checked) targets.set(key(target), target); else targets.delete(key(target)); }
    commitTargets([...targets.values()]);
  }
  async function loadCatalog(id, force = false) {
    let catalog = ui.catalogs.get(id);
    if (!catalog) { catalog = { expanded: false }; ui.catalogs.set(id, catalog); }
    if (catalog.request) return catalog.request;
    if (catalog.databases && !force) return catalog;
    catalog.loading = true; catalog.error = '';
    catalog.request = api('catalog', { target: { id } }).then(databases => { catalog.databases = databases; }).catch(error => { catalog.error = error.message; }).finally(() => {
      catalog.loading = false; catalog.request = null;
      if (ui.catalogs.get(id) === catalog) { renderTree(); renderTargetTree(); }
    }).then(() => catalog);
    renderTree(); renderTargetTree(); return catalog.request;
  }

  async function loadProfiles() {
    ui.profiles = await api('profiles');
    for (const [id] of ui.catalogs) if (!profile(id)) ui.catalogs.delete(id);
    for (const [id, target] of ui.checked) if (!profile(target.id)) ui.checked.delete(id);
    renderTree(); renderTargets(); renderConnectionManager();
    syncTemplates(true);
  }
  function targetList() { return ui.checked.size ? [...ui.checked.values()] : !ui.targetsExplicit && ui.focus ? [{ id: ui.focus.id, database: ui.focus.database }] : []; }
  function singleTargetChoices() {
    const targets = targetList();
    if (ui.focus && !targets.some(target => key(target) === key(ui.focus))) targets.push({ id: ui.focus.id, database: ui.focus.database });
    return targets;
  }
  function singleQueryTarget() { return singleTargetChoices().find(target => key(target) === ui.singleTarget) || null; }
  function renderTargets() {
    const targets = targetList();
    el('db-target-count').textContent = text('targets', { count: targets.length });
    el('db-target-select').dataset.count = targets.length;
    el('db-target-summary').textContent = targets.length ? targets.map(sourceName).join('、') : text('chooseTargets');
    el('db-target-select').title = targets.map(sourceName).join('\n') || text('chooseTargets');
    renderTargetTree();
    const choices = singleTargetChoices();
    if (!choices.some(target => key(target) === ui.singleTarget)) ui.singleTargetManual = false;
    if (!ui.singleTargetManual) ui.singleTarget = (ui.focus && choices.some(target => key(target) === key(ui.focus))) ? key(ui.focus) : choices[0] ? key(choices[0]) : '';
    el('db-single-target').innerHTML = choices.map(target => `<option value="${escaped(key(target))}">${escaped(sourceName(target))}</option>`).join('');
    el('db-single-target').value = ui.singleTarget; el('db-single-target').disabled = !choices.length || Boolean(ui.controller);
    el('db-execute').disabled = !singleQueryTarget() || Boolean(ui.controller);
    el('db-clear-targets').disabled = Boolean(ui.controller);
    syncTemplates();
    const editorTarget = singleQueryTarget() || targets[0];
    if (ui.editor && editorTarget) {
      const p = profile(editorTarget.id), schema = {};
      const cached = ui.objects.get(key(editorTarget));
      for (const object of cached?.items || []) schema[object.name] = ui.table && key(ui.table) === key(editorTarget) && ui.table.schema === object.schema && ui.table.table === object.name ? (ui.tableResponse?.structure.columns || []).map(c => c.name) : [];
      const context = sqlSession()?.templateContext; if (context && key(context.target) === key(editorTarget)) schema[context.object.name] = context.columns.map(c => c.name);
      ui.editor.configure(p?.engine, schema);
    }
    const single = singleQueryTarget();
    ui.editor?.queryActions({ singleEnabled: Boolean(single) && !ui.controller, multiEnabled: targets.length > 1 && !ui.controller, singleLabel: text('singleQueryTitle', { target: single ? sourceName(single) : text('noTargets') }), multiLabel: text('multiQueryTitle', { count: targets.length }) });
  }
  const structureTree = new Map();
  i18n.en['db.enforced']='Enforced';i18n.zh['db.enforced']='启用检查';
  for(const [key,pair] of Object.entries({tableComment:['Table comment','表备注'],saveComment:['Save comment','保存备注'],commentUnsupported:['This engine does not support native table comments.','此数据库不支持原生表备注。'],referencedSchema:['Referenced schema','引用模式'],referencedTable:['Referenced table','引用表'],referencedColumns:['Referenced fields','引用字段'],onDelete:['On delete','删除时'],onUpdate:['On update','更新时'],expression:['Expression','表达式'],timing:['Timing','时机'],event:['Event','事件'],definition:['Definition','定义'],method:['Method','方法'],comment:['Comment','备注']})){i18n.en[`db.${key}`]=pair[0];i18n.zh[`db.${key}`]=pair[1];}
  const structureGroups = [['columns','fields'],['indexes','indexes'],['foreignKeys','foreignKeys'],['checks','checks'],['triggers','triggers']];
  for(const [key,pair] of Object.entries({fields:['Fields','字段'],foreignKeys:['Foreign Keys','外键'],checks:['Checks','检查约束'],triggers:['Triggers','触发器'],emptyStructure:['None','无'],toggleStructure:['Expand / collapse structure','展开 / 收起结构']})){i18n.en[`db.${key}`]=pair[0];i18n.zh[`db.${key}`]=pair[1];}
  function structureIcon(kind){
    const shapes={columns:'<rect x="4" y="3" width="16" height="18" rx="1"/><path d="M10 3v18M4 9h16M4 15h16"/>',primary:'<circle cx="16" cy="7" r="5"/><path d="M12 11 2 21m3-3 3 3m0-6 3 3"/>',indexes:'<path d="m2 17 3.5-10L9 17M3.4 13h4.2M16 7h6l-6 10h6"/><circle cx="12.5" cy="12" r=".8" fill="currentColor" stroke="none"/>',foreignKeys:'<g transform="translate(2 2) scale(.83)"><path d="m10 14 4-4m-6 8-1 1a4 4 0 0 1-6-6l5-5a4 4 0 0 1 6 0m0 8a4 4 0 0 0 6 0l5-5a4 4 0 0 0-6-6l-1 1"/></g>',checks:'<circle cx="12" cy="12" r="9"/><path d="m7 12 3 3 7-7"/>',triggers:'<path d="m14 2-9 12h6l-1 8L20 9h-7Z"/>'};
    return `<svg class="db-structure-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${shapes[kind]||shapes.columns}</svg>`;
  }
  function structureRows(target, branch) {
    const id=tableKey(target), output=[];
    if(branch.loading||branch.error)return `<div class="db-tree-note ${branch.error?'error':''}" style="padding-left:66px">${escaped(branch.error||text('loading'))}</div>`;
    for(const [kind,label] of structureGroups){
      const values=branch.meta?.[kind]||[],expanded=branch.groups.has(kind);
      output.push(`<div class="db-tree-row db-structure-group" style="padding-left:48px"><button class="db-tree-toggle" type="button" data-db-structure-group="${escaped(id)}" data-group="${kind}" aria-expanded="${expanded}" aria-label="${escaped(text('toggleStructure'))}: ${escaped(text(label))}">${expanded?'▾':'▸'}</button><button type="button" data-db-structure-open="${escaped(id)}" data-group="${kind}">${structureIcon(kind)}<span>${escaped(text(label))}</span><small>${values.length}</small></button></div>`);
      if(!expanded)continue;
      if(kind==='triggers'&&branch.meta?.triggerError){output.push(`<div class="db-tree-note error" style="padding-left:80px">${escaped(branch.meta.triggerError)}</div>`);continue;}
      if(!values.length)output.push(`<div class="db-tree-note" style="padding-left:80px">${text('emptyStructure')}</div>`);
      for(const [index,item] of values.entries()){
        const name=typeof item==='string'?item:item.name, detail=kind==='columns'?item.type:kind==='indexes'?(item.columns||[]).join(', '):'',node=JSON.stringify([id,kind,index]);
        output.push(`<div class="db-tree-row ${ui.structureNode===node?'active':''}" style="padding-left:73px"><button type="button" data-db-structure-node="${escaped(node)}" title="${escaped([name,detail].filter(Boolean).join(' · '))}">${structureIcon(kind==='columns'&&item.primary?'primary':kind)}<span>${escaped(name)}</span>${detail?`<small>${escaped(detail)}</small>`:''}</button></div>`);
      }
    }
    return output.join('');
  }
  async function toggleTableStructure(target){
    const id=tableKey(target);let branch=structureTree.get(id);
    if(!branch){branch={expanded:false,groups:new Set(),meta:null};structureTree.set(id,branch);}
    branch.expanded=!branch.expanded;renderTree();
    if(!branch.expanded||branch.loading||branch.meta)return;
    branch.loading=true;branch.error='';renderTree();
    try{branch.meta=await api('structure',{target});}catch(error){branch.error=error.message;}
    finally{branch.loading=false;renderTree();}
  }
  function renderTree() {
    if (!ui.profiles.length) { el('db-tree').innerHTML = `<div class="db-tree-note">${text('emptyConnections')}</div>`; return; }
    const output = [];
    for (const p of ui.profiles) {
      const catalog = ui.catalogs.get(p.id);
      output.push(`<div class="db-tree-row" draggable="true" data-reorder="${escaped(p.id)}"><button type="button" data-db-connect="${escaped(p.id)}"><span>${catalog?.expanded ? '▾' : '▸'} ◉ ${escaped(p.name)}</span><small>${escaped(p.engine)}</small></button><button class="db-tree-edit" type="button" data-db-edit="${escaped(p.id)}" aria-label="${text('editConnection')}">⚙</button></div>`);
      if (!catalog?.expanded) continue;
      if (catalog.loading || catalog.error) { output.push(`<div class="db-tree-note ${catalog.error ? 'error' : ''}">${escaped(catalog.error || text('loading'))}</div>`); continue; }
      for (const database of catalog.databases || []) {
        const target = { id: p.id, database }, targetKey = key(target), objects = ui.objects.get(targetKey), focused = ui.focus && key(ui.focus) === targetKey;
        output.push(`<div class="db-tree-row ${focused && !ui.table ? 'active' : ''}" style="padding-left:14px"><button type="button" data-db-database="${escaped(targetKey)}"><span aria-hidden="true">${objects?.expanded ? '▾' : '▸'}</span><span class="db-warehouse-icon" aria-hidden="true"></span><span>${escaped(database)}</span></button></div>`);
        if (!objects?.expanded) continue;
        if (objects.loading || objects.error) { output.push(`<div class="db-tree-note ${objects.error ? 'error' : ''}">${escaped(objects.error || text('loading'))}</div>`); continue; }
        if (!objects.items.length) output.push(`<div class="db-tree-note">${text('emptyObjects')}</div>`);
        let lastSchema = null;
        for (const [index, object] of objects.items.entries()) {
          if (object.schema !== lastSchema) { lastSchema = object.schema; if (object.schema !== database) output.push(`<div class="db-tree-note" style="padding-left:30px">${escaped(object.schema)}</div>`); }
          const selected = ui.table && key(ui.table) === targetKey && ui.table.schema === object.schema && ui.table.table === object.name;
          const tableTarget={...target,schema:object.schema,table:object.name},branch=structureTree.get(tableKey(tableTarget));
          output.push(`<div class="db-tree-row ${selected ? 'active' : ''}" style="padding-left:24px"><button class="db-tree-toggle" type="button" data-db-table-toggle="${escaped(targetKey)}" data-object-index="${index}" aria-expanded="${Boolean(branch?.expanded)}" aria-label="${escaped(text('toggleStructure'))}: ${escaped(object.name)}">${branch?.expanded?'▾':'▸'}</button><button type="button" data-db-table="${escaped(targetKey)}" data-object-index="${index}">${object.kind === 'view' ? '<span aria-hidden="true">∿</span>' : '<span class="db-table-icon" aria-hidden="true"></span>'}<span>${escaped(object.name)}</span><small>${text(object.kind === 'view' ? 'view' : 'table')}</small></button></div>`);
          if(branch?.expanded)output.push(structureRows(tableTarget,branch));
        }
      }
    }
    el('db-tree').innerHTML = output.join('');
  }
  async function toggleConnection(id) {
    let catalog = ui.catalogs.get(id);
    if (!catalog) { catalog = { expanded: false }; ui.catalogs.set(id, catalog); }
    catalog.expanded = !catalog.expanded; renderTree();
    if (catalog.expanded) await loadCatalog(id, Boolean(catalog.error));
  }
  async function loadObjects(target, force = false) {
    const id = key(target); let objects = ui.objects.get(id);
    ui.focus = target; renderTargets();
    if (objects?.expanded && !force) { objects.expanded = false; renderTree(); return; }
    if (objects?.items && !force) { objects.expanded = true; renderTree(); return; }
    if(force){for(const [branchID,branch] of structureTree){const parts=JSON.parse(branchID);if(parts[0]===target.id&&parts[1]===target.database){branch.meta=null;branch.error='';branch.expanded=false;}}for(const tab of ui.tableTabs){if(key(tab.target)===id){tab.schemaMeta=null;tab.metaError='';}}}
    objects = { expanded: true, loading: true, items: [] }; ui.objects.set(id, objects); renderTree();
    try { objects.items = await api('objects', { target }); }
    catch (error) { objects.error = error.message; }
    finally { objects.loading = false; renderTree(); }
  }
  function showTab(tab) {
    if (tab === 'sql') { const session = sqlSession(); if (session?.query || session?.message || session?.controller) activateBottom({ kind: 'result', id: session.id }); ui.editor.focus(); return; }
    ui.tab = tab; const table = tableSession(); if (table) { table.view = tab; if (ui.activeBottom?.kind !== 'table') { activateBottom({ kind: 'table', id: table.id }); return; } }
    paintBottom(); renderTargets(); if(tab==='structure')renderStructure();
  }
  function resizeGrids() {
    for (const [name, instance] of Object.entries(ui.grids)) {
      const container = el(`db-${name}-grid`);
      if (instance._databaseReady && container.offsetWidth && !container.querySelector('.tabulator-cell.tabulator-editing')) instance.redraw();
    }
    ui.editor?.resize();
  }
  function cellText(value) { if (value === null) return 'NULL'; if (value && typeof value === 'object' && value.blob !== undefined) return `[BLOB ${Math.floor(value.blob.length * .75)}]`; return value === undefined ? '' : String(value); }
  function databaseCellEditor(type) {
    const kind = /^(datetime|timestamp)(?:\b|\d)/i.test(type || '') ? 'datetime-local' : /^date\b/i.test(type || '') ? 'date' : /^time\b/i.test(type || '') ? 'time' : '';
    if (!kind) return 'input';
    return (cell, onRendered, success, cancel) => {
      const input = document.createElement('input'), original = cell.getValue(), raw = original == null ? '' : String(original);
      const zone = kind !== 'date' ? raw.match(/(?:Z|[+-]\d{2}:\d{2})$/)?.[0] || '' : '';
      input.type = kind; input.step = 'any';
      input.value = kind === 'date' ? raw.slice(0, 10) : (zone ? raw.slice(0, -zone.length) : raw).replace(' ', 'T').replace(/(\.\d{3})\d+$/, '$1');
      const initial = input.value;
      let finished = false;
      const commit = () => {
        if (finished) return;
        if (!input.validity.valid || (!input.value && raw && !initial)) { finished = true; cancel(); return; }
        finished = true;
        if (input.value === initial) { success(original); return; }
        if (!input.value) { success(null); return; }
        let value = input.value;
        // Native inputs omit :00 seconds. Keep SQL values explicit and preserve their timezone suffix.
        if (kind !== 'date' && /(?:T|^)\d{2}:\d{2}$/.test(value)) value += ':00';
        if (kind === 'datetime-local' && !raw.includes('T')) value = value.replace('T', ' ');
        success(value + zone);
      };
      input.addEventListener('blur', commit);
      input.addEventListener('keydown', event => {
        if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); if (!finished) { finished = true; cancel(); } }
        else if (event.key === 'Enter') { event.preventDefault(); event.stopPropagation(); commit(); }
      });
      onRendered(() => {
        input.focus({ preventScroll: true });
        // Opening directly from the cell click keeps the browser's user activation.
        try { input.showPicker?.(); } catch (_) { /* The calendar icon remains available for keyboard edits. */ }
      });
      return input;
    };
  }
  function newRowDefault(column) {
    const sql = column?.default?.trim();
    if (sql === undefined) return undefined;
    if (/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(sql)) return sql;
    if (/^'(?:[^']|'')*'$/.test(sql)) return sql.slice(1, -1).replaceAll("''", "'");
    return undefined;
  }
  function clearActiveDataCell(instance) {
    instance._activeDataElement?.classList.remove('db-cell-active');
    instance._activeDataRowElement?.classList.remove('db-row-current');
    instance._activeDataColumnElement?.classList.remove('db-column-current');
    instance._activeDataRowKey = null; instance._activeDataField = null;
  }
  function activateDataCell(cell) {
    const instance = cell.getTable(), row = cell.getRow();
    clearActiveDataCell(instance);
    instance._activeDataElement = cell.getElement(); instance._activeDataElement.classList.add('db-cell-active');
    instance._activeDataRowElement = row.getElement(); instance._activeDataRowElement.classList.add('db-row-current');
    instance._activeDataColumnElement = cell.getColumn().getElement(); instance._activeDataColumnElement.classList.add('db-column-current');
    instance._activeDataRowKey = row.getIndex(); instance._activeDataField = cell.getField();instance._selectionAnchor=row.getIndex();
    instance.deselectRow(); row.select();
  }
  function formatter(cell) {
    const span = document.createElement('span'), value = cell.getValue(); span.textContent = cellText(value); span.classList.toggle('db-cell-null', value === null);
    if (cell.getRow().getData().__rowKey) {
      const row = cell.getRow().getData(), index = Number(cell.getField().slice(1)), original = ui.original.get(row.__rowKey);
      if (row.__new && value === undefined && /^v\d+$/.test(cell.getField())) {
        const column = ui.tableResponse?.structure.columns.find(c => c.name === ui.tableResponse.result.columns[index]?.name), value = newRowDefault(column);
        span.textContent = value === undefined ? '(Default)' : value;
        span.classList.toggle('db-cell-default', value === undefined);
      }
      cell.getElement().classList.toggle('db-cell-dirty', row.__new ? value !== undefined : JSON.stringify(value) !== JSON.stringify(original?.[index]));
    }
    return span;
  }
  function grid(name, columns, data, extra = {}) {
    if (ui.grids[name]) { ui.grids[name]._databaseReady = false; ui.grids[name].destroy(); }
    const instance = new DatabaseUI.Tabulator(el(`db-${name}-grid`), { columns, data, autoResize: false, layout: 'fitDataFill', height: '100%', placeholder: text('noResults'), columnDefaults: { minWidth: 110, resizable: true, headerSort: true, formatter }, clipboard: true, ...extra });
    ui.grids[name] = instance;
    instance.on('tableBuilt', () => { instance._databaseReady = true; requestAnimationFrame(resizeGrids); });
    return instance;
  }
  function changes(tab) {
    const response = tab?.response || ui.tableResponse, rows = tab ? tab.rows : ui.grids.data?.getData(), originals = tab ? new Map(tab.original || []) : ui.original;
    if (!rows || !response) return [];
    const columns = response.result.columns, primary = response.structure.columns.filter(c => c.primary).map(c => c.name);
    return rows.flatMap(row => {
      const old = originals.get(row.__rowKey), values = {}, original = {}, identity = {};
      columns.forEach((c, i) => { const value = row[`v${i}`]; if (row.__new ? value !== undefined : JSON.stringify(value) !== JSON.stringify(old?.[i])) { values[c.name] = value; if (!row.__new) original[c.name] = old[i]; } if (primary.includes(c.name) && old) identity[c.name] = old[i]; });
      if (row.__delete) { columns.forEach((c, i) => { original[c.name] = old[i]; }); return [{ operation: 'delete', key: identity, original }]; }
      if (row.__new) return [{ operation: 'insert', values }];
      return Object.keys(values).length ? [{ operation: 'update', key: identity, original, values }] : [];
    });
  }
  function syncDataActions() {
    const tab = tableSession(), ready = Boolean(ui.grids.data?._databaseReady), busy = Boolean(ui.committing || tab?.saving || tab?.loading || tab?.response && !ready), selected = ready ? ui.grids.data.getSelectedData() : [];
    el('db-data-grid').inert = busy;
    el('db-add-row').disabled = busy || !tab?.response || tab.target.kind === 'view';
    el('db-delete-row').disabled = busy || !selected.some(row => !row.__delete && (row.__new || tab?.response?.editable));
    el('db-save-rows').disabled = busy || !tab?.dirty;
    el('db-rollback-rows').disabled = busy || !tab?.dirty;
    syncCommitAll();
    el('db-reload-table').disabled = busy || !tab;
    el('db-stop-table').disabled = !tab?.loading || tab.saving;
    for (const id of ['db-page-prev', 'db-page-next', 'db-page-last', 'db-page-size']) {
      const disabled = busy || !tab?.response || (id === 'db-page-prev' ? !tab.page : id === 'db-page-size' ? false : !tab.response.result.truncated);
      el(id).disabled = disabled;
    }
  }
  function updateDirty() { ui.dirty = changes().length; const tab = tableSession(); if (tab) { tab.dirty = ui.dirty; tab.message = ''; tab.error = ''; } syncDataActions(); status(ui.dirty ? text('dirty', { count: ui.dirty }) : ui.tableResponse?.editable === false ? text('readOnly') : ''); renderWorkspaceTabs(); }
  function rowSelectionColumn(name, finishEdit) {
    const selectRow = (event, cell) => {
      const row = cell.getRow(), instance = row.getTable(), rows = instance.getRows('active');
      finishEdit(); clearActiveDataCell(instance);
      if (event.shiftKey && instance._selectionAnchor != null) {
        const start = rows.findIndex(item => item.getIndex() === instance._selectionAnchor), end = rows.indexOf(row);
        if (!(event.ctrlKey || event.metaKey)) instance.deselectRow();
        if (start >= 0) rows.slice(Math.min(start, end), Math.max(start, end) + 1).forEach(item => item.select());
        else row.select();
      } else {
        if (event.ctrlKey || event.metaKey) row.toggleSelect();
        else { instance.deselectRow(); row.select(); }
        instance._selectionAnchor = row.getIndex();
      }
      const container=el(`db-${name}-grid`);container.tabIndex=0;container.focus({preventScroll:true});
    };
    return {title:'',field:'__select',width:9,minWidth:9,maxWidth:9,frozen:true,resizable:false,formatter:()=>'',headerSort:false,cellClick:selectRow};
  }
  function renderData(draft) {
    const response = ui.tableResponse; if (!response) return;
    ui.original = draft?.rows ? new Map(draft.original) : new Map();
    const data = draft?.rows ? clone(draft.rows) : response.result.rows.map((values, index) => { const row = { __rowKey: `row-${index}` }; values.forEach((value, i) => { row[`v${i}`] = value; }); ui.original.set(row.__rowKey, clone(values)); return row; });
    const editable = cell => { const row = cell.getRow().getData(), index = Number(cell.getField().slice(1)), name = response.result.columns[index]?.name, meta = response.structure.columns.find(c => c.name === name); return !row.__delete && (row.__new || response.editable) && !meta?.generated && !(cell.getValue() && typeof cell.getValue() === 'object'); };
    const columns = [rowSelectionColumn('data',finishDataEdit), ...response.result.columns.map((c, i) => ({ title: escaped(c.name), field: `v${i}`, headerTooltip: c.type, editor: databaseCellEditor(c.type), editable, contextMenu: [{ label: text('setNull'), action: (event, cell) => { if (editable(cell)) { cell.setValue(null); updateDirty(); } } }] }))];
    const instance = grid('data', columns, data, { clipboard: false, index: '__rowKey', editTriggerEvent: 'click', selectableRows: 'highlight', initialSort: draft?.sort || [], rowFormatter: row => { row.getElement().classList.toggle('db-row-deleted', Boolean(row.getData().__delete)); row.getElement().classList.toggle('db-row-new', Boolean(row.getData().__new)); row.getElement().classList.toggle('db-row-current', row.getIndex() === row.getTable()._activeDataRowKey); } });
    instance.on('cellClick', (event, cell) => { if (/^v\d+$/.test(cell.getField())) activateDataCell(cell); });
    instance.on('cellEditing', activateDataCell);
    instance.on('rowSelectionChanged', syncDataActions);
    instance.on('tableBuilt', syncDataActions);
    instance.on('cellEdited', cell => { const row = cell.getRow(), field = cell.getField(); row.reformat(); activateDataCell(row.getCell(field)); updateDirty(); });
    if (draft) instance.on('tableBuilt', () => { if (ui.currentTableId !== draft.id) return; if (draft.selected?.length) instance.selectRow(draft.selected); requestAnimationFrame(() => { const holder = el('db-data-grid').querySelector('.tabulator-tableholder'); if (holder) holder.scrollTop = draft.scroll || 0; }); });
    ui.dirty = draft?.dirty || 0; el('db-save-rows').disabled = !ui.dirty || Boolean(draft?.saving);
    el('db-delete-row').disabled = !response.editable; el('db-add-row').disabled = ui.table?.kind === 'view';
    el('db-data-count').textContent = text('rows', { count: data.length }); el('db-page-label').textContent = text('page', { page: ui.page + 1 });
    el('db-page-prev').disabled = ui.page === 0; el('db-page-next').disabled = !response.result.truncated; el('db-page-last').disabled = !response.result.truncated;
    syncDataActions();
  }
  function renderStructure() {
    if (!ui.tableResponse) return;
    const tab=tableSession(),meta = tab?.schemaMeta || ui.tableResponse.structure;
    grid('structure', [{ title: text('name'), field: 'name' }, { title: text('type'), field: 'type' }, { title: text('nullable'), field: 'nullable', formatter: 'tickCross' }, { title: text('primary'), field: 'primary', formatter: 'tickCross' }, { title: text('default'), field: 'default' }, { title: text('generated'), field: 'generated', formatter: 'tickCross' }], meta.columns, { selectableRows: 1 });
    ui.grids.structure.on('rowDblClick', (event, row) => { row.select(); openDesigner('alterColumn'); });
    const list=cell=>{const span=document.createElement('span');span.textContent=(cell.getValue()||[]).join(', ');return span;};
    grid('index', [{ title: text('name'), field: 'name' }, { title: text('indexColumns'), field: 'columns', formatter:list }, { title: text('unique'), field: 'unique', formatter: 'tickCross' }, { title: text('primary'), field: 'primary', formatter: 'tickCross' },{title:text('method'),field:'method'},{title:text('comment'),field:'comment'}], meta.indexes||[], { selectableRows: 1 });
    grid('foreign-key',[{title:text('name'),field:'name'},{title:text('fields'),field:'columns',formatter:list},{title:text('referencedSchema'),field:'schema'},{title:text('referencedTable'),field:'table'},{title:text('referencedColumns'),field:'referencedColumns',formatter:list},{title:text('onDelete'),field:'onDelete'},{title:text('onUpdate'),field:'onUpdate'}],meta.foreignKeyDetails||[]);
    grid('check',[{title:text('name'),field:'name'},{title:text('expression'),field:'expression',minWidth:260},{title:text('enforced'),field:'enforced',formatter:'tickCross'}],meta.checkDetails||[]);
    grid('trigger',[{title:text('name'),field:'name'},{title:text('timing'),field:'timing'},{title:text('event'),field:'event'},{title:text('definition'),field:'definition',minWidth:360}],meta.triggerDetails||[]);
    paintStructurePage();ensureDetailedStructure(tab);
  }
  const commentDirty=tab=>Boolean(tab?.schemaMeta && tab.commentDraft!==undefined && tab.commentDraft!==tab.schemaMeta.comment);
  function paintStructurePage(){
    const tab=tableSession();if(!tab)return;
    const kind=tab.structureView||'columns';
    view.querySelectorAll('[data-db-structure-tab]').forEach(button=>button.classList.toggle('active',button.dataset.dbStructureTab===kind));
    view.querySelectorAll('[data-db-structure-page]').forEach(panel=>panel.classList.toggle('hidden',panel.dataset.dbStructurePage!==kind));
    el('db-meta-status').textContent=tab.metaLoading?text('loading'):tab.metaError||(kind==='triggers'?tab.schemaMeta?.triggerError:'')||'';
    el('db-meta-reload').disabled=Boolean(tab.metaLoading||tab.commentBusy);
    const supported=Boolean(tab.schemaMeta?.commentSupported);
    if(tab.commentDraft===undefined&&tab.schemaMeta)tab.commentDraft=tab.schemaMeta.comment||'';
    el('db-table-comment').value=tab.commentDraft||'';el('db-table-comment').disabled=!supported||Boolean(tab.commentBusy);
    el('db-comment-preview').disabled=!supported||Boolean(tab.commentBusy)||!commentDirty(tab);
    el('db-comment-save').disabled=!supported||Boolean(tab.commentBusy)||!tab.commentPlan||tab.commentPlan.comment!==tab.commentDraft;
    el('db-comment-status').textContent=tab.commentMessage||(!supported&&!tab.metaLoading?text('commentUnsupported'):'');
    el('db-comment-sql').textContent=tab.commentPlan?.sql.join(';\n\n')||'';el('db-comment-sql').classList.toggle('hidden',!tab.commentPlan);
    requestAnimationFrame(resizeGrids);
  }
  async function ensureDetailedStructure(tab){
    if(!tab||tab.schemaMeta||tab.metaLoading||tab.metaError)return;
    tab.metaLoading=true;paintStructurePage();
    try{tab.schemaMeta=await api('structure',{target:tab.target});}
    catch(error){tab.metaError=error.message;}
    finally{tab.metaLoading=false;if(ui.tableTabs.includes(tab)&&tableSession()===tab)renderStructure();}
  }
  async function openStructurePage(id,kind){
    const [connection,database,schema,table]=JSON.parse(id),target={id:connection,database,schema,table};
    await openTable(target);
    if(!ui.table||tableKey(ui.table)!==id)return;
    tableSession().structureView=kind;showTab('structure');paintStructurePage();
  }
  async function commentAction(apply){
    const tab=tableSession();if(!tab||tab.commentBusy||!tab.schemaMeta?.commentSupported)return;
    const comment=tab.commentDraft||'',plan=tab.commentPlan;
    if(apply&&(!plan||plan.comment!==comment))return;
    tab.commentBusy=true;tab.commentMessage=text('loading');paintStructurePage();
    try{
      const result=await api('schema',{target:tab.target,body:{action:'setComment',comment,...(apply?{hash:plan.hash}:{})}});
      if(apply){tab.schemaMeta.comment=comment;tab.commentPlan=null;tab.commentMessage=text('applied');const branch=structureTree.get(tableKey(tab.target));if(branch?.meta)branch.meta.comment=comment;}
      else{tab.commentPlan={...result,comment};tab.commentMessage='';}
    }catch(error){tab.commentMessage=error.message;tab.commentPlan=null;}
    finally{tab.commentBusy=false;if(tableSession()===tab)paintStructurePage();}
  }
  window.addEventListener('beforeunload',event=>{if(ui.tableTabs.some(commentDirty)){event.preventDefault();event.returnValue='';}});
  async function fetchTableRecord(tab, page = tab.page || 0, last = false) {
    const previousPage = tab.response ? Math.floor((tab.response.offset || 0) / (tab.response.limit || Number(tab.pageSize || 100))) : 0;
    tab.loadController?.abort(); const controller = new AbortController(); tab.loadController = controller;
    const nonce = (tab.loadNonce || 0) + 1; tab.loadNonce = nonce; tab.loading = true; tab.error = ''; tab.message = ''; tab.page = page;
    const progress=beginDatabaseLoad(tab,controller);
    if (ui.activeBottom?.kind === 'table' && ui.activeBottom.id === tab.id) { el('db-data-grid').inert = true; syncDataActions(); paintBottom(); }
    renderWorkspaceTabs();
    try {
      const response = await api('table', { target: { ...tab.target, offset: page * Number(tab.pageSize || 100), limit: tab.pageSize || 100, ...(last ? { last: 1 } : {}) }, signal: controller.signal,progress });
      if (nonce !== tab.loadNonce || !ui.tableTabs.includes(tab)) return;
      progress.records=response.result.rows.length;
      tab.response = response; if (last) tab.page = Math.floor(response.offset / response.limit); tab.rows = null; tab.original = null; tab.selected = []; tab.dirty = 0; tab.error = '';
    } catch (error) { if (nonce === tab.loadNonce) { tab.page = previousPage; if (tab.response?.limit) tab.pageSize = String(tab.response.limit); if (controller.signal.aborted) { tab.message = text('loadingStopped'); } else tab.error = error.message; } }
    finally {
      finishDatabaseLoad(tab,progress);
      if (nonce === tab.loadNonce && ui.tableTabs.includes(tab)) {
        tab.loading = false; tab.loadController = null;
        if (ui.activeBottom?.kind === 'table' && ui.activeBottom.id === tab.id) { restoreTableState(tab); paintBottom(); renderTree(); renderTargets(); }
        renderWorkspaceTabs();
      }
    }
  }
  async function openTable(target, options = {}) {
    captureTableState();
    const existing = ui.tableTabs.find(tab => tableKey(tab.target) === tableKey(target));
    if (existing?.saving) { activateBottom({ kind: 'table', id: existing.id }); return; }
    const reload = options.reload || options.confirm === false || options.page !== undefined || options.last;
    if (existing && !reload) { activateBottom({ kind: 'table', id: existing.id }); return; }
    if (existing?.dirty && options.confirm !== false && !window.confirm(text('discard'))) return;
    const tab = existing || { id: `table-${++ui.tableSequence}`, target: { ...target }, view: 'data', page: 0, pageSize: '100', response: null, dirty: 0 };
    if (!existing) ui.tableTabs.push(tab);
    if (options.pageSize) tab.pageSize = options.pageSize;
    const sqlId = ui.activeSQL;
    activateBottom({ kind: 'table', id: tab.id });
    await fetchTableRecord(tab, options.page ?? tab.page, options.last);
    if (tab.response && ui.activeSQL === sqlId && !ui.editor.value().trim()) {
      const engine = profile(target.id)?.engine;
      ui.editor.setValue(`SELECT * FROM ${engine === 'postgres' ? quote(engine, target.schema) + '.' : ''}${quote(engine, target.table)} LIMIT 100;`);
    }
  }
  async function saveRows() {
    if (ui.saving || ui.committing) return;
    finishDataEdit();
    const pending = changes(), tab = tableSession(); if (!pending.length || !tab) return;
    captureTableState(); tab.saving = true; ui.saving = true; el('db-data-grid').inert = true; syncDataActions();
    try {
      const result = await api('changes', { target: { ...tab.target }, body: { changes: pending } });
      tab.response = null; tab.rows = null; tab.original = null; tab.dirty = 0;
      if (ui.currentTableId === tab.id) { ui.tableResponse = null; ui.dirty = 0; ui.original = new Map(); }
      await fetchTableRecord(tab);
      tab.message = text('savedRows', { count: result.affected });
    } catch (error) { tab.error = error.message; tab.message = ''; }
    finally {
      tab.saving = false;
      if (ui.currentTableId === tab.id) { ui.saving = false; el('db-data-grid').inert = Boolean(tab.loading); syncDataActions(); }
      if (ui.activeBottom?.kind === 'table' && ui.activeBottom.id === tab.id) paintBottom();
      renderWorkspaceTabs();
    }
  }

  function finishDataEdit() {
    const input = el('db-data-grid').querySelector('.tabulator-cell.tabulator-editing input, .tabulator-cell.tabulator-editing textarea');
    if (input) { input.dispatchEvent(new Event('change', { bubbles: true })); input.blur(); }
  }
  function rollbackRows() {
    const tab = tableSession(); if (!tab?.response || tab.saving || tab.loading) return;
    tab.rows = null; tab.original = null; tab.selected = []; tab.dirty = 0; tab.error = ''; tab.message = '';
    renderData(); updateDirty(); el('db-data-grid').focus({ preventScroll: true });
  }

  async function addDataRow() {
    if (!ui.table || !ui.tableResponse || ui.table.kind === 'view' || ui.saving || tableSession()?.loading) return;
    const row = await ui.grids.data.addRow({ __rowKey: `new-${++ui.sequence}`, __new: true }, false);
    ui.grids.data.deselectRow(); row.select();
    ui.grids.data._selectionAnchor = row.getIndex();
    await row.scrollTo(); el('db-data-grid').focus({ preventScroll: true }); updateDirty();
    const first = row.getCells().find(cell => /^v\d+$/.test(cell.getField()) && !ui.tableResponse.structure.columns.find(c => c.name === ui.tableResponse.result.columns[Number(cell.getField().slice(1))]?.name)?.generated);
    if (first) activateDataCell(first);
  }
  function initializeDataClipboard() {
    const container = el('db-data-grid'), mime = 'application/x-log-agent-rows+json';
    container.tabIndex = 0;
    container.addEventListener('wheel', event => {
      const instance = ui.grids.data;
      if (!instance?._databaseReady || !instance._activeDataField || container.inert || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || !event.deltaY || Math.abs(event.deltaX) > Math.abs(event.deltaY) || !event.target.closest('.tabulator-tableholder')) return;
      event.preventDefault(); event.stopPropagation();
      const delta = event.deltaY * (event.deltaMode === 1 ? 40 : event.deltaMode === 2 ? container.clientHeight : 1), now = performance.now();
      if (now - (instance._wheelTime || 0) > 200 || Math.sign(delta) !== Math.sign(instance._wheelDelta || delta)) instance._wheelDelta = 0;
      instance._wheelTime = now; instance._wheelDelta = (instance._wheelDelta || 0) + delta;
      if (Math.abs(instance._wheelDelta) < 80) return;
      const direction = Math.sign(instance._wheelDelta); instance._wheelDelta = 0;
      const rows = instance.getRows('active').filter(row => !row.getData().__delete), index = rows.findIndex(row => row.getIndex() === instance._activeDataRowKey), row = rows[index + direction], field = instance._activeDataField;
      if (index < 0 || !row) return;
      finishDataEdit(); activateDataCell(row.getCell(field));
      row.scrollTo('nearest', false).catch(() => {});
      container.focus({ preventScroll: true });
    }, { passive: false, capture: true });
    const editing = event => event.target.closest('input:not([type="checkbox"]), textarea, [contenteditable="true"]');
    container.addEventListener('copy', event => {
      if (editing(event) || !event.clipboardData || !ui.tableResponse) return;
      const rows = ui.grids.data.getRows('active').filter(row => row.isSelected() && !row.getData().__delete).map(row => ui.tableResponse.result.columns.map((c, i) => row.getData()[`v${i}`]));
      if (!rows.length) return;
      event.preventDefault(); event.stopImmediatePropagation();
      event.clipboardData.setData(mime, JSON.stringify({ columns: ui.tableResponse.result.columns.map(c => c.name), rows }));
      const encode = value => { const s = value === null || value === undefined ? 'NULL' : typeof value === 'object' ? JSON.stringify(value) : String(value); return /[\t\r\n"]/.test(s) || s === 'NULL' && typeof value === 'string' ? `"${s.replaceAll('"', '""')}"` : s; };
      event.clipboardData.setData('text/plain', rows.map(row => row.map(encode).join('\t')).join('\r\n'));
    }, true);
    // Quoted TSV preserves tabs, line breaks, empty strings and literal "NULL".
    function parseTSV(source) {
      const rows = []; let row = [], value = '', quoted = false, wasQuoted = false;
      const field = () => { row.push(!wasQuoted && value === 'NULL' ? null : value); value = ''; wasQuoted = false; };
      for (let i = 0; i < source.length; i++) {
        const c = source[i];
        if (c === '"' && (quoted || !value)) { if (quoted && source[i + 1] === '"') { value += '"'; i++; } else { quoted = !quoted; wasQuoted = true; } }
        else if (!quoted && (c === '\t' || c === '\n' || c === '\r')) { field(); if (c !== '\t') { if (c === '\r' && source[i + 1] === '\n') i++; rows.push(row); row = []; } }
        else value += c;
      }
      if (quoted) throw new Error(text('invalidRowPaste'));
      if (value || wasQuoted || row.length || !rows.length) { field(); rows.push(row); }
      return rows;
    }
    container.addEventListener('paste', async event => {
      if (editing(event) || !event.clipboardData || !ui.tableResponse || ui.table?.kind === 'view' || ui.saving || tableSession()?.loading) return;
      event.preventDefault(); event.stopImmediatePropagation();
      try {
        const raw = event.clipboardData.getData(mime), plain = event.clipboardData.getData('text/plain');
        if (!raw && !plain) return;
        const columns = ui.tableResponse.result.columns, payload = raw ? JSON.parse(raw) : null;
        if (payload && (JSON.stringify(payload.columns) !== JSON.stringify(columns.map(c => c.name)) || !Array.isArray(payload.rows))) throw new Error(text('invalidRowPaste'));
        const values = payload ? payload.rows : parseTSV(plain);
        if (!values.length || values.some(row => !Array.isArray(row) || row.length !== columns.length)) throw new Error(text('invalidRowPaste'));
        const instance = ui.grids.data, selected = instance.getSelectedRows();
        const targets = selected.filter(row => row.getData().__new && !row.getData().__delete);
        const pasted = [];
        for (let n = 0; n < values.length; n++) {
          const data = {};
          columns.forEach((c, i) => { if (!ui.tableResponse.structure.columns.find(meta => meta.name === c.name)?.generated) data[`v${i}`] = values[n][i]; });
          let row = targets[n];
          if (row) await row.update(data);
          else row = await instance.addRow({ ...data, __rowKey: `new-${++ui.sequence}`, __new: true }, false);
          row.reformat(); pasted.push(row);
        }
        instance.deselectRow(); pasted.forEach(row => row.select());
        await pasted[0].scrollTo(); container.focus({ preventScroll: true }); updateDirty();
      } catch (error) { updateDirty(); status(error.message, true); }
    }, true);
    el('db-panel-data').addEventListener('keydown', event => {
      if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 's' || event.altKey) return;
      event.preventDefault(); event.stopPropagation();
      saveRows();
    }, true);
  }

  function captureResultState() {
    finishResultEdit();
    const item = ui.activeBottom, session = item?.kind === 'result' && ui.sqlTabs.find(tab => tab.id === item.id);
    if (!session?.query || !ui.grids.query?._databaseReady || ui.resultGridOwner !== session.id) return;
    session.selectedResult = ui.grids.query.getSelectedData().map(row => row.__resultRow);
    session.filters = ui.grids.query.getFilters(); session.headerFilters = ui.grids.query.getHeaderFilters(); session.sort = ui.grids.query.getSorters().map(sort => ({ column: sort.field, dir: sort.dir }));
    const holder = el('db-query-grid').querySelector('.tabulator-tableholder'); session.scroll = holder ? { top: holder.scrollTop, left: holder.scrollLeft } : null;
  }
  function queryCellEditable(result, cell) {
    if (result.saving || ui.committing) return false;
    const index = cell.getRow().getData().__resultRow, row = result.rows[index];
    if (!row || row.deleted || !/^v\d+$/.test(cell.getField())) return false;
    const edit = result.sources[row.source]?.edit, column = edit?.columns[result.columns[Number(cell.getField().slice(1))]?.name];
    if (!column || column.generated || (cell.getValue() !== null && typeof cell.getValue() === 'object')) return false;
    return Object.entries(edit.columns).filter(([, meta]) => meta.primary).every(([label]) => {
      const value = row.values[result.columns.findIndex(item => item.name === label)];
      return value != null && typeof value !== 'object';
    });
  }
  const queryDirtyCount = result => (result?.rows || []).filter(row => row.draft || row.deleted).length;
  const resultOwner = () => ui.activeBottom?.kind === 'result' && ui.sqlTabs.find(tab => tab.id === ui.activeBottom.id);
  function finishResultEdit() {
    const input = el('db-query-grid').querySelector('.tabulator-editing input, .tabulator-editing textarea');
    if (input) { input.dispatchEvent(new Event('change', { bubbles:true })); input.blur(); }
  }
  function pendingPageCount() { return ui.sqlTabs.filter(tab => queryDirtyCount(tab.query)).length + ui.tableTabs.filter(tab => tab.dirty).length; }
  function syncCommitAll() {
    for (const id of ['db-query-commit-all','db-data-commit-all']) if (el(id)) { el(id).disabled = Boolean(ui.committing) || !pendingPageCount(); el(id).title = text('pendingPages',{count:pendingPageCount()}); }
  }
  function syncResultActions() {
    const owner = resultOwner(), result = owner?.query, busy = Boolean(ui.committing || result?.saving || owner?.controller), ready = ui.grids.query?._databaseReady && ui.resultGridOwner === owner?.id;
    el('db-query-grid').inert = Boolean(ui.committing || result?.saving);
    el('db-query-delete').disabled = !ready || busy || !(ui.grids.query?.getSelectedData() || []).some(item => { const row = result?.rows[item.__resultRow]; return row && !row.deleted && Boolean(DatabaseUI.resultRowChange(result,{...row,deleted:true})); });
    el('db-query-commit').disabled = busy || !queryDirtyCount(result);
    el('db-query-rollback').disabled = busy || !queryDirtyCount(result);
    el('db-query-reload').disabled = busy || !result?.request;
    el('db-query-stop').disabled = !owner?.controller;
    el('db-query-pending').textContent = queryDirtyCount(result) ? text('resultPending',{count:queryDirtyCount(result)}) : result ? text('rows',{count:result.rows.length}) : '';
    syncCommitAll();
  }
  function stageQueryCell(result, owner, instance, cell) {
    const row = result.rows[cell.getRow().getData().__resultRow], index = Number(cell.getField().slice(1));
    if (!row || owner?.query !== result || !queryCellEditable(result,cell)) return;
    row.draft = [...(row.draft || row.values)]; row.draft[index] = cell.getValue();
    if (JSON.stringify(row.draft) === JSON.stringify(row.values)) delete row.draft;
    result.editMessage = ''; result.editError = false; cell.getRow().reformat();
    const note = el('db-query-summary').querySelector('.db-query-edit-status');
    if (note) { note.textContent = text(queryDirtyCount(result) ? 'resultPending':'resultEditHint',{count:queryDirtyCount(result)}); note.classList.remove('db-error'); }
    syncResultActions(); renderWorkspaceTabs();
  }
  function deleteQueryRows() {
    finishResultEdit(); const owner = resultOwner(), result = owner?.query;
    if (!result || result.saving || ui.committing) return;
    for (const item of ui.grids.query?.getSelectedRows() || []) {
      const row = result.rows[item.getData().__resultRow];
      if (row && DatabaseUI.resultRowChange(result,{...row,deleted:true})) { row.deleted = true; item.reformat(); }
    }
    result.editMessage = ''; result.editError = false; captureResultState(); paintBottom();
  }
  function rollbackQueryRows() {
    const owner = resultOwner(), result = owner?.query;
    if (!result || result.saving || ui.committing) return;
    // Cancel an in-progress editor before restoring the loaded values.
    el('db-query-grid').querySelector('.tabulator-editing input, .tabulator-editing textarea')?.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));
    for (const row of result.rows) { delete row.draft; delete row.deleted; }
    owner.selectedResult = [];
    result.editMessage = ''; result.editError = false; paintBottom();
  }
  async function commitEdits(all = false) {
    if (ui.committing || ui.tableTabs.some(tab => tab.saving) || ui.sqlTabs.some(tab => tab.query?.saving)) return;
    finishResultEdit(); finishDataEdit(); captureTableState(); captureResultState();
    const owners = all ? ui.sqlTabs.filter(tab => queryDirtyCount(tab.query)) : [resultOwner()].filter(Boolean);
    const tableOwners = all ? ui.tableTabs.filter(tab => tab.dirty) : [], entries = [];
    let groups;
    try {
      for (const owner of owners) for (const row of owner.query?.rows || []) {
        const entry = DatabaseUI.resultRowChange(owner.query,row);
        if (entry) entries.push({...entry,owner,row,kind:'result'});
      }
      for (const owner of tableOwners) for (const change of changes(owner)) entries.push({target:owner.target,change,owner,kind:'table'});
      groups = DatabaseUI.mergeChangeEntries(entries);
    } catch (error) { return status(text('conflictingDrafts'),true); }
    if (!groups.length) return;
    ui.committing = true;
    for (const owner of owners) if (owner.query) { owner.query.saving = true; owner.query.editMessage = ''; owner.query.editError = false; }
    for (const owner of tableOwners) { owner.saving = true; owner.error = ''; }
    syncResultActions(); syncDataActions();
    const failures = [], totals = new Map(), savedTables = new Set();
    try {
      await Promise.all(groups.map(async group => {
        try {
          await api('changes',{target:group.target,body:{batches:group.batches}});
          for (const entry of group.entries) {
            if (entry.kind === 'result') {
              if (entry.change.operation === 'delete') entry.row.committedDelete = true;
              else {
                for (const [label,meta] of Object.entries(entry.owner.query.sources[entry.row.source].edit.columns)) {
                  if (Object.hasOwn(entry.change.values,meta.name)) entry.row.values[entry.owner.query.columns.findIndex(column => column.name === label)] = entry.change.values[meta.name];
                }
              }
              delete entry.row.draft; delete entry.row.deleted;
            } else savedTables.add(entry.owner);
            totals.set(entry.owner,(totals.get(entry.owner) || 0)+1);
          }
        } catch (error) {
          failures.push(`${sourceName(group.target)}: ${error.message}`);
          for (const entry of group.entries) {
            if (entry.kind === 'result') { entry.owner.query.editError = true; entry.owner.query.editMessage = error.message; }
            else entry.owner.error = error.message;
          }
        }
      }));
      for (const owner of owners) if (owner.query) {
        owner.query.rows = owner.query.rows.filter(row => !row.committedDelete);
        owner.selectedResult = [];
        if (!owner.query.editError) owner.query.editMessage = text('transactionDone',{count:totals.get(owner) || 0});
        else if (totals.get(owner)) owner.query.editMessage = text('transactionPartial') + ' ' + owner.query.editMessage;
      }
      for (const owner of savedTables) {
        owner.rows = null; owner.original = null; owner.dirty = 0;
        if (ui.currentTableId === owner.id) { ui.dirty = 0; ui.original = new Map(); }
        await fetchTableRecord(owner);
        owner.message = text('transactionDone',{count:totals.get(owner) || 0});
      }
    } finally {
      ui.committing = false;
      for (const owner of owners) if (owner.query) owner.query.saving = false;
      for (const owner of tableOwners) owner.saving = false;
      if (ui.currentTableId) ui.saving = Boolean(tableSession()?.saving);
      if (ui.activeBottom?.kind === 'table' && savedTables.has(tableSession())) restoreTableState(tableSession());
      paintBottom(); syncResultActions(); syncDataActions();
      if (failures.length) status(text('transactionPartial')+' '+failures.join(' · '),true);
    }
  }
  function renderQuery(result) {
    const owner = ui.activeBottom?.kind === 'result' ? ui.sqlTabs.find(tab => tab.id === ui.activeBottom.id) : null;
    const editable = cell => queryCellEditable(result, cell);
    const statementOnly = result.mode === 'execute' && !result.columns.length;
    const selectionColumn = rowSelectionColumn('query',finishResultEdit);
    const columns = [...(statementOnly ? []:[selectionColumn]), { title: text('sourceConnection'), field: '__source', minWidth: 150, formatter: 'plaintext' }, { title: text('sourceDatabase'), field: '__database', formatter: 'plaintext' }, { title: text('sourceEngine'), field: '__engine', formatter: 'plaintext' }, ...(statementOnly ? [ { title: text('executionStatus'), field: '__status', formatter: 'plaintext' }, { title: text('affectedTitle'), field: '__affected', formatter: 'plaintext' }, { title: text('duration'), field: '__duration', formatter: 'plaintext' }, { title: text('errorDetail'), field: '__error', formatter: 'plaintext', minWidth: 220 } ] : result.columns.map((c, i) => ({ title: escaped(c.name), field: `v${i}`, headerTooltip: c.type, editor: databaseCellEditor(c.type), editorParams:{selectContents:true}, editable, formatter: cell => { const node = formatter(cell), row = result.rows[cell.getRow().getData().__resultRow]; cell.getElement().classList.toggle('db-cell-dirty', JSON.stringify(cell.getValue()) !== JSON.stringify(row?.values[i])); return node; }, contextMenu: [{ label: text('setNull'), action: (event, cell) => { if (editable(cell)) cell.setValue(null); } }, {label:text('deleteRows'),action:(event,cell)=>{if (!cell.getRow().isSelected()) {cell.getTable().deselectRow();cell.getRow().select();} deleteQueryRows();}}] })))];
    const origin = source => ({ __source: source.name || profile(source.target.id)?.name || source.target.id, __database: source.edit?.target.database || source.target.database, __engine: source.engine });
    const data = statementOnly ? result.sources.map(source => ({ ...origin(source), __status: text(source.error ? 'failed' : 'completed'), __affected: source.error ? '—' : source.affected ?? text('unknownAffected'), __duration: source.milliseconds, __error: source.error || '' })) : result.rows.map((row, index) => { const record = { ...origin(result.sources[row.source]), __resultRow: index }; (row.draft || row.values).forEach((value, i) => { record[`v${i}`] = value; }); return record; });
    const instance = grid('query', columns, data, { index:'__resultRow', selectableRows:'highlight', rowFormatter: row => {row.getElement().classList.toggle('db-row-deleted', Boolean(result.rows[row.getData().__resultRow]?.deleted));row.getElement().classList.toggle('db-row-current',row.getIndex()===row.getTable()._activeDataRowKey);}, editTriggerEvent: 'click', initialFilter: owner?.filters || [], initialHeaderFilter: owner?.headerFilters || [], initialSort: owner?.sort || [] });
    ui.resultGridOwner = owner?.id;
    el('db-query-grid').inert = Boolean(result.saving);
    if(!statementOnly) {
      instance.on('cellClick',(event,cell)=>{if(cell.getField()!=='__select') activateDataCell(cell);});
      instance.on('cellEditing',activateDataCell);
    }
    instance.on('cellEdited', cell => stageQueryCell(result, owner, instance, cell));
    instance.on('rowSelectionChanged',syncResultActions);
    instance.on('tableBuilt',()=>{if (owner?.selectedResult?.length) instance.selectRow(owner.selectedResult); syncResultActions();});
    if (owner?.scroll) instance.on('tableBuilt', () => requestAnimationFrame(() => { if (ui.resultGridOwner !== owner.id) return; const holder = el('db-query-grid').querySelector('.tabulator-tableholder'); const scroll = owner.scroll; if (holder && scroll) { holder.scrollTop = scroll.top; holder.scrollLeft = scroll.left; } }));
    const succeeded = result.sources.filter(source => !source.error).length;
    const execution = result.mode === 'execute';
    const unknownCount = result.sources.filter(source => !source.error && source.executed && source.affected == null).length;
    const summary = text(execution ? 'executionDone' : 'queryDone', { rows: result.rows.length, count: result.affected, ok: succeeded, total: result.sources.length, ms: result.elapsed || 0 }) + (execution && unknownCount ? ' · ' + text('unknownAffectedSources', { count: unknownCount }) : '');
    const sourceMessage = source => source.error ? escaped(source.error) : [source.affected != null ? text('affectedRows', { count: source.affected }) : source.executed ? text('completed') + ' · ' + text('unknownAffected') : '', source.rowCount || !source.executed ? text('rows', { count: source.rowCount ?? 0 }) : '', source.result.truncated ? text('truncated') : ''].filter(Boolean).join(' · ');
    el('db-query-summary').innerHTML = `${escaped(summary)}${execution && result.sources.length > 1 ? `<div>${text('independentExecution')}</div>` : ''}${succeeded !== result.sources.length ? `<div class="db-error">${text(execution ? 'executionPartial' : 'partial')}</div>` : ''}<details open><summary>${text('details')}</summary>${result.sources.map(source => `<div class="db-query-source ${source.error ? 'error' : ''}">${escaped(source.name || profile(source.target.id)?.name || source.target.id)} / ${escaped(source.target.database)} · ${source.milliseconds} ms · ${sourceMessage(source)}</div>`).join('')}</details>`;
    if (!statementOnly) {
      const message = result.saving ? text('resultSaving') : result.editMessage || (queryDirtyCount(result) ? text('resultPending',{count:queryDirtyCount(result)}) : text(result.sources.some(source => source.edit) ? 'resultEditHint' : 'resultReadOnly'));
      el('db-query-summary').insertAdjacentHTML('afterbegin', `<div class="db-query-edit-status ${result.editError ? 'db-error' : ''}">${escaped(message)}</div>`);
    }
    syncResultActions();
    status(result.editMessage || summary, Boolean(result.editError) || succeeded !== result.sources.length);
  }
  async function runSQL(mode = 'run', options = {}) {
    const session = sqlSession(); if (!session || session.controller || session.query?.saving || ui.committing) return;
    finishResultEdit();
    if (queryDirtyCount(session.query) && !window.confirm(text('discard'))) return;
    const single = singleQueryTarget(), targets = options.targets || (options.scope === 'single' ? (single ? [single] : []) : targetList());
    const sql = (options.sql ?? (ui.editor.selected() || ui.editor.currentStatement() || ui.editor.value())).trim();
    if (options.scope === 'multi' && targets.length < 2) return status(text('chooseMultiple'), true);
    if (!targets.length) return status(text('noTargets'), true); if (!sql) return status(text('emptySql'), true);
    captureSQL();
    const controller = new AbortController(), limit = options.limit || Number(el('db-query-limit').value);
    session.controller = controller; const progress=beginDatabaseLoad(session,controller); session.query = null; session.message = text('running', { count: targets.length }); session.error = false; session.filters = []; session.headerFilters = []; session.sort = []; session.scroll = null;
    ui.controller = controller; el('db-cancel-query').disabled = false; el('db-run-query').disabled = true;
    activateBottom({ kind: 'result', id: session.id }); renderTargets();
    const start = performance.now();
    try {
      const result = await api('query', { body: { targets, sql, mode, limit }, signal: controller.signal,progress });
      if (!ui.sqlTabs.includes(session) || session.controller !== controller) return;
      progress.records=Math.max(progress.records,result.rows.length);
      result.elapsed = Math.round(performance.now() - start); result.request = {targets:clone(targets),sql,mode,limit}; session.query = result; session.message = '';
      if (result.mode === 'execute') {
        // Keep the execution report even if refreshing metadata fails.
        await Promise.allSettled(result.sources.filter(source => !source.error).map(async source => {
          const target = source.target, objects = await api('objects', { target });
          ui.objects.set(key(target), { ...ui.objects.get(key(target)), items: objects });
        })); renderTree();
      }
    } catch (error) { session.message = error.name === 'AbortError' ? text('executionCancelled') : error.message; session.error = error.name !== 'AbortError'; }
    finally {
      finishDatabaseLoad(session,progress);
      if (session.controller === controller) session.controller = null;
      if (ui.activeSQL === session.id) { ui.controller = null; el('db-cancel-query').disabled = true; el('db-run-query').disabled = false; renderTargets(); }
      if (ui.activeBottom?.kind === 'result' && ui.activeBottom.id === session.id) paintBottom();
      renderWorkspaceTabs();
    }
  }

  function exportQuery() {
    if (!ui.query) return;
    const q = value => `"${cellText(value).replaceAll('"', '""')}"`;
    const result = ui.query, statementOnly = result.mode === 'execute' && !result.columns.length;
    const rows = statementOnly ? [[text('sourceConnection'), text('sourceDatabase'), text('sourceEngine'), text('executionStatus'), text('affectedTitle'), text('duration'), text('errorDetail')], ...result.sources.map(source => [source.name || profile(source.target.id)?.name || source.target.id, source.target.database, source.engine, text(source.error ? 'failed' : 'completed'), source.error ? '' : source.affected ?? text('unknownAffected'), source.milliseconds, source.error || ''])] : [[text('sourceConnection'), text('sourceDatabase'), text('sourceEngine'), ...result.columns.map(c => c.name)], ...result.rows.map(row => { const source = result.sources[row.source]; return [source.name, source.target.database, source.engine, ...row.values]; })];
    const url = URL.createObjectURL(new Blob(['\ufeff' + rows.map(row => row.map(q).join(',')).join('\r\n')], { type: 'text/csv;charset=utf-8' }));
    const link = document.createElement('a'); link.href = url; link.download = 'database-results.csv'; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  function renderConnectionManager() {
    const list = el('db-manager-list'), search = el('db-manager-search').value.trim().toLowerCase();
    const profiles = ui.profiles.filter(p => [p.name, p.engine, p.host, p.user, p.database, p.file].some(value => String(value || '').toLowerCase().includes(search)));
    list.innerHTML = profiles.length ? profiles.map(p => `<button type="button" class="db-manager-row" data-db-manager-edit="${escaped(p.id)}"><span><strong>${escaped(p.name)}</strong><small>${escaped(p.engine === 'sqlite' ? p.file : `${p.host}:${p.port} · ${p.user}${p.database ? ` / ${p.database}` : ''}`)}</small></span><span class="db-manager-engine">${escaped(p.engine)}</span><span>${text('editConnection')} ›</span></button>`).join('') : `<p class="db-manager-empty">${text(ui.profiles.length ? 'noConnectionMatches' : 'emptyConnections')}</p>`;
  }
  window.openDatabaseManager = async () => {
    setView('database'); initialize(); el('db-manager-search').value = ''; renderConnectionManager();
    el('db-manager-modal').classList.remove('hidden'); dialogStatus('manager', text('loading'));
    try { await loadProfiles(); dialogStatus('manager', ''); } catch (error) { dialogStatus('manager', error.message, true); }
    if (!el('db-manager-modal').classList.contains('hidden')) el('db-manager-search').focus();
  };
  let passwordRequest = 0;
  function setPasswordVisible(visible) {
    const input = el('db-connection-password'), button = el('db-toggle-password');
    input.type = visible ? 'text' : 'password'; button.dataset.i18n = `db.${visible ? 'hidePassword' : 'showPassword'}`;
    button.textContent = text(visible ? 'hidePassword' : 'showPassword'); button.setAttribute('aria-pressed', String(visible));
  }
  async function toggleConnectionPassword() {
    const form = el('db-connection-form'), input = form.elements.password;
    if (input.type === 'text') { passwordRequest++; setPasswordVisible(false); return; }
    const request = ++passwordRequest, id = form.dataset.id;
    try {
      if (!input.value && form.dataset.passwordDirty !== 'true' && profile(id)?.hasPassword) {
        const result = await api('password', { target: { id } });
        if (request !== passwordRequest || form.dataset.id !== id || el('db-connection-modal').classList.contains('hidden')) return;
        input.value = result.password;
      }
      if (request === passwordRequest) setPasswordVisible(true);
    } catch (error) { if (request === passwordRequest) dialogStatus('connection', error.message, true); }
  }
  function connectionMode() {
    const form = el('db-connection-form'), sqlite = form.elements.engine.value === 'sqlite';
    form.querySelector('.db-connection-network').classList.toggle('hidden', sqlite); el('db-sqlite-path').classList.toggle('hidden', !sqlite);
    form.elements.host.required = !sqlite; form.elements.user.required = !sqlite; form.elements.file.required = sqlite;
  }
  window.openDatabaseConnection = (id = '') => {
    if (id && !connectionTabsGuard(id)) return;
    setView('database'); initialize(); const form = el('db-connection-form'), p = profile(id); form.reset(); form.dataset.id = p?.id || ''; form.dataset.passwordDirty = 'false'; passwordRequest++; setPasswordVisible(false); form.elements.password.placeholder = p?.hasPassword ? text('savedPassword') : '';
    for (const field of ['name', 'engine', 'host', 'port', 'user', 'database', 'file', 'tls']) if (p?.[field] !== undefined) form.elements[field].value = p[field];
    el('db-delete-connection').classList.toggle('hidden', !p); connectionMode(); dialogStatus('connection', ''); el('db-connection-modal').classList.remove('hidden');
  };
  function connectionPayload() {
    const form = el('db-connection-form'), data = Object.fromEntries(new FormData(form)); data.id = form.dataset.id; data.port = Number(data.port) || 0;
    if (form.dataset.passwordDirty !== 'true') delete data.password; return data;
  }
  async function saveConnection(event) {
    event.preventDefault();
    try { const result = await api('profiles', { body: connectionPayload() }); clearConnectionTabs(result.id); if (ui.focus?.id === result.id) ui.focus = null; for (const [id, target] of ui.checked) if (target.id === result.id) ui.checked.delete(id); ui.catalogs.delete(result.id); for (const [id] of ui.objects) if (JSON.parse(id)[0] === result.id) ui.objects.delete(id); await loadProfiles(); closeDialog('connection'); await toggleConnection(result.id); status(text('saved')); }
    catch (error) { dialogStatus('connection', error.message, true); }
  }
  function closeDialog(name) {
    el(`db-${name}-modal`).classList.add('hidden');
    if (name === 'schema') {resetTemplateDesigner();clearPlan();}
    if (name === 'connection') { passwordRequest++; setPasswordVisible(false); el('db-connection-form').reset(); }
  }

  let templateSchemaNonce = 0;
  function resetTemplateDesigner() {
    templateSchemaNonce++; ui.schemaTemplate=null;
    el('db-template-schema-targets').classList.add('hidden');
    el('db-schema-title').setAttribute('data-i18n','db.designer');el('db-schema-title').textContent=text('designer');
    el('db-apply-schema').setAttribute('data-i18n','db.applySchema');el('db-apply-schema').textContent=text('applySchema');
    for(const option of el('db-schema-form').elements.action.options) option.hidden=false;
  }
  async function openTemplateDesigner(id) {
    const tab=sqlSession();if(!tab) return;
    const choices=singleTargetChoices();if(!choices.length) return status(text('noTargets'),true);
    closeInlinePicker();toggleTemplates(false);
    const context=ui.editor.templateAtCursor()?.context || tab.templateContext;
    const preferred=context?.target || singleQueryTarget() || choices[0];
    const mode={id,sqlId:tab.id,context,columns:[],objects:[],target:null,loading:true};
    ui.schemaTemplate=mode;
    const form=el('db-schema-form');form.reset();
    const actions=id==='create'?['createTable']:id==='drop'?['dropTable']:['addColumn','alterColumn','dropColumn'];
    for(const option of form.elements.action.options){option.hidden=!actions.includes(option.value);option.disabled=!actions.includes(option.value);}
    form.elements.action.value=actions[0];
    el('db-schema-title').removeAttribute('data-i18n');el('db-schema-title').textContent=text({create:'templateCreate',drop:'templateDrop',alter:'templateAlter'}[id]);
    el('db-apply-schema').removeAttribute('data-i18n');el('db-apply-schema').textContent=text('applyInline');
    el('db-template-schema-targets').classList.remove('hidden');
    el('db-template-schema-source').innerHTML=choices.map(target=>`<option value="${escaped(key(target))}">${escaped(sourceName(target))}</option>`).join('');
    if(choices.some(target=>key(target)===key(preferred))) el('db-template-schema-source').value=key(preferred);
    el('db-template-schema-table-label').hidden=id==='create';
    el('db-schema-fields').innerHTML='';clearPlan();el('db-schema-modal').classList.remove('hidden');
    await loadTemplateSchemaObjects();
  }
  async function loadTemplateSchemaObjects() {
    const mode=ui.schemaTemplate, nonce=++templateSchemaNonce;if(!mode) return;
    mode.loading=true;mode.target=null;clearPlan();dialogStatus('schema',text('loading'));
    el('db-schema-fields').innerHTML='';el('db-template-schema-schema').disabled=true;el('db-template-schema-table').disabled=true;
    const [id,database]=JSON.parse(el('db-template-schema-source').value),target={id,database};
    try {
      const objects=await api('objects',{target});
      if(ui.schemaTemplate!==mode || nonce!==templateSchemaNonce) return;
      mode.objects=objects.filter(object=>object.kind==='table');mode.source=target;
      const fallback=profile(id)?.engine==='postgres'?'public':database;
      const schemas=[...new Set([fallback,...objects.map(object=>object.schema)].filter(Boolean))];
      const previous=mode.context && key(mode.context.target)===key(target)?mode.context.object.schema:null;
      el('db-template-schema-schema').innerHTML=schemas.map(name=>`<option value="${escaped(name)}" ${name===previous?'selected':''}>${escaped(name)}</option>`).join('');
      el('db-template-schema-schema').disabled=false;
      await selectTemplateSchema();
    } catch(error){if(ui.schemaTemplate===mode && nonce===templateSchemaNonce)dialogStatus('schema',error.message,true);}
  }
  async function selectTemplateSchema() {
    const mode=ui.schemaTemplate;if(!mode) return;
    const schema=el('db-template-schema-schema').value;
    const tables=mode.objects.filter(object=>object.schema===schema);
    el('db-template-schema-table').innerHTML=tables.map(object=>`<option value="${escaped(object.name)}">${escaped(object.name)}</option>`).join('');
    const previous=mode.context && key(mode.context.target)===key(mode.source)?mode.context.object.name:null;
    if(tables.some(object=>object.name===previous)) el('db-template-schema-table').value=previous;
    el('db-template-schema-table').disabled=mode.id==='create'||!tables.length;
    await loadTemplateSchemaColumns();
  }
  async function loadTemplateSchemaColumns() {
    const mode=ui.schemaTemplate,nonce=++templateSchemaNonce;if(!mode) return;
    mode.loading=true;mode.target=null;clearPlan();el('db-schema-fields').innerHTML='';dialogStatus('schema',text('loading'));
    const target={...mode.source,schema:el('db-template-schema-schema').value,table:mode.id==='create'?'':el('db-template-schema-table').value};
    try {
      if(mode.id!=='create' && !target.table) throw new Error(text('emptyObjects'));
      const columns=mode.id==='create'?[]:(await api('structure',{target})).columns;
      if(ui.schemaTemplate!==mode || nonce!==templateSchemaNonce) return;
      mode.columns=columns;mode.target=target;mode.loading=false;
      renderDesignerFields();dialogStatus('schema',text('templateSchemaHint'));
    } catch(error){if(ui.schemaTemplate===mode && nonce===templateSchemaNonce)dialogStatus('schema',error.message,true);}
  }
  function selectedColumn() { return ui.grids.structure?.getSelectedData()[0] || ui.tableResponse?.structure.columns[0]; }
  function selectedIndex() { return ui.grids.index?.getSelectedData()[0]; }
  function clearPlan() { schemaPreviewNonce++; ui.schemaPlan = null; el('db-apply-schema').disabled = true; el('db-schema-preview').classList.add('hidden'); dialogStatus('schema', ''); }
  function schemaColumnRow(spec = {}, create = true) {
    const name = spec.name || '', type = spec.type || 'varchar(255)';
    return `<div class="db-schema-column"><input data-column-name value="${escaped(name)}" placeholder="${text('name')}" ${!create ? 'readonly' : ''} aria-label="${text('name')}"/><input data-column-type value="${escaped(type)}" placeholder="${text('type')}" aria-label="${text('type')}"/><input data-column-default value="${escaped(spec.default || '')}" placeholder="${text('default')}" aria-label="${text('default')}"/>${create ? `<button type="button" class="text-button" data-remove-column>${text('remove')}</button>` : ''}<div class="db-column-flags"><label class="db-check"><input type="checkbox" data-column-null ${spec.nullable ? 'checked' : ''}/>${text('nullable')}</label><label class="db-check"><input type="checkbox" data-column-primary ${spec.primary ? 'checked' : ''} ${!create ? 'disabled' : ''}/>${text('primary')}</label>${create ? `<label class="db-check"><input type="checkbox" data-column-auto ${spec.auto ? 'checked' : ''}/>${text('auto')}</label>` : ''}</div></div>`;
  }
  function renderDesignerFields() {
    clearPlan(); const form = el('db-schema-form'), action = form.elements.action.value, fields = el('db-schema-fields'), columns = ui.schemaTemplate?.columns || ui.tableResponse?.structure.columns || [], column = ui.schemaTemplate ? columns[0] : selectedColumn(), index = selectedIndex();
    if(ui.schemaTemplate?.loading) {fields.innerHTML='';return;}
    ui.schemaTarget = { ...(ui.schemaTemplate ? ui.schemaTemplate.target : action === 'createTable' ? ui.designerCreateTarget : ui.table) };
    if (!ui.schemaTarget.schema) ui.schemaTarget.schema = profile(ui.schemaTarget.id)?.engine === 'postgres' ? (ui.table && key(ui.table) === key(ui.schemaTarget) ? ui.table.schema : 'public') : ui.schemaTarget.database;
    el('db-schema-target').value = `${sourceName(ui.schemaTarget)} / ${ui.schemaTarget.schema}${ui.schemaTarget.table ? '.' + ui.schemaTarget.table : ''}`;
    const columnSelect = `<label><span>${text('column')}</span><select name="oldName">${columns.map(c => `<option ${c.name === column?.name ? 'selected' : ''}>${escaped(c.name)}</option>`).join('')}</select></label>`;
    if (action === 'createTable') fields.innerHTML = `<label><span>${text('tableName')}</span><input name="objectName" required /></label><div class="db-schema-rows">${schemaColumnRow({ name: 'id', type: 'integer', primary: true, auto: true })}${schemaColumnRow({ name: 'name', type: 'varchar(255)', nullable: true })}</div><button class="text-button" type="button" data-add-column>${text('addField')}</button>`;
    else if (action === 'addColumn') fields.innerHTML = `<div class="db-schema-rows">${schemaColumnRow({ name: '', nullable: true }, true)}</div>`;
    else if (action === 'alterColumn') fields.innerHTML = columnSelect + `<div class="db-schema-rows">${schemaColumnRow(column || {}, false)}</div>`;
    else if (action === 'renameColumn') fields.innerHTML = columnSelect + `<label><span>${text('newName')}</span><input name="objectName" required /></label>`;
    else if (action === 'dropColumn') fields.innerHTML = columnSelect;
    else if (action === 'createIndex') fields.innerHTML = `<div class="form-grid"><label><span>${text('indexName')}</span><input name="objectName" required /></label><label><span>${text('indexColumns')}</span><input name="indexColumns" value="${escaped(column?.name || '')}" required /></label></div><label class="db-check"><input type="checkbox" name="unique"/>${text('unique')}</label>`;
    else if (action === 'dropIndex') fields.innerHTML = `<label><span>${text('indexes')}</span><select name="oldName">${(ui.tableResponse?.structure.indexes || []).filter(i => !i.primary).map(i => `<option ${i.name === index?.name ? 'selected' : ''}>${escaped(i.name)}</option>`).join('')}</select></label>`;
    else if (action === 'renameTable') fields.innerHTML = `<label><span>${text('newName')}</span><input name="objectName" value="${escaped(ui.table?.table || '')}" required /></label>`;
    else fields.innerHTML = `<p class="db-error">${escaped(sourceName(ui.schemaTarget))} / ${escaped(ui.schemaTarget.table)}</p>`;
  }
  function openDesigner(action, createTarget) {
    resetTemplateDesigner();
    const base = createTarget || (ui.activeBottom?.kind === 'table' ? ui.table : ui.focus);
    if (!base) return status(text('chooseDatabase'), true);
    ui.designerCreateTarget = { id: base.id, database: base.database, schema: base.schema };
    if (action !== 'createTable' && (!ui.table || ui.table.kind === 'view')) return status(text('chooseTable'), true);
    if (!dirtyGuard()) return;
    const form = el('db-schema-form'); form.reset(); form.elements.action.value = action;
    for (const option of form.elements.action.options) option.disabled = option.value !== 'createTable' && (!ui.table || ui.table.kind === 'view');
    ui.schemaTarget = { ...(action === 'createTable' ? ui.designerCreateTarget : ui.table) };
    if (!ui.schemaTarget.schema) ui.schemaTarget.schema = profile(ui.schemaTarget.id)?.engine === 'postgres' ? ui.table?.schema || 'public' : ui.schemaTarget.database;
    el('db-schema-target').value = `${sourceName(ui.schemaTarget)} / ${ui.schemaTarget.schema}${ui.schemaTarget.table ? '.' + ui.schemaTarget.table : ''}`;
    renderDesignerFields(); el('db-schema-modal').classList.remove('hidden');
  }
  function schemaPayload() {
    const form = el('db-schema-form'), data = Object.fromEntries(new FormData(form)), specs = [...form.querySelectorAll('.db-schema-column')].map(row => ({ name: row.querySelector('[data-column-name]').value.trim(), type: row.querySelector('[data-column-type]').value.trim(), default: row.querySelector('[data-column-default]').value || null, nullable: row.querySelector('[data-column-null]').checked, primary: row.querySelector('[data-column-primary]').checked, auto: row.querySelector('[data-column-auto]')?.checked || false }));
    return { action: data.action, name: data.objectName || '', oldName: data.oldName || '', columns: specs, column: specs[0], index: { name: data.objectName || '', columns: (data.indexColumns || '').split(',').map(s => s.trim()).filter(Boolean), unique: data.unique === 'on' } };
  }
  let schemaPreviewNonce=0;
  async function previewSchema(event) {
    event.preventDefault();clearPlan();
    const mode=ui.schemaTemplate, nonce=++schemaPreviewNonce;
    if(mode && (mode.loading || !mode.target)) return dialogStatus('schema',text('chooseTable'),true);
    const target=clone(ui.schemaTarget);
    try {
      const payload=schemaPayload(),result=await api('schema',{target,body:payload});
      if(nonce!==schemaPreviewNonce || mode!==ui.schemaTemplate || el('db-schema-modal').classList.contains('hidden')) return;
      ui.schemaPlan={payload,result,target,mode};el('db-schema-preview').textContent=result.sql.join(';\n\n')+(result.sql.length?';':'');
      el('db-schema-preview').classList.remove('hidden');el('db-apply-schema').disabled=!result.sql.length;
      dialogStatus('schema',result.sql.length ? mode?text('templateSchemaHint'):text(result.transactional?'transaction':'autoCommit'):text('nothingToChange'));
    } catch(error){if(nonce===schemaPreviewNonce)dialogStatus('schema',error.message,true);}
  }
  async function applySchema() {
    if (!ui.schemaPlan) return dialogStatus('schema', text('previewFirst'), true);
    const {payload,result,mode}=ui.schemaPlan,target={...ui.schemaPlan.target};
    if(mode) {
      if(mode!==ui.schemaTemplate || mode.sqlId!==ui.activeSQL) return dialogStatus('schema',text('templateSqlChanged'),true);
      const sql=result.sql.join(';\n\n')+';',tab=sqlSession();
      const context={target:{id:target.id,database:target.database},object:{schema:target.schema,name:payload.action==='createTable'?payload.name:target.table,kind:'table'},columns:mode.columns,selectedFields:[]};
      tab.templateId=mode.id;tab.templateContext=context;
      ui.editor.setTemplate(sql,{},slotLabels(),{templateId:mode.id,context});
      const name=text({create:'templateCreate',drop:'templateDrop',alter:'templateAlter'}[mode.id]);
      closeDialog('schema');ui.editor.focus();status(text('templateFilled',{name}));return;
    }
    el('db-apply-schema').disabled = true;
    try { await api('schema', { target, body: { ...payload, hash: result.hash } }); closeDialog('schema'); await loadObjects(target, true); if (payload.action === 'dropTable') { clearTable(); renderTree(); } else { if (payload.action === 'renameTable') { const existing = ui.tableTabs.find(tab => tableKey(tab.target) === tableKey(target)); if (existing) { existing.target = { ...existing.target, table: payload.name }; if (ui.currentTableId === existing.id) ui.table = existing.target; } } if (payload.action === 'createTable' || payload.action === 'renameTable') target.table = payload.name; await openTable({ ...target, kind: 'table' }, { confirm: false }); if(tableSession())tableSession().structureView=/Index$/.test(payload.action)?'indexes':'columns'; showTab('structure'); } status(text('applied')); syncTemplates(true); }
    catch (error) { clearPlan(); dialogStatus('schema', error.message, true); }
  }

  function initialize() {
    if (ui.initialized) return; ui.initialized = true;
    ui.editor = DatabaseUI.createEditor(el('db-sql-editor'), () => runSQL(), (scope, sql) => runSQL('run', { scope, sql }), editorChanged, slot => openInlinePicker(slot), slotLabels());
    if (!restoreSQLTabs()) createSQLTab();
    const theme = () => ui.editor.theme(document.documentElement.dataset.theme === 'dark'); theme(); new MutationObserver(theme).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
    loadProfiles().catch(error => status(error.message, true));
    el('db-target-select').addEventListener('click', () => toggleTargetPicker(!targetPicker.open, true));
    el('db-target-select').addEventListener('keydown', event => { if (event.key === 'ArrowDown') { event.preventDefault(); toggleTargetPicker(true, true); } });
    el('db-target-picker-close').addEventListener('click', () => toggleTargetPicker(false, true));
    el('db-target-picker-clear').addEventListener('click', clearTargets);
    el('db-target-tree').addEventListener('change', event => { const input = event.target.closest('input'); if (input) changePickedTargets(input); });
    el('db-target-tree').addEventListener('click', event => {
      const expand = event.target.closest('[data-db-target-expand]'), retry = event.target.closest('[data-db-target-retry]');
      if (expand) { const id = expand.dataset.dbTargetExpand; if (targetPicker.expanded.has(id)) targetPicker.expanded.delete(id); else { targetPicker.expanded.add(id); loadCatalog(id, Boolean(ui.catalogs.get(id)?.error)); } renderTargetTree(); }
      else if (retry) loadCatalog(retry.dataset.dbTargetRetry, true);
    });
    el('db-target-search').addEventListener('input', event => {
      targetPicker.search = event.target.value; renderTargetTree(); clearTimeout(targetPicker.searchTimer);
      if (targetPicker.search.trim()) targetPicker.searchTimer = setTimeout(() => { if (targetPicker.open && targetPicker.search.trim()) for (const p of ui.profiles) loadCatalog(p.id); }, 180);
    });
    document.addEventListener('mousedown', event => { if (targetPicker.open && !event.target.closest('#db-target-picker, #db-target-select')) toggleTargetPicker(false); });
    el('db-tree').addEventListener('click', event => {
      const button = event.target.closest('button'); if (!button) return;
      if (button.dataset.dbConnect) toggleConnection(button.dataset.dbConnect);
      else if (button.dataset.dbEdit) openDatabaseConnection(button.dataset.dbEdit);
      else if (button.dataset.dbDatabase) { const [id, database] = JSON.parse(button.dataset.dbDatabase); loadObjects({ id, database }); }
      else if (button.dataset.dbTableToggle) { const [id,database]=JSON.parse(button.dataset.dbTableToggle),object=ui.objects.get(button.dataset.dbTableToggle).items[Number(button.dataset.objectIndex)];toggleTableStructure({id,database,schema:object.schema,table:object.name}); }
      else if (button.dataset.dbStructureGroup) { const branch=structureTree.get(button.dataset.dbStructureGroup),group=button.dataset.group;if(branch){if(branch.groups.has(group))branch.groups.delete(group);else branch.groups.add(group);renderTree();} }
      else if (button.dataset.dbStructureOpen) {openStructurePage(button.dataset.dbStructureOpen,button.dataset.group).catch(error=>status(error.message,true));}
      else if (button.dataset.dbStructureNode) {ui.structureNode=button.dataset.dbStructureNode;renderTree();}
      else if (button.dataset.dbTable) { const [id, database] = JSON.parse(button.dataset.dbTable), object = ui.objects.get(button.dataset.dbTable).items[Number(button.dataset.objectIndex)]; openTable({ id, database, schema: object.schema, table: object.name, kind: object.kind }); }
    });
    el('db-refresh-tree').addEventListener('click', () => { ui.catalogs.clear(); ui.objects.clear(); structureTree.clear(); loadProfiles().catch(error => status(error.message, true)); });
    el('db-tree-create-table').addEventListener('click', () => openDesigner('createTable', ui.focus));
    view.querySelectorAll('[data-db-tab]').forEach(button => button.addEventListener('click', () => showTab(button.dataset.dbTab)));
    view.querySelectorAll('[data-db-structure-tab]').forEach(button=>button.addEventListener('click',()=>{const tab=tableSession();if(tab){tab.structureView=button.dataset.dbStructureTab;paintStructurePage();}}));
    el('db-table-comment').addEventListener('input',()=>{const tab=tableSession();if(tab){tab.commentDraft=el('db-table-comment').value;tab.commentPlan=null;tab.commentMessage='';paintStructurePage();}});
    el('db-comment-preview').addEventListener('click',()=>commentAction(false));el('db-comment-save').addEventListener('click',()=>commentAction(true));
    el('db-meta-reload').addEventListener('click',()=>{const tab=tableSession();if(!tab||tab.commentBusy||(commentDirty(tab)&&!confirm(text('discard'))))return;tab.schemaMeta=null;tab.commentDraft=undefined;tab.commentPlan=null;tab.commentMessage='';tab.metaError='';ensureDetailedStructure(tab);});
    el('db-index-create').addEventListener('click',()=>openDesigner('createIndex'));el('db-index-drop').addEventListener('click',()=>openDesigner('dropIndex'));
    el('db-run-query').addEventListener('click', () => runSQL()); el('db-execute').addEventListener('click', () => runSQL('run', { scope: 'single' })); el('db-cancel-query').addEventListener('click', () => ui.controller?.abort()); el('db-export-query').addEventListener('click', exportQuery); el('db-clear-targets').addEventListener('click', clearTargets);
    el('db-single-target').addEventListener('change', () => { ui.singleTarget = el('db-single-target').value; ui.singleTargetManual = true; renderTargets(); });
    el('db-toggle-templates').addEventListener('click', () => toggleTemplates(!templates.open));
    el('db-close-templates').addEventListener('click', () => { toggleTemplates(false); el('db-toggle-templates').focus(); });
    el('db-add-sql-tab').addEventListener('click', createSQLTab);
    el('db-sql-tabs').addEventListener('click', event => { const close = event.target.closest('[data-db-close-sql]'), tab = event.target.closest('[data-db-sql]'); if (close) closeSQLTab(close.dataset.dbCloseSql); else if (tab) activateSQLTab(tab.dataset.dbSql); });
    el('db-table-tabs').addEventListener('click', event => { const close = event.target.closest('[data-db-close-bottom]'), tab = event.target.closest('[data-db-bottom]'); if (close) closeBottomTab({ kind: close.dataset.kind, id: close.dataset.dbCloseBottom }); else if (tab) activateBottom({ kind: tab.dataset.kind, id: tab.dataset.dbBottom }); });
    enableReorder(el('db-sql-tabs'), '.db-session-tab', item => item.dataset.reorder, (from, to, placeAfter) => { if (moveItem(ui.sqlTabs, from, to, placeAfter, tab => tab.id)) renderWorkspaceTabs(); });
    enableReorder(el('db-table-tabs'), '.db-session-tab', item => item.dataset.reorder, (from, to, placeAfter) => { if (moveItem(ui.bottomTabs, from, to, placeAfter, item => `${item.kind}:${item.id}`)) renderWorkspaceTabs(); });
    enableReorder(el('db-tree'), '.db-tree-row[data-reorder]', item => item.dataset.reorder, async (from, to, placeAfter) => {
      if (!moveItem(ui.profiles, from, to, placeAfter, profile => profile.id)) return;
      renderTree();
      try { await api('profiles', { method: 'PUT', body: { order: ui.profiles.map(profile => profile.id) } }); } catch (error) { status(error.message, true); }
    }, false);
    el('db-inline-close').addEventListener('click', closeInlinePicker);
    el('db-inline-apply').addEventListener('click', () => applyInlinePicker());
    el('db-inline-body').addEventListener('change',event=>{
      if(inline.kind!=='join') return;
      if(['db-join-left-table','db-join-table'].includes(event.target.id)) loadJoinColumns();
      else if(event.target.matches('[data-join-left-field],[data-join-right-field]')) validateJoin();
    });
    el('db-inline-body').addEventListener('input',event=>{if(event.target.id==='db-join-alias') validateJoin();});
    el('db-inline-body').addEventListener('click',event=>{
      if(inline.kind!=='join' || inline.join?.loading) return;
      if(event.target.id==='db-join-add-condition') {
        inline.join.conditions=[...joinConditions(),{left:inline.join.leftColumns[0]?.name||'',right:inline.join.rightColumns[0]?.name||''}];renderJoinConditions();
      } else if(event.target.matches('[data-join-remove]')) {
        inline.join.conditions=joinConditions().filter((_,index)=>index!==Number(event.target.dataset.joinRemove));renderJoinConditions();
      }
    });
    el('db-inline-body').addEventListener('keydown', event => { if (event.key === 'Enter' && event.target.closest('[data-operand]') && (event.target.tagName !== 'TEXTAREA' || event.ctrlKey || event.metaKey)) { event.preventDefault(); applyInlinePicker(); } });
    el('db-inline-body').addEventListener('change', event => { if (event.target.id === 'db-inline-operator') syncOperandInputs(); else if (event.target.id === 'db-inline-source') loadInlineObjects(); else if (event.target.id === 'db-inline-table') loadInlineMetadata(); else if (event.target.name === 'inline-column') el('db-inline-apply').disabled = !el('db-inline-body').querySelector('[name="inline-column"]:checked'); });
    el('db-inline-body').addEventListener('change',event=>{if(event.target.matches('[data-insert-null]'))event.target.closest('[data-insert-cell]').querySelector('input[type="text"]').disabled=event.target.checked;});
    el('db-inline-body').addEventListener('click',event=>{
      const remove=event.target.closest('[data-insert-remove]');
      if(event.target.id==='db-insert-import')importInsertValues();
      else if(event.target.id==='db-insert-add')renderInsertValues([...captureInsertValues(),inline.insertColumns.map(()=>({value:'',isNull:false}))]);
      else if(remove){const rows=captureInsertValues();rows.splice(Number(remove.dataset.insertRemove),1);renderInsertValues(rows);}
    });
    el('db-inline-body').addEventListener('click', event => { if (event.target.id === 'db-inline-all' || event.target.id === 'db-inline-none') { el('db-inline-body').querySelectorAll('[name="inline-column"]').forEach(input => { input.checked = event.target.id === 'db-inline-all'; }); el('db-inline-apply').disabled = event.target.id === 'db-inline-none'; } else if (event.target.id === 'db-inline-star') applyInlinePicker(true); });
    document.addEventListener('mousedown', event => { if (inline.open && !event.target.closest('#db-inline-picker, [data-db-slot]')) closeInlinePicker(); });
    el('db-template-list').addEventListener('click', event => { const button = event.target.closest('[data-db-template]'); if (button && !button.disabled) fillTemplate(button.dataset.dbTemplate); });
    el('db-reload-table').addEventListener('click', () => ui.table && openTable(ui.table, { page: ui.page, reload: true }));
    el('db-query-grid').tabIndex = 0;
    el('db-query-delete').addEventListener('click',deleteQueryRows);
    el('db-query-commit').addEventListener('click',()=>commitEdits(false));
    el('db-query-rollback').addEventListener('click',rollbackQueryRows);
    for (const id of ['db-query-commit-all','db-data-commit-all']) el(id).addEventListener('click',()=>commitEdits(true));
    el('db-query-reload').addEventListener('click',()=>{const owner=resultOwner(),request=owner?.query?.request;if(request){activateSQLTab(owner.id);runSQL(request.mode,request);}});
    el('db-query-load-stop').addEventListener('click',()=>resultOwner()?.controller?.abort());
    el('db-data-load-stop').addEventListener('click',()=>tableSession()?.loadController?.abort());
    el('db-query-stop').addEventListener('click',()=>resultOwner()?.controller?.abort());
    el('db-panel-result').addEventListener('keydown',event=>{
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase()==='s') {event.preventDefault();event.stopPropagation();commitEdits(event.shiftKey);}
      else if (event.key==='Delete' && !event.target.closest('input:not([type="checkbox"]),textarea,[contenteditable="true"]')) {event.preventDefault();deleteQueryRows();}
    });
    el('db-save-rows').addEventListener('click', saveRows);
    el('db-rollback-rows').addEventListener('click', rollbackRows);
    el('db-stop-table').addEventListener('click', () => tableSession()?.loadController?.abort());
    initializeDataClipboard();
    el('db-add-row').addEventListener('click', () => addDataRow().catch(error => status(error.message, true)));
    el('db-delete-row').addEventListener('click', async () => { await Promise.all((ui.grids.data?.getSelectedRows() || []).filter(row => row.getData().__new || ui.tableResponse?.editable).map(row => row.getData().__new ? row.delete() : row.update({ __delete: true }))); updateDirty(); });
    el('db-page-prev').addEventListener('click', () => ui.table && openTable(ui.table, { page: Math.max(0, ui.page - 1) })); el('db-page-next').addEventListener('click', () => ui.table && openTable(ui.table, { page: ui.page + 1 })); el('db-page-last').addEventListener('click', () => ui.table && openTable(ui.table, { last: true })); el('db-page-size').addEventListener('change', () => ui.table && openTable(ui.table, { page: 0, pageSize: el('db-page-size').value, reload: true }));
    for (const [id, action] of [['db-create-table', 'createTable'], ['db-add-column', 'addColumn'], ['db-alter-column', 'alterColumn'], ['db-create-index', 'createIndex'], ['db-drop-index', 'dropIndex']]) el(id).addEventListener('click', () => openDesigner(action));
    el('db-manager-search').addEventListener('input', renderConnectionManager);
    el('db-manager-new').addEventListener('click', () => window.openDatabaseConnection());
    el('db-manager-list').addEventListener('click', event => { const button = event.target.closest('[data-db-manager-edit]'); if (button) window.openDatabaseConnection(button.dataset.dbManagerEdit); });
    el('db-toggle-password').addEventListener('click', toggleConnectionPassword);
    const connectionForm = el('db-connection-form'); connectionForm.addEventListener('submit', saveConnection); connectionForm.elements.password.addEventListener('input', () => { connectionForm.dataset.passwordDirty = 'true'; passwordRequest++; }); connectionForm.elements.engine.addEventListener('change', () => { connectionForm.elements.port.value = connectionForm.elements.engine.value === 'postgres' ? '5432' : '3306'; connectionMode(); });
    el('db-test-connection').addEventListener('click', async event => { const button = event.currentTarget; button.disabled = true; dialogStatus('connection', text('loading')); try { await api('test', { body: connectionPayload() }); dialogStatus('connection', text('connected')); } catch (error) { dialogStatus('connection', error.message, true); } finally { button.disabled = false; } });
    el('db-delete-connection').addEventListener('click', async () => { const id = connectionForm.dataset.id; if (!id || !window.confirm(text('confirmDeleteConnection'))) return; try { await api('profiles', { target: { id }, method: 'DELETE' }); if (ui.focus?.id === id) ui.focus = null; clearConnectionTabs(id); await loadProfiles(); closeDialog('connection'); } catch (error) { dialogStatus('connection', error.message, true); } });
    el('db-template-schema-source').addEventListener('change',loadTemplateSchemaObjects);
    el('db-template-schema-schema').addEventListener('change',selectTemplateSchema);
    el('db-template-schema-table').addEventListener('change',loadTemplateSchemaColumns);
    const schemaForm = el('db-schema-form'); schemaForm.addEventListener('submit', previewSchema); schemaForm.elements.action.addEventListener('change', renderDesignerFields); schemaForm.addEventListener('input', clearPlan); schemaForm.addEventListener('change', event => { clearPlan(); if (event.target.name === 'oldName' && schemaForm.elements.action.value === 'alterColumn') { const column = (ui.schemaTemplate?.columns || ui.tableResponse.structure.columns).find(c => c.name === event.target.value); el('db-schema-fields').querySelector('.db-schema-rows').innerHTML = schemaColumnRow(column, false); } });
    el('db-schema-fields').addEventListener('click', event => { if (event.target.closest('[data-add-column]')) { el('db-schema-fields').querySelector('.db-schema-rows').insertAdjacentHTML('beforeend', schemaColumnRow({ nullable: true })); clearPlan(); } if (event.target.closest('[data-remove-column]')) { event.target.closest('.db-schema-column').remove(); clearPlan(); } }); el('db-apply-schema').addEventListener('click', applySchema);
    document.querySelectorAll('[data-db-close]').forEach(button => button.addEventListener('click', () => closeDialog(button.dataset.dbClose))); for (const name of ['connection', 'schema', 'manager']) bindBackdropDismissal(el(`db-${name}-modal`), () => closeDialog(name));
    document.addEventListener('keydown', event => { if (event.key === 'Escape') { if (targetPicker.open) { toggleTargetPicker(false, true); return; } if (!el('db-connection-modal').classList.contains('hidden')) closeDialog('connection'); else closeDialog('manager'); closeDialog('schema'); if (inline.open) closeInlinePicker(); else if (templates.open) toggleTemplates(false); } });
    window.addEventListener('resize', () => { if (!view.classList.contains('hidden')) { resizeGrids(); positionTemplates(); positionTargetPicker(); if (inline.open) positionInlinePicker(inline.slotId); } });
  }
  window.newDatabaseQuery = () => { setView('database'); createSQLTab(); };
  window.renderDatabaseView = () => {
    initialize();
    if (ui.language !== activeLanguage()) {
      ui.language = activeLanguage();
      captureTableState(); captureResultState(); if (tableSession()?.response) restoreTableState(tableSession()); syncTemplates();
    }
    renderTree(); renderTargets(); paintBottom();
  };
})();
