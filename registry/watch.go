package registry

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Watcher 表示一个带自动重连能力的 watch 会话。
type Watcher struct {
	client *Client

	ctx    context.Context
	cancel context.CancelFunc

	options WatchOptions

	events chan WatchEvent
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func newWatcher(client *Client, parent context.Context, options WatchOptions) *Watcher {
	ctx, cancel := context.WithCancel(parent)
	watcher := &Watcher{
		client:  client,
		ctx:     ctx,
		cancel:  cancel,
		options: options,
		events:  make(chan WatchEvent, 128),
		done:    make(chan struct{}),
	}
	go watcher.run()
	return watcher
}

// Events 返回 watch 事件通道。
func (w *Watcher) Events() <-chan WatchEvent {
	return w.events
}

// Close 主动关闭 watch。
func (w *Watcher) Close() error {
	w.cancel()
	<-w.done
	return w.Err()
}

// Wait 等待 watch 结束。
func (w *Watcher) Wait() error {
	<-w.done
	return w.Err()
}

// Done 返回 watch 完成信号。
func (w *Watcher) Done() <-chan struct{} {
	return w.done
}

// Err 返回 watch 最终错误。
func (w *Watcher) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

func (w *Watcher) setErr(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = err
}

func (w *Watcher) run() {
	defer close(w.done)
	defer close(w.events)

	options := w.options
	if options.ReconnectInterval <= 0 {
		options.ReconnectInterval = w.client.watchReconnectInterval
	}
	if !options.AutoReconnect {
		options.AutoReconnect = true
	}

	sinceRevision := options.SinceRevision
	for {
		nextRevision, err := w.consumeStream(options, sinceRevision)
		if err == nil {
			return
		}
		if w.ctx.Err() != nil {
			return
		}

		var apiErr *APIError
		if errors.As(err, &apiErr) {
			if apiErr.IsRevisionExpired() {
				includeSnapshot := true
				if options.IncludeSnapshot != nil {
					includeSnapshot = *options.IncludeSnapshot
				}
				if !includeSnapshot {
					w.setErr(err)
					return
				}
				sinceRevision = 0
			} else if !isRetryableWatchStatus(apiErr.HTTPStatus) {
				w.setErr(err)
				return
			}
		} else {
			sinceRevision = nextRevision
		}

		if !options.AutoReconnect {
			w.setErr(err)
			return
		}

		if !sleepWithContext(w.ctx, options.ReconnectInterval) {
			return
		}
		if nextRevision > 0 {
			sinceRevision = nextRevision
		}
	}
}

func (w *Watcher) consumeStream(options WatchOptions, sinceRevision uint64) (uint64, error) {
	values, err := buildQueryValues(options.QueryOptions)
	if err != nil {
		return sinceRevision, err
	}
	if sinceRevision > 0 {
		values.Set("sinceRevision", strconv.FormatUint(sinceRevision, 10))
	}

	includeSnapshot := true
	if options.IncludeSnapshot != nil {
		includeSnapshot = *options.IncludeSnapshot
	}
	if !includeSnapshot {
		values.Set("includeSnapshot", "false")
	} else if sinceRevision > 0 {
		values.Set("includeSnapshot", "true")
	}

	headers, err := callerHeaders(options.Caller)
	if err != nil {
		return sinceRevision, err
	}
	if headers == nil {
		headers = make(map[string]string)
	}
	headers["Accept"] = "text/event-stream"

	response, err := w.client.doRawWithHTTPClient(w.ctx, requestSpec{
		Method: http.MethodGet,
		Path:   routeRegistryWatch,
		Query:  values,
		Header: headers,
	}, false, w.client.watchHTTPClient())
	if err != nil {
		return sinceRevision, err
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	currentRevision := sinceRevision
	for {
		message, err := readSSEMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return currentRevision, io.EOF
			}
			return currentRevision, err
		}
		if len(message.Data) == 0 || message.Event == "" || isWatchPingMessage(message) {
			continue
		}

		event, err := decodeWatchEvent(message)
		if err != nil {
			return currentRevision, err
		}
		if event.Revision > currentRevision {
			currentRevision = event.Revision
		}

		select {
		case <-w.ctx.Done():
			return currentRevision, w.ctx.Err()
		case w.events <- event:
		}
	}
}

type sseMessage struct {
	ID    string
	Event string
	Data  []string
}

func readSSEMessage(reader *bufio.Reader) (sseMessage, error) {
	var message sseMessage
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && len(strings.TrimSpace(line)) == 0 && len(message.Data) == 0 && message.Event == "" && message.ID == "" {
				return sseMessage{}, io.EOF
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" && (message.Event != "" || len(message.Data) > 0 || message.ID != "") {
				return message, nil
			}
			if err == io.EOF {
				if strings.TrimSpace(line) != "" {
					applySSELine(&message, line)
				}
				if message.Event != "" || len(message.Data) > 0 || message.ID != "" {
					return message, nil
				}
			}
			return sseMessage{}, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if message.Event == "" && len(message.Data) == 0 && message.ID == "" {
				continue
			}
			return message, nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		applySSELine(&message, line)
	}
}

func isWatchPingMessage(message sseMessage) bool {
	switch strings.ToLower(strings.TrimSpace(message.Event)) {
	case "ping", "heartbeat", "keepalive":
		return true
	default:
		return false
	}
}

func applySSELine(message *sseMessage, line string) {
	switch {
	case strings.HasPrefix(line, "id:"):
		message.ID = strings.TrimSpace(line[len("id:"):])
	case strings.HasPrefix(line, "event:"):
		message.Event = strings.TrimSpace(line[len("event:"):])
	case strings.HasPrefix(line, "data:"):
		value := line[len("data:"):]
		if strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		message.Data = append(message.Data, value)
	}
}

func decodeWatchEvent(message sseMessage) (WatchEvent, error) {
	var event WatchEvent
	payload := strings.Join(message.Data, "\n")
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return WatchEvent{}, fmt.Errorf("decode watch event payload: %w", err)
	}

	if event.Type == "" {
		event.Type = WatchEventType(message.Event)
	}
	if event.Revision == 0 && strings.TrimSpace(message.ID) != "" {
		revision, err := strconv.ParseUint(strings.TrimSpace(message.ID), 10, 64)
		if err == nil {
			event.Revision = revision
		}
	}

	return event, nil
}

func isRetryableWatchStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		duration = time.Second
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
