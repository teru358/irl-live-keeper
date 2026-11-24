# irl-live-keeper

[日本語](README.md)

irl-live-keeper is a tool for outdoor streaming (IRL). When the connection becomes unstable, it automatically switches OBS to a standby scene. It switches back when the connection recovers.

It combines video reception from phones and other devices (RTMP / SRT), input to OBS, and automatic scene switching in one program. The goal is to make it usable by simply extracting and running a zip archive.

> **In development.** Available now: reception and automatic switching (multiple cameras, per-scene fallback scenes, hiding a single camera) and part of the management page (status and controls, scene switching, sender settings with URL copy, logs, Japanese and English, light and dark themes). Items marked "(in development)" under Features are not available yet.

## Features

- **Reception:** Receive video over both RTMP and SRT from phone streaming apps (such as Larix Broadcaster, IRL Pro, and Moblin) or OBS. The bundled [MediaMTX](https://github.com/bluenviron/mediamtx) handles reception.
- **Multiple cameras:** Receive 1 to 8 cameras (main, sub, hand camera, and so on) separately, using a URL for each camera, and check each camera's connection separately. The app automatically finds which cameras appear in which scenes by inspecting OBS scenes. You do not list this in the configuration. You can place two cameras side by side in one scene.
- **Automatic switching:** Check the received bitrate every second. While on a scene that shows camera video (a monitored scene), the app switches as follows:
  - If any visible camera stops sending video (for about 2 seconds), switch to the standby scene.
  - If the bitrate stays low (the default is below 800 kbps for 5 seconds), switch to the standby scene.
  - When all visible cameras have recovered and are stable (the default is at least 1500 kbps for 10 seconds), return to the scene that was active before entering standby. This also returns to the scene in use if you changed camera scenes with IRL Link or a similar app.
- **Use with external tools:** The app assumes you normally control scenes with IRL Link or OBS. If you switch to a camera scene while the connection is poor, the app sends OBS to the standby scene and returns to that camera scene after recovery. Turn automatic switching OFF when you intentionally want to show a disrupted video.
- **Switch when streaming starts** (optional, OFF by default): When streaming starts, switch to a specified scene, such as a starting screen.
- **Hide only cameras** (`on_bad: hide`): Hide only the OBS source for a camera with a poor connection, then show it again after recovery. Also restore hidden sources from the record (`state/hidden.json`) after a crash. See “Multiple cameras” for details.
- **Manual controls for testing:** Start and stop streaming, turn standby mode on and off manually, and show all camera sources again.
- **Management page:** View status, start and stop streaming, use manual standby, switch scenes, view and copy sender URLs, and read logs. It works at phone width. You can open it from outdoors over a VPN such as Tailscale (see "Open the management page from outside with Tailscale").
- **Rest of the management page** (in development): bitrate graph, settings editor, stream history, and QR codes for sender URLs.
- **Small window** (in development, Windows): a window that shows the status in a few lines. The current Windows build runs in a console window.
- **Sources** (in development): Build text templates on the management page and load them in an OBS browser source to show connection status and bitrate on the stream.
- **Distribution** (Phase 4): Zip packages for Windows, macOS, and Linux.

## How it works

```text
[Phone streaming app]
        │ RTMP :1935/TCP, SRT :8890/UDP (forward only these two ports on the router)
        ▼
[MediaMTX] (bundled; started and monitored by irl-live-keeper) ── video (per camera) ──► [OBS media source]
        │ received byte counts (API)
        ▼
[irl-live-keeper] ── scene switching (obs-websocket) ──► [OBS]
```

irl-live-keeper calculates each camera's bitrate from the change in the number of bytes received by MediaMTX. On mobile connections, the connection can remain open while data stops. So the app detects interruptions by checking whether data is arriving, not by relying on disconnect notifications.

## Requirements

- **OBS Studio 30.1.0 or later** (with obs-websocket 5.4.0 or later built in). Notifications about scene UUIDs and media source setting changes are available from this version. On older OBS versions, automatic switching stops because the app cannot read the scene map (a warning is shown). Manual controls and switching when streaming starts still work.
- A PC running OBS, or a PC on the same LAN (Windows is the primary target. macOS and Linux are also expected to work).

## Setup

On Linux, `irl-live-keeper service install` registers a user service (the default). To keep it running while logged out, enable lingering with `loginctl enable-linger <user>`. Register a system service with `sudo irl-live-keeper service install --system --run-as <user>`. Remove it with `irl-live-keeper service uninstall [--system]`. The command prints the `systemctl` steps and management page URL after registration.

1. Extract the zip and run `irl-live-keeper` (`irl-live-keeper.exe` on Windows). On first start, it creates `config.yaml`. Set the admin password on the same PC in the admin page.
2. In OBS, enable the server in “Tools → WebSocket Server Settings”. Put its password in `obs.password` in `config.yaml`.
3. Create a standby scene in OBS and put its name in `obs.standby_scene` in `config.yaml`. You do not need to list monitored scenes (scenes showing camera video). The app finds them automatically from OBS (scenes containing camera media sources).
   - Give the standby scene an obvious name, such as “⚠Connection standby”. This makes it easy to notice in IRL Link when OBS enters standby.
4. List the cameras under `cameras` in `config.yaml` (see “Multiple cameras” below). The initial configuration has one camera, `main` (the main camera).
5. In OBS, create **one media source per camera** and enter its OBS URL, shown at startup (`rtsp://…/live/<id>`). The media source name is up to you. The app finds a camera's source by matching the URL path to that camera's path (`live/<id>`). If you upgraded from an earlier version (where the path was just `<id>`), change the OBS media source URL once to `…/live/<id>`. The old form will not show video, and `status.warnings` will say “URL is in the old format”.
   - Turn off “Local File” and add `rtsp_transport=tcp` to the FFmpeg options.
   - Enable “Use hardware decoding”. If it is disabled, OBS may fail to keep up with decoding, causing delayed or choppy video. The app log may repeatedly show MediaMTX's `reader is too slow, discarding N frames`. This occurred in a Windows check when two sources read one 720p30 camera. Two or more cameras increase the load further.
     - If OBS cannot read frames fast enough and MediaMTX discards video (3 or more times in 30 seconds), `status.warnings` says “OBS is not keeping up with <camera name>…”. This is only a warning. It does not switch to standby. The warning is not shown for a stopped camera. It may appear just after reconnecting and disappear after a while. If it keeps appearing before a stream, lower the sending bitrate or resolution, and enable hardware decoding if it is disabled.
   - Turn off “Restart playback when source becomes active”. If enabled, playback restarts right after this app switches back to a camera scene following recovery, leaving a black screen for about 2 seconds. Even with this option off, video appears immediately when you or IRL Link switches to a camera scene.
   - A reconnect delay of 2 seconds is recommended. This value affects the time from a camera connection being established until video appears in OBS. When this app switches back automatically after recovery, it restarts the scene's media source before switching, so this delay does not apply.
   - You can read the same camera from multiple media sources in multiple scenes. A camera without a matching media source URL appears in `status.warnings`.
6. Set each camera's sender URL in your phone streaming app. The app automatically detects this PC's LAN address for the URL host at startup. When sending from outside, replace the host with the address for port forwarding, a tunnel, or a VPN. Set `stream.publish_host` in `config.yaml` to use a fixed address.
   - SRT: `srt://<this PC's address>:8890?streamid=publish:live/<camera id>:<user>:<password>`
     - If the app has separate “URL” and “Stream ID” (or “Key”) fields, put the part before `?streamid=` in the URL field. Put the part after it (`publish:…`) in the Stream ID field. Leave the passphrase (encryption) empty.
     - `loss_rate` in `status.cameras[].srt` is the percentage of retransmitted packets. `recv_mbps` is the speed received at the SRT layer. If `recv_mbps` is high enough but `kbps` is low, the app's sending format may not work with MediaMTX. This can happen with apps that do not declare video tracks at the start.
   - RTMP: `rtmp://<this PC's address>:1935/live/<camera id>?user=<user>&pass=<password>`
     - In apps with separate “URL” and “Stream Key” (app name and key) fields, such as Larix Broadcaster and IRL Pro, enter `rtmp://<this PC's address>:1935/live` in the URL field. Enter `<camera id>?user=<user>&pass=<password>` in the key field (for example, `main?user=irl&pass=…`). The app combines these into `live/<camera id>`. In apps with one URL field, such as OBS, It's My Live, and Android RTMP Streamer, enter the full URL above.
     - If the app has “login” and “password” fields for RTMP authentication, **leave them empty**. Put the username and password in the URL or key as `?user=…&pass=…`. MediaMTX does not use the RTMP authentication procedure (Adobe style). It reads only `?user=…&pass=…`.
     - If the format still does not work in an app, send over SRT.
     - You can change `live` in `config.yaml` with `stream.app`. Set it to an empty string to use the earlier path format, which contains only `<camera id>`.
7. When sending from outside, forward only 1935/TCP and 8890/UDP from the router to this PC.

## Open the management page from outside with Tailscale

1. Install Tailscale on the streaming PC and phone. Connect both with the same account.
2. Set `admin.lan` to `true` in `config.yaml`, then restart the app (the default `admin.port` is `8787`).
3. On the phone, open `http://<streaming PC Tailscale address>:<admin.port>/` in a browser. Log in as `admin` with the management password. Set the password on the streaming PC first.

Do not forward the management page port on your router. Do not expose it directly to the internet. Tailscale VPN traffic is encrypted, but devices connected to the VPN can also view the management password and change settings. VPN requests are treated like requests from LAN devices, so `mediamtx.binary` and `log.dir` cannot be changed.

To send video from an outdoor phone, choose the streaming PC's Tailscale address as the destination host in the “Send” settings on the management page. You can copy each part of the displayed RTMP / SRT URLs.

## Multiple cameras

List 1 to 8 cameras in order under `cameras` in `config.yaml`.

```yaml
cameras:
  - id: main              # URL path. 1 to 32 lowercase letters, digits, - or _. Must be unique.
    name: Main camera     # Display name.
    on_bad: standby       # When the connection is poor: standby (switch to standby scene) / hide (hide this camera's OBS sources).
  - id: sub
    name: Sub camera
    on_bad: standby
    switch:               # Optional. Override common switch values for this camera only. Only listed values are overridden.
      fail_kbps: 400
      recover_kbps: 800
```

- These old settings have been removed: `obs.media_source`, `obs.monitored_scenes`, and `stream.name`. If any remain, startup fails with an error. Media sources and scenes are detected from OBS, and `stream.name` has been replaced by the camera `id` in `cameras`.
- Connection health is checked per camera. The thresholds combine the common `switch` settings with any per-camera `switch` overrides.
- **Whether to switch to the standby scene depends only on the scene currently on screen.** If any camera visible in that scene is not OK, switch to standby. Return to the original scene after all visible cameras recover (after `recover_sec`). If two cameras appear in one scene, either one going down sends OBS to standby.
- **Cameras not used that day:** Nothing happens if a camera in another scene is DOWN, even if you are not using it that day. For example, if the hand camera is in a separate scene, switching to that scene sends OBS to standby until the hand camera arrives and is OK. Then the app automatically returns to the hand camera scene. To shorten the wait, override `recover_sec` for that camera only. Before stopping the hand camera, switch back to the main scene. A camera source that a person manually hid in OBS is not checked in that scene.
- **`on_bad: hide` (hide only that camera in OBS):** This does not switch to the standby scene. It hides that camera's OBS sources (OBS items) only while streaming. It shows them again after the camera is OK (after `recover_sec`). Keep the following details in mind:
  - A camera is hidden only while streaming, automatic switching is ON, and the camera is not OK. Every item for that camera in every OBS scene is hidden, including items inside nested scenes and groups. Sources are shown again if streaming stops, `switch.auto_enabled` becomes false, or `on_bad` changes to `standby`.
  - If hiding would leave the current scene with no visible cameras, the app switches to the standby scene first and does not hide the camera. This keeps the scene from becoming empty.
  - The app does not touch items that a person hid or showed in OBS. If a person shows a camera item, the app will not hide it again until that camera next becomes OK or streaming stops.
  - Items hidden by the app are recorded in `state/hidden.json` (under the same folder as `config.yaml`). If the app or OBS crashes, the app uses this record to show them again when it next connects to OBS. Records are kept separately for each OBS connection (`obs.host` and `obs.port`) and scene collection name. Records for other connections or collections are left untouched, and their count appears in `status.warnings`. If `hidden.json` is corrupt, the app warns, starts with an empty record, and keeps the original file as `hidden.json.broken`.
  - When the app exits (Ctrl+C, an exit signal, or closing the Windows console), it shows the items it hid first. It waits up to 2 seconds. If it is not connected to OBS, it keeps the record instead.
  - **Show all:** This is a recovery option for rare failures, such as OBS freezing or the app crashing, that leave items hidden. You can run it from the management page. The API is `POST /api/cameras/show-all` (it requires the `X-IRL-Live-Keeper: 1` header, like other POST requests; it returns 503 if OBS is not connected). It shows every hidden camera item, including items hidden manually.
  - If OBS does not respond to a hide or show request within 5 seconds, it may apply the request later. The app keeps the item in `hidden.json` as “unsettled”. If a delayed hide arrives, the app shows the item again as one it hid. If an item still remains hidden, use “Show all”.
- When sending **two or more cameras over the same uplink**, they share the available bandwidth. Each camera gets less bandwidth, so lower the thresholds (`fail_kbps`, `recover_kbps`) to account for the number of cameras.
- Changing a camera's `id` changes its sender URL and OBS URL. Changing `stream.app` also changes them. Update the URLs manually in the phone app and OBS media sources.
- While OBS is switching scene collections, the app treats OBS as disconnected and rejects manual controls.

### Stop reception for unused protocols

Use `mediamtx.rtmp_enabled` and `mediamtx.srt_enabled` to disable RTMP or SRT reception individually (both default to `true`; keep at least one enabled). The app does not open a disabled protocol's port or create its URLs. Disabling an unused protocol reduces the ways the server can be attacked. Restart the app for the change to take effect. RTSP cannot be disabled because OBS uses it to read video.

## Using a relay PC on the LAN

- The basic setup is to run this app and MediaMTX on the streaming PC with OBS. You can also run this app and MediaMTX on a separate LAN PC (the relay PC), with OBS on the streaming PC.
- Settings in `config.yaml` on the relay PC:
  - `obs.host`: The streaming PC's address (obs-websocket uses port 4455).
  - `stream.obs_read_host`: The relay PC's LAN address (the address the streaming PC's OBS uses to read RTSP).
  - If router port forwarding changes the external ports, set the external ports in `stream.publish_rtmp_port` / `stream.publish_srt_port`.
