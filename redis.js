// Redis transport is go-redis; the value editor reuses the bundled CodeMirror 6.
(() => {
  const labels = {
    add:['New connection','新建连接'], edit:['Edit connection','编辑连接'], remove:['Delete connection','删除连接'], refresh:['Refresh','刷新'], openDB:['Open','打开'], pattern:['Key pattern, e.g. biz:*','键名模式，例如 biz:*'], search:['Search','搜索'], more:['Load more','加载更多'], title:['Redis manager','Redis 管理'], newKey:['New key','新增键'], empty:['Create a connection or select a key to manage its data.','新建连接或选择左侧的键，查看并管理数据。'], applyTTL:['Apply TTL','修改 TTL'], ttlHint:['Seconds; -1 persists, -2 keeps expiry','秒；-1 永不过期，-2 保留到期时间'], text:['Text','文本'], pretty:['Format JSON','格式化 JSON'], reload:['Reload value','重载键值'], delete:['Delete key','删除键'], save:['Save · Ctrl+S','保存 · Ctrl+S'], preview:['Preview only: large collection, Stream or binary data; overwriting is disabled.','仅预览：集合过大、Stream 或包含二进制数据，不能覆盖保存。'], connection:['Redis connection','Redis 连接'], name:['Name','名称'], host:['Host','主机'], port:['Port','端口'], user:['ACL username (optional)','ACL 用户名（可选）'], password:['Password (blank keeps saved password)','密码（留空保留已保存密码）'], clearPassword:['Clear saved password','清除已保存密码'], tls:['TLS (verify server certificate)','TLS（验证服务器证书）'], test:['Test connection','测试连接'], saveConnection:['Save connection','保存连接'], none:['Choose a connection','选择连接'], working:['Working…','处理中…'], saved:['Saved','已保存'], connected:['Connection successful','连接成功'], discard:['Discard unsaved changes?','放弃尚未保存的修改？'], confirmRemove:['Delete this connection? Redis data will be retained.','删除此连接配置？Redis 中的数据会保留。'], confirmDelete:['Delete key “{key}”?','删除键“{key}”？'], deleted:['Key deleted','键已删除'], keys:['{count} loaded keys','已加载 {count} 个键'], noKeys:['No matching keys in this batch. Continue loading if more are available.','本批没有匹配的键；如有后续批次，可继续加载。'], loaded:['Loaded','已加载'], dirty:['Unsaved changes','有未保存修改'], ttl:['TTL: {ttl} s','TTL：{ttl} 秒'], persistent:['No expiry','永不过期'], size:['{size} bytes / items','{size} 字节 / 项'], collectionHint:['Collections use JSON: hash {"field":"value"}; list/set ["value"]; zset [{"Score":1,"Member":"value"}].','集合使用 JSON：Hash {"字段":"值"}；List/Set ["值"]；ZSet [{"Score":1,"Member":"值"}]。'], stringHint:['Text saves the exact text. JSON view validates and formats JSON before saving.','文本模式原样保存；JSON 模式会校验 JSON 格式。'], badTTL:['TTL must be -2, -1 or a positive number of seconds.','TTL 必须为 -2、-1 或正整数秒。'], needKey:['Enter a key name.','请输入键名。'], database:['DB {db} · {count} keys','DB {db} · {count} 个键'], scanLimit:['10,000 keys loaded. Narrow the search to browse more.','已加载 10,000 个键，请缩小搜索范围。'], credentials:['Credentials are stored locally in data/redis.json.','连接凭据保存在本地 data/redis.json。'], unsupported:['This action is unavailable for the selected key.','当前键不支持此操作。']
  };
  for (const [key, pair] of Object.entries(labels)) { i18n.en[`redis.${key}`]=pair[0]; i18n.zh[`redis.${key}`]=pair[1]; }
  i18n.en['nav.redis']='Redis'; i18n.zh['nav.redis']='Redis';
  for (const [key,pair] of Object.entries({manage:['Manage connections…','管理连接…'],connect:['Connect','连接'],noConnections:['No connections. Use Redis → New connection to add one.','暂无连接，请从顶部“Redis → 新增连接”添加。']})) { i18n.en[`redis.${key}`]=pair[0];i18n.zh[`redis.${key}`]=pair[1]; }
  i18n.zh['redis.add']='新增连接';
  for(const [key,pair] of Object.entries({namespaceReload:['Reload namespace (Ctrl+R)','重载命名空间（Ctrl+R）'],namespaceAdd:['Add key','添加新键'],namespaceDelete:['Delete namespace','删除命名空间'],namespaceConfirm:['Delete {count} keys in “{prefix}” from {source}? This cannot be undone.','删除 {source} 中命名空间“{prefix}”下的 {count} 个键？此操作无法撤销。'],namespaceDeleted:['Deleted {count} keys','已删除 {count} 个键'],namespaceReloaded:['Reloaded {prefix}: {count} keys','已重载 {prefix}：{count} 个键'],keyName:['Key name','键名'],type:['Type','类型'],newTTLHint:['Seconds; -1 persists','秒；-1 永不过期'],importValue:['Import value from a UTF-8 file (optional)','从 UTF-8 文件导入值（可选）'],cancel:['Cancel','取消'],saveNew:['Save','保存'],newTitle:['Add key to {source}','添加新键到 {source}'],fileTooLarge:['The value file must be no larger than 2 MiB.','值文件不能超过 2 MiB。']})){i18n.en[`redis.${key}`]=pair[0];i18n.zh[`redis.${key}`]=pair[1];}
  for(const [key,pair] of Object.entries({copyValue:['Copy value (Ctrl+C)','复制键值（Ctrl+C）'],copied:['Value copied','键值已复制'],copyIncomplete:['The preview is incomplete or contains binary data and cannot be copied in full.','预览不完整或包含二进制数据，无法复制完整键值。'],copyFailed:['Could not access the clipboard.','无法访问剪贴板。']})){i18n.en[`redis.${key}`]=pair[0];i18n.zh[`redis.${key}`]=pair[1];}
  const el=id=>document.getElementById(`redis-${id}`), tr=(key,args)=>t(`redis.${key}`,args);
  const ui={initialized:false,profiles:[],id:'',db:0,keys:new Set(),cursor:'0',complete:true,pattern:'*',key:null,value:null,creating:false,editor:null,original:'',dirty:false,busy:false,editingID:'',counts:{},groups:new Set()};
  function status(message,error=false) { el('status').textContent=message; el('status').classList.toggle('error',error); }
  async function api(action,{body,method,params={},signal}={}) {
    const q=new URLSearchParams({id:ui.id,db:String(ui.db),...params});
    const response=await fetch(`/api/redis/${action}?${q}`,{method:method || (body===undefined?'GET':'POST'),headers:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body),signal});
    const result=await response.json(); if(!response.ok) throw new Error(result.detail||result.error||`HTTP ${response.status}`); return result;
  }
  function controls() {
    ui.dirty=Boolean(ui.editor && (ui.editor.value()!==ui.original || el('ttl').value!=='-2' || (ui.creating&&el('key').value!=='')));
    const has=Boolean(ui.id), loaded=ui.value!==null || ui.creating, editable=loaded && !ui.value?.truncated;
    for(const id of ['add','manage','profile','pattern','new-key','refresh','more','save','delete','reload','expire','type','key','format','pretty','ttl']) el(id).disabled=ui.busy;
    for(const id of ['new-key','pattern']) el(id).disabled=ui.busy||!has;
    el('editor').inert=ui.busy;
    el('more').disabled=ui.busy||!has||ui.complete||ui.keys.size>=10000;
    el('save').disabled=ui.busy||!editable;
    for(const id of ['delete','reload','expire']) el(id).disabled=ui.busy||!loaded||ui.creating;
    el('type').disabled=ui.busy||!ui.creating;
    el('type').querySelector('option[value="stream"], option:last-child').disabled=ui.creating;
    el('key').disabled=ui.busy||!ui.creating;
    el('format').disabled=ui.busy||!loaded||el('type').value!=='string';
    el('pretty').disabled=ui.busy||!editable;
    el('search-form').querySelector('button').disabled=ui.busy||!has;
    el('dirty').textContent=ui.dirty?tr('dirty'):'';
    el('tree').querySelectorAll('button').forEach(button=>{button.disabled=ui.busy;});
    el('new-key-form').querySelectorAll('button,input,select').forEach(control=>{control.disabled=ui.busy;});
    el('new-key-format').disabled=ui.busy||el('new-key-type').value!=='string';el('new-key-editor').inert=ui.busy;
  }
  async function run(action) {
    if(ui.busy) return;
    ui.busy=true; controls(); status(tr('working'));
    try { await action(); } catch(error) { status(error.message,true); }
    finally { ui.busy=false; controls(); }
  }
  const guard=()=>!ui.dirty||window.confirm(tr('discard'));
  function clearValue() {
    ui.editor?.destroy(); ui.editor=null; ui.value=null; ui.key=null; ui.creating=false; ui.dirty=false;
    el('detail').classList.add('hidden'); el('empty').classList.remove('hidden'); controls();
  }
  function mountEditor(value,readOnly=false) {
    ui.editor?.destroy(); el('editor').replaceChildren();
    ui.editor=DatabaseUI.createRemoteTextEditor(el('editor'),{path:el('format').value==='json'?'value.json':'value.txt',text:value,readOnly,save:()=>save(),change:()=>{ui.dirty=ui.editor?.value()!==ui.original;controls();}});
  }
  function paintValue(value,creating=false,key=ui.key) {
    ui.value=creating?null:value; ui.creating=creating; ui.key=key; ui.type=value.type;
    el('detail').classList.remove('hidden'); el('empty').classList.add('hidden');
    el('type').value=value.type; el('key').value=key??''; el('ttl').value='-2';
    el('format').value=value.type==='string'?'text':'json';
    const content=value.type==='string'?value.value:JSON.stringify(value.value,null,2);
    ui.original=content; ui.dirty=false; mountEditor(content,value.truncated);
    paintMetadata(); paintTree(); controls();
  }
  function paintMetadata() {
    const v=ui.value;
    el('current-ttl').textContent=!v?'':v.ttl===-1?tr('persistent'):tr('ttl',{ttl:v.ttl});
    el('size').textContent=v?tr('size',{size:v.size}):'';
    el('preview-note').classList.toggle('hidden',!v?.truncated);
    el('value-hint').textContent=tr(el('type').value==='string'?'stringHint':'collectionHint');
    el('source').textContent=ui.id?`${ui.profiles.find(p=>p.id===ui.id)?.name} / db${ui.db}`:tr('title');
  }
  function paintProfiles() {
    el('profile').replaceChildren();
    if(!ui.profiles.length)el('profile').add(new Option(tr('none'),''));
    for(const p of ui.profiles) el('profile').add(new Option(p.name,p.id));
    el('profile').value=ui.id; paintMetadata(); controls();
  }
  function treeIcon(kind) {
    const ns='http://www.w3.org/2000/svg',svg=document.createElementNS(ns,'svg');
    svg.setAttribute('viewBox','0 0 24 24');svg.setAttribute('aria-hidden','true');svg.classList.add('redis-tree-icon');
    if(kind==='folder') {
      svg.setAttribute('fill','currentColor');
      for(const d of ['M2 3h6l2 2h10a1 1 0 0 1 1 1v2H5L1 20V4a1 1 0 0 1 1-1Z','M6 9h17l-4 11H2Z']) {const path=document.createElementNS(ns,'path');path.setAttribute('d',d);svg.append(path);}
    } else if(kind==='key') {
      svg.setAttribute('fill','none');svg.setAttribute('stroke','currentColor');svg.setAttribute('stroke-width','3');svg.setAttribute('stroke-linecap','round');svg.setAttribute('stroke-linejoin','round');
      const ring=document.createElementNS(ns,'circle');ring.setAttribute('cx','16');ring.setAttribute('cy','7');ring.setAttribute('r','5');
      const shaft=document.createElementNS(ns,'path');shaft.setAttribute('d','M12 11 2 21m3-3 3 3m0-6 3 3');svg.append(ring,shaft);
    } else {
      svg.setAttribute('fill','none');svg.setAttribute('stroke','currentColor');svg.setAttribute('stroke-width','1.8');svg.setAttribute('stroke-linecap','round');svg.setAttribute('stroke-linejoin','round');
      const paths={copy:'M8 8H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3M10 2h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Z',reload:'M20 8a9 9 0 1 0 1 8M21 2v6h-6',add:'M12 5v14M5 12h14',delete:'M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7'};
      const path=document.createElementNS(ns,'path');path.setAttribute('d',paths[kind]||paths.delete);svg.append(path);
    }
    return svg;
  }
  function paintTree() {
    const root={groups:new Map(),keys:[]};
    for(const key of [...ui.keys].sort()) {
      const parts=key.split(':'); const leaf=parts.pop(); let node=root;
      // Avoid pathological nesting while preserving the exact key on the leaf.
      for(const part of parts.slice(0,20)) { if(!node.groups.has(part)) node.groups.set(part,{groups:new Map(),keys:[]}); node=node.groups.get(part); }
      node.keys.push({key,label:parts.length>20?key:leaf});
    }
    const fragment=document.createDocumentFragment();
    function append(node,parent,path=[]) {
      for(const [name,child] of node.groups) {
        const group=document.createElement('details'), summary=document.createElement('summary'), id=JSON.stringify([...path,name]);
        const prefix=[...path,name].join(':')+':',line=document.createElement('span'),label=document.createElement('span'),actions=document.createElement('span');line.className='redis-namespace-line';label.className='redis-namespace-label';label.textContent=name||'(empty)';summary.title=prefix;summary.dataset.redisNamespace=prefix;summary.classList.toggle('active',ui.namespace===prefix);line.append(treeIcon('folder'),label);actions.className='redis-namespace-actions';
        for(const [kind,title] of [['reload','namespaceReload'],['add','namespaceAdd'],['delete','namespaceDelete']]){const action=document.createElement('button');action.type='button';action.className='redis-key-action';action.dataset.namespaceAction=kind;action.dataset.prefix=prefix;action.title=tr(title);action.setAttribute('aria-label',`${tr(title)}: ${prefix}`);action.append(treeIcon(kind));action.onclick=event=>{event.preventDefault();event.stopPropagation();if(ui.busy)return;selectNamespace(prefix);if(kind==='reload')run(()=>reloadNamespace(prefix));else if(kind==='add')openNewKey(prefix);else deleteNamespace(prefix);};actions.append(action);}
        line.append(actions);group.open=ui.groups.has(id);summary.append(line);summary.addEventListener('click',()=>{if(!ui.busy)selectNamespace(prefix);});group.append(summary);
        group.addEventListener('toggle',()=>{ if(!group.isConnected)return;if(group.open)ui.groups.add(id);else ui.groups.delete(id); });
        append(child,group,[...path,name]); parent.append(group);
      }
      for(const item of node.keys) {
        const row=document.createElement('div');row.className='redis-key-row';row.classList.toggle('active',ui.key===item.key);
        const button=document.createElement('button'); button.type='button'; button.className='redis-key-select'; button.title=item.key;
        const label=document.createElement('span');label.className='redis-key-label';label.textContent=item.label || '(empty)';button.append(treeIcon('key'),label);
        button.dataset.redisKey=item.key; button.onclick=()=>{ if(!ui.busy&&guard()){selectNamespace('');run(()=>loadValue(item.key));} };row.append(button);
        const actions=document.createElement('span');actions.className='redis-key-actions';
        for(const [kind,title] of [['copy','copyValue'],['delete','delete']]){
          const action=document.createElement('button');action.type='button';action.className='redis-key-action';action.dataset.redisAction=kind;action.dataset.key=item.key;action.title=tr(title);action.setAttribute('aria-label',`${tr(title)}: ${item.key}`);action.append(treeIcon(kind));action.onclick=()=>kind==='copy'?copyKey(item.key):deleteKey(item.key);actions.append(action);
        }
        row.append(actions);parent.append(row);
      }
    }
    append(root,fragment); el('tree').replaceChildren(fragment);
    if(!ui.keys.size) { const note=document.createElement('p');note.className='redis-hint';note.textContent=tr(ui.id?'noKeys':'empty');el('tree').append(note); }
    el('key-count').textContent=tr('keys',{count:ui.keys.size});
  }
  async function scan(reset=false) {
    if(reset) { ui.keys.clear();ui.groups.clear();ui.cursor='0';ui.complete=false;paintTree(); }
    const result=await api('scan',{params:{cursor:ui.cursor,pattern:ui.pattern}});
    for(const key of result.keys) { if(ui.keys.size>=10000)break; ui.keys.add(key); }
    ui.cursor=result.cursor;ui.complete=result.cursor==='0';paintTree();
    status(ui.keys.size>=10000?tr('scanLimit'):tr('keys',{count:ui.keys.size}));
  }
  async function loadValue(key) { const value=await api('value',{params:{key}}); paintValue(value,false,key);status(tr('loaded')); }
  function selectNamespace(prefix){ui.namespace=prefix;el('tree').querySelectorAll('[data-redis-namespace]').forEach(summary=>summary.classList.toggle('active',summary.dataset.redisNamespace===prefix));}
  async function reloadNamespace(prefix){
    const found=new Set(),controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),30000);let cursor='0';
    try{do{const page=await api('scan',{params:{prefix,cursor},signal:controller.signal});for(const key of page.keys){found.add(key);if(found.size>10000)throw new Error(tr('scanLimit'));}cursor=page.cursor;}while(cursor!=='0');}finally{clearTimeout(timeout);}
    const next=new Set([...ui.keys].filter(key=>!key.startsWith(prefix)));for(const key of found)next.add(key);if(next.size>10000)throw new Error(tr('scanLimit'));
    ui.keys=next;paintTree();status(tr('namespaceReloaded',{prefix,count:found.size}));return found;
  }
  async function deleteNamespace(prefix){
    if(ui.busy)return;if(ui.key?.startsWith(prefix)&&!guard())return;
    await run(async()=>{
      const preview=await api('namespace',{body:{prefix,operation:'preview'}}),source=`${ui.profiles.find(p=>p.id===ui.id)?.name} / db${ui.db}`;
      if(!confirm(tr('namespaceConfirm',{prefix,count:preview.count,source}))) {status('');return;}
      const result=await api('namespace',{body:{prefix,operation:'delete',token:preview.token}});
      for(const key of [...ui.keys])if(key.startsWith(prefix))ui.keys.delete(key);
      if(ui.key?.startsWith(prefix))clearValue();selectNamespace('');paintTree();status(tr('namespaceDeleted',{count:result.deleted}));
    });
  }
  async function copyKey(key) {
    await run(async()=>{
      const value=ui.key===key&&ui.value?ui.value:await api('value',{params:{key}});
      if(value.truncated)throw new Error(tr('copyIncomplete'));
      const text=value.type==='string'?value.value:JSON.stringify(value.value,null,2);
      try {await navigator.clipboard.writeText(text);}
      catch(_) {
        const focus=document.activeElement,selection=window.getSelection(),ranges=[];if(selection)for(let i=0;i<selection.rangeCount;i++)ranges.push(selection.getRangeAt(i).cloneRange());
        const area=document.createElement('textarea');area.value=text;area.style.cssText='position:fixed;left:-9999px;top:0';document.body.append(area);area.select();
        let copied=false;try{copied=document.execCommand('copy');}finally{area.remove();focus?.focus({preventScroll:true});if(selection){selection.removeAllRanges();for(const range of ranges)selection.addRange(range);}}
        if(!copied)throw new Error(tr('copyFailed'));
      }
      status(tr('copied'));showToast(tr('copied'));
    });
  }
  async function deleteKey(key) {
    if(ui.busy||key===null||(key===ui.key&&!guard())||!confirm(tr('confirmDelete',{key})))return;
    await run(async()=>{
      const value=key===ui.key&&ui.value?ui.value:await api('value',{params:{key}});
      await api('write',{body:{key,version:value.version,ttl:-2,operation:'delete'}});
      ui.keys.delete(key);if(key===ui.key)clearValue();paintTree();status(tr('deleted'));
    });
  }
  document.addEventListener('keydown',event=>{
    if(!el('view').classList.contains('hidden')&&ui.namespace&&!ui.busy&&!document.querySelector('dialog[open]')&&(event.ctrlKey||event.metaKey)&&!event.altKey&&!event.shiftKey&&event.key.toLowerCase()==='r'&&!event.target.closest('input,textarea,select,[contenteditable="true"],.cm-editor')){event.preventDefault();run(()=>reloadNamespace(ui.namespace));return;}
    if(el('view')?.classList.contains('hidden')!==false||ui.namespace||ui.busy||ui.key===null||!ui.value||event.defaultPrevented||event.altKey||event.shiftKey||!(event.ctrlKey||event.metaKey)||event.key.toLowerCase()!=='c')return;
    if(event.target.closest('input,textarea,select,[contenteditable="true"],.cm-editor')||document.querySelector('dialog[open]')||window.getSelection()?.toString())return;
    event.preventDefault();copyKey(ui.key);
  });
  async function selectConnection(id,db) {
    if(!ui.profiles.some(profile=>profile.id===id))id=ui.profiles[0]?.id||'';
    ui.id=id;ui.db=db;ui.namespace='';clearValue();paintProfiles();ui.keys.clear();ui.complete=true;paintTree();
    if(id) { await scan(true); } else { status(''); }
  }
  function ttl() { const n=Number(el('ttl').value);if(!el('ttl').value||!Number.isInteger(n)||n===0||n< -2||n>315360000)throw new Error(tr('badTTL'));return n; }
  async function save() {
    if(ui.busy || el('save').disabled) return;
    await run(async()=>{
      const key=el('key').value; if(!key&&ui.creating) throw new Error(tr('needKey'));
      const type=el('type').value, raw=ui.editor.value();
      let value=raw;
      if(type!=='string') value=JSON.parse(raw);
      else if(el('format').value==='json') JSON.parse(raw);
      await api('write',{body:{key,type,value,version:ui.value?.version||'',create:ui.creating,ttl:ttl(),operation:'save'}});
      ui.dirty=false;ui.keys.add(key);await loadValue(key);status(tr('saved'));
    });
  }
  function connectionDialog(id='') {
    if(ui.busy) return; ui.editingID=id;
    const form=el('connection-form');form.reset();const p=ui.profiles.find(p=>p.id===id);
    for(const field of ['name','host','port','user']) if(p)form.elements[field].value=p[field]??'';
    form.elements.tls.checked=Boolean(p?.tls);el('connection-status').textContent=tr('credentials');el('connection-status').classList.remove('error');el('connection-dialog').showModal();
  }
  function connectionBody() {
    const f=el('connection-form').elements;
    const body={id:ui.editingID,name:f.name.value,host:f.host.value,port:Number(f.port.value),user:f.user.value,tls:f.tls.checked};
    if(f.clearPassword.checked)body.password=''; else if(f.password.value||!ui.editingID)body.password=f.password.value;
    return body;
  }
  async function submitConnection(test) {
    const form=el('connection-form');if(!form.reportValidity()||ui.busy)return;
    if(!test&&!guard())return;
    ui.busy=true;controls();el('test').disabled=true;el('save-connection').disabled=true;el('close-dialog').disabled=true;
    el('connection-status').textContent=tr('working');el('connection-status').classList.remove('error');
    try {
      const p=await api(test?'test':'profiles',{body:connectionBody()});
      if(test)el('connection-status').textContent=tr('connected');
      else { ui.profiles=await api('profiles');el('connection-dialog').close();await selectConnection(p.id,0); }
    }catch(error){el('connection-status').textContent=error.message;el('connection-status').classList.add('error');status(error.message,true);}
    finally{ui.busy=false;controls();el('test').disabled=false;el('save-connection').disabled=false;el('close-dialog').disabled=false;}
  }
  el('connection-dialog').addEventListener('cancel',event=>{if(ui.busy)event.preventDefault();});
  el('connection-form').onsubmit=event=>{event.preventDefault();submitConnection(false);};
  el('test').onclick=()=>submitConnection(true);
  el('close-dialog').onclick=()=>el('connection-dialog').close();
  function paintManager() {
    const list=el('manager-list');list.replaceChildren();
    if(!ui.profiles.length){const p=document.createElement('p');p.textContent=tr('noConnections');list.append(p);}
    for(const p of ui.profiles){
      const row=document.createElement('div');row.className='redis-manager-row';row.draggable=true;row.dataset.redisProfile=p.id;
      const handle=document.createElement('span');handle.className='connection-order-handle';handle.title=t('commands.dragToReorder');handle.setAttribute('aria-hidden','true');handle.textContent='⁙';row.append(handle);
      const info=document.createElement('div'),name=document.createElement('strong'),address=document.createElement('small');name.textContent=p.name;address.textContent=`${p.host}:${p.port}${p.tls?' · TLS':''}`;info.append(name,address);row.append(info);
      for(const [action,label] of [['connect','connect'],['edit','edit'],['remove','remove']]){
        const button=document.createElement('button');button.type='button';button.className='text-button';button.textContent=tr(label);button.dataset.redisManage=action;button.dataset.id=p.id;
        button.onclick=()=>{
          if(ui.busy)return;
          if(action==='edit'){el('manager-dialog').close();connectionDialog(p.id);}
          else if(action==='connect'){if(!guard())return;el('manager-dialog').close();run(()=>selectConnection(p.id,0));}
          else if((p.id!==ui.id||guard())&&confirm(tr('confirmRemove')))run(async()=>{await api('profiles',{method:'DELETE',params:{id:p.id}});ui.profiles=await api('profiles');if(ui.id===p.id)await selectConnection('',0);else paintProfiles();paintManager();status(tr('saved'));});
        };row.append(button);
      }
      list.append(row);
    }
  }
  bindConnectionReorder(el('manager-list'), '.redis-manager-row', row => row.dataset.redisProfile, (from,to,after)=>run(async()=>{
    const previous=[...ui.profiles];
    if(!moveConnectionItem(ui.profiles,from,to,after))return;
    paintProfiles();paintManager();
    try{await api('profiles',{method:'PUT',body:{order:ui.profiles.map(profile=>profile.id)}});status(tr('saved'));}
    catch(error){ui.profiles=previous;paintProfiles();paintManager();throw error;}
  }),()=>ui.busy);
  el('close-manager').onclick=()=>el('manager-dialog').close();
  el('add').onclick=async()=>{setView('redis');await ui.ready;connectionDialog();};
  el('manage').onclick=async()=>{setView('redis');await ui.ready;run(async()=>{ui.profiles=await api('profiles');if(!ui.profiles.some(profile=>profile.id===ui.id)){if(!guard())return;await selectConnection('',0);}else paintProfiles();paintManager();el('manager-dialog').showModal();status('');});};
  el('profile').onchange=()=>{const id=el('profile').value;if(!guard()){el('profile').value=ui.id;return;}run(()=>selectConnection(id,0));};
  el('search-form').onsubmit=event=>{event.preventDefault();if(!ui.id||ui.busy)return;ui.pattern=el('pattern').value||'*';run(()=>scan(true));};
  el('refresh').onclick=()=>run(async()=>{ui.profiles=await api('profiles');if(!ui.profiles.some(profile=>profile.id===ui.id)){if(!guard())return;await selectConnection('',0);return;}paintProfiles();if(ui.id){await scan(true);}else{paintTree();status('');}});
  el('more').onclick=()=>run(()=>scan());
  const newKey={editor:null,generation:0,initial:''};
  const newKeyDefaults={string:'',hash:'{\n  "field": "value"\n}',list:'["value"]',set:'["value"]',zset:'[{"Score": 1, "Member": "value"}]'};
  function newKeySnapshot(){return JSON.stringify([el('new-key-name').value,el('new-key-type').value,el('new-key-ttl').value,newKey.editor?.value()||'']);}
  function mountNewKey(value){
    newKey.editor?.destroy();el('new-key-editor').replaceChildren();
    newKey.editor=DatabaseUI.createRemoteTextEditor(el('new-key-editor'),{path:el('new-key-format').value==='json'?'value.json':'value.txt',text:value,readOnly:false,save:()=>saveNewKey(),change:()=>{}});
    el('new-key-hint').textContent=tr(el('new-key-type').value==='string'?'stringHint':'collectionHint');
  }
  function openNewKey(prefix=''){
    if(ui.busy||!ui.id)return;newKey.generation++;el('new-key-form').reset();el('new-key-name').value=prefix;el('new-key-type').value='string';el('new-key-format').value='text';el('new-key-ttl').value='-1';
    el('new-key-title').textContent=tr('newTitle',{source:`${ui.profiles.find(p=>p.id===ui.id)?.name} / db${ui.db}`});el('new-key-status').textContent='';el('new-key-status').classList.remove('error');
    el('new-key-dialog').showModal();mountNewKey('');newKey.initial=newKeySnapshot();controls();el('new-key-name').focus();el('new-key-name').setSelectionRange(prefix.length,prefix.length);
  }
  function closeNewKey(force=false){if(ui.busy||(!force&&newKeySnapshot()!==newKey.initial&&!confirm(tr('discard'))))return;newKey.generation++;el('new-key-dialog').close();newKey.editor?.destroy();newKey.editor=null;}
  async function saveNewKey(){
    if(ui.busy||!el('new-key-form').reportValidity())return;
    await run(async()=>{try{
      const key=el('new-key-name').value,type=el('new-key-type').value,raw=newKey.editor.value(),ttl=Number(el('new-key-ttl').value);
      if(!key)throw new Error(tr('needKey'));if(!Number.isInteger(ttl)||ttl===0||ttl< -1||ttl>315360000)throw new Error(tr('badTTL'));
      const value=type==='string'?raw:JSON.parse(raw);if(type==='string'&&el('new-key-format').value==='json')JSON.parse(raw);
      await api('write',{body:{key,type,value,ttl,create:true,operation:'save'}});
      newKey.initial=newKeySnapshot();newKey.generation++;el('new-key-dialog').close();newKey.editor.destroy();newKey.editor=null;ui.keys.add(key);
      // A new-key dialog must not discard an unrelated pending editor draft.
      if(!ui.dirty){selectNamespace('');await loadValue(key);}else paintTree();status(tr('saved'));
    }catch(error){el('new-key-status').textContent=error.message;el('new-key-status').classList.add('error');throw error;}});
  }
  el('new-key').onclick=()=>openNewKey();
  el('new-key-form').onsubmit=event=>{event.preventDefault();saveNewKey();};
  el('new-key-close').onclick=()=>closeNewKey();el('new-key-cancel').onclick=()=>closeNewKey();
  el('new-key-dialog').addEventListener('cancel',event=>{event.preventDefault();closeNewKey();});
  el('new-key-type').onchange=()=>{const type=el('new-key-type').value;el('new-key-format').value=type==='string'?'text':'json';mountNewKey(newKeyDefaults[type]);controls();};
  el('new-key-format').onchange=()=>mountNewKey(newKey.editor.value());
  el('new-key-pretty').onclick=()=>{try{mountNewKey(JSON.stringify(JSON.parse(newKey.editor.value()),null,2));}catch(error){el('new-key-status').textContent=error.message;el('new-key-status').classList.add('error');}};
  el('new-key-file').onchange=async()=>{
    const file=el('new-key-file').files?.[0],generation=newKey.generation,previous=newKey.editor?.value();if(!file)return;
    try{if(file.size>2*1024*1024)throw new Error(tr('fileTooLarge'));const value=new TextDecoder('utf-8',{fatal:true}).decode(await file.arrayBuffer());if(generation!==newKey.generation||!el('new-key-dialog').open||newKey.editor?.value()!==previous)return;mountNewKey(value);el('new-key-status').textContent='';}catch(error){if(generation===newKey.generation){el('new-key-status').textContent=error.message;el('new-key-status').classList.add('error');}}
  };
  el('type').onchange=()=>{if(!guard()){el('type').value=ui.type;return;}const type=el('type').value;if(type==='stream'){el('type').value=ui.type;status(tr('unsupported'),true);return;}const values={string:'',hash:{field:'value'},list:['value'],set:['value'],zset:[{Score:1,Member:'value'}]};paintValue({type,value:values[type],truncated:false},true,el('key').value);};
  el('ttl').oninput=()=>{ui.dirty=true;controls();};el('key').oninput=()=>{ui.dirty=true;controls();};
  el('save').onclick=()=>save();
  el('reload').onclick=()=>{if(guard())run(()=>loadValue(ui.key));};
  el('delete').onclick=()=>deleteKey(ui.key);
  el('expire').onclick=()=>{if(ui.editor.value()!==ui.original&&!confirm(tr('discard')))return;run(async()=>{await api('write',{body:{key:ui.key,version:ui.value.version,ttl:ttl(),operation:'ttl'}});await loadValue(ui.key);status(tr('saved'));});};
  el('format').onchange=()=>{const value=ui.editor.value();mountEditor(value,ui.value?.truncated);};
  el('pretty').onclick=()=>{try {const value=JSON.stringify(JSON.parse(ui.editor.value()),null,2);mountEditor(value,ui.value?.truncated);ui.dirty=value!==ui.original;controls();}catch(error){status(error.message,true);}};
  window.addEventListener('beforeunload',event=>{if(ui.dirty||(el('new-key-dialog').open&&newKeySnapshot()!==newKey.initial)){event.preventDefault();event.returnValue='';}});
  window.renderRedisView=()=>{
    if(!ui.initialized) {
      ui.initialized=true;ui.ready=run(async()=>{ui.profiles=await api('profiles');await selectConnection(ui.profiles[0]?.id||'',0);});
    }else{paintProfiles();paintTree();paintMetadata();ui.editor?.resize();}
  };
})();
