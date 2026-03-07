# Monitor Agent Makefile
BINARY_NAME=agent
MAIN_PATH=./cmd/agent

.PHONY: build build-darwin build-linux clean run test

build:
	go build -o $(BINARY_NAME) $(MAIN_PATH)

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)

build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 $(MAIN_PATH)

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-darwin-amd64 $(BINARY_NAME)-darwin-arm64 $(BINARY_NAME)-linux-amd64

run: build
	./$(BINARY_NAME) -config=./config/config.yaml

test:
	go test ./...
