PWD := $(shell pwd)
ifeq '${CI}' 'true'
VERSION ?= dev
else
VERSION ?= $(shell git describe --tags)
endif
TAG ?= "minio/operator:$(VERSION)"
LDFLAGS ?= "-s -w -X main.Version=$(VERSION)"

GOPATH := $(shell go env GOPATH)
GOARCH := $(shell go env GOARCH)
GOOS := $(shell go env GOOS)

KUSTOMIZE_HOME=operator-kustomize
KUSTOMIZE_CRDS=$(KUSTOMIZE_HOME)/crds/

PLUGIN_HOME=kubectl-minio

LOGSEARCHAPI=logsearchapi
LOGSEARCHAPI_TAG ?= "minio/logsearchapi:$(VERSION)"

all: build logsearchapi

getdeps:
	@echo "Checking dependencies"
	@mkdir -p ${GOPATH}/bin
	@which golangci-lint 1>/dev/null || (echo "Installing golangci-lint" && curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.27.0)

verify: getdeps govet gotest lint

build: regen-crd verify plugin
	@CGO_ENABLED=0 GOOS=linux go build -trimpath --ldflags $(LDFLAGS) -o minio-operator
	@docker build -t $(TAG) .

install: all
	@docker push $(TAG)

lint:
	@echo "Running $@ check"
	@GO111MODULE=on golangci-lint cache clean
	@GO111MODULE=on golangci-lint run --timeout=5m --config ./.golangci.yml

govet:
	@go vet ./...

gotest:
	@go test -race ./...

clean:
	@echo "Cleaning up all the generated files"
	@find . -name '*.test' | xargs rm -fv
	@find . -name '*~' | xargs rm -fv
	@find . -name '*.zip' | xargs rm -fv
	@rm -rf dist/

regen-crd:
	@GO111MODULE=on go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1
	@controller-gen crd:trivialVersions=true paths="./..." output:crd:artifacts:config=$(KUSTOMIZE_CRDS)

statik:
	@echo "Building static assets"
	@GO111MODULE=on go get github.com/rakyll/statik
	@statik -src=$(KUSTOMIZE_HOME) -dest $(PLUGIN_HOME) -f

plugin: regen-crd
	@echo "Building 'kubectl-minio' binary"
	@(cd $(PLUGIN_HOME); go build -o kubectl-minio main.go)

.PHONY: logsearchapi
logsearchapi:
	@echo "Building 'logsearchapi' binary"
	@(cd $(LOGSEARCHAPI); \
		go vet ./... && \
		go test -race ./... && \
		GO111MODULE=on ${GOPATH}/bin/golangci-lint cache clean && \
		GO111MODULE=on ${GOPATH}/bin/golangci-lint run --timeout=5m --config ../.golangci.yml && \
		go build && \
		docker build -t $(LOGSEARCHAPI_TAG) . \
   )

getconsoleuiyaml:
	@echo "Getting the latest Console UI"
	@kustomize build github.com/minio/console/k8s/operator-console/base > operator-kustomize/console-ui.yaml
	@make statik
	@echo "Done"
