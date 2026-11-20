#!/usr/bin/env bash
# 試験用: irl-live-keeper が使うポートを一時的に占有する（ポートの事前確認の試験に使う。Linux / macOS）。
# 本体と同じ形で待ち受ける。RTMP / RTSP は全インターフェースの TCP、SRT は全インターフェースの UDP、
# MediaMTX の API と管理 API は 127.0.0.1 の TCP。Enter（または Ctrl+C）で閉じて終わる。python3 を使う。
#
# 使い方:
#   scripts/occupy-ports.sh                     # 5つとも既定のポートで
#   scripts/occupy-ports.sh rtmp srt            # 選んだものだけ
#   RTMP=11935 SRT=18890 scripts/occupy-ports.sh rtmp srt   # ポートを変える
set -euo pipefail
[ $# -eq 0 ] && set -- rtmp srt rtsp api admin
# プログラムは -c で渡す（標準入力は Enter を待つのに使うので、ヒアドキュメントで渡さない）
prog=$(cat <<'PY'
import os, socket, sys
e = os.environ.get
ports = {"rtmp": ("tcp", "", int(e("RTMP", "1935")), "mediamtx.rtmp_port"),
         "srt":  ("udp", "", int(e("SRT", "8890")), "mediamtx.srt_port"),
         "rtsp": ("tcp", "", int(e("RTSP", "8554")), "mediamtx.rtsp_port"),
         "api":  ("tcp", "127.0.0.1", int(e("API", "9997")), "mediamtx.api_port"),
         "admin":("tcp", "127.0.0.1", int(e("ADMIN", "8787")), "admin.listen")}
held = []
for t in sys.argv[1:]:
    if t not in ports:
        sys.exit(f"不明な対象: {t}（rtmp / srt / rtsp / api / admin）")
    proto, addr, port, name = ports[t]
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM if proto == "tcp" else socket.SOCK_DGRAM)
    try:
        s.bind((addr, port))
        if proto == "tcp":
            s.listen(1)
        held.append(s)
        print(f"占有した: {name:<20} {port}/{proto} ({addr or '0.0.0.0'})", flush=True)
    except OSError as err:
        print(f"占有できない（すでに使われている?）: {name} {port}/{proto}: {err}", flush=True)
if not held:
    sys.exit("占有したポートはない。終わる。")
print("\nこの状態で irl-live-keeper を起動して、止まり方を確かめる。", flush=True)
try:
    input("Enter で閉じて終わる\n")
except (EOFError, KeyboardInterrupt):
    pass
for s in held:
    s.close()
print("閉じた。")
PY
)
exec python3 -c "$prog" "$@"