- Firewall rules (allow LAN access only on both PCs):
  - Streaming PC: obs-websocket on 4455/TCP.
  - Relay PC: RTSP on 8554/TCP.
- LAN traffic is not encrypted. The obs-websocket connection and OBS's RTSP read (including read credentials) can be intercepted on the LAN. Do not use this setup on a LAN with untrusted people.
- Hosting on an external server (such as a VPS) is technically possible over a VPN such as Tailscale, but is not officially supported. Do not expose it directly to the internet.

## Configuration (main `config.yaml` settings)

| Setting | Default | Purpose |
|---|---|---|
| `switch.fail_kbps` / `low_sec` | 800 / 5 | Switch to standby if the bitrate stays below this value for this many seconds. You can override it per camera with `cameras[].switch`. |
| `switch.recover_kbps` / `recover_sec` | 1500 / 10 | Return to the streaming scene after the bitrate stays at or above this value for this many seconds. |
| `switch.down_sec` | 2 | Time without incoming video before switching to the standby scene. |
| `stream.publish_host` | empty | Host in sender URLs. Empty detects a LAN address automatically. Set this if the forwarded or tunnel address is known. |
| `stream.app` | `live` | First part of the MediaMTX path (the app name). The path becomes `live/<camera id>`. This is for apps such as Larix and IRL Pro that split the URL into an “app name” and a “key”. Set it to `""` for the previous one-part path. Allowed characters are ASCII letters, digits, `_` and `-`. Restart after changing it. |
| `stream.obs_read_host` | empty | Host in OBS URLs. Empty uses `127.0.0.1` when running on the streaming PC. When using a relay PC on the LAN, set the relay PC's LAN address. |
| `stream.publish_rtmp_port` / `publish_srt_port` | `0` / `0` | RTMP and SRT ports in sender URLs. Zero uses the receiving port (`mediamtx.rtmp_port` / `srt_port`). Set these when router port forwarding changes the external ports. |
| `admin.port` / `admin.lan` | `8787` / `false` | Management API port and whether to accept connections from LAN/VPN. `false` listens only on `127.0.0.1`. Set to `true` for access from another PC or VPN. |
| `language` | `auto` | UI language: `auto` (OS language for the small window, browser language for the management page), `ja`, or `en`. In the management page, the browser-side language choice takes priority. |
| `obs.standby_scene` | `Standby` | Standby scene shown while the connection is poor. |
| `obs.scene_fallbacks` | empty | List of fallback scenes (`scene`: original scene, `fallback`: destination). Scenes not listed fall back to `obs.standby_scene`. You can chain three or more levels by using another row's `scene` as a `fallback` (you may also specify `standby_scene` explicitly). Each `scene` must appear once. Do not make a loop or use the standby scene as a `scene`. Use the exact OBS scene names, including leading and trailing spaces. Update the names if you rename scenes in OBS. If you change `standby_scene` and save while the old standby name is still in this list, a warning appears. That old name remains as a regular scene. |
| `obs.studio_mode_hide` | `standby` | How `on_bad: hide` cameras behave while OBS Studio Mode is on: `standby` (switch to standby instead of hiding), `ignore` (do nothing; disrupted video may be shown), or `hide` (hide; this affects the stream only when OBS “Duplicate scene” is off). |
| `cameras` | one `main` camera | Camera list (`id`, `name`, `on_bad`, and optional `switch`). See “Multiple cameras” above. |
| `mediamtx.rtmp_enabled` / `srt_enabled` | `true` / `true` | Accept RTMP and SRT input. You can disable an unused protocol. Restart for changes to take effect. |
| `switch.auto_enabled` | `true` | Turn automatic switching on or off. |
| `on_stream_start.enabled` / `scene` | `false` / empty | Whether to switch scenes when streaming starts, and which scene to use. |
| `log.dir` / `keep_days` | `logs` / 14 | Log folder and retention period in days. |

