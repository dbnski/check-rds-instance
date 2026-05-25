BINARY      := check-rds-instance
VERSION     := $(shell cat VERSION)
COMMIT_HASH := $(shell git rev-parse --short HEAD)
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOARCH      ?= $(shell go env GOARCH)

LDFLAGS  := -w -s
LDFLAGS  += -X main.Version=$(VERSION)
LDFLAGS  += -X main.CommitHash=$(COMMIT_HASH)
LDFLAGS  += -X main.BuildTime=$(BUILD_TIME)

all:
	@echo "Building $(BINARY) $(VERSION) ($(COMMIT_HASH)) for target architecture $(GOARCH)"
	GOARCH=$(GOARCH) go build -a -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...
