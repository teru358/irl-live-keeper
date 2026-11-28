import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {checkAssetPath} from '../assets/app/html.js';
const rules=JSON.parse(readFileSync(new URL('../../sources/html_rules.json',import.meta.url)));
const cases=JSON.parse(readFileSync(new URL('../../sources/asset_name_cases.json',import.meta.url)));
for(const c of cases) test('asset '+c.name,()=>assert.equal(checkAssetPath(c.name,rules),!c.reason));
test('isolated surrogates',()=>{for(const c of ['\ud800','\udfff']) assert.equal(checkAssetPath(c+'.png',rules),false);});
