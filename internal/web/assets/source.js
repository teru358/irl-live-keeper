import {updateRendered} from './reconcile.js';
const root = document.querySelector('.irl-source');
const rules = await fetch('/source/assets/html_rules.json').then(r => r.json());
const parser = new DOMParser();
const events = new EventSource(window.location.pathname.replace(/\/$/, '') + '/events');
events.onmessage = event => {
 try {updateRendered(root, JSON.parse(event.data), rules, parser);} catch (_) { /* Keep the last display. */ }
};
