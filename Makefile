VERSION = 0.1.0
TARGET = secret-injector
SOURCE = $(shell find . -type f -name "*.go" -not -name "*_test.go")

IMAGE_NAME = masa213f/secret-injector
IMAGE_TAG = $(VERSION)


# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build

setup:
	go get -u golang.org/x/tools/cmd/goimports
	go get -u golang.org/x/lint/golint

mod:
	go mod tidy
	go mod vendor

build: mod $(TARGET)

$(TARGET): go.mod $(SOURCE)
	CGO_ENABLED=0 go build -o ./bin/$@ ./cmd/$@

clean:
	-rm bin/$(TARGET)

manifests:
	controller-gen rbac:roleName=secret-injector webhook paths="./hooks"

image-build: $(TARGET)
	docker build . -t $(IMAGE_PREFIX)$(IMAGE_NAME):$(IMAGE_TAG)

image-push: image-build
	docker push $(IMAGE_PREFIX)$(IMAGE_NAME):$(IMAGE_TAG)

image-clean:
	-docker image rm $(IMAGE_PREFIX)$(IMAGE_NAME):$(IMAGE_TAG)

distclean: clean image-clean

fmt:
	goimports -w $$(find . -type d -name 'vendor' -prune -o -type f -name '*.go' -print)

test:
	test -z "$$(goimports -l $$(find . -type d -name 'vendor' -prune -o -type f -name '*.go' -print) | tee /dev/stderr)"
	test -z "$$(golint $$(go list ./... | grep -v '/vendor/') | tee /dev/stderr)"
	CGO_ENABLED=0 go test -v ./...

.PHONY: all setup mod build clean manifests image-build image-push image-clean distclean fmt test
