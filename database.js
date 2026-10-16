// Database workspace. Mature editor/grid components are bundled locally; all
// database traffic goes through the existing in-process Go bridge.
const databaseLabels = {
  resultEditHint: ['Double-click a cell to edit; Enter or leaving the cell saves to its source database. Right-click to set NULL.', '双击单元格修改，回车或离开单元格时保存到来源数据库；右键可设为 NULL。'], resultReadOnly: ['Editing requires a direct single-table query with its full primary key. Joins, aggregates and expressions are read-only.', '仅可修改包含完整主键的单表直接查询结果；关联、聚合和表达式结果只读。'], resultSaving: ['Saving to the source database…', '正在保存到来源数据库…'], resultSaved: ['Saved: {source} / {column}', '已保存：{source} / {column}'],
  subtitle: ['Query several databases with one SQL statement and one combined result.', '连接多个数据库，用一条 SQL 汇总查询结果。'],
  newConnection: ['New connection…', '新建连接…'], newItem: ['New', '新建'], newQuery: ['New SQL tab', '新建 SQL 页签'], connections: ['Connections & objects', '连接与结构'], refresh: ['Refresh', '刷新'], chooseObject: ['Choose a table or check databases to query', '选择数据表，或勾选多个库查询'],
  chooseTargets: ['Choose target databases', '选择目标数据库'], searchTargets: ['Search connections or databases', '搜索连接或数据库'], targetsLimit: ['{count} selected · maximum 16 databases', '已选 {count} 个库 · 最多 16 个'], noTargetMatches: ['No matching databases', '没有匹配的数据库'], emptyDatabases: ['No databases', '暂无数据库'], expandConnection: ['Expand or collapse {name}', '展开或收起 {name}'],
  data: ['Data', '数据'], structure: ['Structure', '结构'], sql: ['SQL query', 'SQL 查询'], addRow: ['Add row', '新增行'], deleteRows: ['Delete selected rows', '删除选中行'], saveRows: ['Save changes', '保存修改'], reload: ['Reload', '重新加载'], editHint: ['Double-click to edit; right-click for NULL. Save to commit changes.', '双击编辑，右键可设为 NULL；保存后写入数据库。'],
  previous: ['Previous', '上一页'], next: ['Next', '下一页'], last: ['Last', '尾页'], createTable: ['Create table', '新建表'], addColumn: ['Add column', '添加字段'], alterColumn: ['Edit column', '修改字段'], renameColumn: ['Rename column', '重命名字段'], dropColumn: ['Drop column', '删除字段'], createIndex: ['Create index', '新增索引'], dropIndex: ['Drop index', '删除索引'], renameTable: ['Rename table', '重命名表'], dropTable: ['Drop table', '删除表'], columns: ['Columns — select one to edit', '字段（选中后可修改）'], indexes: ['Indexes', '索引'],
  runQuery: ['Run SQL · Ctrl+Enter', '执行 SQL · Ctrl+Enter'], execute: ['Execute statement (one DB)', '执行语句（单库）'], cancel: ['Cancel', '取消'], clearTargets: ['Clear selection', '清空选择'], exportCsv: ['Export CSV', '导出 CSV'], queryHint: ['Choose databases in the target dropdown. SQL runs on each; result columns are aligned and combined.', '在目标数据库下拉框中多选，同一条 SQL 分别执行，返回结果按列名对齐合并。'],
  connection: ['Database connection', '数据库连接'], connectionName: ['Connection name', '连接名称'], engine: ['Database type', '数据库类型'], host: ['Host', '主机'], port: ['Port', '端口'], user: ['User', '用户名'], password: ['Password (blank keeps the saved password)', '密码（留空保留已保存密码）'], defaultDatabase: ['Default database (optional)', '默认数据库（可留空）'], tlsDisable: ['Disabled', '不启用'], tlsRequire: ['Encrypted connection', '加密连接'], tlsVerify: ['Encrypted, verify certificate', '加密并验证证书'], sqliteFile: ['Absolute SQLite file path (created if missing)', 'SQLite 文件绝对路径（不存在时创建）'], deleteConnection: ['Delete connection', '删除连接'], testConnection: ['Test connection', '测试连接'], save: ['Save', '保存'],
  designer: ['Schema designer', '结构设计器'], operation: ['Operation', '操作'], currentObject: ['Current object', '当前对象'], previewSql: ['Preview SQL', '预览 SQL'], applySchema: ['Apply schema changes', '应用结构变更'],
  emptyConnections: ['No connections yet. Add MySQL, PostgreSQL or SQLite.', '暂无连接，可新增 MySQL、PostgreSQL 或 SQLite。'], loading: ['Loading…', '正在加载…'], emptyObjects: ['No tables or views', '暂无表或视图'], table: ['Table', '表'], view: ['View', '视图'], editConnection: ['Edit connection', '编辑连接'], confirmDeleteConnection: ['Delete this connection configuration?', '确定删除该连接配置吗？'], connected: ['Connection successful', '连接成功'], saved: ['Saved', '已保存'], name: ['Name', '名称'], type: ['Type', '类型'], nullable: ['Nullable', '允许 NULL'], primary: ['Primary key', '主键'], default: ['Default SQL expression', '默认 SQL 表达式'], generated: ['Generated', '生成列'], unique: ['Unique', '唯一'], indexColumns: ['Columns (comma-separated, in index order)', '字段（逗号分隔，按索引顺序）'], auto: ['Auto increment / identity', '自增 / 标识列'], remove: ['Remove', '移除'], addField: ['Add a column', '添加一列'], column: ['Column', '字段'], newName: ['New name', '新名称'], tableName: ['Table name', '表名'], indexName: ['Index name', '索引名'],
  chooseDatabase: ['Choose a database in the tree first', '请先点击左侧的数据库'], chooseTable: ['Open a table first', '请先打开一个数据表'], chooseColumn: ['Select a column in the structure grid first', '请先在结构列表中选中字段'], chooseIndex: ['Select a non-primary index first', '请先选中一个非主键索引'], readOnly: ['No primary key or view: existing rows are read-only; use SQL for custom changes.', '无主键表或视图的现有记录只读，可通过 SQL 自行修改。'], setNull: ['Set to NULL', '设为 NULL'], discard: ['Discard unsaved row changes?', '放弃未保存的数据修改吗？'], dirty: ['{count} pending row change(s)', '{count} 行待保存'], rows: ['{count} rows loaded', '已加载 {count} 行'], page: ['Page {page}', '第 {page} 页'], savedRows: ['Committed {count} row change(s)', '已提交 {count} 行修改'],
  targets: ['{count} database target(s)', '目标数据库：{count} 个'], noTargets: ['Choose databases in the target dropdown first.', '请先在目标数据库下拉框中选择数据库。'], maxTargets: ['At most 16 databases per execution', '单次最多对 16 个数据库执行'], running: ['Running SQL on {count} database(s)…', '正在对 {count} 个数据库执行 SQL…'], emptySql: ['Enter or select one SQL statement', '请输入或选中一条 SQL 语句'], queryDone: ['{rows} rows · {ok}/{total} databases succeeded · {ms} ms', '{rows} 行 · {ok}/{total} 个库成功 · {ms} ms'], sourceConnection: ['Source connection', '来源连接'], sourceDatabase: ['Source database', '来源数据库'], sourceEngine: ['Engine', '数据库类型'], truncated: ['Result limited by the row/size cap', '结果已达到行数或大小上限'], partial: ['Some databases failed; successful results remain in the grid.', '部分数据库查询失败，表格保留已成功数据库的结果。'], cancelled: ['Query cancelled', '已取消查询'], executed: ['Statement executed; {count} affected row(s)', '语句执行完成，影响 {count} 行'], details: ['Per-database results', '各数据库结果'], crossHint: ['SQL runs unchanged on every selected database. Use syntax and table names supported by those targets.', 'SQL 原文分别在各库执行，请使用各目标都支持的语法和表名。'],
  previewFirst: ['Generate and review the SQL preview before applying.', '请先生成并检查 SQL 预览，再应用变更。'], transaction: ['These schema changes run in a transaction.', '结构变更在事务中执行。'], autoCommit: ['This database may auto-commit DDL; each statement is shown above.', '该数据库可能自动提交 DDL，上方列出了将执行的语句。'], applied: ['Schema changes applied', '结构变更已应用'], nothingToChange: ['No schema changes', '结构未发生变化'],
  noResults: ['No data', '暂无数据'], executionDone: ['{ok}/{total} databases succeeded · {count} affected row(s) reported · {ms} ms', '{ok}/{total} 个库成功 · 已报告影响 {count} 行 · {ms} ms'], affectedRows: ['{count} affected row(s)', '影响 {count} 行'], affectedTitle: ['Affected rows', '影响行数'], executionStatus: ['Status', '执行状态'], completed: ['Completed', '已完成'], failed: ['Failed', '失败'], duration: ['Duration (ms)', '耗时（毫秒）'], errorDetail: ['Error', '错误详情'], unknownAffected: ['Not reported', '未报告'], unknownAffectedSources: ['{count} database(s) did not report affected rows', '{count} 个库未报告影响行数'], independentExecution: ['Each database executes independently. Successful changes remain committed if another database fails.', '各库独立执行；部分失败时，已成功库的修改不会回滚。'], executionCancelled: ['Execution cancelled. Completed changes remain; verify the outcome of databases that were still running.', '执行已取消。已完成的修改仍保留；正在执行的库请核实最终结果。'], executionPartial: ['Some databases failed. Review each database result before retrying; successful changes remain applied.', '部分数据库执行失败，请逐库检查后再重试；已成功的修改仍然生效。'], inlineOperator: ['Choose a comparison operator', '选择条件运算符'], operatorHint: ['Choose an operator and enter the value to compare; leave it empty to edit it in the SQL text later.', '选择运算符并输入比较值；留空则稍后直接在 SQL 文本中修改。'], operandValue: ['Comparison value', '比较值'], operandList: ['Values (comma separated)', '值列表（逗号分隔）'], operandFrom: ['From', '起始值'], operandTo: ['To', '结束值'],
  templates: ['SQL templates', '模板语句'], templateHint: ['Click a template, then choose tables and columns directly in the SQL text.', '点击模板后，在 SQL 文本中点选表名和字段。'], templateSource: ['Schema source', '结构来源'], templateColumns: ['Select columns', '选择字段'], selectAll: ['All', '全选'], selectNone: ['Clear', '清空'], templateField: ['Filter / sort / group column', '条件 / 排序 / 分组字段'], templateWriteHint: ['Templates update the current statement, or insert a new statement on an empty line. Edit values and conditions before execution.', '当前行有语句时修改该语句，空白行新增语句；值和条件可直接在文本中修改。'], templateFilled: ['Filled template: {name}', '已填入模板：{name}'],
  singleTarget: ['Single-database target', '单库目标'], singleQueryTitle: ['Run SQL on one database: {target}', '单库执行：{target}'], multiQueryTitle: ['Run SQL on {count} checked databases', '多库执行：已勾选的 {count} 个数据库'], chooseMultiple: ['Check at least two databases for multi-database execution.', '多库执行请至少勾选两个数据库。'],
  templateSelect: ['Select columns', '查询所选字段'], templateWhere: ['Filter non-empty values', '条件查询'], templateOrder: ['Sort results', '排序查询'], templateCount: ['Count rows', '统计行数'], templateDistinct: ['Distinct values', '去重查询'], templateGroup: ['Group and count', '分组统计'], templateInsert: ['Insert row', '插入数据'], templateUpdate: ['Update rows', '更新数据'], templateDelete: ['Delete rows', '删除数据'],
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
  async function api(action, { target, body, method, signal } = {}) {
    const response = await fetch(endpoint(action, target), { method: method || (body === undefined ? 'GET' : 'POST'), headers: { Accept: 'application/json', ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) }, body: body === undefined ? undefined : JSON.stringify(body), signal });
    const result = await response.json();
    if (!response.ok) throw new Error(result.detail || result.error || `HTTP ${response.status}`);
    return result;
  }
  function status(message, error = false) { el('db-status').textContent = message; el('db-status').classList.toggle('error', error); }
  function dialogStatus(name, message, error = false) { const output = el(`db-${name}-status`); output.textContent = message; output.classList.toggle('error', error); }
  const dirtyGuard = () => !ui.saving && (!ui.dirty || window.confirm(text('discard')));
  const quote = (engine, name) => { const mark = engine === 'mysql' ? '`' : '"'; return mark + name.replaceAll(mark, mark + mark) + mark; };
  const templates = { open: false };
  const templateDefinitions = [
    ['select', 'templateSelect', 'SELECT'], ['where', 'templateWhere', 'WHERE'], ['order', 'templateOrder', 'ORDER BY'],
    ['count', 'templateCount', 'COUNT'], ['distinct', 'templateDistinct', 'DISTINCT'], ['group', 'templateGroup', 'GROUP BY'],
    ['insert', 'templateInsert', 'INSERT'], ['update', 'templateUpdate', 'UPDATE'], ['delete', 'templateDelete', 'DELETE'],
  ];
  const templateSQL = {
    select: 'SELECT {{columns}} FROM {{table}} LIMIT 100;',
    where: 'SELECT {{columns}} FROM {{table}} WHERE {{field}} {{operator}} LIMIT 100;',
    order: 'SELECT {{columns}} FROM {{table}} ORDER BY {{field}} ASC LIMIT 100;',
    count: 'SELECT COUNT(*) AS row_count FROM {{table}};',
    distinct: 'SELECT DISTINCT {{columns}} FROM {{table}} LIMIT 100;',
    group: 'SELECT {{field}}, COUNT(*) AS row_count FROM {{table}} GROUP BY {{field}} LIMIT 100;',
    insert: 'INSERT INTO {{table}} ({{columns}}) VALUES ({{values}});',
    update: 'UPDATE {{table}} SET {{field}} = {{value}} WHERE {{condition}};',
    delete: 'DELETE FROM {{table}} WHERE {{condition}};',
  };
  const inline = { nonce: 0, open: false };
  function syncTemplates() {
    el('db-template-list').innerHTML = templateDefinitions.map(([id, label, sql]) => `<button type="button" data-db-template="${id}"><span>${text(label)}</span><code>${sql}</code></button>`).join('');
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
    return { table: context ? inlineTableName(context) : '{{table}}', columns: names.length ? names.map(c => quote(engine, c.name)).join(', ') : write ? '{{columns}}' : '*', field: first ? quote(engine, first.name) : '{{field}}', values: Array(Math.max(1, names.length)).fill('NULL').join(', '), value: 'NULL', condition: '1 = 0' };
  }
  function fillTemplate(id) {
    const tab = sqlSession(); if (!tab || !templateSQL[id]) return;
    closeInlinePicker();
    const currentTemplate = ui.editor.templateAtCursor();
    if (currentTemplate) tab.templateContext = currentTemplate.context ? clone(currentTemplate.context) : null;
    if (!tab.templateContext && ui.tableResponse && singleQueryTarget() && key(singleQueryTarget()) === key(ui.table)) tab.templateContext = { target: { id: ui.table.id, database: ui.table.database }, object: { schema: ui.table.schema, name: ui.table.table, kind: ui.table.kind }, columns: clone(ui.tableResponse.structure.columns), selectedFields: [] };
    tab.templateId = id;
    ui.editor.setTemplate(templateSQL[id], { ...templateValues(tab.templateContext, id), operator: 'IS NOT NULL' }, { table: text('inlineTable'), columns: text('inlineColumns'), field: text('inlineField'), operator: text('inlineOperator'), values: text('inlineValue'), value: text('inlineValue'), condition: text('inlineCondition') }, { templateId: id, context: tab.templateContext ? clone(tab.templateContext) : null });
    ui.editor.focus(); status(text('templateFilled', { name: text(templateDefinitions.find(item => item[0] === id)[1]) }));
  }
  function closeInlinePicker() { inline.open = false; inline.nonce++; el('db-inline-picker').classList.add('hidden'); }
  function inlineMessage(message, error = false) { el('db-inline-status').textContent = message; el('db-inline-status').classList.toggle('error', error); }
  function positionInlinePicker(id) {
    const rect = ui.editor.slotRect(id), picker = el('db-inline-picker');
    const anchor = rect || el('db-sql-editor').getBoundingClientRect();
    picker.style.left = Math.max(8, Math.min(anchor.left, innerWidth - 350)) + 'px';
    picker.style.top = Math.max(8, Math.min(anchor.bottom + 5, innerHeight - Math.min(390, picker.scrollHeight) - 12)) + 'px';
  }
  async function openInlinePicker(slot, requestedKind = slot.kind) {
    // A bare condition (DELETE/UPDATE ... WHERE) is picked as field + operator, like the WHERE template.
    const isCondition = slot.kind === 'condition'; if (isCondition && requestedKind === 'condition') requestedKind = 'field';
    if (!isCondition && ['value', 'values', 'filterValue'].includes(slot.kind)) { closeInlinePicker(); ui.editor.selectSlot(slot.id); return; }
    const tab = sqlSession(); if (!tab) return;
    if (slot.template) { tab.templateId = slot.template.templateId; tab.templateContext = slot.template.context ? clone(slot.template.context) : null; }
    closeInlinePicker(); inline.open = true; inline.sqlId = tab.id; inline.slotId = slot.id; inline.condition = isCondition; inline.kind = requestedKind; inline.pendingKind = null;
    if (requestedKind === 'operator') {
      el('db-inline-title').textContent = text('inlineOperator');
      const operators = ['IS NOT NULL', 'IS NULL', '=', '<>', '>', '>=', '<', '<=', 'LIKE', 'NOT LIKE', 'IN', 'NOT IN', 'BETWEEN', 'NOT BETWEEN'];
      el('db-inline-body').innerHTML = `<label class="db-template-label"><span>${text('inlineOperator')}</span><select id="db-inline-operator">${operators.map(value => `<option value="${escaped(value)}">${escaped(value)}</option>`).join('')}</select></label>${[['single', 'db-inline-value', 'operandValue'], ['list', 'db-inline-list', 'operandList'], ['range', 'db-inline-from', 'operandFrom'], ['range', 'db-inline-to', 'operandTo']].map(([group, id, label]) => `<label class="db-template-label" data-operand="${group}"><span>${text(label)}</span><input type="text" id="${id}" autocomplete="off" spellcheck="false"/></label>`).join('')}<p class="db-template-hint">${text('operatorHint')}</p>`;
      const current = ui.editor.slotText('operator'); if (operators.includes(current)) el('db-inline-operator').value = current;
      const group = operandGroup(current), shown = group === 'none' ? [] : operandDisplay(group, ui.editor.slotText('filterValue'), operandEngine());
      inline.operandGroup = group; inline.operandShown = shown;
      operandInputs(group).forEach((input, index) => { input.value = shown[index] || ''; });
      el('db-inline-picker').classList.remove('hidden'); el('db-inline-apply').disabled = false; inlineMessage(''); syncOperandInputs(); positionInlinePicker(slot.id); return;
    }
    if (requestedKind !== 'table' && (!tab.templateContext || ui.editor.slotText('table') !== inlineTableName(tab.templateContext))) { inline.kind = 'table'; inline.pendingKind = requestedKind; }
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
    } else renderInlineColumns();
    positionInlinePicker(slot.id);
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
    const columns = context.columns.filter(c => !write || !c.generated), selected = context.selectedFields?.length ? context.selectedFields : columns.map(c => c.name);
    inlineMessage('');
    el('db-inline-body').innerHTML = `<div class="db-inline-object">${escaped(sourceName(context.target))} / ${escaped(context.object.name)}</div>${inline.kind === 'columns' ? `<div class="db-actions"><button class="text-button" type="button" id="db-inline-all">${text('selectAll')}</button><button class="text-button" type="button" id="db-inline-none">${text('selectNone')}</button>${!write ? '<button class="text-button" type="button" id="db-inline-star">*</button>' : ''}</div>` : ''}<div class="db-template-columns">${columns.map((c, index) => `<label><input type="${inline.kind === 'field' ? 'radio' : 'checkbox'}" name="inline-column" value="${escaped(c.name)}" ${inline.kind === 'field' ? index === 0 ? 'checked' : '' : selected.includes(c.name) ? 'checked' : ''}/><span>${escaped(c.name)}</span><small>${escaped(c.type)}</small></label>`).join('')}</div>`;
    el('db-inline-apply').disabled = !columns.length;
  }
  const operandGroup = operator => !operator || operator.startsWith('IS ') ? 'none' : operator.includes('BETWEEN') ? 'range' : /^(NOT )?IN$/.test(operator) ? 'list' : 'single';
  const operandInputs = group => ({ single: ['db-inline-value'], list: ['db-inline-list'], range: ['db-inline-from', 'db-inline-to'] }[group] || []).map(id => el(id));
  const splitOperandList = value => (value.match(/'(?:[^']|'')*'|[^,]+/g) || []).map(item => item.trim()).filter(Boolean);
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
    if (inline.kind === 'operator') {
      const operator = el('db-inline-operator').value, current = ui.editor.slotText('operator'), previousValue = ui.editor.slotText('filterValue');
      const group = operandGroup(operator), engine = operandEngine(), inputs = operandInputs(group).map(input => input.value);
      const untouched = group === inline.operandGroup && inputs.join('\u0000') === inline.operandShown.join('\u0000'), literal = value => operandLiteral(value, engine, operator);
      let operand;
      if (group === 'none') operand = '';
      else if (untouched && previousValue && operandGroup(current) === group) operand = previousValue;
      else if (group === 'range') operand = inputs.map(value => value.trim() ? literal(value) : 'NULL').join(' AND ');
      else if (group === 'list') { const items = splitOperandList(inputs[0]); operand = `(${items.length ? items.map(literal).join(', ') : 'NULL'})`; }
      else operand = inputs[0].trim() ? literal(inputs[0]) : operator.includes('LIKE') ? "''" : 'NULL';
      ui.editor.replacePredicate(operator, operand, text('inlineValue')); closeInlinePicker(); ui.editor.focus(); return;
    }
    if (!inline.context) return;
    const context = inline.context, kind = inline.kind, pending = inline.pendingKind, slotId = inline.slotId, engine = profile(context.target.id)?.engine;
    if (kind === 'table') { tab.templateContext = context; ui.editor.replaceSlots(templateValues(context, tab.templateId)); }
    else {
      const selected = [...el('db-inline-body').querySelectorAll('[name="inline-column"]:checked')].map(input => input.value);
      if (!selected.length && !star) return;
      if (kind === 'field' && inline.condition) ui.editor.replaceCondition(slotId, quote(engine, selected[0]), { field: text('inlineField'), operator: text('inlineOperator') });
      else if (kind === 'field') ui.editor.replaceSlots({ field: quote(engine, selected[0]) });
      else { const useStar = star || (tab.templateId !== 'insert' && selected.length === context.columns.length); context.selectedFields = useStar ? [] : selected; tab.templateContext = context; ui.editor.replaceSlots({ columns: useStar ? '*' : selected.map(name => quote(engine, name)).join(', '), values: Array(Math.max(1, selected.length)).fill('NULL').join(', ') }); }
    }
    ui.editor.setTemplateContext(tab.templateContext ? clone(tab.templateContext) : null);
    closeInlinePicker(); renderTargets(); ui.editor.focus();
    if (pending) { const slot = ui.editor.slots().find(slot => slot.id === slotId); if (slot) openInlinePicker(slot, pending); }
  }

  const sqlSession = () => ui.sqlTabs?.find(tab => tab.id === ui.activeSQL);
  const tableSession = () => ui.tableTabs?.find(tab => tab.id === ui.currentTableId);
  const tableKey = target => JSON.stringify([target.id, target.database, target.schema, target.table]);
  function renderWorkspaceTabs() { persistSQL();
    el('db-sql-tabs').innerHTML = ui.sqlTabs.map(tab => `<div class="db-session-tab ${tab.id === ui.activeSQL ? 'active' : ''}" draggable="true" data-reorder="${tab.id}"><button type="button" role="tab" aria-selected="${tab.id === ui.activeSQL}" data-db-sql="${tab.id}">${escaped(tab.name)}${tab.controller ? ' ◌' : tab.text?.trim() ? ' •' : ''}</button><button type="button" class="db-session-close" data-db-close-sql="${tab.id}" aria-label="${escaped(text('closeTab'))}">×</button></div>`).join('');
    el('db-table-tabs').innerHTML = ui.bottomTabs.map(item => {
      const tab = item.kind === 'table' ? ui.tableTabs.find(tab => tab.id === item.id) : ui.sqlTabs.find(tab => tab.id === item.id);
      if (!tab) return '';
      const label = item.kind === 'table' ? `▦ ${tab.target.table}${tab.dirty ? ' •' : ''}` : `${tab.name} · ${text('results')}`;
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
    tab.editor = ui.editor.snapshot(); tab.text = ui.editor.value(); tab.checked = new Map(ui.checked); tab.focus = ui.focus ? { ...ui.focus } : null;
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
      const tabs = ui.sqlTabs.map(tab => ({ id: tab.id, name: tab.name, text: tab.id === ui.activeSQL && ui.editor && !ui.restoringSQL ? ui.editor.value() : tab.text }));
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
    for (const item of tabs) ui.sqlTabs.push({ id: item.id, name: String(item.name || item.id), text: item.text, checked: new Map(), targetsExplicit: false, focus: null, singleTarget: undefined, singleTargetManual: false, limit: el('db-query-limit').value || '500', controller: null, query: null });
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
    if (tab.editor) ui.editor.restore(tab.editor); else ui.editor.setValue(tab.text);
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
    if (tab.query?.saving) return status(text('resultSaving'));
    if ((tab.text.trim() || tab.controller) && !window.confirm(text('closeSqlConfirm'))) return;
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
    if (tab.response) { renderData(tab); renderStructure(); }
    el('db-panel-data').inert = Boolean(tab.saving || tab.loading);
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
    el('db-panel-data').classList.toggle('hidden', !hasTable || ui.tab === 'structure'); el('db-panel-structure').classList.toggle('hidden', !hasTable || ui.tab !== 'structure');
    el('db-table-subtabs').classList.toggle('hidden', !hasTable); el('db-panel-result').classList.toggle('hidden', !result); el('db-table-empty').classList.toggle('hidden', Boolean(hasTable || result));
    el('db-table-empty').textContent = table?.loading ? text('loading') : table?.error || text('emptyWorkspace');
    view.querySelectorAll('[data-db-tab]').forEach(button => button.classList.toggle('active', button.dataset.dbTab === ui.tab));
    el('db-object-title').textContent = table ? `${sourceName(table.target)} / ${table.target.schema}.${table.target.table}` : result ? `${result.name} / ${text('results')}` : text('chooseObject');
    el('db-engine-label').textContent = table ? profile(table.target.id)?.engine || '' : '';
    if (result) {
      ui.query = result.query;
      if (result.query) renderQuery(result.query); else { if (ui.grids.query?._databaseReady) ui.grids.query.clearData(); el('db-query-summary').textContent = result.message || text('queryHint'); status(result.message || '', result.error); }
    } else if (table) status(table.error || table.message || (table.dirty ? text('dirty', { count: table.dirty }) : (table.response?.editable === false ? text('readOnly') : '')), Boolean(table.error));
    else status('');
    renderWorkspaceTabs(); requestAnimationFrame(resizeGrids);
  }
  function closeBottomTab(item, force = false) {
    captureTableState();
    if (item.kind === 'result' && ui.sqlTabs.find(tab => tab.id === item.id)?.query?.saving) return false;
    const tab = item.kind === 'table' ? ui.tableTabs.find(tab => tab.id === item.id) : null;
    if (tab?.saving || (!force && tab?.dirty && !window.confirm(text('discard')))) return false;
    if (tab) ui.tableTabs = ui.tableTabs.filter(value => value !== tab);
    ui.bottomTabs = ui.bottomTabs.filter(value => !(value.kind === item.kind && value.id === item.id));
    if (ui.currentTableId === item.id) { ui.currentTableId = null; ui.table = null; ui.tableResponse = null; ui.dirty = 0; ui.original = new Map(); ui.saving = false; }
    if (ui.activeBottom?.kind === item.kind && ui.activeBottom.id === item.id) { ui.activeBottom = null; if (ui.bottomTabs.length) activateBottom(ui.bottomTabs.at(-1)); else paintBottom(); }
    renderTree(); renderWorkspaceTabs(); return true;
  }
  function clearTable() { if (ui.currentTableId) closeBottomTab({ kind: 'table', id: ui.currentTableId }, true); }
  function connectionTabsGuard(id) {
    captureTableState(); const tabs = ui.tableTabs.filter(tab => tab.target.id === id);
    return !tabs.some(tab => tab.saving) && (!tabs.some(tab => tab.dirty) || window.confirm(text('discard')));
  }
  function clearConnectionTabs(id) {
    for (const tab of [...ui.tableTabs]) if (tab.target.id === id) closeBottomTab({ kind: 'table', id: tab.id }, true);
    for (const tab of ui.sqlTabs) { for (const [key, target] of tab.checked) if (target.id === id) tab.checked.delete(key); if (tab.focus?.id === id) tab.focus = null; if (tab.templateContext?.target.id === id) tab.templateContext = null; }
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
    renderTree(); renderTargets();
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
        output.push(`<div class="db-tree-row ${focused && !ui.table ? 'active' : ''}" style="padding-left:14px"><button type="button" data-db-database="${escaped(targetKey)}"><span>${objects?.expanded ? '▾' : '▸'} ◫ ${escaped(database)}</span></button></div>`);
        if (!objects?.expanded) continue;
        if (objects.loading || objects.error) { output.push(`<div class="db-tree-note ${objects.error ? 'error' : ''}">${escaped(objects.error || text('loading'))}</div>`); continue; }
        if (!objects.items.length) output.push(`<div class="db-tree-note">${text('emptyObjects')}</div>`);
        let lastSchema = null;
        for (const [index, object] of objects.items.entries()) {
          if (object.schema !== lastSchema) { lastSchema = object.schema; if (object.schema !== database) output.push(`<div class="db-tree-note" style="padding-left:30px">${escaped(object.schema)}</div>`); }
          const selected = ui.table && key(ui.table) === targetKey && ui.table.schema === object.schema && ui.table.table === object.name;
          output.push(`<div class="db-tree-row ${selected ? 'active' : ''}" style="padding-left:32px"><button type="button" data-db-table="${escaped(targetKey)}" data-object-index="${index}"><span>${object.kind === 'view' ? '∿' : '▦'} ${escaped(object.name)}</span><small>${text(object.kind === 'view' ? 'view' : 'table')}</small></button></div>`);
          if (selected) for (const c of ui.tableResponse?.structure.columns || []) output.push(`<div class="db-tree-note" style="padding-left:49px">${c.primary ? '⚿' : '·'} ${escaped(c.name)} <small>${escaped(c.type)}</small></div>`);
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
    objects = { expanded: true, loading: true, items: [] }; ui.objects.set(id, objects); renderTree();
    try { objects.items = await api('objects', { target }); }
    catch (error) { objects.error = error.message; }
    finally { objects.loading = false; renderTree(); }
  }
  function showTab(tab) {
    if (tab === 'sql') { const session = sqlSession(); if (session?.query || session?.message || session?.controller) activateBottom({ kind: 'result', id: session.id }); ui.editor.focus(); return; }
    ui.tab = tab; const table = tableSession(); if (table) { table.view = tab; if (ui.activeBottom?.kind !== 'table') { activateBottom({ kind: 'table', id: table.id }); return; } }
    paintBottom(); renderTargets();
  }
  function resizeGrids() {
    for (const [name, instance] of Object.entries(ui.grids)) {
      const container = el(`db-${name}-grid`);
      if (instance._databaseReady && container.offsetWidth && !container.querySelector('.tabulator-cell.tabulator-editing')) instance.redraw();
    }
    ui.editor?.resize();
  }
  function cellText(value) { if (value === null) return 'NULL'; if (value && typeof value === 'object' && value.blob !== undefined) return `[BLOB ${Math.floor(value.blob.length * .75)}]`; return value === undefined ? '' : String(value); }
  function formatter(cell) {
    const span = document.createElement('span'), value = cell.getValue(); span.textContent = cellText(value); span.classList.toggle('db-cell-null', value === null);
    if (cell.getRow().getData().__rowKey) {
      const row = cell.getRow().getData(), index = Number(cell.getField().slice(1)), original = ui.original.get(row.__rowKey);
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
  function changes() {
    if (!ui.grids.data || !ui.tableResponse) return [];
    const columns = ui.tableResponse.result.columns, primary = ui.tableResponse.structure.columns.filter(c => c.primary).map(c => c.name);
    return ui.grids.data.getData().flatMap(row => {
      const old = ui.original.get(row.__rowKey), values = {}, original = {}, identity = {};
      columns.forEach((c, i) => { const value = row[`v${i}`]; if (row.__new ? value !== undefined : JSON.stringify(value) !== JSON.stringify(old?.[i])) { values[c.name] = value; if (!row.__new) original[c.name] = old[i]; } if (primary.includes(c.name) && old) identity[c.name] = old[i]; });
      if (row.__delete) { columns.forEach((c, i) => { original[c.name] = old[i]; }); return [{ operation: 'delete', key: identity, original }]; }
      if (row.__new) return [{ operation: 'insert', values }];
      return Object.keys(values).length ? [{ operation: 'update', key: identity, original, values }] : [];
    });
  }
  function updateDirty() { ui.dirty = changes().length; const tab = tableSession(); if (tab) { tab.dirty = ui.dirty; tab.message = ''; tab.error = ''; } el('db-save-rows').disabled = !ui.dirty; status(ui.dirty ? text('dirty', { count: ui.dirty }) : ui.tableResponse?.editable === false ? text('readOnly') : ''); renderWorkspaceTabs(); }
  function renderData(draft) {
    const response = ui.tableResponse; if (!response) return;
    ui.original = draft?.rows ? new Map(draft.original) : new Map();
    const data = draft?.rows ? clone(draft.rows) : response.result.rows.map((values, index) => { const row = { __rowKey: `row-${index}` }; values.forEach((value, i) => { row[`v${i}`] = value; }); ui.original.set(row.__rowKey, clone(values)); return row; });
    const editable = cell => { const row = cell.getRow().getData(), index = Number(cell.getField().slice(1)), name = response.result.columns[index]?.name, meta = response.structure.columns.find(c => c.name === name); return !row.__delete && (row.__new || response.editable) && !meta?.generated && !(cell.getValue() && typeof cell.getValue() === 'object'); };
    const columns = [{ title: '', field: '__select', width: 36, minWidth: 36, formatter: 'rowSelection', titleFormatter: 'rowSelection', headerSort: false, cellClick: (event, cell) => cell.getRow().toggleSelect() }, ...response.result.columns.map((c, i) => ({ title: escaped(c.name), field: `v${i}`, headerTooltip: c.type, editor: 'input', editable, contextMenu: [{ label: text('setNull'), action: (event, cell) => { if (editable(cell)) { cell.setValue(null); updateDirty(); } } }] }))];
    const instance = grid('data', columns, data, { index: '__rowKey', editTriggerEvent: 'dblclick', selectableRows: 'highlight', initialSort: draft?.sort || [], rowFormatter: row => row.getElement().classList.toggle('db-row-deleted', Boolean(row.getData().__delete)) });
    instance.on('cellEdited', cell => { cell.getRow().reformat(); updateDirty(); });
    if (draft) instance.on('tableBuilt', () => { if (ui.currentTableId !== draft.id) return; if (draft.selected?.length) instance.selectRow(draft.selected); requestAnimationFrame(() => { const holder = el('db-data-grid').querySelector('.tabulator-tableholder'); if (holder) holder.scrollTop = draft.scroll || 0; }); });
    ui.dirty = draft?.dirty || 0; el('db-save-rows').disabled = !ui.dirty || Boolean(draft?.saving);
    el('db-delete-row').disabled = !response.editable; el('db-add-row').disabled = ui.table?.kind === 'view';
    el('db-data-count').textContent = text('rows', { count: data.length }); el('db-page-label').textContent = text('page', { page: ui.page + 1 });
    el('db-page-prev').disabled = ui.page === 0; el('db-page-next').disabled = !response.result.truncated; el('db-page-last').disabled = !response.result.truncated;
  }
  function renderStructure() {
    if (!ui.tableResponse) return;
    const meta = ui.tableResponse.structure;
    grid('structure', [{ title: text('name'), field: 'name' }, { title: text('type'), field: 'type' }, { title: text('nullable'), field: 'nullable', formatter: 'tickCross' }, { title: text('primary'), field: 'primary', formatter: 'tickCross' }, { title: text('default'), field: 'default' }, { title: text('generated'), field: 'generated', formatter: 'tickCross' }], meta.columns, { selectableRows: 1 });
    ui.grids.structure.on('rowDblClick', (event, row) => { row.select(); openDesigner('alterColumn'); });
    grid('index', [{ title: text('name'), field: 'name' }, { title: text('indexColumns'), field: 'columns', formatter: cell => { const span = document.createElement('span'); span.textContent = cell.getValue().join(', '); return span; } }, { title: text('unique'), field: 'unique', formatter: 'tickCross' }, { title: text('primary'), field: 'primary', formatter: 'tickCross' }], meta.indexes, { selectableRows: 1 });
  }
  async function fetchTableRecord(tab, page = tab.page || 0, last = false) {
    const nonce = (tab.loadNonce || 0) + 1; tab.loadNonce = nonce; tab.loading = true; tab.error = ''; tab.page = page;
    if (ui.activeBottom?.kind === 'table' && ui.activeBottom.id === tab.id) { el('db-panel-data').inert = true; paintBottom(); }
    renderWorkspaceTabs();
    try {
      const response = await api('table', { target: { ...tab.target, offset: page * Number(tab.pageSize || 100), limit: tab.pageSize || 100, ...(last ? { last: 1 } : {}) } });
      if (nonce !== tab.loadNonce || !ui.tableTabs.includes(tab)) return;
      tab.response = response; if (last) tab.page = Math.floor(response.offset / response.limit); tab.rows = null; tab.original = null; tab.selected = []; tab.dirty = 0; tab.error = '';
    } catch (error) { if (nonce === tab.loadNonce) tab.error = error.message; }
    finally {
      if (nonce === tab.loadNonce && ui.tableTabs.includes(tab)) {
        tab.loading = false;
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
    if (ui.saving) return;
    const pending = changes(), tab = tableSession(); if (!pending.length || !tab) return;
    captureTableState(); tab.saving = true; ui.saving = true; el('db-panel-data').inert = true; el('db-save-rows').disabled = true;
    try {
      const result = await api('changes', { target: { ...tab.target }, body: { changes: pending } });
      tab.response = null; tab.rows = null; tab.original = null; tab.dirty = 0;
      if (ui.currentTableId === tab.id) { ui.tableResponse = null; ui.dirty = 0; ui.original = new Map(); }
      await fetchTableRecord(tab);
      tab.message = text('savedRows', { count: result.affected });
    } catch (error) { tab.error = error.message; tab.message = ''; }
    finally {
      tab.saving = false;
      if (ui.currentTableId === tab.id) { ui.saving = false; el('db-panel-data').inert = Boolean(tab.loading); el('db-save-rows').disabled = !tab.dirty; }
      if (ui.activeBottom?.kind === 'table' && ui.activeBottom.id === tab.id) paintBottom();
      renderWorkspaceTabs();
    }
  }

  function captureResultState() {
    const item = ui.activeBottom, session = item?.kind === 'result' && ui.sqlTabs.find(tab => tab.id === item.id);
    if (!session?.query || !ui.grids.query?._databaseReady || ui.resultGridOwner !== session.id) return;
    session.filters = ui.grids.query.getFilters(); session.headerFilters = ui.grids.query.getHeaderFilters(); session.sort = ui.grids.query.getSorters().map(sort => ({ column: sort.field, dir: sort.dir }));
    const holder = el('db-query-grid').querySelector('.tabulator-tableholder'); session.scroll = holder ? { top: holder.scrollTop, left: holder.scrollLeft } : null;
  }
  function queryCellEditable(result, cell) {
    if (result.saving) return false;
    const index = cell.getRow().getData().__resultRow, row = result.rows[index];
    if (!row || !/^v\d+$/.test(cell.getField())) return false;
    const edit = result.sources[row.source]?.edit, column = edit?.columns[result.columns[Number(cell.getField().slice(1))]?.name];
    if (!column || column.generated || (cell.getValue() !== null && typeof cell.getValue() === 'object')) return false;
    return Object.entries(edit.columns).filter(([, meta]) => meta.primary).every(([label]) => {
      const value = row.values[result.columns.findIndex(item => item.name === label)];
      return value != null && typeof value !== 'object';
    });
  }
  async function saveQueryCell(result, owner, instance, cell) {
    const rowIndex = cell.getRow().getData().__resultRow, row = result.rows[rowIndex], columnIndex = Number(cell.getField().slice(1));
    const previous = row?.values[columnIndex], next = cell.getValue();
    const revert = () => { if (ui.grids.query !== instance) return; instance._revertingResult = true; try { cell.setValue(previous); } finally { instance._revertingResult = false; } };
    if (JSON.stringify(previous) === JSON.stringify(next)) return;
    if (!owner || owner.query !== result || !queryCellEditable(result, cell)) { revert(); return; }
    const source = result.sources[row.source], edit = source.edit, column = edit.columns[result.columns[columnIndex].name], identity = {}, original = {};
    for (const [label, meta] of Object.entries(edit.columns)) {
      const value = row.values[result.columns.findIndex(item => item.name === label)];
      if (meta.name === column.name) original[meta.name] = value;
      if (meta.primary) identity[meta.name] = value;
    }
    result.saving = true; result.editMessage = ''; result.editError = false;
    el('db-query-grid').inert = true; status(text('resultSaving'));
    const editStatus = el('db-query-summary').querySelector('.db-query-edit-status'); if (editStatus) editStatus.textContent = text('resultSaving');
    try {
      await api('changes', { target: edit.target, body: { changes: [{ operation: 'update', key: identity, original, values: { [column.name]: next } }] } });
      row.values[columnIndex] = next;
      result.editMessage = text('resultSaved', { source: sourceName(edit.target), column: column.name });
    } catch (error) { revert(); result.editMessage = error.message; result.editError = true; }
    finally {
      result.saving = false;
      if (ui.activeBottom?.kind === 'result' && ui.activeBottom.id === owner.id && owner.query === result) { captureResultState(); paintBottom(); }
    }
  }
  function renderQuery(result) {
    const owner = ui.activeBottom?.kind === 'result' ? ui.sqlTabs.find(tab => tab.id === ui.activeBottom.id) : null;
    const editable = cell => queryCellEditable(result, cell);
    const statementOnly = result.mode === 'execute' && !result.columns.length;
    const columns = [{ title: text('sourceConnection'), field: '__source', minWidth: 150, formatter: 'plaintext' }, { title: text('sourceDatabase'), field: '__database', formatter: 'plaintext' }, { title: text('sourceEngine'), field: '__engine', formatter: 'plaintext' }, ...(statementOnly ? [ { title: text('executionStatus'), field: '__status', formatter: 'plaintext' }, { title: text('affectedTitle'), field: '__affected', formatter: 'plaintext' }, { title: text('duration'), field: '__duration', formatter: 'plaintext' }, { title: text('errorDetail'), field: '__error', formatter: 'plaintext', minWidth: 220 } ] : result.columns.map((c, i) => ({ title: escaped(c.name), field: `v${i}`, headerTooltip: c.type, editor: 'input', editable, contextMenu: [{ label: text('setNull'), action: (event, cell) => { if (editable(cell)) cell.setValue(null); } }] })))];
    const origin = source => ({ __source: source.name || profile(source.target.id)?.name || source.target.id, __database: source.edit?.target.database || source.target.database, __engine: source.engine });
    const data = statementOnly ? result.sources.map(source => ({ ...origin(source), __status: text(source.error ? 'failed' : 'completed'), __affected: source.error ? '—' : source.affected ?? text('unknownAffected'), __duration: source.milliseconds, __error: source.error || '' })) : result.rows.map((row, index) => { const record = { ...origin(result.sources[row.source]), __resultRow: index }; row.values.forEach((value, i) => { record[`v${i}`] = value; }); return record; });
    const instance = grid('query', columns, data, { editTriggerEvent: 'dblclick', initialFilter: owner?.filters || [], initialHeaderFilter: owner?.headerFilters || [], initialSort: owner?.sort || [] });
    ui.resultGridOwner = owner?.id;
    el('db-query-grid').inert = Boolean(result.saving);
    instance.on('cellEdited', cell => { if (!instance._revertingResult) void saveQueryCell(result, owner, instance, cell); });
    if (owner?.scroll) instance.on('tableBuilt', () => requestAnimationFrame(() => { if (ui.resultGridOwner !== owner.id) return; const holder = el('db-query-grid').querySelector('.tabulator-tableholder'); const scroll = owner.scroll; if (holder && scroll) { holder.scrollTop = scroll.top; holder.scrollLeft = scroll.left; } }));
    const succeeded = result.sources.filter(source => !source.error).length;
    const execution = result.mode === 'execute';
    const unknownCount = result.sources.filter(source => !source.error && source.executed && source.affected == null).length;
    const summary = text(execution ? 'executionDone' : 'queryDone', { rows: result.rows.length, count: result.affected, ok: succeeded, total: result.sources.length, ms: result.elapsed || 0 }) + (execution && unknownCount ? ' · ' + text('unknownAffectedSources', { count: unknownCount }) : '');
    const sourceMessage = source => source.error ? escaped(source.error) : [source.affected != null ? text('affectedRows', { count: source.affected }) : source.executed ? text('completed') + ' · ' + text('unknownAffected') : '', source.rowCount || !source.executed ? text('rows', { count: source.rowCount ?? 0 }) : '', source.result.truncated ? text('truncated') : ''].filter(Boolean).join(' · ');
    el('db-query-summary').innerHTML = `${escaped(summary)}${execution && result.sources.length > 1 ? `<div>${text('independentExecution')}</div>` : ''}${succeeded !== result.sources.length ? `<div class="db-error">${text(execution ? 'executionPartial' : 'partial')}</div>` : ''}<details open><summary>${text('details')}</summary>${result.sources.map(source => `<div class="db-query-source ${source.error ? 'error' : ''}">${escaped(source.name || profile(source.target.id)?.name || source.target.id)} / ${escaped(source.target.database)} · ${source.milliseconds} ms · ${sourceMessage(source)}</div>`).join('')}</details>`;
    if (!statementOnly) {
      const message = result.saving ? text('resultSaving') : result.editMessage || text(result.sources.some(source => source.edit) ? 'resultEditHint' : 'resultReadOnly');
      el('db-query-summary').insertAdjacentHTML('afterbegin', `<div class="db-query-edit-status ${result.editError ? 'db-error' : ''}">${escaped(message)}</div>`);
    }
    status(result.editMessage || summary, Boolean(result.editError) || succeeded !== result.sources.length);
  }
  async function runSQL(mode = 'run', options = {}) {
    const session = sqlSession(); if (!session || session.controller || session.query?.saving) return;
    const single = singleQueryTarget(), targets = options.scope === 'single' ? (single ? [single] : []) : targetList();
    const sql = (options.sql ?? (ui.editor.selected() || ui.editor.currentStatement() || ui.editor.value())).trim();
    if (options.scope === 'multi' && targets.length < 2) return status(text('chooseMultiple'), true);
    if (!targets.length) return status(text('noTargets'), true); if (!sql) return status(text('emptySql'), true);
    captureSQL();
    const controller = new AbortController(), limit = Number(el('db-query-limit').value);
    session.controller = controller; session.query = null; session.message = text('running', { count: targets.length }); session.error = false; session.filters = []; session.headerFilters = []; session.sort = []; session.scroll = null;
    ui.controller = controller; el('db-cancel-query').disabled = false; el('db-run-query').disabled = true;
    activateBottom({ kind: 'result', id: session.id }); renderTargets();
    const start = performance.now();
    try {
      const result = await api('query', { body: { targets, sql, mode, limit }, signal: controller.signal });
      if (!ui.sqlTabs.includes(session) || session.controller !== controller) return;
      result.elapsed = Math.round(performance.now() - start); session.query = result; session.message = '';
      if (result.mode === 'execute') {
        // Keep the execution report even if refreshing metadata fails.
        await Promise.allSettled(result.sources.filter(source => !source.error).map(async source => {
          const target = source.target, objects = await api('objects', { target });
          ui.objects.set(key(target), { ...ui.objects.get(key(target)), items: objects });
        })); renderTree();
      }
    } catch (error) { session.message = error.name === 'AbortError' ? text('executionCancelled') : error.message; session.error = error.name !== 'AbortError'; }
    finally {
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

  function connectionMode() {
    const form = el('db-connection-form'), sqlite = form.elements.engine.value === 'sqlite';
    form.querySelector('.db-connection-network').classList.toggle('hidden', sqlite); el('db-sqlite-path').classList.toggle('hidden', !sqlite);
    form.elements.host.required = !sqlite; form.elements.user.required = !sqlite; form.elements.file.required = sqlite;
  }
  window.openDatabaseConnection = (id = '') => {
    if (id && !connectionTabsGuard(id)) return;
    setView('database'); initialize(); const form = el('db-connection-form'), p = profile(id); form.reset(); form.dataset.id = p?.id || ''; form.dataset.passwordDirty = 'false';
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
  function closeDialog(name) { el(`db-${name}-modal`).classList.add('hidden'); }

  function selectedColumn() { return ui.grids.structure?.getSelectedData()[0] || ui.tableResponse?.structure.columns[0]; }
  function selectedIndex() { return ui.grids.index?.getSelectedData()[0]; }
  function clearPlan() { ui.schemaPlan = null; el('db-apply-schema').disabled = true; el('db-schema-preview').classList.add('hidden'); dialogStatus('schema', ''); }
  function schemaColumnRow(spec = {}, create = true) {
    const name = spec.name || '', type = spec.type || 'varchar(255)';
    return `<div class="db-schema-column"><input data-column-name value="${escaped(name)}" placeholder="${text('name')}" ${!create ? 'readonly' : ''} aria-label="${text('name')}"/><input data-column-type value="${escaped(type)}" placeholder="${text('type')}" aria-label="${text('type')}"/><input data-column-default value="${escaped(spec.default || '')}" placeholder="${text('default')}" aria-label="${text('default')}"/>${create ? `<button type="button" class="text-button" data-remove-column>${text('remove')}</button>` : ''}<div class="db-column-flags"><label class="db-check"><input type="checkbox" data-column-null ${spec.nullable ? 'checked' : ''}/>${text('nullable')}</label><label class="db-check"><input type="checkbox" data-column-primary ${spec.primary ? 'checked' : ''} ${!create ? 'disabled' : ''}/>${text('primary')}</label>${create ? `<label class="db-check"><input type="checkbox" data-column-auto ${spec.auto ? 'checked' : ''}/>${text('auto')}</label>` : ''}</div></div>`;
  }
  function renderDesignerFields() {
    clearPlan(); const form = el('db-schema-form'), action = form.elements.action.value, fields = el('db-schema-fields'), columns = ui.tableResponse?.structure.columns || [], column = selectedColumn(), index = selectedIndex();
    ui.schemaTarget = { ...(action === 'createTable' ? ui.designerCreateTarget : ui.table) };
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
  async function previewSchema(event) {
    event.preventDefault(); clearPlan();
    try { const payload = schemaPayload(), result = await api('schema', { target: ui.schemaTarget, body: payload }); ui.schemaPlan = { payload, result }; el('db-schema-preview').textContent = result.sql.join(';\n\n') + (result.sql.length ? ';' : ''); el('db-schema-preview').classList.remove('hidden'); el('db-apply-schema').disabled = !result.sql.length; dialogStatus('schema', result.sql.length ? text(result.transactional ? 'transaction' : 'autoCommit') : text('nothingToChange')); }
    catch (error) { dialogStatus('schema', error.message, true); }
  }
  async function applySchema() {
    if (!ui.schemaPlan) return dialogStatus('schema', text('previewFirst'), true);
    const { payload, result } = ui.schemaPlan, target = { ...ui.schemaTarget }; el('db-apply-schema').disabled = true;
    try { await api('schema', { target, body: { ...payload, hash: result.hash } }); closeDialog('schema'); await loadObjects(target, true); if (payload.action === 'dropTable') { clearTable(); renderTree(); } else { if (payload.action === 'renameTable') { const existing = ui.tableTabs.find(tab => tableKey(tab.target) === tableKey(target)); if (existing) { existing.target = { ...existing.target, table: payload.name }; if (ui.currentTableId === existing.id) ui.table = existing.target; } } if (payload.action === 'createTable' || payload.action === 'renameTable') target.table = payload.name; await openTable({ ...target, kind: 'table' }, { confirm: false }); showTab('structure'); } status(text('applied')); syncTemplates(true); }
    catch (error) { clearPlan(); dialogStatus('schema', error.message, true); }
  }

  function initialize() {
    if (ui.initialized) return; ui.initialized = true;
    ui.editor = DatabaseUI.createEditor(el('db-sql-editor'), () => runSQL(), (scope, sql) => runSQL('run', { scope, sql }), editorChanged, slot => openInlinePicker(slot));
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
      else if (button.dataset.dbTable) { const [id, database] = JSON.parse(button.dataset.dbTable), object = ui.objects.get(button.dataset.dbTable).items[Number(button.dataset.objectIndex)]; openTable({ id, database, schema: object.schema, table: object.name, kind: object.kind }); }
    });
    el('db-refresh-tree').addEventListener('click', () => { ui.catalogs.clear(); ui.objects.clear(); loadProfiles().catch(error => status(error.message, true)); });
    el('db-tree-create-table').addEventListener('click', () => openDesigner('createTable', ui.focus));
    view.querySelectorAll('[data-db-tab]').forEach(button => button.addEventListener('click', () => showTab(button.dataset.dbTab)));
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
    el('db-inline-body').addEventListener('keydown', event => { if (event.key === 'Enter' && event.target.closest('[data-operand]')) { event.preventDefault(); applyInlinePicker(); } });
    el('db-inline-body').addEventListener('change', event => { if (event.target.id === 'db-inline-operator') syncOperandInputs(); else if (event.target.id === 'db-inline-source') loadInlineObjects(); else if (event.target.id === 'db-inline-table') loadInlineMetadata(); else if (event.target.name === 'inline-column') el('db-inline-apply').disabled = !el('db-inline-body').querySelector('[name="inline-column"]:checked'); });
    el('db-inline-body').addEventListener('click', event => { if (event.target.id === 'db-inline-all' || event.target.id === 'db-inline-none') { el('db-inline-body').querySelectorAll('[name="inline-column"]').forEach(input => { input.checked = event.target.id === 'db-inline-all'; }); el('db-inline-apply').disabled = event.target.id === 'db-inline-none'; } else if (event.target.id === 'db-inline-star') applyInlinePicker(true); });
    document.addEventListener('mousedown', event => { if (inline.open && !event.target.closest('#db-inline-picker, [data-db-slot]')) closeInlinePicker(); });
    el('db-template-list').addEventListener('click', event => { const button = event.target.closest('[data-db-template]'); if (button && !button.disabled) fillTemplate(button.dataset.dbTemplate); });
    el('db-reload-table').addEventListener('click', () => ui.table && openTable(ui.table, { page: ui.page, reload: true }));
    el('db-save-rows').addEventListener('click', saveRows);
    el('db-add-row').addEventListener('click', async () => { if (!ui.table || !ui.tableResponse) return status(text('chooseTable'), true); await ui.grids.data.addRow({ __rowKey: `new-${++ui.sequence}`, __new: true }, false); updateDirty(); });
    el('db-delete-row').addEventListener('click', async () => { await Promise.all((ui.grids.data?.getSelectedRows() || []).map(row => row.getData().__new ? row.delete() : row.update({ __delete: true }))); updateDirty(); });
    el('db-page-prev').addEventListener('click', () => ui.table && openTable(ui.table, { page: Math.max(0, ui.page - 1) })); el('db-page-next').addEventListener('click', () => ui.table && openTable(ui.table, { page: ui.page + 1 })); el('db-page-last').addEventListener('click', () => ui.table && openTable(ui.table, { last: true })); el('db-page-size').addEventListener('change', () => ui.table && openTable(ui.table, { page: 0, pageSize: el('db-page-size').value, reload: true }));
    for (const [id, action] of [['db-create-table', 'createTable'], ['db-add-column', 'addColumn'], ['db-alter-column', 'alterColumn'], ['db-create-index', 'createIndex'], ['db-drop-index', 'dropIndex']]) el(id).addEventListener('click', () => openDesigner(action));
    const connectionForm = el('db-connection-form'); connectionForm.addEventListener('submit', saveConnection); connectionForm.elements.password.addEventListener('input', () => { connectionForm.dataset.passwordDirty = 'true'; }); connectionForm.elements.engine.addEventListener('change', () => { connectionForm.elements.port.value = connectionForm.elements.engine.value === 'postgres' ? '5432' : '3306'; connectionMode(); });
    el('db-test-connection').addEventListener('click', async event => { const button = event.currentTarget; button.disabled = true; dialogStatus('connection', text('loading')); try { await api('test', { body: connectionPayload() }); dialogStatus('connection', text('connected')); } catch (error) { dialogStatus('connection', error.message, true); } finally { button.disabled = false; } });
    el('db-delete-connection').addEventListener('click', async () => { const id = connectionForm.dataset.id; if (!id || !window.confirm(text('confirmDeleteConnection'))) return; try { await api('profiles', { target: { id }, method: 'DELETE' }); if (ui.focus?.id === id) ui.focus = null; clearConnectionTabs(id); await loadProfiles(); closeDialog('connection'); } catch (error) { dialogStatus('connection', error.message, true); } });
    const schemaForm = el('db-schema-form'); schemaForm.addEventListener('submit', previewSchema); schemaForm.elements.action.addEventListener('change', renderDesignerFields); schemaForm.addEventListener('input', clearPlan); schemaForm.addEventListener('change', event => { clearPlan(); if (event.target.name === 'oldName' && schemaForm.elements.action.value === 'alterColumn') { const column = ui.tableResponse.structure.columns.find(c => c.name === event.target.value); el('db-schema-fields').querySelector('.db-schema-rows').innerHTML = schemaColumnRow(column, false); } });
    el('db-schema-fields').addEventListener('click', event => { if (event.target.closest('[data-add-column]')) { el('db-schema-fields').querySelector('.db-schema-rows').insertAdjacentHTML('beforeend', schemaColumnRow({ nullable: true })); clearPlan(); } if (event.target.closest('[data-remove-column]')) { event.target.closest('.db-schema-column').remove(); clearPlan(); } }); el('db-apply-schema').addEventListener('click', applySchema);
    document.querySelectorAll('[data-db-close]').forEach(button => button.addEventListener('click', () => closeDialog(button.dataset.dbClose))); for (const name of ['connection', 'schema']) bindBackdropDismissal(el(`db-${name}-modal`), () => closeDialog(name));
    document.addEventListener('keydown', event => { if (event.key === 'Escape') { if (targetPicker.open) { toggleTargetPicker(false, true); return; } closeDialog('connection'); closeDialog('schema'); if (inline.open) closeInlinePicker(); else if (templates.open) toggleTemplates(false); } });
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
