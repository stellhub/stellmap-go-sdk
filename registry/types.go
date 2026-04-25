package registry

import (
	"encoding/json"
	"time"
)

const (
	// DefaultLeaseTTLSeconds 与服务端默认租约 TTL 保持一致。
	DefaultLeaseTTLSeconds int64 = 30
	// DefaultEndpointWeight 与服务端默认端点权重保持一致。
	DefaultEndpointWeight int32 = 100
)

// Endpoint 表示实例对外暴露的一个协议端点。
type Endpoint struct {
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int32  `json:"port"`
	Path     string `json:"path,omitempty"`
	Weight   int32  `json:"weight,omitempty"`
}

// RegisterRequest 表示实例注册请求。
type RegisterRequest struct {
	Namespace        string            `json:"namespace"`
	Service          string            `json:"service,omitempty"`
	Organization     string            `json:"organization,omitempty"`
	BusinessDomain   string            `json:"businessDomain,omitempty"`
	CapabilityDomain string            `json:"capabilityDomain,omitempty"`
	Application      string            `json:"application,omitempty"`
	Role             string            `json:"role,omitempty"`
	InstanceID       string            `json:"instanceId"`
	Zone             string            `json:"zone,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	Endpoints        []Endpoint        `json:"endpoints"`
	LeaseTTLSeconds  int64             `json:"leaseTtlSeconds,omitempty"`
}

// DeregisterRequest 表示实例注销请求。
type DeregisterRequest struct {
	Namespace        string `json:"namespace"`
	Service          string `json:"service,omitempty"`
	Organization     string `json:"organization,omitempty"`
	BusinessDomain   string `json:"businessDomain,omitempty"`
	CapabilityDomain string `json:"capabilityDomain,omitempty"`
	Application      string `json:"application,omitempty"`
	Role             string `json:"role,omitempty"`
	InstanceID       string `json:"instanceId"`
}

// HeartbeatRequest 表示实例心跳请求。
type HeartbeatRequest struct {
	Namespace        string `json:"namespace"`
	Service          string `json:"service,omitempty"`
	Organization     string `json:"organization,omitempty"`
	BusinessDomain   string `json:"businessDomain,omitempty"`
	CapabilityDomain string `json:"capabilityDomain,omitempty"`
	Application      string `json:"application,omitempty"`
	Role             string `json:"role,omitempty"`
	InstanceID       string `json:"instanceId"`
	LeaseTTLSeconds  int64  `json:"leaseTtlSeconds,omitempty"`
}

// Instance 表示服务端返回的实例候选项。
type Instance struct {
	Namespace         string            `json:"namespace"`
	Service           string            `json:"service"`
	Organization      string            `json:"organization,omitempty"`
	BusinessDomain    string            `json:"businessDomain,omitempty"`
	CapabilityDomain  string            `json:"capabilityDomain,omitempty"`
	Application       string            `json:"application,omitempty"`
	Role              string            `json:"role,omitempty"`
	InstanceID        string            `json:"instanceId"`
	Zone              string            `json:"zone,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Endpoints         []Endpoint        `json:"endpoints,omitempty"`
	LeaseTTLSeconds   int64             `json:"leaseTtlSeconds"`
	RegisteredAtUnix  int64             `json:"registeredAtUnix"`
	LastHeartbeatUnix int64             `json:"lastHeartbeatUnix"`
}

// QueryOptions 表示实例查询条件。
type QueryOptions struct {
	Namespace        string
	Service          string
	Services         []string
	ServicePrefix    string
	ServicePrefixes  []string
	Organization     string
	BusinessDomain   string
	CapabilityDomain string
	Application      string
	Role             string
	Zone             string
	Endpoint         string
	Selectors        []string
	Labels           []string
	Limit            int
}

// CallerIdentity 表示 watch 调用方身份。
type CallerIdentity struct {
	Namespace        string
	Service          string
	Organization     string
	BusinessDomain   string
	CapabilityDomain string
	Application      string
	Role             string
}

// WatchOptions 表示 watch 订阅参数。
type WatchOptions struct {
	QueryOptions
	SinceRevision     uint64
	IncludeSnapshot   *bool
	Caller            *CallerIdentity
	AutoReconnect     bool
	ReconnectInterval time.Duration
}

// WatchEventType 表示 watch 事件类型。
type WatchEventType string

const (
	WatchEventSnapshot WatchEventType = "snapshot"
	WatchEventUpsert   WatchEventType = "upsert"
	WatchEventDelete   WatchEventType = "delete"
)

// WatchEvent 表示 registry watch 事件。
type WatchEvent struct {
	Revision         uint64         `json:"revision"`
	Type             WatchEventType `json:"type"`
	Namespace        string         `json:"namespace,omitempty"`
	Service          string         `json:"service,omitempty"`
	Organization     string         `json:"organization,omitempty"`
	BusinessDomain   string         `json:"businessDomain,omitempty"`
	CapabilityDomain string         `json:"capabilityDomain,omitempty"`
	Application      string         `json:"application,omitempty"`
	Role             string         `json:"role,omitempty"`
	InstanceID       string         `json:"instanceId,omitempty"`
	Instance         *Instance      `json:"instance,omitempty"`
	Instances        []Instance     `json:"instances,omitempty"`
}

type successEnvelope struct {
	Code      string          `json:"code"`
	Message   string          `json:"message,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
}

type successEnvelopeRaw struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	Data      any    `json:"data,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

type apiErrorResponse struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	RequestID  string `json:"requestId,omitempty"`
	LeaderID   uint64 `json:"leaderId,omitempty"`
	LeaderAddr string `json:"leaderAddr,omitempty"`
}
