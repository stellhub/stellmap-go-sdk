package main

import (
	"context"
	"log"
	"time"

	"github.com/stellhub/stellmap-go-sdk/registry"
)

func main() {
	client, err := registry.NewClient("http://127.0.0.1:8080")
	if err != nil {
		log.Fatal(err)
	}

	registrar, err := registry.NewRegistrar(client, registry.RegisterRequest{
		Namespace:  "prod",
		Service:    "company.trade.order.order-center.api",
		InstanceID: "order-center-api-10.0.1.23",
		Endpoints: []registry.Endpoint{
			{Name: "http", Protocol: "http", Host: "10.0.1.23", Port: 8080},
		},
	}, registry.WithHeartbeatInterval(10*time.Second))
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := registrar.Start(ctx); err != nil {
		log.Fatal(err)
	}

	time.Sleep(30 * time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := registrar.Stop(stopCtx); err != nil {
		log.Fatal(err)
	}
}
