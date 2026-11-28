// The save row of the camera and settings pages (S2-94, A-7 and A-8; spec v0.37). It is one row in the same place on every group of a page:
// the notice of the page, then 「保存していない変更があります　保存する　読み直す」. While there is a draft (or a notice to read) the row sticks to the
// bottom of the screen, so that the person who changed a field in the middle of a long page does not have to scroll to save, and sees the answer where they pressed.
// The message of the draft is drawn only while there is a draft. No DOM and no lit-html here: the renderer (html) is handed in, so Node can run it with fakes.

// Sticks while there is a draft, and while a notice is on the screen (the answer of the save or of the reload would otherwise be left at the end of a long page).
// The notice of a reload (quietNotice, S2-97) does not stick: there is no draft left, and the answer is at the end of the page, where the button was pressed.
export const barSticky=({dirty,notice='',applyNotice='',quietNotice=''})=>!!dirty||(!!notice&&notice!==quietNotice)||!!applyNotice;

// page: 'cameras' or 'settings' (the ids of the buttons are kept: <page>-save and <page>-reload).
export function saveBar(html,{page,dirty,busy,notice='',quietNotice='',applyNotice='',t,onSave,onReload}){
 const sticky=barSticky({dirty,notice,applyNotice,quietNotice});
 return html`<div id=${page+'-bar'} class=${'save-bar'+(sticky?' sticky':'')} data-dirty=${String(!!dirty)}><p id=${page+'-result'} class="notice" role="status">${notice}</p><p class="notice" role="status">${applyNotice}</p><div class="save-bar-main">${dirty?html`<p class="save-bar-msg" id=${page+'-unsaved'}>${t('settings_unsaved_bar')}</p>`:''}<div class="save-actions"><button id=${page+'-save'} type="button" class="btn primary" ?disabled=${busy} @click=${onSave}>${t('settings_save')}</button><button id=${page+'-reload'} type="button" class="btn" ?disabled=${busy} @click=${onReload}>${t('settings_reload')}</button></div></div></div>`;
}

// The restart is a different kind of operation (it restarts the app and changes no setting), so it is below the line, apart from the save row.
export function restartZone(html,{page,busy,t,onRestart}){
 return html`<div class="restart-zone"><button id=${page+'-restart'} type="button" class="btn" ?disabled=${busy} @click=${onRestart}>${t('settings_restart')}</button></div>`;
}
