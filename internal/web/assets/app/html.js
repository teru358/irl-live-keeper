// Pure HTML policy checks. Callers provide the policy and DOMParser boundary.
const issue = code => ({code: 'template.' + code});
const ws = c => c !== undefined && /[ \t\n\r\f]/.test(c);
const ident = c => c !== undefined && /[A-Za-z0-9_-]/.test(c);
const matches = (patterns, value) => patterns.some(p => new RegExp(p).test(value));
export function checkURL(value, rules, css = false) {
  const low = value.toLowerCase(), a = rules.asset;
  if (low.startsWith('data:image/') || css && low.startsWith('data:font/')) return value.includes(',');
  if (css && new RegExp(rules.css.fragmentPattern).test(value)) return true;
  if (!value.startsWith(a.prefix)) return false;
  let path = value;
  if (path.includes('?')) {
    const i = path.indexOf('?');
    if (!new RegExp(a.renderedQueryPattern).test(path.slice(i + 1))) return false;
    path = path.slice(0, i);
  }
  if (path.length > a.maxPathLength || path.includes('..') || path.includes('//') || /[?#%]/.test(path)) return false;
  const parts = path.slice(a.prefix.length).split('/');
  if (parts.length > a.maxSegments || parts.some(p => !new RegExp(a.segmentPattern).test(p) || p.endsWith('.') || /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)/i.test(p))) return false;
  return a.extensions.includes(parts.at(-1).split('.').at(-1).toLowerCase());
}
export function checkCSS(s, rules) {
  for (let i = 0; i < s.length;) {
    const start = i;
    if (s[i] === '\\') return issue('css_escape');
    if (s.startsWith('/*', i)) {
      const end = s.indexOf('*/', i + 2);
      if (end < 0) return issue('css_syntax');
      if (s.slice(i, end).includes('\\')) return issue('css_escape');
      i = end + 2; continue;
    }
    if (s[i] === '"' || s[i] === "'") {
      const q = s[i++];
      while (i < s.length && s[i] !== q) {
        if (s[i] === '\\') return issue('css_escape');
        if (/[\n\r\f]/.test(s[i])) return issue('css_syntax');
        i++;
      }
      if (i === s.length) return issue('css_syntax');
      i++; continue;
    }
    const at = s[i] === '@'; if (at) i++;
    const a = i; while (ident(s[i])) i++;
    if (a === i) { if (s[i] === '\\') return issue('css_escape'); i++; continue; }
    const name = s.slice(a, i).toLowerCase();
    if (at && rules.css.rejectAtRules.includes(name)) return issue('css_import');
    if (s[i] !== '(') continue;
    let base = name;
    for (const prefix of rules.css.stripVendorPrefixes) if (base.startsWith(prefix)) base = base.slice(prefix.length);
    if (rules.css.rejectFunctions.includes(base)) return issue('css_function');
    if (name !== 'url') continue;
    i++; while (ws(s[i])) i++;
    let value;
    if (s[i] === '"' || s[i] === "'") {
      const q = s[i++], a = i;
      while (i < s.length && s[i] !== q) {
        if (s[i] === '\\') return issue('css_escape');
        if (/[\n\r\f]/.test(s[i])) return issue('css_syntax');
        i++;
      }
      if (i === s.length) return issue('css_syntax');
      value = s.slice(a, i++);
      for (;;) {
        if (ws(s[i])) {i++; continue;}
        if (s.startsWith('/*', i)) {
          const end = s.indexOf('*/', i + 2);
          if (end < 0) return issue('css_syntax');
          if (s.slice(i, end).includes('\\')) return issue('css_escape');
          i = end + 2; continue;
        }
        break;
      }
      if (s[i] !== ')') return issue('css_syntax');
    } else {
      const end = s.indexOf(')', i);
      if (end < 0) return issue('css_syntax');
      value = s.slice(i, end).replace(/^[ \t\r\n\f]+|[ \t\r\n\f]+$/g, '');
      if (value.includes('\\')) return issue('css_escape');
      i = end;
    }
    if (!checkURL(value, rules, true)) return issue('html_url');
    i++;
  }
  return null;
}
export function decodeReferences(s) {
  let out = '';
  const named = {'amp':'&', 'lt':'<', 'gt':'>', 'quot':'"', 'apos':"'", 'nbsp':'\u00a0'};
  for (let i = 0; i < s.length; i++) {
    if (s[i] !== '&') {out += s[i]; continue;}
    const m = /^&(#(?:[xX][0-9a-fA-F]+|[0-9]+)|amp|lt|gt|quot|apos|nbsp);/.exec(s.slice(i));
    if (!m) return {error: issue('html_lex')};
    let v = named[m[1]];
    if (m[1][0] === '#') {
      const hex = /^#[xX]/.test(m[1]), n = Number.parseInt(m[1].slice(hex ? 2 : 1), hex ? 16 : 10);
      if (!Number.isSafeInteger(n) || n === 0 || n > 0x10ffff || n >= 0xd800 && n <= 0xdfff || n >= 0x80 && n <= 0x9f) return {error: issue('html_lex')};
      v = String.fromCodePoint(n);
    }
    out += v; i += m[0].length - 1;
  }
  return {value: out};
}
const allowedAttribute = (tag, attr, r) => !r.attributeFreeElements.includes(tag) &&
  (r.globalAttributes.includes(attr) || (r.elementAttributes[tag] || []).includes(attr) || matches(r.attributeNamePatterns, attr));
function nesting(stack, name, r) {
  const n = r.nesting, parent = stack.at(-1);
  if (stack.length >= n.maxDepth || n.parents[name] && !n.parents[name].includes(parent) || n.children[parent] && !n.children[parent].includes(name)) return false;
  for (let i = stack.length - 1; i >= 0; i--) {
    const a = stack[i];
    if (a === 'p' && n.pForbiddenDescendants.includes(name) || n.headingElements.includes(a) && n.headingElements.includes(name)) return false;
    if (name === 'li') {if (n.liAncestorBoundary.includes(a)) break; if (a === 'li') return false;}
  }
  return true;
}
export function validateRendered(s, rules) {
  const stack = [];
  for (let i = 0; i < s.length;) {
    if (s[i] === '\0') return issue('html_nul');
    if (s.startsWith('<!--', i)) {
      const end = s.indexOf('-->', i + 4);
      if (end < 0 && s.slice(i).includes('\0')) return issue('html_nul');
      if (end < 0 || s.slice(i + 4, end).includes('--!>')) return issue('html_comment');
      if (s.slice(i, end).includes('\0')) return issue('html_nul');
      i = end + 3; continue;
    }
    if (s[i] !== '<') {
      if (rules.nesting.restrictedTextParents.includes(stack.at(-1)) && !ws(s[i])) {
        const m = /^&#(?:[0-9]+|[xX][0-9a-fA-F]+);/.exec(s.slice(i));
        if (m) {const d = decodeReferences(m[0]); if (d.value?.length === 1 && ws(d.value)) {i += m[0].length; continue;}}
        return issue('html_nesting');
      }
      if (/^&#(?:0+|[xX]0+);/.test(s.slice(i))) return issue('html_nul');
      i++; continue;
    }
    i++; const closing = s[i] === '/'; if (closing) i++;
    const m = /^[A-Za-z][A-Za-z0-9-]*/.exec(s.slice(i));
    if (!m) return issue('html_lex');
    const name = m[0].toLowerCase(); i += m[0].length;
    if (!rules.elements.includes(name)) return issue('html_element');
    const attrs = new Set(); let self = false;
    for (;;) {
      const before = i; while (ws(s[i])) i++;
      if (i === s.length) return issue('html_lex');
      if (s[i] === '>') {i++; break;}
      if (!closing && s.startsWith('/>', i)) {self = true; i += 2; break;}
      if (closing || before === i) return issue('html_lex');
      const am = /^[A-Za-z0-9_.:-]+/.exec(s.slice(i)); if (!am) return issue('html_lex');
      const attr = am[0].toLowerCase(); i += am[0].length;
      if (!allowedAttribute(name, attr, rules)) return issue('html_attribute');
      if (attrs.has(attr)) return issue('html_duplicate'); attrs.add(attr);
      while (ws(s[i])) i++;
      if (s[i++] !== '=') return issue('html_quote');
      while (ws(s[i])) i++;
      if (s[i++] !== '"') return issue('html_quote');
      const a = i, end = s.indexOf('"', i); if (end < 0) return issue('html_lex');
      const raw = s.slice(a, end); i = end + 1;
      if (raw.includes('\0')) return issue('html_nul');
      if (attr === 'src' || attr === 'style') {
        const d = decodeReferences(raw); if (d.error) return d.error;
        if (attr === 'src') {if (!checkURL(d.value, rules)) return issue('html_url');}
        else {const e = checkCSS(d.value, rules); if (e) return e;}
      } else if (/&#(?:0+|[xX]0+);/.test(raw)) return issue('html_nul');
    }
    if (closing) {if (stack.pop() !== name || rules.voidElements.includes(name)) return issue('html_nesting'); continue;}
    if (!nesting(stack, name, rules) || self && !rules.voidElements.includes(name)) return issue('html_nesting');
    if (name === 'style') {
      const rest = s.slice(i), m = /<\/style[\t\n\r\f />]/i.exec(rest);
      if (!m) return issue(rest.includes('\0') ? 'html_nul' : 'html_style_unclosed');
      const css = rest.slice(0, m.index);
      if (css.includes('\0')) return issue('html_nul');
      const e = checkCSS(css, rules); if (e) return e;
      i += m.index;
    }
    if (!rules.voidElements.includes(name)) stack.push(name);
  }
  return stack.length ? issue('html_nesting') : null;
}
export function validateTree(doc, rules) {
  const ns = 'http://www.w3.org/1999/xhtml';
  if (!doc.head || !doc.body || doc.head.childNodes.length || doc.body.attributes.length) return issue('html_nesting');
  function walk(node, stack) {
    if (node.nodeType === 8) return null;
    if (node.nodeType === 3) {
      if (node.textContent.includes('\0')) return issue('html_nul');
      return rules.nesting.restrictedTextParents.includes(stack.at(-1)) && /[^ \t\r\n\f]/.test(node.textContent) ? issue('html_nesting') : null;
    }
    if (node.nodeType !== 1 || node.namespaceURI !== ns || !rules.elements.includes(node.localName)) return issue('html_element');
    const name = node.localName;
    if (!nesting(stack, name, rules)) return issue('html_nesting');
    for (const attr of node.attributes) {
      if (attr.namespaceURI || !allowedAttribute(name, attr.name, rules)) return issue('html_attribute');
      if (attr.value.includes('\0')) return issue('html_nul');
      if (name === 'img' && attr.name === 'src' && !checkURL(attr.value, rules)) return issue('html_url');
      if (attr.name === 'style') {const e = checkCSS(attr.value, rules); if (e) return e;}
    }
    if (name === 'style') {if (node.textContent.includes('\0')) return issue('html_nul'); return checkCSS(node.textContent, rules);}
    for (const child of node.childNodes) {const e = walk(child, [...stack, name]); if (e) return e;}
    return null;
  }
  for (const child of doc.body.childNodes) {const e = walk(child, []); if (e) return e;}
  return null;
}
export function parseRendered(html, rules, parser) {
  const error = validateRendered(html, rules);
  if (error) return {error};
  const document = parser.parseFromString('<!doctype html><html><head></head><body>' + html, 'text/html');
  const treeError = validateTree(document, rules);
  return treeError ? {error: treeError} : {document};
}
