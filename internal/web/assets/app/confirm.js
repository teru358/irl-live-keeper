// The one confirmation window of the admin page: askConfirm(spec, opener) resolves to true for 「はい」 (see confirm-dialog.js).
import {html,render} from '../vendor/lit-html-3.3.3/lit-html.js';
import {createAsk} from './confirm-dialog.js';
let ask;
export const askConfirm=(spec,opener)=>(ask||=createAsk({document,html,render}))(spec,opener);
