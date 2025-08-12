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

# --- Docker ---
IMAGE ?= gh-copilot-svcs:$(VERSION)
PLATFORMS ?= linux/amd64

docker-build:
	docker buildx build --platform $(PLATFORMS) --build-arg VERSION=$(VERSION) -t $(IMAGE) . --load

docker-run:
	docker run --rm -p 8081:8081 -v ghcs-data:/home/nonroot/.local/share/github-copilot-svcs $(IMAGE)

docker-auth:
	docker run --rm -it -v ghcs-data:/home/nonroot/.local/share/github-copilot-svcs $(IMAGE) auth

docker-status:
	docker run --rm -v ghcs-data:/home/nonroot/.local/share/github-copilot-svcs $(IMAGE) status
