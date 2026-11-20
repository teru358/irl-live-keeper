# goobs への報告の文案（未送信。ユーザーの了承を得てから投稿する。2026-11-20）

投稿先: https://github.com/andreykaipov/goobs/issues/new
タイトル: `New` leaks goroutines (and the connection) when the handshake fails or times out

---

**Version:** v1.11.1 (the same code paths exist in every v1.x release we checked: v1.0.0, v1.2.3, v1.3.0, v1.4.1, v1.5.0, v1.7.0, v1.8.0, v1.9.1, v1.10.0, v1.11.0)

First, a disclosure: this report comes from a small hobby project that is being "vibe coded", i.e. built mostly by AI agents under a human's direction. The project is a Go tool that watches an incoming IRL stream and switches OBS scenes when the connection gets bad. This issue text was also written by Claude (Anthropic) on the maintainer's behalf. Everything below was checked against the goobs source and reproduced on a real machine, but please read it with that context in mind.

## How we found it

- While Claude was implementing the OBS connection wrapper, we asked an OpenAI Codex model (gpt-6.1-sol) to review the code with the question "can any path leave goroutines or connections behind?". It read `client.go` and pointed out that `connect()` returns on the auth timeout without closing `c.conn`.
- We worked around that by passing our own `WithDialer` and closing the recorded `net.Conn` ourselves. A second Codex pass (gpt-6.1-terra) then found that even with the connection closed, the opcode goroutine is still left behind (point 2 below).
- Claude Opus 5.5 then read the relevant lines in v1.11.1 and the older releases listed above, and reproduced the leak with the program below.

## What happens

When the WebSocket connection is established but authentication does not complete (OBS is hung and never sends `Hello`, the `Identified` reply comes after `ResponseTimeout`, or the connection drops before auth finishes), `goobs.New` returns an error, and:

1. `connect()` returns `timeout waiting for authentication` without closing `c.conn`, and `New` returns `nil, err`, so the caller cannot call `Disconnect()`. The TCP connection stays open, together with the reader and opcode goroutines.
2. Even if the caller closes the underlying `net.Conn` (for example through a custom `WithDialer`), `handleRawServerMessages` gets a plain read error (`use of closed network connection`), takes the `default:` branch, logs `Unhandled error`, and returns **without calling `markDisconnected()`**. `c.client.Opcodes` is never closed, so the goroutine running `handleOpcodes` stays blocked on `range c.client.Opcodes` forever. The same happens if the connection drops with an ordinary read error before auth finishes.
3. `authComplete` is unbuffered. If `Identified` arrives, writing `Identify` fails, or the 4009 close arrives after `connect()` has already returned on the timer, `handleOpcodes` / `handleRawServerMessages` block forever on `auth <- ...`.

An application that retries every few seconds while OBS is unresponsive accumulates goroutines on every attempt (in our case, one retry every 5 s, so several hundred per hour).

It does not happen when OBS is not running (the dial fails before any goroutine starts), or when the password is wrong (OBS sends close code 4009, which goes through the `markDisconnected()` path while `connect()` is still waiting).

## How to reproduce

A server that accepts the WebSocket upgrade but never sends `Hello`:

```go
up := websocket.Upgrader{}
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	c, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	for {
		if _, _, err := c.ReadMessage(); err != nil {
			return
		}
	}
}))
defer srv.Close()

before := runtime.NumGoroutine()
for i := 0; i < 10; i++ {
	_, err := goobs.New(strings.TrimPrefix(srv.URL, "http://"),
		goobs.WithResponseTimeoutDuration(200*time.Millisecond))
	_ = err // timeout waiting for authentication
}
time.Sleep(time.Second)
fmt.Println(runtime.NumGoroutine() - before)
```

Measured with Go 1.27.4 on Linux:

| Case | Extra goroutines after 10 failed attempts |
|---|---|
| goobs as is | 30 (reader, opcode handler, and the server-side handler, because the connection is never closed) |
| With a custom `WithDialer` that closes the recorded `net.Conn` after `New` fails | 10 (the `handleOpcodes` goroutine, point 2) |

Minor, related: the timeout error prints the duration as nanoseconds with an `ms` suffix (`timeout waiting for authentication: 200000000ms` for a 200 ms timeout), because `%dms` is applied to a `time.Duration`.

## Possible fix (not verified, just a direction)

We have not written or tested a patch, so please take this as a sketch only:

- When `connect()` fails after a successful dial, close the connection and tell the reader / opcode goroutines to stop, so the caller does not need to.
- In `handleRawServerMessages`, stop the opcode loop on every read error, not only on `*websocket.CloseError` and `ErrCloseSent`.
- Make sure the auth result never blocks its sender (for example, a buffered channel plus sending at most once, or a `select` on a done channel).

One caveat the second review pointed out: `c.client.Opcodes` has more than one sender (`handleRawServerMessages`, and the goroutine started on `Hello` that sends `Identify`). Simply calling `markDisconnected()` (which closes `Opcodes`) from `connect()` could race with those senders and panic on a closed channel. The senders probably need to select on a done channel, or the channel should only be closed after they have finished.

## Our workaround

We pass a custom `*websocket.Dialer` through `WithDialer` that records the `net.Conn`, close it when `New` fails or our context is cancelled, and document the remaining goroutine (point 2) as a known limitation until it is fixed here.

Thanks for maintaining goobs. It has been very pleasant to use otherwise.
