UNAME_S := $(shell uname -s)

ifeq ($(OS),Windows_NT)
PLUGIN_EXT := dll
else ifeq ($(UNAME_S),Darwin)
PLUGIN_EXT := dylib
else
PLUGIN_EXT := so
endif

OUTPUT := mirasim.$(PLUGIN_EXT)

.PHONY: all build test clean

all: build

build:
	CGO_ENABLED=1 go build -buildmode=c-shared -o $(OUTPUT) .
	@rm -f mirasim.h

test:
	go test -v ./...

clean:
	rm -f *.dll *.so *.dylib *.h
