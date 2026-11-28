// The confirmation window (spec v0.37, chapter 14). The title ends with 「〜しますか？」, the buttons are 「はい」「いいえ」, and what happens
// is written under the title. An operation that ends or deletes something gets a red 「はい」 and opens with the focus on 「いいえ」;
// the others get a blue 「はい」. The decision (confirmSpec) has no DOM; this file imports no lit-html, so Node can run it with fakes.
// The lit-html renderer is handed in (html, render) by the pages.

// spec: {title, lines, danger, yesLabel, noLabel, yesClass, focus}. title and lines are translated texts.
export function confirmSpec(t,{title,lines=[],danger=false}){
 return {title,lines:lines.filter(Boolean),danger:!!danger,yesLabel:t('confirm_yes'),noLabel:t('confirm_no'),yesClass:danger?'btn stop':'btn primary',focus:danger?'no':'yes'};
}

// The inside of a window. ids = {title, lines, yes, no} (the window is named by its title with aria-labelledby); extra is more content between
// the lines and the buttons (the checkbox of the settings window).
export function confirmContent(html,spec,{ids,onYes,onNo,disabled=false,extra='',linesClass=''}){
 return html`<h2 id=${ids.title}>${spec.title}</h2><div id=${ids.lines} class=${'confirm-lines'+(linesClass?' '+linesClass:'')}>${spec.lines.map(line=>html`<p>${line}</p>`)}</div>${extra}<div class="row confirm-actions"><button id=${ids.yes} type="button" class=${spec.yesClass+' confirm-yes'} data-confirm="yes" ?disabled=${disabled} @click=${onYes}>${spec.yesLabel}</button><button id=${ids.no} type="button" class="btn confirm-no" data-confirm="no" ?disabled=${disabled} @click=${onNo}>${spec.noLabel}</button></div>`;
}
export const confirmIds=prefix=>({title:prefix+'-title',lines:prefix+'-lines',yes:prefix+'-yes',no:prefix+'-no'});

// Opens a window whose content is already drawn: showModal, the first focus by the spec, and, when it is closed (by a button or by Escape,
// which is the same as 「いいえ」), the focus goes back to the button that opened it. The opener is often disabled while the request that
// follows 「はい」 (or preceded the window) runs, so the focus is put back once it is enabled again (tried every 50 ms, at most 40 times),
// and only if nobody else took the focus meanwhile (a wrong field has it).
export function showConfirm(dialog,spec,opener,{doc=globalThis.document,defer=(fn,ms=0)=>setTimeout(fn,ms)}={}){
 dialog.showModal();
 dialog.querySelector(spec.focus==='no'?'[data-confirm="no"]':'[data-confirm="yes"]')?.focus();
 const restore=()=>{
  dialog.removeEventListener('close',restore);
  let tries=0;
  const back=()=>{
   if(!opener||!opener.isConnected)return;
   const active=doc.activeElement;
   if(active&&active!==doc.body&&active!==opener&&!dialog.contains(active))return;
   if(opener.disabled){if(++tries<40)defer(back,50);return;}
   opener.focus();
  };
  defer(back,0);
 };
 dialog.addEventListener('close',restore);
}

// A window that is made for one question: ask(spec, opener) resolves to true for 「はい」, false for 「いいえ」 and for Escape.
export function createAsk({document:doc,html,render,defer}){
 let count=0;
 return function ask(spec,opener=doc.activeElement){
  return new Promise(resolve=>{
   const prefix='confirm-'+(++count),dialog=doc.createElement('dialog');
   let answer=false;
   dialog.className='confirm-dialog';
   dialog.setAttribute('aria-labelledby',prefix+'-title');
   const finish=value=>{answer=value;dialog.close();};
   render(confirmContent(html,spec,{ids:confirmIds(prefix),onYes:()=>finish(true),onNo:()=>finish(false)}),dialog);
   dialog.addEventListener('close',()=>{dialog.remove();resolve(answer);});
   doc.body.append(dialog);
   showConfirm(dialog,spec,opener,{doc,defer});
  });
 };
}
