default: build

# Compile the provider.
build:
	go build ./...

# Run unit tests.
test:
	go test ./... -timeout 120s

# Run acceptance tests against the mock API. Needs no credentials.
testmock:
	TF_ACC=1 go test ./internal/provider -run TestMock -timeout 20m

# Run acceptance tests against the real API. Needs NETCUP_TEST_*, see README.
testlive:
	TF_ACC=1 go test ./internal/provider -run TestAcc -timeout 120m

# Static analysis.
lint:
	golangci-lint run ./...

# Generate the registry documentation. Needs only Go (tfplugindocs and terraform are fetched automatically).
generate:
	go generate ./...

.PHONY: build test testmock testlive lint generate
