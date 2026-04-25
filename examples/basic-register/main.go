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

	request := registry.RegisterRequest{
		Namespace:        "prod",
		Organization:     "company",
		BusinessDomain:   "trade",
		CapabilityDomain: "order",
		Application:      "order-center",
		Role:             "api",
		InstanceID:       "order-center-api-10.0.1.23",
		Zone:             "az1",
		Endpoints: []registry.Endpoint{
			{Name: "http", Protocol: "http", Host: "10.0.1.23", Port: 8080},
			{Name: "grpc", Protocol: "grpc", Host: "10.0.1.23", Port: 9090},
		},
	}

	ctx := context.Background()
	if err := client.Register(ctx, request); err != nil {
		log.Fatal(err)
	}

	instances, err := client.QueryInstances(ctx, registry.QueryOptions{
		Namespace: "prod",
		Service:   "company.trade.order.order-center.api",
		Endpoint:  "http",
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("instances=%+v", instances)

	if err := client.Deregister(ctx, registry.DeregisterRequest{
		Namespace:        request.Namespace,
		Organization:     request.Organization,
		BusinessDomain:   request.BusinessDomain,
		CapabilityDomain: request.CapabilityDomain,
		Application:      request.Application,
		Role:             request.Role,
		InstanceID:       request.InstanceID,
	}); err != nil {
		log.Fatal(err)
	}
}
