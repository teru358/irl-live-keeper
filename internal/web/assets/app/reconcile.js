import {parseRendered} from './html.js';

// Match siblings only, consuming each candidate once even with duplicate ids.
export function reconcile(root, incoming) {
 const old = [...root.childNodes], used = new Set();
 for (const [index, next] of [...incoming.childNodes].entries()) {
  const id = next.nodeType === 1 && next.getAttribute('id');
  let current = id ? old.find(n => !used.has(n) && n.nodeType === 1 && n.getAttribute('id') === id) : old[index];
  if (used.has(current) || current?.nodeType !== next.nodeType || current?.localName !== next.localName || !id && current?.nodeType === 1 && current.getAttribute('id')) current = null;
  if (current) {
   used.add(current);
   if (current.nodeType === 1) {
    for (const attr of [...current.attributes]) if (next.getAttribute(attr.name) === null) current.removeAttribute(attr.name);
    for (const attr of next.attributes) if (current.getAttribute(attr.name) !== attr.value) current.setAttribute(attr.name, attr.value);
    if (current.localName === 'style') {
     if (current.textContent !== next.textContent) current.textContent = next.textContent;
    } else if (current.localName !== 'img') reconcile(current, next);
   } else if (current.textContent !== next.textContent) current.textContent = next.textContent;
  } else current = root.ownerDocument.importNode(next, true);
  if (root.childNodes[index] !== current) root.insertBefore(current, root.childNodes[index] || null);
 }
 for (const node of old) if (!used.has(node)) root.removeChild(node);
}
export function updateRendered(root, html, rules, parser) {
 const result = parseRendered(html, rules, parser);
 if (result.error) {console.warn('Rejected rendered source:', result.error.code);return false;}
 reconcile(root, result.document.body);
 return true;
}
