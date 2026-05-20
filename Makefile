PROTO_DIR   := proto
GEN_DIR     := gen
GOPATH      := $(shell go env GOPATH)
GOBIN       := $(shell go env GOBIN)
PROTOC_GEN_GO      := $(GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(GOPATH)/bin/protoc-gen-go-grpc
MOCKGEN     := $(GOBIN)/mockgen
MOCK_DIR    := _mock

.PHONY: proto proto-install mock mock-install

## mock-install: install mockgen tool
mock-install:
	go install go.uber.org/mock/mockgen@latest

## mock: regenerate all mocks from source interfaces
mock:
	@echo "Generating mocks..."
	$(MOCKGEN) \
		-source=internal/reservation/repository/init.go \
		-destination=$(MOCK_DIR)/reservation/mock_repository.go \
		-package=mockreservation \
		-mock_names=Reservation=MockReservationRepository
	$(MOCKGEN) \
		-source=internal/reservation/usecase/init.go \
		-destination=$(MOCK_DIR)/reservation/mock_usecase.go \
		-package=mockreservation \
		-mock_names=Reservation=MockReservationUsecase
	$(MOCKGEN) \
		-source=pkg/paymentclient/client.go \
		-destination=$(MOCK_DIR)/pkg/paymentclient/mock_paymentclient.go \
		-package=mockpaymentclient \
		-mock_names=PaymentService=MockPaymentService
	@echo "Done."

## proto-install: install protoc-gen-go and protoc-gen-go-grpc plugins
proto-install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

## proto: regenerate Go code from all .proto files under proto/
proto:
	@echo "Generating proto files..."
	@find $(PROTO_DIR) -name "*.proto" | while read proto_file; do \
		protoc \
			--proto_path=$(PROTO_DIR) \
			--go_out=$(GEN_DIR) \
			--go_opt=paths=source_relative \
			--go-grpc_out=$(GEN_DIR) \
			--go-grpc_opt=paths=source_relative \
			--plugin=protoc-gen-go=$(PROTOC_GEN_GO) \
			--plugin=protoc-gen-go-grpc=$(PROTOC_GEN_GO_GRPC) \
			$$(echo $$proto_file | sed 's|$(PROTO_DIR)/||'); \
	done
	@echo "Done."

mod-reset:
	rm -rf go.sum
	go clean --modcache
	go mod tidy

mod-tidy:
	go mod tidy

run:
	go run cmd/main.go

build:
	@env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/reservation cmd/main.go

check-cognitive-complexity:
	find . -type f -name '*.go' -not -name "mock*.go" \
	-exec gocognit -over 15 {} \;

unit-test-coverage:
	go test -v -covermode=count ./... -coverprofile=coverage.cov
	go tool cover -func=coverage.cov

test:
	go test -v ./...

test-unit:
	go test -v ./internal/reservation/usecase/... ./internal/reservation/handler/... ./internal/reservation/repository/...

gen-mock-source:
	$(MOCKGEN) -package=${pkg} -destination=$(destination) -source=${source}

docker-build: build
	docker build -f Dockerfile -t payment-migration:v2.0 .

docker-run:
	docker run -it --rm --env-file example.env --memory="3g" --cpus="1" -p 8080:8080 --name payment-migration payment-migration:v2.0

golint:
	golangci-lint run --timeout 5m --output.code-climate.path stdout

gosec:
	gosec -exclude=G401,G304,G501,G505 -fmt=sonarqube -out=sonar-gosec.json ./...
