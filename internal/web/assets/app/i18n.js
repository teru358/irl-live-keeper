export function selectLanguage(saved, config, languages) {
 if(saved==='ja'||saved==='en')return saved;
 if(config==='ja'||config==='en')return config;
 return (languages[0]||'').toLowerCase().split('-')[0]==='ja'?'ja':'en';
}
export function createI18n(boot, storage=localStorage, languages=navigator.languages) {
 let saved;try{saved=storage.getItem('language');}catch(_){}
 const lang=selectLanguage(saved,boot.language,languages);
 return {lang,t:key=>boot.catalogs[lang][key],setLanguage(next){try{storage.setItem('language',next);}catch(_){}location.reload();}};
}
