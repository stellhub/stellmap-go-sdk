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

	watcher := client.WatchInstances(context.Background(), registry.WatchOptions{
		QueryOptions: registry.QueryOptions{
			Namespace: "prod",
			Service:   "company.trade.order.order-center.api",
			Endpoint:  "http",
		},
		Caller: &registry.CallerIdentity{
			Namespace: "prod",
			Service:   "company.trade.gateway.gateway.public-api",
		},
		ReconnectInterval: time.Second,
	})
	defer watcher.Close()

	for event := range watcher.Events() {
		log.Printf("event=%s revision=%d service=%s instanceId=%s", event.Type, event.Revision, event.Service, event.InstanceID)
	}
}
