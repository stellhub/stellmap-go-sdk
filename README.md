# StellMap Go SDK

`stellmap-go-sdk` 是 `StellMap` 注册中心的 Go 客户端 SDK。

当前仓库已经按职责拆成多个 package：

- `registry`：公共注册发现客户端
- `framework`：给自研业务框架做生命周期接入的适配层

## 安装

```bash
go get github.com/stellhub/stellmap-go-sdk
```

## Package 说明

### 1. registry

用于业务服务接入注册中心的公共能力：

- 注册
- 注销
- 心跳
- 实例查询
- SSE watch
- 自动注册与优雅注销

示例：

- [examples/basic-register/main.go](examples/basic-register/main.go)
- [examples/watch/main.go](examples/watch/main.go)
- [examples/auto-registrar/main.go](examples/auto-registrar/main.go)

基础用法：

```go
package main

import (
	"context"
	"log"

	"github.com/stellhub/stellmap-go-sdk/registry"
)

func main() {
	client, err := registry.NewClient("http://127.0.0.1:8080")
	if err != nil {
		log.Fatal(err)
	}

	err = client.Register(context.Background(), registry.RegisterRequest{
		Namespace:  "prod",
		Service:    "company.trade.order.order-center.api",
		InstanceID: "order-center-api-10.0.1.23",
		Endpoints: []registry.Endpoint{
			{Name: "http", Protocol: "http", Host: "10.0.1.23", Port: 8080},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

### 2. framework

这个 package 不是绑定某个现成框架，而是给你自研框架提供“组件模型”的接入层。

核心接口如下：

```go
type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
```

示例：

- [examples/custom-framework/main.go](examples/custom-framework/main.go)

推荐你的框架核心具备类似能力：

```go
type App interface {
	AddComponent(component framework.Component)
}
```

然后 `framework` 包会提供一个可直接挂进去的组件：

```go
component, err := framework.NewRegistryComponent(client, provider, options...)
```

推荐方式是：

- 业务服务直接使用 `registry`
- 你的自研框架在内部接入 `framework`

## 设计说明

### not_leader 自动重试

公共写请求遇到服务端 `503 not_leader` 时，SDK 会根据返回里的 `leaderAddr` 自动重试一次。

### 本地一致性校验

SDK 会在客户端侧先校验：

- `namespace/service/instanceId` 必填约束
- 五段式服务标识一致性
- endpoint 合法性
- 默认 `leaseTtlSeconds`
- 默认 endpoint `weight`

### watch 断线续传

`registry.WatchInstances` 支持基于 `sinceRevision` 的自动重连恢复。

SDK 当前只保留面向客户端的公共接口，不包含注册中心服务端之间使用的 internal 接口。

### 包入口

当前 SDK 不再保留根包兼容层，统一推荐显式使用子包：

- `github.com/stellhub/stellmap-go-sdk/registry`
- `github.com/stellhub/stellmap-go-sdk/framework`

## 发布准备

仓库里已经补充：

- [VERSION](VERSION)
- [CHANGELOG.md](CHANGELOG.md)
- [RELEASE.md](RELEASE.md)
- [Makefile](Makefile)

当前建议发布版本：`v0.1.0`

注意：当前仓库还没有任何 git commit，因此还不适合直接打正式 tag。正确顺序是先提交当前代码，再按 [RELEASE.md](RELEASE.md) 打 `v0.1.0` 标签。
