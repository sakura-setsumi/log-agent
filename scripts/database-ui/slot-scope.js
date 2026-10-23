import { recognizeStatements } from './statements.js';

let sequence = 0;
const session = Date.now().toString(36) + '-' + Math.random().toString(36).slice(2);
export const uniqueSlotGroup = prefix => `${prefix}-${session}-${++sequence}`;

// Old saved slots can reuse both group IDs and slot IDs in different statements.
// The clicked position and the adjacent predicate define the editing scope.
export function slotScope(source, slots, selected, engine) {
  const active = slots.find(slot => slot.id === selected);
  if (!active) return { own: [], related: [], statementSlots: [] };
  const statement = recognizeStatements(source, engine, { includeIncomplete: true })
    .find(range => range.from <= active.from && active.to <= range.to);
  // Fail closed for SQL whose statement boundaries cannot be recognized.
  const statementSlots = statement ? slots.filter(slot => slot.from >= statement.from && slot.to <= statement.to) : [active];
  let own = statementSlots.filter(slot => slot.template?.id === active.template?.id);
  const whitespace = (from, to) => from <= to && /^\s*$/.test(source.slice(from, to));
  const operator = active.kind === 'operator' ? active
    : active.kind === 'field' ? own.find(slot => slot.kind === 'operator' && whitespace(active.to, slot.from))
    : active.kind === 'filterValue' ? own.find(slot => slot.kind === 'operator' && whitespace(slot.to, active.from)) : null;
  if (operator) {
    const field = own.find(slot => slot.kind === 'field' && whitespace(slot.to, operator.from));
    const value = own.find(slot => slot.kind === 'filterValue' && whitespace(operator.to, slot.from));
    own = [field, operator, value].filter(Boolean);
  } else if (active.kind === 'field' && active.template?.templateId !== 'group') {
    // A sort/group/SET field must not also rewrite a WHERE field in the root group.
    own = [active];
  }
  const root = active.template?.statementId || active.template?.id;
  const table = !own.some(slot => slot.kind === 'table')
    ? statementSlots.find(slot => slot.kind === 'table' && slot.template?.id === root) : null;
  return { own, related: table ? [...own, table] : own, statementSlots };
}
