GO := go

build:
	$(GO) build -o hperf main.go

run:
	$(GO) run main.go run "$(RUN_ARGS)"

test:
	$(GO) test -cover -race -shuffle=on -count=1 ./...
