import { MySQL, PostgreSQL, SQLite, SQLDialect } from '@codemirror/lang-sql';

const mysql = SQLDialect.define({ ...MySQL.spec, backslashEscapes: true });
const dialects = { mysql, postgres: PostgreSQL, sqlite: SQLite };
export const sqlDialect = engine => dialects[engine] || mysql;
function closedComment(value) {
  let depth = 0;
  for (const match of value.matchAll(/\/\*|\*\//g)) depth += match[0] === '/*' ? 1 : -1;
  return depth === 0;
}

function closedLiteral(value, dialect, identifier = false) {
  const dollar = /^\$[\w]*\$/.exec(value);
  if (dollar) return value.length >= dollar[0].length * 2 && value.endsWith(dollar[0]);
  const start = value.search(/['"`\[]/);
  if (start < 0) return true;
  const end = value[start] === '[' ? ']' : value[start];
  if (value.length <= start + 1 || !value.endsWith(end)) return false;
  if (!identifier && (dialect.spec.backslashEscapes || /^[eE]'/.test(value))) {
    let slashes = 0;
    for (let i = value.length - 2; i > start && value[i] === '\\'; i--) slashes++;
    if (slashes % 2) return false;
  }
  return true;
}

// Use the editor's SQL dialect parser so semicolons inside literals/comments
// and nested queries do not split a statement. SQL execution remains the
// database's responsibility; both queries and modifying statements get actions.
export function recognizeStatements(source, engine = 'mysql', { includeIncomplete = false } = {}) {
  const dialect = sqlDialect(engine), tree = dialect.language.parser.parse(source), result = [];
  for (let node = tree.topNode.firstChild; node; node = node.nextSibling) {
    if (node.name !== 'Statement') continue;
    const next = node.nextSibling;
    if (!source.slice(node.from, node.to).trimEnd().endsWith(';') && next?.name === 'BlockComment' && !closedComment(source.slice(next.from, next.to))) continue;
    const tokens = [];
    for (let child = node.firstChild; child; child = child.nextSibling) {
      if (!/Comment$/.test(child.name) && child.name !== ';') tokens.push(child);
    }
    if (!tokens.length) continue;
    let invalid = false;
    node.toTree().iterate({ enter(token) {
      const value = source.slice(node.from + token.from, node.from + token.to);
      // Lezer emits a zero-width recovery node for the valid projection alias.*.
      const qualifiedStar = token.type.isError && token.from === token.to && token.node.parent?.name === 'CompositeIdentifier' && /^\s*\*/.test(source.slice(node.from + token.from));
      if (token.type.isError && !qualifiedStar || (['String', 'QuotedIdentifier'].includes(token.name) && !closedLiteral(value, dialect, token.name === 'QuotedIdentifier'))) invalid = true;
      if (token.name === 'BlockComment' && !closedComment(value)) invalid = true;
    } });
    if (invalid) continue;
    const from = tokens[0].from, to = node.to, sql = source.slice(from, to).trim();
    if (!includeIncomplete && /\b(?:SELECT|FROM|WHERE|JOIN|ON|HAVING|BY|LIMIT|OFFSET|AS|UNION(?:\s+ALL)?|AND|OR|WITH|DISTINCT|UPDATE|SET|INSERT|INTO|DELETE|CREATE|ALTER|DROP|TABLE|INDEX)\s*;?$/i.test(source.slice(tokens.at(-1).from, tokens.at(-1).to))) continue;
    result.push({ from, to, sql });
  }
  return result;
}

export function queryAtCursor(statements, position, source) {
  return statements.find(statement => statement.from <= position && position < statement.to)
    || statements.find(statement => statement.to === position)
    || (source !== undefined && [...statements].reverse().find(statement => statement.to < position && !source.slice(statement.to, position).trim()))
    || (source !== undefined && statements.find(statement => statement.from > position && !source.slice(position, statement.from).trim()))
    || null;
}
