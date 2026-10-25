IMAGE ?= symbol-web-docker
CONTAINER ?= symbol-web-container
PORT ?= 8080

.PHONY: test check build image run stop clean clean-unused smoke monitor lint

test:
	go test -race ./...

check:
	test -z "$$(gofmt -l $$(find cmd internal -name '*.go'))"
	go vet ./...
	go test -race ./...

build:
	go build -o bin/server ./cmd/server
	go build -o bin/health-monitor ./cmd/health-monitor

image:
	docker image build -f Dockerfile -t $(IMAGE) .

run:
	docker container run --detach --name $(CONTAINER) --label io.tomorrow.project=symbol-web-dockerize --read-only --cap-drop ALL --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 1 -p $(PORT):8080 $(IMAGE)

stop:
	docker container stop $(CONTAINER)

# Удаляются только остановленные контейнеры и неиспользуемые образы проекта.
clean:
	docker container prune --force --filter label=io.tomorrow.project=symbol-web-dockerize
	docker image prune --all --force --filter label=io.tomorrow.project=symbol-web-dockerize

clean-unused: clean

smoke:
	bash scripts/docker-smoke.sh

monitor:
	go run ./cmd/health-monitor $(CONTAINER)

lint:
	go run ./cmd/health-monitor --lint Dockerfile
