PLUGIN_ID ?= doujie-oauth
VERSION ?= 0.1.6
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
EXT = $(if $(filter darwin,$(GOOS)),dylib,$(if $(filter windows,$(GOOS)),dll,so))
LDFLAGS = -s -w -X main.pluginVersion=$(VERSION)

.PHONY: test vet build package clean

test:
	cd go && CGO_ENABLED=1 go test ./...
	cd go && CGO_ENABLED=1 go vet ./...

vet:
	cd go && CGO_ENABLED=1 go vet ./...

build:
	mkdir -p dist
	cd go && CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildmode=c-shared -ldflags "$(LDFLAGS)" -o ../dist/$(PLUGIN_ID).$(EXT) .
	rm -f dist/$(PLUGIN_ID).h

package: build
	cd dist && python3 -c "import sys,zipfile; z=zipfile.ZipFile(sys.argv[1], 'w', zipfile.ZIP_DEFLATED); [z.write(p, p.split('/')[-1]) for p in sys.argv[2:]]; z.close()" $(PLUGIN_ID)_$(VERSION)_$(GOOS)_$(GOARCH).zip $(PLUGIN_ID).$(EXT) ../README.md ../README.zh-CN.md ../LICENSE
	cd dist && python3 -c "import hashlib,glob; p=glob.glob('$(PLUGIN_ID)_$(VERSION)_$(GOOS)_$(GOARCH).zip')[0]; open('checksums.txt','w').write(hashlib.sha256(open(p,'rb').read()).hexdigest()+'  '+p+'\\n')"

clean:
	rm -rf dist
