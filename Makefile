default: build

# Compile the provider.
build:
	go build ./...

# Run unit tests.
test:
	go test ./... -timeout 120s

# Run acceptance tests (creates real resources; needs TF_ACC=1 and NETCUP_*).
testacc:
	TF_ACC=1 go test ./... -timeout 120m

# Static analysis.
lint:
	golangci-lint run ./...

# Generate the registry documentation. Needs only Go (tfplugindocs and terraform are fetched automatically).
generate:
	go generate ./...

.PHONY: build test testacc lint generate