With `on_bad: hide` and OBS Studio Mode's default (“Duplicate scene” on), hiding an item in the program scene does not affect the stream while Studio Mode is on. It takes effect on the next scene transition. This app cannot read this OBS setting, so choose how `hide` cameras behave while Studio Mode is on with `obs.studio_mode_hide`. The default `standby` is safe because it switches to the standby scene. Choose `hide` only if OBS “Duplicate scene” is off. When Studio Mode is off, all values use the usual hide behavior. While Studio Mode is on, `status.warnings` reports the current behavior (switching to standby, doing nothing, or hiding). With `ignore`, items already hidden by the app are not restored just because Studio Mode is turned on. They are restored even in Studio Mode if the camera becomes OK, streaming stops, `switch.auto_enabled` becomes false, or `on_bad` changes to `standby`.

Studio Mode is not recommended for outdoor streams. You cannot operate OBS from outside, and while Studio Mode is on, hiding may not affect the stream (`standby` or `ignore`) or may depend on the “Duplicate scene” setting (`hide`). When OBS turns Studio Mode off, it briefly switches program to the preview scene and then switches back. The app ignores this brief switch as a manual change, so the standby owner and return scene are preserved.

See the comments in the `config.yaml` created on first start for all settings.

## Language

`language` accepts `auto` (the default), `ja`, or `en`. The management page remembers the language selected with its on-screen switch in that browser, and gives that choice priority. With `auto`, it uses the browser language. Logs are always in English, regardless of this setting.

