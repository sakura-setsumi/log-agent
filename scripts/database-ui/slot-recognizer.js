import { recognizeStatements } from './statements.js';

// Finds the table / column / field / operator / value positions of simple
// statements in plain SQL, so text that lost its template slots (typed by hand,
// or saved before slots were persisted) gets the same pickers again.

const TOKEN = /\s+|--[^\n]*|\/\*[\s\S]*?(?:\*\/|$)|`(?:[^`]|``)*`|"(?:[^"]|"")*"|\[[^\]\n]*\]|'(?:[^']|'')*(?:'|$)|[A-Za-z_][\w$]*|\d+(?:\.\d+)?|<>|!=|>=|<=|[\s\S]/y;
const COMPARISON = new Set(['=', '<>', '!=', '<', '>', '<=', '>=']);
const CLAUSE_END = new Set(['AND', 'OR', 'ORDER', 'GROUP', 'LIMIT', 'HAVING', 'OFFSET']);

export function tokenize(sql, base = 0) {
  const tokens = []; TOKEN.lastIndex = 0;
  for (let match; TOKEN.lastIndex < sql.length && (match = TOKEN.exec(sql));) {
    const text = match[0];
    if (/^\s/.test(text) || text.startsWith('--') || text.startsWith('/*')) continue;
    const quoted = /^[`"\[]/.test(text), string = text.startsWith("'"), word = /^[A-Za-z_]/.test(text);
    tokens.push({ text, upper: word ? text.toUpperCase() : text, type: quoted ? 'ident' : string ? 'string' : word ? 'word' : /^\d/.test(text) ? 'number' : 'punct', from: base + match.index, to: base + match.index + text.length });
  }
  return tokens;
}

