import { splitSQLList } from './template-input.js';
import { recognizeStatements } from './statements.js';
import { tokenize, recognizeStatementSlots } from './slot-recognizer.js';

export function topTokens(sql, base = 0) {
  let depth = 0;
  return tokenize(sql, base).filter(token => {
    if (token.text === ')') depth--;
    const top = depth === 0;
    if (token.text === '(') depth++;
    return top;
  });
}
const word = (token, value) => token?.type === 'word' && token.upper === value;

// Only a real SQL keyword at the caret triggers: never literals, comments,
// identifiers such as order_id, nested queries, or other statements.
export function incrementalAt(source, head, engine) {
  const statement = recognizeStatements(source, engine, { includeIncomplete: true }).find(s => s.from < head && (head <= s.to || /^\s*$/.test(source.slice(s.to, head))));
  if (!statement) return null;
  const tokens = topTokens(statement.sql, statement.from);
  let index = tokens.findIndex(token => token.to <= head && /^\s*$/.test(source.slice(token.to, head)));
  if (index < 0) return null;
  let token = tokens[index], end = token.to;
  if (word(token, 'BY') && ['ORDER', 'GROUP'].includes(tokens[index - 1]?.upper)) token = tokens[--index];
  if (word(token, 'JOIN') || word(token, 'OUTER')) {
    let previous = index - 1;
    if (word(tokens[previous], 'OUTER')) previous--;
    if (['INNER','LEFT','RIGHT'].includes(tokens[previous]?.upper)) token = tokens[previous];
  }
  if (token.type !== 'word' || !['WHERE', 'ORDER', 'COUNT', 'DISTINCT', 'GROUP', 'INNER', 'LEFT', 'RIGHT'].includes(token.upper)) return null;
  const verb = tokens[0]?.upper, kind = token.upper.toLowerCase();
  if (kind === 'where' ? !['SELECT','UPDATE','DELETE'].includes(verb) : verb !== 'SELECT') return null;
  const table = recognizeStatementSlots(statement.sql, statement.from).find(slot => slot.kind === 'table');
  if (!table || ['inner','left','right'].includes(kind) && token.from <= table.to) return null;
  return {kind, from: token.from, to: end, statement, table};
}

// Remove the typed keyword, then insert the selected fragment in SQL clause
// order. All SQL outside this one statement is left to the caller unchanged.
export function applyIncrement(source, pending, values) {
  const {statement, kind} = pending;
  let sql = source.slice(statement.from, pending.from) + source.slice(pending.to, statement.to);
  let tokens = topTokens(sql);
  const from = tokens.find(token => word(token, 'FROM'));
  if (['inner','left','right'].includes(kind)) {
    if (!values.table || !values.alias || !values.conditions?.length) return null;
    const stop = joinBoundary(sql);
    const on = values.conditions.map(pair => `${pair.left} = ${pair.right}`).join(' AND ');
    sql = sql.slice(0,stop).replace(/[ \t]+$/, '') + ` ${kind.toUpperCase()} JOIN ${values.table} AS ${values.alias} ON ${on} ` + sql.slice(stop);
  } else if (kind === 'count' || kind === 'distinct') {
    if (!from || !word(tokens[0], 'SELECT')) return null;
    const modifier = word(tokens[1], 'DISTINCT') ? tokens[1] : null;
    const start = (modifier || tokens[0]).to;
    const old = sql.slice(start, from.from).trim().replace(/,\s*$/, '');
    const expression = kind === 'count' ? `COUNT(${values.field || '*'}) AS ${values.alias}` : values.columns;
    const projection = kind === 'count' && old && old !== '*' ? old + ', ' + expression : expression;
    sql = sql.slice(0, tokens[0].to) + (modifier || kind === 'distinct' ? ' DISTINCT' : '') + ' ' + projection + ' ' + sql.slice(from.from);
  } else {
    const order = ['WHERE','GROUP','HAVING','ORDER','LIMIT','OFFSET','RETURNING'];
    const keyword = kind.toUpperCase(), rank = order.indexOf(keyword);
    const clauses = tokens.filter(token => token.type === 'word' && order.includes(token.upper));
    const existing = clauses.find(token => token.upper === keyword);
    const later = clauses.find(token => order.indexOf(token.upper) > rank && (!existing || token.from > existing.from));
    const stop = later?.from ?? tokens.findLast(token => token.text === ';')?.from ?? sql.length;
    const expression = kind === 'where' ? values.field + ' IS NOT NULL' : kind === 'order' ? splitSQLList(values.columns).map(column => column + ' ' + (values.direction || 'ASC')).join(', ') : values.columns;
    if (existing) {
      const body = sql.slice(existing.to, stop).trim();
      sql = sql.slice(0, stop).replace(/[ \t]+$/, '') + (body && body !== 'BY' ? kind === 'where' ? ' AND ' : ', ' : ' ') + expression + ' ' + sql.slice(stop);
    } else {
      sql = sql.slice(0, stop).replace(/[ \t]+$/, '') + ' ' + keyword + (['order','group'].includes(kind) ? ' BY' : '') + ' ' + expression + ' ' + sql.slice(stop);
    }
  }
  return sql.replace(/\s+;$/, ';').trimEnd();
}

const tableStops = new Set(['WHERE','INNER','LEFT','RIGHT','FULL','OUTER','CROSS','NATURAL','JOIN','ON','USING','GROUP','ORDER','HAVING','LIMIT','OFFSET','UNION','EXCEPT','INTERSECT','FOR','RETURNING']);
const isName = token => token && (token.type === 'ident' || token.type === 'word' && !tableStops.has(token.upper));

// Keep each table occurrence, including repeated tables with different aliases.
export function statementTables(sql) {
  const tokens = topTokens(sql), result = [];
  for (let i=0; i<tokens.length; i++) {
    if (!word(tokens[i],'FROM') && !word(tokens[i],'JOIN')) continue;
    let next=i+1;
    if (!isName(tokens[next])) continue;
    const start=tokens[next].from;
    let end=tokens[next++].to;
    while (tokens[next]?.text==='.' && isName(tokens[next+1])) {end=tokens[next+1].to;next+=2;}
    const table=sql.slice(start,end);
    if(word(tokens[next],'AS')) next++;
    const alias=isName(tokens[next]) ? tokens[next].text : null;
    result.push({table,alias,qualifier:alias || table});
  }
  return result;
}

export function joinBoundary(sql) {
  return topTokens(sql).find(token => token.type==='word' && ['WHERE','GROUP','HAVING','ORDER','LIMIT','OFFSET','UNION','EXCEPT','INTERSECT','FOR','RETURNING'].includes(token.upper))?.from
    ?? topTokens(sql).findLast(token=>token.text===';')?.from ?? sql.length;
}
