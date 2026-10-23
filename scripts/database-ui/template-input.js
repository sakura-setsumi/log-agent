// Text helpers for template pickers. Numeric values stay strings to preserve precision.
export function identifierParts(raw) {
  const parts=[]; let i=0;
  while (i<raw.length) {
    while (/\s/.test(raw[i] || '') && i<raw.length) i++;
    let value='', ch=raw[i];
    if (['`','"','['].includes(ch)) {
      const end=ch==='['?']':ch; i++; let closed=false;
      while(i<raw.length) { if(raw[i]===end) {if(raw[i+1]===end){value+=end;i+=2;}else{i++;closed=true;break;}}else value+=raw[i++]; }
      if(!closed)return [];
    } else {const match=/^[\p{L}_][\p{L}\p{N}_$]*/u.exec(raw.slice(i));if(!match)return [];value=match[0];i+=value.length;}
    parts.push(value);while(i<raw.length && /\s/.test(raw[i]))i++;
    if(i===raw.length)break;if(raw[i++]!=='.')return [];
  }
  return parts;
}
export function splitSQLList(raw,engine='mysql') {
  const pieces=[]; let start=0,depth=0,quote='';
  for(let i=0;i<raw.length;i++) {
    const ch=raw[i];
    if(quote) {if(ch==='\\' && engine==='mysql'){i++;continue;}if(ch===quote){if(raw[i+1]===quote)i++;else quote='';}continue;}
    if(ch==="'"||ch==='"'||ch==='`'){quote=ch;continue;}
    if(ch==='(')depth++;else if(ch===')'){if(--depth<0)throw new Error('invalid values');}
    else if(ch===',' && depth===0){pieces.push(raw.slice(start,i).trim());start=i+1;}
  }
  if(quote||depth)throw new Error('invalid values');pieces.push(raw.slice(start).trim());return pieces;
}
export function insertRows(raw,engine) {
  return splitSQLList('('+raw+')',engine).map(tuple=>{
    if(!tuple.startsWith('(')||!tuple.endsWith(')'))throw new Error('invalid values');
    return splitSQLList(tuple.slice(1,-1),engine);
  });
}
export function bulkRows(raw) {
  let quoted=false, delimiter=',';
  for(let i=0;i<raw.length;i++){if(raw[i]==='"'){if(quoted&&raw[i+1]==='"')i++;else quoted=!quoted;}else if(!quoted&&raw[i]==='\t'){delimiter='\t';break;}else if(!quoted&&/[\r\n]/.test(raw[i]))break;}
  const rows=[];let row=[],value='',inQuote=false,wasQuoted=false;
  const field=()=>{row.push(!wasQuoted&&/^NULL$/i.test(value.trim())?null:value);value='';wasQuoted=false;};
  for(let i=0;i<raw.length;i++) {
    const ch=raw[i];
    if(ch==='"'&&(inQuote||!value)){if(inQuote&&raw[i+1]==='"'){value+='"';i++;}else{inQuote=!inQuote;wasQuoted=true;}}
    else if(!inQuote&&(ch===delimiter||ch==='\n'||ch==='\r')){field();if(ch!==delimiter){if(ch==='\r'&&raw[i+1]==='\n')i++;rows.push(row);row=[];}}
    else value+=ch;
  }
  if(inQuote)throw new Error('invalid values');if(value||wasQuoted||row.length){field();rows.push(row);}return rows;
}
export function templateLiteral(value,engine,type='') {
  if(value===null)return 'NULL';value=String(value);
  if(/^(?:(?:tiny|small|medium|big)?int(?:eger|\d+)?|serial\d*|decimal|numeric|number|float\d*|double|real|money)\b/i.test(type)&&/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(value.trim()))return value.trim();
  if(/^(bool|boolean)\b/i.test(type)&&/^(true|false)$/i.test(value.trim()))return value.trim().toUpperCase();
  if(engine==='mysql')value=value.replaceAll('\\','\\\\');return "'"+value.replaceAll("'","''")+"'";
}
