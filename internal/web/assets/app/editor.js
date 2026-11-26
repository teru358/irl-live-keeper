// Safe to import from a screen module: CodeMirror loads only when mounted.
const mounted=new WeakMap();
export async function mountEditor(container, options={}) {
 if(mounted.has(container))return mounted.get(container);
 const pending=import('./editor-codemirror.js').then(({createEditor})=>createEditor(container,options));
 mounted.set(container,pending);
 try {
  const editor=await pending;
  const destroy=editor.destroy;
  editor.destroy=()=>{destroy();mounted.delete(container);};
  return editor;
 } catch(error){mounted.delete(container);throw error;}
}
