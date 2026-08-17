GO := go

.PHONY: build run
build:
	$(GO) build -o hperf main.go

run:
	$(GO) run main.go run "$(RUN_ARGS)"
