GO_VERSION := $(shell go version | awk '{split($$3,a,"go"); print a[2]}')
BUILD_OUTPUT := go-trace
BUILD_OUTPUT_DARWIN := main

.PHONY: update-go-version install-deps update-deps default build-darwin run-local-test test

# Task: Get the current Go version and modify go.mod
update-go-version:
	go mod edit -go=$(GO_VERSION)

# Task: Installs the dependencies
install-deps: update-deps
	go mod tidy

# Task: Updates the dependencies
update-deps: update-go-version
	go get -u ./...

# Task: Default task, installs dependencies and builds the app (darwin/arm64)
default: install-deps
	go build -o $(BUILD_OUTPUT) -ldflags="-w -s" -tags netgo .

# Task: Build for Mac (darwin/arm64)
build-darwin: install-deps
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s" -gcflags "all=-N -l" -tags netgo -o $(BUILD_OUTPUT_DARWIN) .

# Task: Run locally (using go run) on port tcp/8080
run-local-test:
	CGO_ENABLED=0 SERVE=tcp PORT=8080 HOST=127.0.0.1 go run .

# Task: Run tests
test: install-deps
	go test ./...

