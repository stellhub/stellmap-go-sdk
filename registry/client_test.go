package registry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRegisterRetriesNotLeader(t *testing.T) {
	t.Parallel()

	var leaderRequests atomic.Int32
	leader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaderRequests.Add(1)
		if r.URL.Path != routeRegistryRegister {
			t.Fatalf("unexpected leader path: %s", r.URL.Path)
		}

		var request RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Service != "company.trade.order.order-center.api" {
			t.Fatalf("unexpected service: %s", request.Service)
		}
		if request.LeaseTTLSeconds != DefaultLeaseTTLSeconds {
			t.Fatalf("unexpected lease ttl: %d", request.LeaseTTLSeconds)
		}
		if len(request.Endpoints) != 1 || request.Endpoints[0].Weight != DefaultEndpointWeight {
			t.Fatalf("unexpected endpoints: %+v", request.Endpoints)
		}

		writeJSONResponse(t, w, http.StatusOK, map[string]any{
			"code":    "ok",
			"message": "instance registered",
		})
	}))
	defer leader.Close()

	leaderURL, err := url.Parse(leader.URL)
	if err != nil {
		t.Fatalf("parse leader url: %v", err)
	}

	var followerRequests atomic.Int32
	follower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followerRequests.Add(1)
		writeJSONResponse(t, w, http.StatusServiceUnavailable, apiErrorResponse{
			Code:       "not_leader",
			Message:    "current node is not leader",
			LeaderAddr: leaderURL.Host,
		})
	}))
	defer follower.Close()

	client, err := NewClient(follower.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	err = client.Register(context.Background(), RegisterRequest{
		Namespace:        "prod",
		Organization:     "company",
		BusinessDomain:   "trade",
		CapabilityDomain: "order",
		Application:      "order-center",
		Role:             "api",
		InstanceID:       "order-center-api-1",
		Endpoints: []Endpoint{
			{Protocol: "http", Host: "127.0.0.1", Port: 8080},
		},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if followerRequests.Load() != 1 {
		t.Fatalf("expected 1 follower request, got %d", followerRequests.Load())
	}
	if leaderRequests.Load() != 1 {
		t.Fatalf("expected 1 leader request, got %d", leaderRequests.Load())
	}
}

func TestQueryInstancesBuildsStructuredPrefixQuery(t *testing.T) {
	t.Parallel()

	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query()
		writeJSONResponse(t, w, http.StatusOK, map[string]any{
			"code": "ok",
			"data": []Instance{
				{
					Namespace:       "prod",
					Service:         "company.trade.order.order-center.api",
					InstanceID:      "order-center-api-1",
					LeaseTTLSeconds: 30,
				},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	items, err := client.QueryInstances(context.Background(), QueryOptions{
		Namespace:        "prod",
		Organization:     "company",
		BusinessDomain:   "trade",
		CapabilityDomain: "order",
		Selectors:        []string{"color=gray", "version in (v2)"},
		Labels:           []string{"label=color=gray"},
		Limit:            20,
	})
	if err != nil {
		t.Fatalf("query instances: %v", err)
	}

	if len(items) != 1 || items[0].InstanceID != "order-center-api-1" {
		t.Fatalf("unexpected query result: %+v", items)
	}
	if captured.Get("namespace") != "prod" {
		t.Fatalf("unexpected namespace query: %s", captured.Get("namespace"))
	}
	if captured.Get("servicePrefix") != "company.trade.order" {
		t.Fatalf("unexpected servicePrefix query: %s", captured.Get("servicePrefix"))
	}
	if captured.Get("limit") != "20" {
		t.Fatalf("unexpected limit query: %s", captured.Get("limit"))
	}
	if len(captured["selector"]) != 2 {
		t.Fatalf("unexpected selector query: %+v", captured["selector"])
	}
	if len(captured["label"]) != 1 {
		t.Fatalf("unexpected label query: %+v", captured["label"])
	}
}

func TestWatcherReconnectsWithSinceRevision(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	var lastSinceRevision atomic.Uint64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 2 {
			since, _ := strconv.ParseUint(r.URL.Query().Get("sinceRevision"), 10, 64)
			lastSinceRevision.Store(since)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		if count == 1 {
			_, _ = w.Write([]byte("id: 12\n"))
			_, _ = w.Write([]byte("event: snapshot\n"))
			_, _ = w.Write([]byte("data: {\"revision\":12,\"type\":\"snapshot\",\"namespace\":\"prod\",\"service\":\"company.trade.order.order-center.api\",\"instances\":[{\"namespace\":\"prod\",\"service\":\"company.trade.order.order-center.api\",\"instanceId\":\"order-center-api-1\",\"leaseTtlSeconds\":30}]}\n\n"))
			return
		}

		_, _ = w.Write([]byte("id: 13\n"))
		_, _ = w.Write([]byte("event: upsert\n"))
		_, _ = w.Write([]byte("data: {\"revision\":13,\"type\":\"upsert\",\"namespace\":\"prod\",\"service\":\"company.trade.order.order-center.api\",\"instanceId\":\"order-center-api-2\",\"instance\":{\"namespace\":\"prod\",\"service\":\"company.trade.order.order-center.api\",\"instanceId\":\"order-center-api-2\",\"leaseTtlSeconds\":30}}\n\n"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, WithWatchReconnectInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	watcher := client.WatchInstances(ctx, WatchOptions{
		QueryOptions: QueryOptions{
			Namespace: "prod",
			Service:   "company.trade.order.order-center.api",
		},
		ReconnectInterval: 10 * time.Millisecond,
	})

	var events []WatchEvent
	for len(events) < 2 {
		select {
		case event, ok := <-watcher.Events():
			if !ok {
				t.Fatalf("watcher closed unexpectedly: %v", watcher.Err())
			}
			events = append(events, event)
		case <-ctx.Done():
			t.Fatalf("wait watch events timeout: %v", ctx.Err())
		}
	}

	if err := watcher.Close(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("close watcher: %v", err)
	}

	if events[0].Type != WatchEventSnapshot || events[0].Revision != 12 {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Type != WatchEventUpsert || events[1].Revision != 13 {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if lastSinceRevision.Load() != 12 {
		t.Fatalf("unexpected sinceRevision on reconnect: %d", lastSinceRevision.Load())
	}
}

func TestWatcherIgnoresPingEvents(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: ping\n"))
		_, _ = w.Write([]byte("data: {}\n\n"))
		_, _ = w.Write([]byte("id: 14\n"))
		_, _ = w.Write([]byte("event: upsert\n"))
		_, _ = w.Write([]byte("data: {\"revision\":14,\"type\":\"upsert\",\"namespace\":\"prod\",\"service\":\"company.trade.order.order-center.api\",\"instanceId\":\"order-center-api-2\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, WithWatchReconnectInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	watcher := client.WatchInstances(ctx, WatchOptions{
		QueryOptions: QueryOptions{
			Namespace: "prod",
			Service:   "company.trade.order.order-center.api",
		},
	})
	defer watcher.Close()

	select {
	case event, ok := <-watcher.Events():
		if !ok {
			t.Fatalf("watcher closed unexpectedly: %v", watcher.Err())
		}
		if event.Type != WatchEventUpsert || event.Revision != 14 {
			t.Fatalf("unexpected event after ping: %+v", event)
		}
	case <-ctx.Done():
		t.Fatalf("wait watch event timeout: %v", ctx.Err())
	}
}

func TestWatcherDisablesHTTPClientOverallTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("id: 15\n"))
		_, _ = w.Write([]byte("event: upsert\n"))
		_, _ = w.Write([]byte("data: {\"revision\":15,\"type\":\"upsert\",\"namespace\":\"prod\",\"service\":\"company.trade.order.order-center.api\",\"instanceId\":\"order-center-api-3\"}\n\n"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, WithHTTPClient(&http.Client{Timeout: 30 * time.Millisecond}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	watcher := client.WatchInstances(ctx, WatchOptions{
		QueryOptions: QueryOptions{
			Namespace: "prod",
			Service:   "company.trade.order.order-center.api",
		},
		ReconnectInterval: 10 * time.Millisecond,
	})
	defer watcher.Close()

	select {
	case event, ok := <-watcher.Events():
		if !ok {
			t.Fatalf("watcher closed unexpectedly: %v", watcher.Err())
		}
		if event.Type != WatchEventUpsert || event.Revision != 15 {
			t.Fatalf("unexpected watch event: %+v", event)
		}
	case <-ctx.Done():
		t.Fatalf("wait watch event timeout: %v", ctx.Err())
	}
}

func TestWatcherReconnectsAfterIdleTimeout(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	client, err := NewClient("http://stellmap.local", WithHTTPClient(&http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if requestCount.Add(1) == 1 {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       timeoutReadCloser{},
					Request:    r,
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					"id: 16\n" +
						"event: upsert\n" +
						"data: {\"revision\":16,\"type\":\"upsert\",\"namespace\":\"prod\",\"service\":\"company.trade.order.order-center.api\",\"instanceId\":\"order-center-api-4\"}\n\n",
				)),
				Request: r,
			}, nil
		}),
	}))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	watcher := client.WatchInstances(ctx, WatchOptions{
		QueryOptions: QueryOptions{
			Namespace: "prod",
			Service:   "company.trade.order.order-center.api",
		},
		ReconnectInterval: 10 * time.Millisecond,
	})
	defer watcher.Close()

	select {
	case event, ok := <-watcher.Events():
		if !ok {
			t.Fatalf("watcher closed unexpectedly: %v", watcher.Err())
		}
		if event.Type != WatchEventUpsert || event.Revision != 16 {
			t.Fatalf("unexpected watch event after reconnect: %+v", event)
		}
	case <-ctx.Done():
		t.Fatalf("wait watch event timeout: %v", ctx.Err())
	}
	if requestCount.Load() < 2 {
		t.Fatalf("expected reconnect after idle timeout, got %d request(s)", requestCount.Load())
	}
}

func TestRegistrarSendsHeartbeatAndDeregister(t *testing.T) {
	t.Parallel()

	var registerCount atomic.Int32
	var heartbeatCount atomic.Int32
	var deregisterCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case routeRegistryRegister:
			registerCount.Add(1)
		case routeRegistryHeartbeat:
			heartbeatCount.Add(1)
		case routeRegistryDeregister:
			deregisterCount.Add(1)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		writeJSONResponse(t, w, http.StatusOK, map[string]any{
			"code": "ok",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	registrar, err := NewRegistrar(client, RegisterRequest{
		Namespace:  "prod",
		Service:    "company.trade.order.order-center.api",
		InstanceID: "order-center-api-1",
		Endpoints: []Endpoint{
			{Name: "http", Protocol: "http", Host: "127.0.0.1", Port: 8080, Weight: 100},
		},
	}, WithHeartbeatInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("new registrar: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := registrar.Start(ctx); err != nil {
		t.Fatalf("start registrar: %v", err)
	}

	waitUntil(t, time.Second, func() bool {
		return heartbeatCount.Load() >= 2
	})

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := registrar.Stop(stopCtx); err != nil {
		t.Fatalf("stop registrar: %v", err)
	}

	if registerCount.Load() != 1 {
		t.Fatalf("unexpected register count: %d", registerCount.Load())
	}
	if heartbeatCount.Load() < 2 {
		t.Fatalf("unexpected heartbeat count: %d", heartbeatCount.Load())
	}
	if deregisterCount.Load() != 1 {
		t.Fatalf("unexpected deregister count: %d", deregisterCount.Load())
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type timeoutReadCloser struct{}

func (timeoutReadCloser) Read([]byte) (int, error) {
	return 0, timeoutError{}
}

func (timeoutReadCloser) Close() error {
	return nil
}

type timeoutError struct{}

func (timeoutError) Error() string {
	return "idle timeout"
}

func (timeoutError) Timeout() bool {
	return true
}

func (timeoutError) Temporary() bool {
	return true
}

func waitUntil(t *testing.T, timeout time.Duration, predicate func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, status int, payload any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode json response: %v", err)
	}
}
