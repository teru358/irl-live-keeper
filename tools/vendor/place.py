import json, os, shutil, hashlib, sys
d, repo = sys.argv[1], sys.argv[2]
info = json.load(open(os.path.join(d,"info.json")))
V = os.path.join(repo, "internal/web/assets/vendor")
rows = []
def put(name, ver, src_rel, dst_rel):
    src = os.path.join(d, name.replace("/","__")+"@"+ver, "package", src_rel)
    dst = os.path.join(V, dst_rel); os.makedirs(os.path.dirname(dst), exist_ok=True)
    shutil.copyfile(src, dst)
    h = hashlib.sha256(open(dst,"rb").read()).hexdigest()
    rows.append(f"{dst_rel}\t{name}@{ver}\tpackage/{src_rel}\tsha256:{h}\t{info[name]['tarball']}\t{info[name]['integrity']}")
def lic(name, ver, dst_rel):
    root = os.path.join(d, name.replace("/","__")+"@"+ver, "package")
    for c in ["LICENSE","LICENSE.md","LICENSE.txt","license"]:
        p = os.path.join(root,c)
        if os.path.exists(p):
            put(name, ver, c, dst_rel); return
    raise SystemExit("no license "+name)
ver = info["lit-html"]["version"]; base = f"lit-html-{ver}/"
for f in ["lit-html.js","directive.js","directive-helpers.js","directives/live.js","directives/repeat.js","directives/class-map.js"]:
    put("lit-html", ver, f, base+f)
lic("lit-html", ver, base+"LICENSE")
for name, v in info.items():
    if name == "lit-html": continue
    root = os.path.join(d, name.replace("/","__")+"@"+v["version"], "package")
    pj = json.load(open(os.path.join(root,"package.json")))
    e = pj.get("exports")
    if isinstance(e, dict):
        e = e.get(".", e)
        if isinstance(e, dict): e = e.get("import") or e.get("default")
    entry = (e if isinstance(e,str) else pj.get("module") or pj.get("main")).lstrip("./")
    flat = name.lstrip("@").replace("/","-")
    put(name, v["version"], entry, f"codemirror/{flat}@{v['version']}.js")
    lic(name, v["version"], f"codemirror/LICENSES/{flat}.txt")
hdr = "# 同梱した外のファイル（spec v0.20 2章）。取得: 2026-11-26、npm の登録所から tarball を取り、dist.integrity（sha512）と照合して、ファイルを手を加えずに置いた\n# 列: 置いた場所（vendor/ から）\tパッケージ@版\ttarball の中のパス\tsha256\ttarball の URL\tintegrity\n"
open(os.path.join(V,"VENDOR.txt"),"w").write(hdr+"\n".join(rows)+"\n")
print(len(rows), "files")
