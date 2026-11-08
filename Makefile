PLUGIN_ID ?= omc-doubao-oauth
VERSION ?= 0.1.0
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
EXT = $(if $(filter darwin,$(GOOS)),dylib,$(if $(filter windows,$(GOOS)),dll,so))
LDFLAGS = -s -w -X main.pluginVersion=$(VERSION)

.PHONY: test vet build package clean

test:
	cd go && go test ./...
	cd go && go vet ./...

vet:
	cd go && go vet ./...

build:
	mkdir -p dist
	cd go && CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildmode=c-shared -ldflags "$(LDFLAGS)" -o ../dist/$(PLUGIN_ID).$(EXT) .
	rm -f dist/$(PLUGIN_ID).h

package: build
	cd dist && zip -r $(PLUGIN_ID)_$(VERSION)_$(GOOS)_$(GOARCH).zip $(PLUGIN_ID).$(EXT) ../README.md ../README.zh-CN.md ../LICENSE
	python3 -c "import hashlib,sys; p='dist/$(PLUGIN_ID)_$(VERSION)_$(GOOS)_$(GOARCH).zip'; print(hashlib.sha256(open(p,'rb').read()).hexdigest()+'  '+p)" > dist/$(PLUGIN_ID)_$(VERSION)_$(GOOS)_$(GOARCH).zip.sha256

clean:
	rm -rf dist
