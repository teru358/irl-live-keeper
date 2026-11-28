import {updateRendered} from './reconcile.js';
const root = document.querySelector('.irl-source');
async function loadRules() {
 for (;;) {
  try {
   const response = await fetch('/source/assets/html_rules.json');
   if (!response.ok) throw Error('HTML rules unavailable');
   return await response.json();
  } catch (_) { await new Promise(resolve => setTimeout(resolve, 1000)); }
 }
}
const rules = await loadRules();
const parser = new DOMParser();
const events = new EventSource(window.location.pathname.replace(/\/$/, '') + '/events');
events.onmessage = event => {
 try {updateRendered(root, JSON.parse(event.data), rules, parser);} catch (_) { /* Keep the last display. */ }
};
