.PHONY: build test clean

build:
	CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -o mirasim-proxy.so .
	rm -f mirasim-proxy.h

test:
	go test -v ./...
	go test -v ./.github/scripts

clean:
	rm -f mirasim-proxy.so mirasim-proxy.dll mirasim-proxy.dylib mirasim-proxy.h
