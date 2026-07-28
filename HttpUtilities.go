package httputilities

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type JSONObject map[string]any
type Headers map[string]string

// FormFields represents simple text fields for a multipart form request.
// File uploads are not supported; extend MakePostFormRequest if needed.
type FormFields map[string]string

const NO_STATUS_CODE = -1

func MakeGetRequest(
	endpoint string,
	headers Headers,
	parseJson bool,
) (any, int, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, NO_STATUS_CODE, err
	}
	applyHeaders(req, headers)

	return doAndDecodeRequest(req, parseJson)
}

func MakePostJsonRequest(
	endpoint string,
	data JSONObject,
	headers Headers,
	parseJson bool,
) (any, int, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, NO_STATUS_CODE, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, NO_STATUS_CODE, err
	}

	merged := mergeHeaders(headers, Headers{"Content-Type": "application/json"})
	applyHeaders(req, merged)

	return doAndDecodeRequest(req, parseJson)
}

func MakePostFormRequest(
	endpoint string,
	data FormFields,
	headers Headers,
	parseJson bool,
) (any, int, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for key, value := range data {
		if err := writer.WriteField(key, value); err != nil {
			return nil, NO_STATUS_CODE, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, NO_STATUS_CODE, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, NO_STATUS_CODE, err
	}

	merged := mergeHeaders(headers, Headers{"Content-Type": writer.FormDataContentType()})
	applyHeaders(req, merged)

	return doAndDecodeRequest(req, parseJson)
}

func MakePostUrlEncodedRequest(
	endpoint string,
	data JSONObject,
	headers Headers,
	parseJson bool,
) (any, int, error) {
	values := url.Values{}
	for key, value := range data {
		values.Set(key, fmt.Sprintf("%v", value))
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, NO_STATUS_CODE, err
	}

	merged := mergeHeaders(headers, Headers{"Content-Type": "application/x-www-form-urlencoded"})
	applyHeaders(req, merged)

	return doAndDecodeRequest(req, parseJson)
}

func BuildPath(
	hostOrigin string,
	path string,
	params Headers,
) string {
	if !strings.HasSuffix(hostOrigin, "/") {
		hostOrigin += "/"
	}
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	if len(params) == 0 {
		return hostOrigin + path
	}

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}
	query := values.Encode()
	if query == "" {
		return hostOrigin + path
	}

	return fmt.Sprintf("%s%s?%s", hostOrigin, path, query)
}

type HttpClient struct {
	HostOrigin string
	mu         sync.RWMutex
	headers    Headers
}

func NewHttpClient(hostOrigin string) *HttpClient {
	return &HttpClient{
		HostOrigin: hostOrigin,
		headers:    Headers{},
	}
}

func (instance *HttpClient) Headers() Headers {
	instance.mu.RLock()
	defer instance.mu.RUnlock()

	out := make(Headers, len(instance.headers))
	for k, v := range instance.headers {
		out[k] = v
	}

	return out
}

func (instance *HttpClient) SetHeaders(headers Headers) {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	for k, v := range headers {
		instance.headers[k] = v
	}
}

func (instance *HttpClient) ClearHeaders() {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	instance.headers = Headers{}
}

func (instance *HttpClient) PostJson(
	path string,
	data JSONObject,
	params Headers,
	parseJson bool,
) (any, int, error) {
	fullPath := BuildPath(instance.HostOrigin, path, params)
	return MakePostJsonRequest(fullPath, data, instance.Headers(), parseJson)
}

func (instance *HttpClient) PostUrlEncoded(
	path string,
	data JSONObject,
	params Headers,
	parseJson bool,
) (any, int, error) {
	fullPath := BuildPath(instance.HostOrigin, path, params)
	return MakePostUrlEncodedRequest(fullPath, data, instance.Headers(), parseJson)
}

func (instance *HttpClient) PostForm(
	path string,
	data FormFields,
	params Headers,
	parseJson bool,
) (any, int, error) {
	fullPath := BuildPath(instance.HostOrigin, path, params)
	return MakePostFormRequest(fullPath, data, instance.Headers(), parseJson)
}

func (instance *HttpClient) Get(
	path string,
	params Headers,
	parseJson bool,
) (any, int, error) {
	fullPath := BuildPath(instance.HostOrigin, path, params)
	return MakeGetRequest(fullPath, instance.Headers(), parseJson)
}

func applyHeaders(req *http.Request, headers Headers) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

