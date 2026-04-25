package framework

import (
	"context"
	"fmt"
	"sync"

	"github.com/stellhub/stellmap-go-sdk/registry"
)

// Component 表示可以挂载到业务框架中的组件。
type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// RegistrationProvider 负责在启动时构造注册请求。
type RegistrationProvider interface {
	BuildRegistration(context.Context) (registry.RegisterRequest, error)
}

// RegistryComponent 表示 StellMap 注册中心组件。
type RegistryComponent struct {
	client   *registry.Client
	provider RegistrationProvider
	options  []registry.RegistrarOption

	mu        sync.Mutex
	registrar *registry.Registrar
}

// StaticProvider 用固定请求构造注册信息。
type StaticProvider struct {
	Request registry.RegisterRequest
}

// BuildRegistration 返回固定注册请求。
func (p StaticProvider) BuildRegistration(context.Context) (registry.RegisterRequest, error) {
	return p.Request, nil
}

// NewRegistryComponent 创建注册中心组件。
func NewRegistryComponent(client *registry.Client, provider RegistrationProvider, options ...registry.RegistrarOption) (*RegistryComponent, error) {
	if client == nil {
		return nil, fmt.Errorf("client is nil")
	}
	if provider == nil {
		return nil, fmt.Errorf("provider is nil")
	}

	return &RegistryComponent{
		client:   client,
		provider: provider,
		options:  options,
	}, nil
}

// Name 返回组件名。
func (c *RegistryComponent) Name() string {
	return "stellmap-registry"
}

// Start 在框架启动时执行注册和心跳任务。
func (c *RegistryComponent) Start(ctx context.Context) error {
	request, err := c.provider.BuildRegistration(ctx)
	if err != nil {
		return err
	}

	registrar, err := registry.NewRegistrar(c.client, request, c.options...)
	if err != nil {
		return err
	}
	if err := registrar.Start(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	c.registrar = registrar
	c.mu.Unlock()
	return nil
}

// Stop 在框架停止时执行优雅注销。
func (c *RegistryComponent) Stop(ctx context.Context) error {
	c.mu.Lock()
	registrar := c.registrar
	c.registrar = nil
	c.mu.Unlock()

	if registrar == nil {
		return nil
	}
	return registrar.Stop(ctx)
}
