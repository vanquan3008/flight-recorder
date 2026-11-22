const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { createRequire } = require('node:module');

const root = path.resolve(__dirname, '..');
process.chdir(root);
const frontendRequire = createRequire(path.join(root, 'frontend', 'package.json'));
const { parseAst } = frontendRequire('rolldown/parseAst');
const diagramPath = 'docs/diagrams.html';
const catalogStart = '<div id="source-catalog">';
const catalogEnd = '</div></section>';
const scriptExtension = /\.(?:[cm]?js|jsx|ts|tsx)$/i;
const escape = value => String(value).replace(/[&<>"']/g, character => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
})[character]);
const slug = value => value.replace(/[^a-z0-9]+/gi, '-').replace(/(^-|-$)/g, '');
const lineAt = (source, offset) => source.slice(0, offset).split('\n').length;
const short = (value, limit) => value.length > limit ? `${value.slice(0, limit)}…` : value;

function kindOf(file) {
  if (file.endsWith('.sql')) return 'sql';
  if (file.startsWith('backend/')) return 'backend';
  if (file.startsWith('frontend/')) return 'frontend';
  return 'operations';
}

function jsName(node) {
  if (!node) return '';
  if (node.type === 'Identifier' || node.type === 'JSXIdentifier') return node.name;
  if (node.type === 'Literal' && typeof node.value === 'string') return short(node.value, 100);
  if (['MemberExpression', 'JSXMemberExpression'].includes(node.type)) {
    return [jsName(node.object), jsName(node.property)].filter(Boolean).join('.');
  }
  return '';
}

const jsFunctions = new Set([
  'FunctionDeclaration', 'FunctionExpression', 'ArrowFunctionExpression', 'TSDeclareFunction',
  'TSMethodSignature', 'TSDeclareMethod', 'TSCallSignatureDeclaration',
  'TSConstructSignatureDeclaration', 'TSFunctionType', 'TSConstructorType',
]);

function jsSymbols(file, source) {
  const lang = file.endsWith('.d.ts') ? 'dts' : path.extname(file).slice(1).replace(/^[cm]js$/, 'js');
  const tree = parseAst(source, { lang }, file);
  const symbols = [];
  const sourceText = (start, end) => source.slice(start, end).trim();

  function visit(node, parent, current) {
    if (!node || typeof node.type !== 'string') return;
    if (jsFunctions.has(node.type)) {
      const line = lineAt(source, node.start);
      const assigned = parent.type === 'VariableDeclarator' ? parent.id : parent.key;
      const name = jsName(node.id || node.key || assigned);
      const caller = parent.type === 'CallExpression' ? jsName(parent.callee) : '';
      const testTitle = /^(?:test|it|describe)(?:\.|$)/.test(caller) ? jsName(parent.arguments[0]) : '';
      const start = parent.type === 'MethodDefinition' || (parent.type === 'Property' && parent.method) ? parent.start : node.start;
      const kind = node.type.startsWith('TS') ? 'Signature' : name ? 'Function' : 'Closure';
      current = {
        name: name || (testTitle ? `${caller} ${testTitle}` : `${caller || kind} @${line}`),
        kind, line, signature: sourceText(start, node.body?.start ?? node.end), calls: new Set(),
      };
      symbols.push(current);
    } else if (['TSInterfaceDeclaration', 'TSTypeAliasDeclaration', 'ClassDeclaration'].includes(node.type)) {
      symbols.push({ name: jsName(node.id), kind: 'Type', line: lineAt(source, node.start), signature: sourceText(node.start, node.end) });
    } else if (current && ['CallExpression', 'NewExpression', 'JSXOpeningElement'].includes(node.type)) {
      const name = jsName(node.callee || node.name);
      if (name) current.calls.add(name);
    }
    for (const value of Object.values(node)) {
      if (Array.isArray(value)) value.forEach(child => visit(child, node, current));
      else if (value && typeof value === 'object') visit(value, node, current);
    }
  }

  visit(tree, null, null);
  return symbols.sort((a, b) => a.line - b.line);
}

function otherSymbols(file, source) {
  const rules = file.endsWith('.sql') ? [
    ['Query', /^\s*-- name:\s*(\w+)[^\r\n]*/gm],
    ['Schema', /^(?:CREATE(?:\s+UNIQUE)?\s+(?:MATERIALIZED VIEW|TABLE|INDEX|EXTENSION|FUNCTION|TRIGGER|TYPE)|ALTER\s+TABLE|DROP\s+(?:TABLE|FUNCTION|VIEW))\s+(?:IF\s+(?:NOT\s+)?EXISTS\s+)?([\w.]+)/gim],
  ] : file.endsWith('.sh') ? [
    ['Function', /^([\w-]+)\s*\(\s*\)\s*\{/gm],
  ] : file.endsWith('.ps1') ? [
    ['Function', /^function\s+([\w-]+)[^\r\n]*/gim],
  ] : [];
  return rules.flatMap(([kind, pattern]) => Array.from(source.matchAll(pattern), match => ({
    name: match[1], kind, signature: match[0].trim(), line: lineAt(source, match.index + match[0].search(/\S/)),
  }))).sort((a, b) => a.line - b.line);
}

function oldNotes(catalog) {
  const notes = new Map();
  const cards = /<details class="source-file" id="([^"]+)"[^>]*><summary><code>([^<]+)<\/code>.*?<\/summary>(?:<p>([\s\S]*?)<\/p>)?/g;
  for (const [, id, file, description] of catalog.matchAll(cards)) notes.set(file, { id, description });
  return notes;
}

