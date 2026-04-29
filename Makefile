VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=$(VERSION)
BUILD_FLAGS := -ldflags "$(LDFLAGS)"

BINARIES := easywall-core easywall-web

.PHONY: all build css test lint vuln clean install release

## Build both binaries
all: build

## Build CSS from Tailwind source
css:
	npx @tailwindcss/cli -i web/src/app.css -o web/static/style.css --minify

build: css
	@echo "Building $(VERSION)..."
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o bin/easywall-core ./cmd/easywall-core
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o bin/easywall-web  ./cmd/easywall-web

## Cross-compile for Linux amd64 + arm64
release:
	@mkdir -p dist
	GOOS=linux GOARCH=amd64  CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/easywall-core-linux-amd64  ./cmd/easywall-core
	GOOS=linux GOARCH=amd64  CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/easywall-web-linux-amd64   ./cmd/easywall-web
	GOOS=linux GOARCH=arm64  CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/easywall-core-linux-arm64  ./cmd/easywall-core
	GOOS=linux GOARCH=arm64  CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/easywall-web-linux-arm64   ./cmd/easywall-web

## Run all tests with race detector
test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./internal/...
	go tool cover -html=coverage.out -o coverage.html

## Lint via golangci-lint
lint:
	golangci-lint run

## Security vulnerability scan
vuln:
	govulncheck ./...

## Security linter
gosec:
	gosec -fmt text ./...

## Update i18n messages (requires go-i18n CLI)
i18n-extract:
	goi18n extract -outdir locales -format json ./...
	goi18n merge  -outdir locales -format json locales/en.json

## Install to system (requires root)
install: build
	install -d /usr/sbin /usr/share/easywall /etc/easywall
	install -m 0755 bin/easywall-core /usr/sbin/
	install -m 0755 bin/easywall-web  /usr/sbin/
	cp -r web locales /usr/share/easywall/
	install -m 0644 systemd/easywall-core.service /lib/systemd/system/
	install -m 0644 systemd/easywall-web.service  /lib/systemd/system/
	systemctl daemon-reload

## Docker image
docker:
	docker build --build-arg VERSION=$(VERSION) -t easywall:$(VERSION) .

## Build Debian package
deb:
	dpkg-buildpackage -us -uc -b

clean:
	rm -rf bin/ dist/ coverage.out coverage.html
