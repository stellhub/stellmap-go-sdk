package main

import (
	"context"
	"log"
	"time"

	"github.com/stellhub/stellmap-go-sdk/framework"
	"github.com/stellhub/stellmap-go-sdk/registry"
)

type app struct {
	components []framework.Component
}

func (a *app) AddComponent(component framework.Component) {
	if component == nil {
		return
	}
	a.components = append(a.components, component)
}

func (a *app) Start(ctx context.Context) error {
	for _, component := range a.components {
		log.Printf("start component=%s", component.Name())
		if err := component.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) Stop(ctx context.Context) error {
	for i := len(a.components) - 1; i >= 0; i-- {
		component := a.components[i]
		log.Printf("stop component=%s", component.Name())
		if err := component.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	client, err := registry.NewClient("http://127.0.0.1:8080")
	if err != nil {
		log.Fatal(err)
	}

	registryComponent, err := framework.NewRegistryComponent(client, framework.StaticProvider{
		Request: registry.RegisterRequest{
			Namespace:  "prod",
			Service:    "company.trade.order.order-center.api",
			InstanceID: "order-center-api-10.0.1.23",
			Endpoints: []registry.Endpoint{
				{Name: "http", Protocol: "http", Host: "10.0.1.23", Port: 8080},
			},
		},
	}, registry.WithHeartbeatInterval(10*time.Second))
	if err != nil {
		log.Fatal(err)
	}

	application := &app{}
	application.AddComponent(registryComponent)

	ctx := context.Background()
	if err := application.Start(ctx); err != nil {
		log.Fatal(err)
	}

	time.Sleep(30 * time.Second)

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.Stop(stopCtx); err != nil {
		log.Fatal(err)
	}
}
