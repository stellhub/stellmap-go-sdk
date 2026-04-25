GOFILES := $(shell git ls-files '*.go')

fmt:
	gofmt -w $(GOFILES)

test:
	go test ./...

release-check: fmt test