export function recognizeStatementSlots(sql, base = 0) {
  const tokens = tokenize(sql, base), slots = [];
  let i = 0;
  const peek = (offset = 0) => tokens[i + offset];
  const isWord = (token, ...words) => token?.type === 'word' && words.includes(token.upper);
  const add = (kind, from, to) => { if (to > from) slots.push({ kind, from, to }); };
  const isName = token => token && (token.type === 'ident' || (token.type === 'word' && !CLAUSE_END.has(token.upper)));
  // schema.name, quoted or not.
  function reference() {
    if (!isName(peek())) return null;
    const first = peek(); let last = first; i++;
    while (peek()?.text === '.' && isName(peek(1))) { last = peek(1); i += 2; }
    return { from: first.from, to: last.to };
  }
  function skipParens() {
    let depth = 0;
    do { if (peek()?.text === '(') depth++; else if (peek()?.text === ')') depth--; i++; } while (i < tokens.length && depth > 0);
  }
  function predicates() {
    for (;;) {
      const field = reference(); if (!field) return;
      add('field', field.from, field.to);
      const start = peek(); if (!start) return;
      let operatorEnd = null, operandless = false, between = false;
      if (start.type === 'punct' || start.type === 'word') {
        if (COMPARISON.has(start.text)) { operatorEnd = start.to; i++; }
        else if (isWord(start, 'LIKE', 'IN', 'BETWEEN', 'REGEXP', 'RLIKE')) { operatorEnd = start.to; between = start.upper === 'BETWEEN'; i++; }
        else if (isWord(start, 'NOT') && isWord(peek(1), 'LIKE', 'IN', 'BETWEEN', 'REGEXP')) { operatorEnd = peek(1).to; between = peek(1).upper === 'BETWEEN'; i += 2; }
        else if (isWord(start, 'IS')) {
          let j = 1; if (isWord(peek(j), 'NOT')) j++;
          if (!isWord(peek(j), 'NULL')) return;
          operatorEnd = peek(j).to; i += j + 1; operandless = true;
        }
      }
      if (operatorEnd === null) return;
      add('operator', start.from, operatorEnd);
      if (!operandless) {
        const operandStart = peek(); if (!operandStart) return;
        let end = operandStart.from;
        while (peek() && peek().text !== ';') {
          const token = peek();
          if (token.text === '(') { skipParens(); end = tokens[i - 1].to; continue; }
          if (token.type === 'word' && CLAUSE_END.has(token.upper)) { if (token.upper === 'AND' && between) between = false; else break; }
          end = token.to; i++;
        }
        add('filterValue', operandStart.from, end);
      }
      if (!isWord(peek(), 'AND', 'OR')) return;
      const logical = peek(); i++;
      if (!peek() || isWord(peek(), ...CLAUSE_END) || [';', ')'].includes(peek().text)) {
        slots.push({ kind: 'condition', from: logical.from, to: logical.to, logical: logical.upper });
        return;
      }
    }
  }

  const verb = peek()?.upper;
  if (verb === 'SELECT') {
    i++; if (isWord(peek(), 'DISTINCT')) i++;
    const columnsFrom = peek()?.from; let columnsTo = columnsFrom, plain = true;
    while (peek() && !isWord(peek(), 'FROM')) { if (peek().text === '(') { plain = false; skipParens(); columnsTo = tokens[i - 1].to; continue; } columnsTo = peek().to; i++; }
    if (!peek()) return [];
    if (plain) add('columns', columnsFrom, columnsTo);
    for (let j = 1; j < i; j++) {
      if (!isWord(tokens[j], 'COUNT') || tokens[j + 1]?.text !== '(') continue;
      let end = j + 2, depth = 1;
      while (end < i && depth) { if (tokens[end].text === '(') depth++; if (tokens[end].text === ')') depth--; if (depth) end++; }
      if (depth || end === j + 2) continue;
      const aggregate = tokens[j].from;
      slots.push({kind: 'countField', from: tokens[j + 2].from, to: tokens[end - 1].to, aggregate});
      if (isWord(tokens[end + 1], 'AS') && isName(tokens[end + 2])) slots.push({kind: 'alias', from: tokens[end + 2].from, to: tokens[end + 2].to, aggregate});
      j = end;
    }
    i++;
    const table = reference(); if (!table) return [];
    add('table', table.from, table.to);
    while (peek()) {
      if (isWord(peek(), 'WHERE')) { i++; predicates(); }
      else if (isWord(peek(), 'ORDER', 'GROUP') && isWord(peek(1), 'BY')) { i += 2; const field = reference(); if (field) add('field', field.from, field.to); }
      else i++;
    }
  } else if (verb === 'DELETE' && isWord(peek(1), 'FROM')) {
    i += 2; const table = reference(); if (!table) return [];
    add('table', table.from, table.to);
    if (isWord(peek(), 'WHERE')) { i++; predicates(); }
  } else if (verb === 'UPDATE') {
    i++; const table = reference(); if (!table) return [];
    add('table', table.from, table.to);
    if (isWord(peek(), 'SET')) {
      i++; const field = reference();
      if (field && peek()?.text === '=') {
        add('field', field.from, field.to); i++;
        const valueStart = peek(); let end = valueStart?.from;
        while (peek() && !isWord(peek(), 'WHERE') && peek().text !== ',' && peek().text !== ';') { if (peek().text === '(') { skipParens(); end = tokens[i - 1].to; continue; } end = peek().to; i++; }
        if (valueStart) add('value', valueStart.from, end);
      }
      while (peek() && !isWord(peek(), 'WHERE')) i++;
    }
    if (isWord(peek(), 'WHERE')) { i++; predicates(); }
  } else if (verb === 'INSERT' && isWord(peek(1), 'INTO')) {
    i += 2; const table = reference(); if (!table) return [];
    add('table', table.from, table.to);
    if (peek()?.text === '(') { const open = peek(); skipParens(); if (tokens[i - 1].from > open.to) add('columns', open.to, tokens[i - 1].from); }
    if (isWord(peek(), 'VALUES') && peek(1)?.text === '(') {
      i++; const open = peek(); skipParens();
      while (peek()?.text === ',' && peek(1)?.text === '(') { i++; skipParens(); }
      if (tokens[i - 1].from > open.to) add('values', open.to, tokens[i - 1].from);
    }
  } else return [];
  return slots;
}

function templateOf(sql, found) {
  const verb = (sql.match(/[A-Za-z]+/) || [''])[0].toLowerCase();
  return verb === 'select' ? (found.some(slot => slot.kind === 'operator') ? 'where' : 'select') : verb;
}

// Slots for every recognizable statement in `source`, ready for setSlots.
export function recognizeSlots(source, engine, labels = {}) {
  const result = [];
  recognizeStatements(source, engine, { includeIncomplete: true }).forEach((statement, index) => {
    const found = recognizeStatementSlots(statement.sql, statement.from);
    if (!found.some(slot => slot.kind === 'table')) return;
    const group = { id: 'template-restored-' + index, templateId: templateOf(statement.sql, found), context: null };
    found.forEach((slot, order) => {
      const template = slot.aggregate !== undefined ? { ...group, id: `${group.id}-count-${slot.aggregate}`, statementId: group.id, templateId: 'count' } : slot.logical ? { ...group, id: `${group.id}-logical-${order}`, statementId: group.id } : group;
      result.push({ ...slot, id: `${group.id}-${order}`, template, label: labels[slot.kind] || slot.kind });
    });
  });
  return result.sort((a, b) => a.from - b.from);
}
