# goobs への報告の文案（未送信。ユーザーの了承を得てから投稿する。2026-11-20）

投稿先: https://github.com/andreykaipov/goobs/issues/new
タイトル: `New` leaks the connection and goroutines when the handshake fails or times out

---

**Version:** v1.11.1 (the same code paths exist since v1.0.0)

**What happens**

When the WebSocket connection is established but authentication does not complete (for example, OBS is hung and never sends `Hello`, or the `Identified` reply arrives after `ResponseTimeout`), `goobs.New` returns an error, but:

1. `connect()` returns `timeout waiting for authentication` without closing `c.conn`, and `New` returns `nil, err`, so the caller cannot call `Disconnect()`. The TCP connection stays open.
2. Even if the caller closes the underlying `net.Conn` itself (e.g. through a custom `WithDialer`), `handleRawServerMessages` gets a plain read error (`use of closed network connection`), falls into the `default:` branch, logs `Unhandled error`, and returns **without calling `markDisconnected()`**. `c.client.Opcodes` is never closed, so the goroutine running `handleOpcodes` stays blocked on `range c.client.Opcodes` forever.
3. `authComplete` is unbuffered. If `Hello`/`Identified` (or the 4009 close) arrives after `connect()` has already returned on the timer, `handleOpcodes` / `handleRawServerMessages` block forever on `auth <- ...`.

An application that retries the connection every few seconds while OBS is unresponsive accumulates one or more goroutines (and, without a custom dialer, one TCP connection) per attempt.

It does not happen when OBS is not running (dial fails before any goroutine starts) or when the password is wrong (OBS sends close code 4009, which goes through the `markDisconnected()` path).

**How to reproduce**

A server that accepts the WebSocket upgrade but never sends `Hello`:

```go
up := websocket.Upgrader{}
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    c, _ := up.Upgrade(w, r, nil)
    defer c.Close()
    for { if _, _, err := c.ReadMessage(); err != nil { return } }
}))
defer srv.Close()

before := runtime.NumGoroutine()
for i := 0; i < 10; i++ {
    _, err := goobs.New(strings.TrimPrefix(srv.URL, "http://"),
        goobs.WithResponseTimeoutDuration(200*time.Millisecond))
    // err: timeout waiting for authentication
    _ = err
}
time.Sleep(time.Second)
fmt.Println(runtime.NumGoroutine() - before) // grows with the number of attempts
```

Measured with Go 1.27.4 on Linux: 10 attempts leave **30** extra goroutines (client reader, opcode handler, and the server-side handler, because the connection is never closed). With a custom `WithDialer` that closes the recorded `net.Conn` after `New` fails, 10 attempts still leave **10** goroutines (the `handleOpcodes` goroutine, see point 2).

Minor, related: the timeout error prints the duration as nanoseconds with an `ms` suffix (`timeout waiting for authentication: 200000000ms` for a 200 ms timeout), because `%dms` is applied to a `time.Duration`.

**Suggested fix**

- In `connect()`, when returning an error after a successful dial (timeout or auth error), call `c.markDisconnected()` and `c.conn.Close()`.
- In `handleRawServerMessages`, call `c.markDisconnected()` for every read error before returning (not only for `*websocket.CloseError` and `ErrCloseSent`).
- Make `authComplete` buffered (`make(chan error, 1)`), so a late result never blocks the sender.

We currently work around (1) by passing a custom dialer and closing the recorded `net.Conn` ourselves, but (2) and (3) cannot be fixed from outside the package.
