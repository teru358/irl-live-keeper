import json, urllib.request, hashlib, base64, tarfile, io, os, sys
PKGS = [("lit-html","3.3.3"),("@codemirror/state","6.7.6"),("@codemirror/view","6.43.15"),("@codemirror/language","6.13.1"),("@codemirror/streamparser","6.0.0"),("@codemirror/commands","6.11.1"),("@codemirror/autocomplete","6.20.4"),("@codemirror/lint","6.9.7"),("@codemirror/lang-css","6.3.1"),("@lezer/common","1.5.5"),("@lezer/highlight","1.2.5"),("@lezer/lr","1.4.12"),("@lezer/css","1.3.9"),("style-mod","4.1.4"),("crelt","1.0.7"),("w3c-keyname","2.2.8"),("@marijn/find-cluster-break","1.0.4")]
out = sys.argv[1]
info = {}
for name, ver in PKGS:
    meta = json.load(urllib.request.urlopen(f"https://registry.npmjs.org/{name.replace('/','%2F')}/{ver}"))
    url = meta["dist"]["tarball"]; integ = meta["dist"]["integrity"]
    data = urllib.request.urlopen(url).read()
    algo, b64 = integ.split("-",1)
    assert algo == "sha512" and base64.b64encode(hashlib.sha512(data).digest()).decode() == b64, name
    dst = os.path.join(out, name.replace("/","__")+"@"+ver)
    os.makedirs(dst, exist_ok=True)
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tf:
        tf.extractall(dst, filter="data")
    info[name] = {"version": ver, "tarball": url, "integrity": integ, "date": None, "license": meta.get("license")}
    print("ok", name, ver, meta.get("license"))
json.dump(info, open(os.path.join(out, "info.json"), "w"), indent=1)