// mergeHeaders returns a new Headers map containing every entry from
// headers, filled in with any entries from defaults that aren't already
// present. Entries already in headers always win.
func mergeHeaders(headers Headers, defaults Headers) Headers {
	merged := make(Headers, len(headers)+len(defaults))
	for k, v := range headers {
		merged[k] = v
	}
	for k, v := range defaults {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	return merged
}

func doAndDecodeRequest(req *http.Request, parseJson bool) (any, int, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, NO_STATUS_CODE, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if !parseJson {
		return body, resp.StatusCode, nil
	}

	if len(body) == 0 {
		return nil, resp.StatusCode, nil
	}

	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		return body, resp.StatusCode, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return result, resp.StatusCode, nil
}

// Websocket client

type OnOpenFunc func(conn *websocket.Conn)
type OnErrorFunc func(conn *websocket.Conn, err error)
type OnCloseFunc func(conn *websocket.Conn, code int, text string)
type OnMessageFunc func(conn *websocket.Conn, messageType int, data []byte)

type WebsocketClient struct {
	URL    string
	Header http.Header

	OnOpen    OnOpenFunc
	OnError   OnErrorFunc
	OnClose   OnCloseFunc
	OnMessage OnMessageFunc

	mu        sync.Mutex
	conn      *websocket.Conn
	connected bool
	closing   bool
	done      chan struct{}
}

func NewWebsocketClient(
	url string,
	onOpen OnOpenFunc,
	onError OnErrorFunc,
	onClose OnCloseFunc,
	onMessage OnMessageFunc,
) *WebsocketClient {
	return &WebsocketClient{
		URL:       url,
		OnOpen:    onOpen,
		OnError:   onError,
		OnClose:   onClose,
		OnMessage: onMessage,
	}
}

func (instance *WebsocketClient) Connect() error {
	instance.mu.Lock()
	if instance.connected {
		instance.mu.Unlock()
		return errors.New("wsclient: already connected")
	}
	instance.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(instance.URL, instance.Header)
	if err != nil {
		if instance.OnError != nil {
			instance.OnError(nil, err)
		}

		return err
	}

	instance.mu.Lock()
	instance.conn = conn
	instance.connected = true
	instance.closing = false
	instance.done = make(chan struct{})
	instance.mu.Unlock()

	// NOTE: we deliberately do NOT register a SetCloseHandler here.
	// gorilla/websocket invokes the close handler AND still returns the
	// *websocket.CloseError from ReadMessage in readLoop below, so having
	// both would fire OnClose twice for the same event. readLoop's error
	// branch is the single source of truth for OnClose.

	if instance.OnOpen != nil {
		instance.OnOpen(conn)
	}

	go instance.readLoop()

	return nil
}

func (instance *WebsocketClient) readLoop() {
	defer func() {
		instance.mu.Lock()
		instance.connected = false
		if instance.done != nil {
			close(instance.done)
		}
		instance.mu.Unlock()
	}()

	for {
		messageType, data, err := instance.conn.ReadMessage()
		if err != nil {
			instance.mu.Lock()
			selfClosed := instance.closing
			instance.mu.Unlock()

			switch {
			case selfClosed:
				// The local Close() call tore down the connection; report
				// this as a normal close, not a read error.
				if instance.OnClose != nil {
					instance.OnClose(instance.conn, websocket.CloseNormalClosure, "")
				}
			default:
				if closeErr, ok := err.(*websocket.CloseError); ok {
					if instance.OnClose != nil {
						instance.OnClose(instance.conn, closeErr.Code, closeErr.Text)
					}
				} else if instance.OnError != nil {
					instance.OnError(instance.conn, err)
				}
			}

			return
		}

		if instance.OnMessage != nil {
			instance.OnMessage(instance.conn, messageType, data)
		}
	}
}

func (instance *WebsocketClient) Close() error {
	instance.mu.Lock()
	if !instance.connected || instance.conn == nil {
		instance.mu.Unlock()
		return errors.New("wsclient: connection not open")
	}

	conn := instance.conn
	instance.closing = true
	instance.mu.Unlock()

	writeErr := conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	if writeErr != nil && instance.OnError != nil {
		instance.OnError(conn, writeErr)
	}

	closeErr := conn.Close()

	instance.mu.Lock()
	instance.connected = false
	instance.mu.Unlock()

	return closeErr
}

func (instance *WebsocketClient) Send(
	messageType int,
	data []byte,
) error {
	instance.mu.Lock()
	if !instance.connected || instance.conn == nil {
		instance.mu.Unlock()
		return errors.New("wsclient: connection not open")
	}
	conn := instance.conn
	instance.mu.Unlock()

	if err := conn.WriteMessage(messageType, data); err != nil {
		if instance.OnError != nil {
			instance.OnError(conn, err)
		}

		return err
	}

	return nil
}

func (instance *WebsocketClient) IsConnected() bool {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	return instance.connected
}

// Done returns a channel that is closed once the read loop has exited
// (i.e. the connection is no longer being read from, whether due to a
// clean close, a remote close, or an error).
func (instance *WebsocketClient) Done() <-chan struct{} {
	instance.mu.Lock()
	defer instance.mu.Unlock()

	return instance.done
}