## Security

- Sending and reading video each require a username and password. They are generated randomly on first start.
- Forward only 1935/TCP and 8890/UDP on your router. Do not forward the management API (8787 by default), RTSP (8554), or the MediaMTX API (9997). Use a VPN to open the management page from outside.
- If the management API is exposed to the LAN, its traffic is not encrypted (a warning is shown at startup).
- Passwords in SRT stream IDs and RTMP URLs are sent without encryption.

## Known limitations

- If the OBS WebSocket connection succeeds but repeatedly fails before authentication completes (for example, OBS is frozen and does not respond, or the connection drops during authentication), one small processing task (goroutine) remains in the app on each reconnect. This is a bug in the goobs library and has been reported upstream ([andreykaipov/goobs#277](https://github.com/andreykaipov/goobs/issues/277)). This does not happen when OBS is not running or when the password is wrong. Restart the app if this is a concern.
- See Chapter 13 of the design document for details.

## Development

```sh
go test -race ./...
```

The design is in `docs/superpowers/specs/2026-11-18-core-design.md`. The task list is in `docs/superpowers/plans/`.

## License

[MIT License](LICENSE)

See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for the bundled MediaMTX (MIT License, copyright holder: aler9) and the licenses for the libraries used. irl-live-keeper is not official software from the MediaMTX author.

The admin password starts empty. On the same PC, open `http://127.0.0.1:8787/` (or your `admin.port`) and enter a password of at least 8 characters twice. On Windows, use “Open admin page” in the window. On Linux / macOS, you can also set `admin.password` in `config.yaml` and restart. LAN, VPN and Basic authentication are unavailable until setup is complete. Existing passwords continue to work.
