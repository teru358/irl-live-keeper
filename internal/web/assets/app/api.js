export function apiError(data,status,fallback){const error=Error(data.error||fallback);error.code=data.code;error.fields=status===422?(data.fields||{}):data.fields;error.status=status;return error;}
export function createAPI({lang,t,login},fetcher=fetch,redirect=url=>location.replace(url)){
 return async function request(path,body,method){const r=await fetcher('/api/'+path+(path.includes('?')?'&':'?')+'lang='+lang,{method:method||(body===undefined?'GET':'POST'),headers:{'Content-Type':'application/json','X-IRL-Live-Keeper':'1'},body:body===undefined?undefined:JSON.stringify(body)});if(r.status===401&&!login)redirect('/?login');const data=await r.json();if(!r.ok)throw apiError(data,r.status,t('failed'));return data;};
}
