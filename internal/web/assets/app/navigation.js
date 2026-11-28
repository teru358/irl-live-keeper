// How the admin page moves (S2-93): the page in the URL (#status, #cameras, #sources, #settings), the focus after a move, and the phone menu as a window.
// No DOM of its own and no lit-html: window, document and the elements are handed in, so Node can run it with fakes.

export const pages=['status','cameras','sources','settings'];
export const panelSelector={status:'#status',cameras:'#cameras-panel',sources:'#sources-panel',settings:'#settings-panel'};

// An empty or unknown hash is the status page.
export const pageFromHash=hash=>{const name=String(hash||'').replace(/^#/,'');return pages.includes(name)?name:'status';};
export const hashOf=page=>'#'+page;

// before(page) asks whether the page may be left (false: a page keeps a draft and asks first; it asks again by itself with keeper-navigate).
// show(page) puts the page on the screen. current() is the page on the screen.
// A move made by the person writes a new history entry (pushState fires no event). Back, forward and an edited hash arrive as popstate / hashchange:
// the page is read from the hash and nothing is written. When such a move is refused, the URL is written back to the page that stays.
export function createRouter({win,current,before,show}){
 function go(page,{fromHistory=false}={}){
  if(!pages.includes(page))return false;
  if(!before(page)){
   if(fromHistory)win.history.replaceState(null,'',hashOf(current()));
   return false;
  }
  show(page);
  if(!fromHistory&&pageFromHash(win.location.hash)!==page)win.history.pushState(null,'',hashOf(page));
  return true;
 }
 function onHistory(){
  const page=pageFromHash(win.location.hash);
  if(page!==current())go(page,{fromHistory:true});
 }
 win.addEventListener('popstate',onHistory);
 win.addEventListener('hashchange',onHistory);
 return {go,initial:()=>pageFromHash(win.location.hash)};
}

// The first heading of a panel that is on the screen (the status page has several, and the tab of the phone hides some of them).
export function visibleHeading(panel){
 for(const heading of panel?.querySelectorAll('h2')||[])if(heading.getClientRects().length)return heading;
 return null;
}
// After a move, the focus goes to the heading of the page that was opened. tabindex -1 lets it take the focus without joining the Tab order.
export function focusHeading(panel){
 const heading=visibleHeading(panel);
 if(!heading)return false;
 heading.setAttribute('tabindex','-1');
 heading.focus();
 return true;
}

// The phone menu works as a window: it has a name (the label of the element), Escape closes it, the focus moves in when it opens and goes back to the
// 「メニュー」 button when it closes, and everything behind it cannot be reached (inert). It is not showModal(), because the button is in the header and
// must stay usable: while the menu is open its name is 「閉じる」 and pressing it closes the menu. Tab goes from the last item back to the button.
// sheet: the dialog element. button: the 「メニュー」 button. background(): the elements behind. close(): asks the owner to close (the state is the owner's).
export function createMenu({sheet,button,background,close,doc}){
 let open=false;
 const items=()=>[...sheet.querySelectorAll('button')].filter(b=>!b.hidden&&!b.disabled);
 function onKey(e){
  if(e.key==='Escape'){e.preventDefault();close();return;}
  if(e.key==='Tab'&&!e.shiftKey){
   const list=items();
   if(list.length&&doc.activeElement===list[list.length-1]){e.preventDefault();button.focus();}
  }
 }
 return {
  get open(){return open;},
  sync(want){
   want=!!want;
   if(want===open)return;
   open=want;
   if(want){
    sheet.show();
    for(const element of background())element.inert=true;
    doc.addEventListener('keydown',onKey,true);
    (sheet.querySelector('[aria-current="page"]')||items()[0])?.focus();
   }else{
    doc.removeEventListener('keydown',onKey,true);
    for(const element of background())element.inert=false;
    sheet.close();
    button.focus();
   }
  },
 };
}
// The name of the button: 「メニュー」, and 「閉じる」 while the menu is open.
export const menuLabel=(t,open)=>t(open?'menu_close':'menu');
