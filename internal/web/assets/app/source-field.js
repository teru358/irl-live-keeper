// A text or number field of the source edit (S2-97). The wrong value of the name or the display name is told right under the field it belongs to, and the
// field says so (aria-invalid, aria-describedby), the way the login does. No DOM here: lit-html's html, live and nothing are handed in, so Node can run it.

// Which field a refusal of the save belongs to: the server's code of the name (empty, characters, long, reserved, taken) or of the display name (long).
const nameCodes=['sources.name_empty','sources.name_chars','sources.name_long','sources.reserved_name','sources.duplicate'];
export const errorField=code=>nameCodes.includes(code)?'name':code==='sources.title_long'?'title':'';
export const fieldErrorID=key=>'source-'+key+'-error';

// A failed save: a wrong name or display name goes to its field (the name is only a field while the source is new); anything else is the error of the edit.
// Returns the field that has to take the focus ('' when there is none).
export function saveFailed(state,error){
 const key=errorField(error.code),field=key==='name'&&!state.fresh?'':key;
 if(field)state.fieldErrors={...state.fieldErrors,[field]:error.message};
 else state.editorError=error.message;
 state.failNetwork=!!error.network;
 return field;
}
// Typing in a field takes back the error of that field only.
export const clearFieldError=(state,key)=>{if(state.fieldErrors?.[key])state.fieldErrors={...state.fieldErrors,[key]:''};};

export function sourceField({html,live,nothing},{s,key,type='text',t,onInput}){
 const value=s.draft[key]??'';
 const error=s.fieldErrors?.[key]||'';
 const named=key==='name'||key==='title';
 const ids=[key==='name'?'source-name-help':'',error?fieldErrorID(key):''].filter(Boolean);
 const input=html`<input name=${'source-'+key} aria-label=${t('src_'+key)} aria-describedby=${ids.length?ids.join(' '):nothing} aria-invalid=${error?'true':nothing} type=${type} step="any" .value=${live(String(value))} ?disabled=${s.busy} @input=${e=>{
  const raw=e.target.value;
  onInput(key,type==='number'?(raw===''?0:Number(raw)):raw);
 }}>`;
 const unit=['obs_width','obs_height'].includes(key)?'px':'';
 // The URL name: the error right under the field, then the characters that can be used and that it cannot be changed after it is made (C-2). Both are inside the label, so they stay under the field.
 return html`<label class="source-field">${t('src_'+key)}<span class="source-input">${input}${unit?html`<span class="hint">${unit}</span>`:''}</span>${named?html`<small id=${fieldErrorID(key)} class="warn source-field-error" role="alert">${error}</small>`:''}${key==='name'?html`<small id="source-name-help" class="settings-help">${t('src_name_help')}</small>`:''}</label>`;
}
