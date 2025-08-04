BINARY=github-copilot-svcs
VERSION ?= dev

all: build

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) .

run: build
	./$(BINARY) run

auth:
	./$(BINARY) auth

models:
	./$(BINARY) models

config:
	./$(BINARY) config

clean:
	rm -f $(BINARY)
