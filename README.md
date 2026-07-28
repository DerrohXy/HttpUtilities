# HttpUtilities

A small Go package for making JSON / URL-encoded / multipart HTTP requests,
plus a callback-driven WebSocket client. Wraps `net/http` and
`github.com/gorilla/websocket`.

## Install

```bash
go get github.com/DerrohXy/HttpUtilities
go get github.com/gorilla/websocket
```

## HTTP Client

### Create a client

```go
client := httputilities.NewHttpClient("https://api.example.com")

client.SetHeaders(httputilities.Headers{
    "Authorization": "Bearer some-token",
})
```

- `SetHeaders` merges headers into the client's defaults; call again to add more.
- `ClearHeaders` wipes all default headers.
- Every request method takes a `params Headers` argument that gets appended to the URL as a query string via `BuildPath`.

### The `parseJson` parameter

Every request method takes a trailing `parseJson bool`:

| `parseJson` | Return type                                        | Use when...                                        |
| ----------- | -------------------------------------------------- | -------------------------------------------------- |
| `true`      | `any` (decoded JSON — `map[string]any` or `[]any`) | you just want to inspect/log the response          |
| `false`     | `[]byte` (raw body)                                | you want to decode into a specific struct yourself |

Because every method returns `any`, you'll need a type assertion either way.

### Example: raw `[]byte` → typed struct

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	httputilities "github.com/DerrohXy/HttpUtilities"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	client := httputilities.NewHttpClient("https://api.example.com")

	// parseJson = false -> get the raw response body
	raw, err := client.GetJson("users/1", nil, false)
	if err != nil {
		log.Fatalf("request failed: %v", err)
	}

	body, ok := raw.([]byte)
	if !ok {
		log.Fatalf("expected []byte, got %T", raw)
	}

	var user User
	if err := json.Unmarshal(body, &user); err != nil {
		log.Fatalf("failed to unmarshal into User: %v", err)
	}

	fmt.Printf("%+v\n", user)
}
```

### Example: generic decoded response

```go
// parseJson = true -> decode into map[string]any / []any for you
parsed, err := client.GetJson("users/1", nil, true)
if err != nil {
    log.Fatalf("request failed: %v", err)
}

fmt.Printf("%+v\n", parsed) // map[string]any{"id":1, "name":"...", ...}
```

### POST JSON

```go
result, err := client.PostJson(
    "users",
    httputilities.JSONObject{"name": "Ada", "email": "ada@example.com"},
    nil,   // no query params
    false, // return raw []byte
)
```

### POST URL-encoded

```go
result, err := client.PostUrlEncoded(
    "login",
    httputilities.JSONObject{"username": "ada", "password": "secret"},
    nil,
    true,
)
```

### POST multipart form

```go
result, err := client.PostForm(
    "upload",
    httputilities.FormFields{"caption": "hello world"},
    nil,
    true,
)
```

> `FormFields` only supports plain text fields — no file uploads out of the box.

### Package-level functions

If you don't need a persistent client (headers, base URL), the underlying
functions are exported directly:

```go
result, err := httputilities.MakeGetJsonRequest(
    "https://api.example.com/users/1",
    httputilities.Headers{"Authorization": "Bearer token"},
    true,
)
```

Available: `MakeGetJsonRequest`, `MakePostJsonRequest`, `MakePostFormRequest`,
`MakePostUrlEncodedRequest`, and `BuildPath` for constructing URLs with query
params.

## WebSocket Client

### Create and connect

```go
ws := httputilities.NewWebsocketClient(
    "wss://echo.websocket.org",
    func(conn *websocket.Conn) {
        fmt.Println("connected:", conn.RemoteAddr())
    },
    func(conn *websocket.Conn, err error) {
        fmt.Println("error:", err)
    },
    func(conn *websocket.Conn, code int, text string) {
        fmt.Println("closed:", code, text)
    },
    func(conn *websocket.Conn, messageType int, data []byte) {
        fmt.Println("message:", string(data))
    },
)

if err := ws.Connect(); err != nil {
    log.Fatal(err)
}
defer ws.Close()
```

- `OnOpen` fires once, right after a successful dial.
- `OnMessage` fires for every incoming frame (runs in a background goroutine started by `Connect`).
- `OnClose` fires exactly once, whether the peer closed cleanly or you called `Close()` yourself.
- `OnError` fires for dial failures or unexpected read/write errors — not for ordinary closes.

### Sending messages

```go
err := ws.Send(websocket.TextMessage, []byte("hello"))
```

### Checking state / waiting for shutdown

```go
if ws.IsConnected() {
    // ...
}

<-ws.Done() // blocks until the read loop has exited
```

## Notes / Limitations

- No built-in request timeouts — `http.DefaultClient` and
  `websocket.DefaultDialer` are used as-is. Wrap calls with your own
  `context.Context`/timeout handling if you need bounded requests.
- HTTP responses are returned regardless of status code (mirrors `fetch()`
  behavior) — check the decoded body yourself for error payloads.
- `WebsocketClient` supports one connection per instance; call
  `NewWebsocketClient` again for a new connection after `Close()`.
