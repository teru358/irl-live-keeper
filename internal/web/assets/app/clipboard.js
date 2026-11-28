// Copies a text: the clipboard API (it exists only on a secure page), then execCommand (a page opened over http from the LAN).
// Returns whether it worked. The environment is passed in so that Node can run it with fakes.
export async function copyValue(value,nav=globalThis.navigator,doc=globalThis.document){
 if(nav?.clipboard?.writeText){try{await nav.clipboard.writeText(value);return true;}catch(_){}}
 const area=doc.createElement('textarea');
 area.value=value;area.readOnly=true;area.contentEditable='true';area.style.cssText='position:fixed;top:0;left:0;opacity:0';
 doc.body.append(area);
 const active=doc.activeElement;
 try{area.focus();area.select();area.setSelectionRange(0,value.length);return doc.execCommand('copy');}
 catch(_){return false;}
 finally{area.remove();active?.focus();}
}
