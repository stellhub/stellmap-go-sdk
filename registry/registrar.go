package registry

import (
	"context"
	"errors"
	"sync"
	"time"
)

// RegistrarOption 表示自动注册器可选项。
type RegistrarOption func(*Registrar)

// WithHeartbeatInterval 设置心跳间隔。
func WithHeartbeatInterval(interval time.Duration) RegistrarOption {
	return func(registrar *Registrar) {
		if interval > 0 {
			registrar.heartbeatInterval = interval
		}
	}
}

// WithRetryInterval 设置失败后的重试间隔。
func WithRetryInterval(interval time.Duration) RegistrarOption {
	return func(registrar *Registrar) {
		if interval > 0 {
			registrar.retryInterval = interval
		}
	}
}

// WithDeregisterOnStop 设置停止时是否自动注销。
func WithDeregisterOnStop(enabled bool) RegistrarOption {
	return func(registrar *Registrar) {
		registrar.deregisterOnStop = enabled
	}
}

// WithHeartbeatErrorHandler 设置心跳错误回调。
func WithHeartbeatErrorHandler(handler func(error)) RegistrarOption {
	return func(registrar *Registrar) {
		registrar.onError = handler
	}
}

// Registrar 负责自动注册、续约和优雅注销。
type Registrar struct {
	client  *Client
	request RegisterRequest

	heartbeatInterval time.Duration
	retryInterval     time.Duration
	deregisterOnStop  bool
	onError           func(error)

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
	err     error
}

// NewRegistrar 创建自动注册器。
func NewRegistrar(client *Client, request RegisterRequest, options ...RegistrarOption) (*Registrar, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}

	request, err := normalizeRegisterRequest(request)
	if err != nil {
		return nil, err
	}

	registrar := &Registrar{
		client:            client,
		request:           request,
		heartbeatInterval: EffectiveHeartbeatInterval(request.LeaseTTLSeconds),
		retryInterval:     time.Second,
		deregisterOnStop:  true,
		done:              make(chan struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(registrar)
		}
	}
	if registrar.heartbeatInterval <= 0 {
		registrar.heartbeatInterval = EffectiveHeartbeatInterval(request.LeaseTTLSeconds)
	}
	if registrar.retryInterval <= 0 {
		registrar.retryInterval = time.Second
	}

	return registrar, nil
}

// Start 启动自动注册循环。
func (r *Registrar) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return errors.New("registrar already started")
	}
	r.mu.Unlock()

	if err := r.client.Register(ctx, r.request); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.cancel = cancel
	r.started = true
	r.mu.Unlock()
	go r.run(runCtx)
	return nil
}

// Stop 停止自动注册循环，并按配置执行注销。
func (r *Registrar) Stop(ctx context.Context) error {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return nil
	}
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()

	cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return r.Err()
	}
}

// Wait 等待注册器结束。
func (r *Registrar) Wait() error {
	<-r.done
	return r.Err()
}

// Done 返回结束信号。
func (r *Registrar) Done() <-chan struct{} {
	return r.done
}

// Err 返回最后错误。
func (r *Registrar) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *Registrar) run(ctx context.Context) {
	defer close(r.done)
	registered := true

	ticker := time.NewTicker(r.heartbeatInterval)
	defer ticker.Stop()

	heartbeatRequest := HeartbeatRequest{
		Namespace:        r.request.Namespace,
		Service:          r.request.Service,
		Organization:     r.request.Organization,
		BusinessDomain:   r.request.BusinessDomain,
		CapabilityDomain: r.request.CapabilityDomain,
		Application:      r.request.Application,
		Role:             r.request.Role,
		InstanceID:       r.request.InstanceID,
		LeaseTTLSeconds:  r.request.LeaseTTLSeconds,
	}

	for {
		select {
		case <-ctx.Done():
			r.finish(ctx, registered)
			return
		case <-ticker.C:
			if err := r.client.Heartbeat(ctx, heartbeatRequest); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
					r.finish(ctx, registered)
					return
				}
				r.handleError(err)

				var apiErr *APIError
				if errors.As(err, &apiErr) && apiErr.Code == "not_found" {
					registered = false
					for {
						if err := r.client.Register(ctx, r.request); err != nil {
							if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
								r.finish(ctx, registered)
								return
							}
							r.handleError(err)
							if !sleepWithContext(ctx, r.retryInterval) {
								r.finish(ctx, registered)
								return
							}
							continue
						}
						registered = true
						break
					}
					continue
				}

				if !sleepWithContext(ctx, r.retryInterval) {
					r.finish(ctx, registered)
					return
				}
			}
		}
	}
}

func (r *Registrar) finish(ctx context.Context, registered bool) {
	var finalErr error
	if r.deregisterOnStop && registered {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := r.client.Deregister(stopCtx, DeregisterRequest{
			Namespace:        r.request.Namespace,
			Service:          r.request.Service,
			Organization:     r.request.Organization,
			BusinessDomain:   r.request.BusinessDomain,
			CapabilityDomain: r.request.CapabilityDomain,
			Application:      r.request.Application,
			Role:             r.request.Role,
			InstanceID:       r.request.InstanceID,
		})
		if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
			finalErr = err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = false
	r.cancel = nil
	if finalErr != nil {
		r.err = finalErr
	}
}

func (r *Registrar) handleError(err error) {
	if err == nil {
		return
	}
	if r.onError != nil {
		r.onError(err)
	}
}