function renderFile(file, notes, statistics) {
  const previous = notes.get(escape(file));
  const id = previous?.id ?? `src-${file.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`;
  const kind = kindOf(file);
  const test = /(?:_test\.go$|\.(?:test|spec)\.[jt]sx?$)/.test(file) || file.startsWith('frontend/e2e/');
  const generated = file.startsWith('backend/internal/storage/postgres/sqlc/');
  const symbols = statistics.symbols;
  const sourceLink = `../${file.split('/').map(encodeURIComponent).join('/')}`;
  const uses = symbols.flatMap(symbol => [...(symbol.calls ?? [])]);
  const search = [file, previous?.description ?? '', symbols.map(symbol => symbol.name).join(' '), [...new Set(uses)].join(' ')].map(escape).join(' ');
  if (!test && !generated) statistics.visible++;
  const ids = new Set();
  const cards = symbols.map(symbol => {
    const base = `${id}-${symbol.kind === 'Type' ? 'type-' : ''}${slug(symbol.name)}`;
    const anchor = ids.has(base) ? `${base}-line-${symbol.line}-${ids.size}` : base;
    ids.add(anchor);
    const code = escape(short(symbol.signature.replace(/\r\n/g, '\n'), 1400));
    const calls = [...(symbol.calls ?? [])].map(name => `<code>${escape(name)}</code>`).join(', ');
    return `<details class="symbol" id="${anchor}"><summary><code>${escape(symbol.name)}</code><span class="line">${symbol.kind} · dòng ${symbol.line}</span></summary><pre class="signature"><code>${code}</code></pre>${symbol.comment ? `<p class="source-comment">${escape(symbol.comment)}</p>` : ''}${calls ? `<p class="calls">Gọi hoặc render ${calls}</p>` : ''}<a class="source-link" href="${sourceLink}#L${symbol.line}">Mở source · dòng ${symbol.line}</a></details>`;
  }).join('');
  return `<details class="source-file" id="${id}" data-kind="${kind}" data-test="${test}" data-generated="${generated}" data-search="${search}"><summary><code>${escape(file)}</code><span class="badge">${kind}</span></summary>${previous?.description ? `<p>${previous.description}</p>` : ''}<p class="file-meta">${symbols.length} mục · <a href="${sourceLink}">Mở file gốc</a></p>${cards}</details>`;
}

function main() {
  const files = [...new Set(execFileSync('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard'], { encoding: 'utf8' }).split('\0'))]
    .filter(file => file && fs.existsSync(file) && !/\.(?:png|jpe?g|gif|webp|ico|woff2?|ttf|exe|zip|lock)$/i.test(file) && !file.endsWith('package-lock.json')).sort();
  const goFiles = files.filter(file => file.endsWith('.go'));
  const goSymbols = JSON.parse(execFileSync('go', ['run', './scripts/source-map-go.go'], {
    input: JSON.stringify(goFiles), encoding: 'utf8', maxBuffer: 24 << 20,
  }));
  const html = fs.readFileSync(diagramPath, 'utf8');
  const start = html.indexOf(catalogStart);
  const end = html.indexOf(catalogEnd, start);
  if (start < 0 || end < 0) throw new Error('Source catalog markers are missing');
  const notes = oldNotes(html.slice(start + catalogStart.length, end));
  const statistics = { symbols: [], visible: 0 };
  const totals = { functions: 0, signatures: 0, queries: 0, other: 0 };
  const cards = files.map(file => {
    try {
      const source = file.endsWith('.go') ? '' : fs.readFileSync(file, 'utf8');
      statistics.symbols = goSymbols[file] ?? (scriptExtension.test(file) ? jsSymbols(file, source) : otherSymbols(file, source));
      for (const symbol of statistics.symbols) {
        if (symbol.kind === 'Signature') totals.signatures++;
        else if (symbol.kind === 'Query') totals.queries++;
        else if (symbol.kind === 'Function' || symbol.kind === 'Closure') totals.functions++;
        else totals.other++;
      }
      return renderFile(file, notes, statistics);
    } catch (error) {
      throw new Error(`Cannot index ${file}`, { cause: error });
    }
  });
  const intro = `<p>${files.length} file · ${totals.functions} hàm, method và closure · ${totals.signatures} chữ ký callable · ${totals.queries} named query · ${totals.other} kiểu và khai báo schema. Bật bộ lọc để xem test và mã SQLC sinh.</p><p>Go và JavaScript/TypeScript lấy khai báo từ AST, SQL và shell lấy khai báo có tên. Lời gọi ghi theo biểu thức trong source. Các lời gọi qua interface, callback và JSX có thể cần theo chuỗi gọi mẫu để xác định đích khi chạy.</p>`;
  const eol = html.includes('\r\n') ? '\r\n' : '\n';
  const head = html.slice(0, start).replace(/(<section id="source-map">[\s\S]*?<\/h2>)[\s\S]*?(<form id="source-filter")/, `$1${intro}$2`)
    .replace(/(<p id="source-count"[^>]*>)[^<]+/, (_, opening) => `${opening}${statistics.visible} / ${files.length} file`);
  const next = `${head}${catalogStart}${eol}${cards.join(eol)}${eol}${html.slice(end)}`;
  const summary = `${files.length} files, ${totals.functions} functions, ${totals.signatures} callable signatures, ${totals.queries} named SQL queries`;
  if (process.argv.includes('--check') && next !== html) throw new Error('Source catalog is stale. Run node scripts/update-source-map.cjs');
  if (next !== html) fs.writeFileSync(diagramPath, next);
  console.log(summary);
}

main();
