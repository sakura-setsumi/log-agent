import { EditorView, basicSetup } from 'codemirror';
import { EditorState, Compartment, Prec } from '@codemirror/state';
import { keymap } from '@codemirror/view';
import { StreamLanguage } from '@codemirror/language';
import { openSearchPanel } from '@codemirror/search';
import { oneDark } from '@codemirror/theme-one-dark';
import { sql } from '@codemirror/lang-sql';
import { javascript, typescript, json } from '@codemirror/legacy-modes/mode/javascript';
import { shell } from '@codemirror/legacy-modes/mode/shell';
import { python } from '@codemirror/legacy-modes/mode/python';
import { java, c, cpp } from '@codemirror/legacy-modes/mode/clike';
import { go } from '@codemirror/legacy-modes/mode/go';
import { css } from '@codemirror/legacy-modes/mode/css';
import { xml, html } from '@codemirror/legacy-modes/mode/xml';
import { yaml } from '@codemirror/legacy-modes/mode/yaml';
import { properties } from '@codemirror/legacy-modes/mode/properties';
import { nginx } from '@codemirror/legacy-modes/mode/nginx';
import { powerShell } from '@codemirror/legacy-modes/mode/powershell';

const log = {
  token(stream) {
    if (stream.eatSpace()) return null;
    if (stream.match(/(?:\d{4}[-/]\d{2}[-/]\d{2}[ T])?\d{2}:\d{2}:\d{2}(?:[.,]\d+)?/)) return 'number';
    if (stream.match(/(?:ERROR|FATAL|SEVERE|Exception|Caused by)\b/)) return 'invalid';
    if (stream.match(/WARN(?:ING)?\b/)) return 'atom';
    if (stream.match(/(?:INFO|DEBUG|TRACE)\b/)) return 'keyword';
    if (stream.match(/"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'/)) return 'string';
    if (stream.match(/\[[^\]]*\]/)) return 'meta';
    if (stream.match(/\b\d+(?:\.\d+)?\b/)) return 'number';
    if (stream.match(/[\w$]+(?:\.[\w$]+)+/)) return 'def';
    if (stream.match(/\b(?:at|true|false|null)\b/)) return 'keyword';
    stream.next(); return null;
  },
};
function languageFor(path) {
  const name = path.split('/').pop().toLowerCase(), ext = name.split('.').pop();
  if (ext === 'sql') return { name: 'SQL', extension: sql() };
  const modes = { js: javascript, mjs: javascript, cjs: javascript, jsx: javascript, ts: typescript, tsx: typescript, json, sh: shell, bash: shell, zsh: shell, py: python, java, c, h: c, cpp, cc: cpp, hpp: cpp, go, css, xml, html, htm: html, vue: html, yaml, yml: yaml, ini: properties, properties, env: properties, toml: properties, ps1: powerShell, log, out: log, err: log, txt: log };
  const labels = new Map([[javascript, 'JavaScript'], [typescript, 'TypeScript'], [json, 'JSON'], [shell, 'Shell'], [python, 'Python'], [java, 'Java'], [go, 'Go'], [css, 'CSS'], [xml, 'XML'], [html, 'HTML'], [yaml, 'YAML'], [properties, 'Config'], [powerShell, 'PowerShell'], [log, 'Log / Text'], [c, 'C'], [cpp, 'C++'], [nginx, 'Nginx']]);
  const mode = name === 'dockerfile' || name.endsWith('rc') ? shell : name.includes('nginx') ? nginx : ext === 'conf' || ext === 'cfg' ? properties : modes[ext] || log;
  return { name: labels.get(mode), extension: StreamLanguage.define(mode) };
}
export function createRemoteTextEditor(parent, options) {
  const theme = new Compartment(), wrapping = new Compartment(), language = languageFor(options.path);
  const newline = options.text.includes('\r\n') ? '\r\n' : '\n';
  const editor = new EditorView({ parent, doc: options.text, extensions: [basicSetup, language.extension,
    theme.of(document.documentElement.dataset.theme === 'dark' ? oneDark : []), wrapping.of(EditorView.lineWrapping),
    EditorState.readOnly.of(options.readOnly), EditorView.editable.of(!options.readOnly),
    Prec.highest(keymap.of([{ key: 'Mod-s', run: () => { options.save(); return true; } }])),
    EditorView.updateListener.of(update => { if (update.docChanged || update.selectionSet) options.change(); }),
    EditorView.theme({ '&': { height: '100%', fontSize: '13px' }, '.cm-scroller': { fontFamily: 'Consolas, "Cascadia Mono", monospace', overflow: 'auto' }, '.cm-content': { minHeight: '100%' }, '.cm-gutters': { minHeight: '100%' } }),
  ] });
  const observer = new MutationObserver(() => editor.dispatch({ effects: theme.reconfigure(document.documentElement.dataset.theme === 'dark' ? oneDark : []) }));
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
  return {
    value: () => editor.state.doc.toString().replace(/\n/g, newline),
    language: language.name,
    position: () => { const at = editor.state.selection.main.head, line = editor.state.doc.lineAt(at); return { line: line.number, column: at - line.from + 1, lines: editor.state.doc.lines }; },
    search: () => { openSearchPanel(editor); editor.focus(); },
    wrap: enabled => editor.dispatch({ effects: wrapping.reconfigure(enabled ? EditorView.lineWrapping : []) }),
    focus: () => editor.focus(), resize: () => editor.requestMeasure(),
    destroy: () => { observer.disconnect(); editor.destroy(); },
  };
}