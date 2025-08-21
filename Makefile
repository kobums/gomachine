#all: router request model
all: install

# router: router.go router_fiber.go
# 	go build -o bin/router router.go
router: router_fiber.go
	go build -o bin/router_fiber router_fiber.go

request: request.go
	go build -o bin/request request.go

# Build model generator using Go modules
model: main.go util/utils.go go/main.go go/router.go dart/main.go
	go build -o bin/model .

doc: doc.go
	go build -o bin/doc doc.go

watch: watch.go
	go build -o bin/watch watch.go

# Run model generator from source files
run: main.go util/utils.go go/main.go go/router.go dart/main.go
	go run .

# Build Linux binary for model generator
model-linux: main.go util/utils.go go/main.go go/router.go dart/main.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-s' -o bin/model.linux .

install: model
	rm -rf ~/bin/buildtool*
	mkdir -p ~/bin/buildtool
	# cp bin/router ~/bin/buildtool-router
	#cp bin/router ~/bin/buildtool-router_fiber
	# cp bin/request ~/bin/buildtool-request
	cp bin/model ~/bin/buildtool-model
	# cp bin/doc ~/bin/buildtool-doc
	# cp bin/watch ~/bin/buildtool-watch
	cp -rf views/* ~/bin/buildtool/

# Install with Linux binary
install-linux: model-linux
	rm -rf ~/bin/buildtool*
	mkdir -p ~/bin/buildtool
	cp bin/model.linux ~/bin/buildtool-model.linux
	cp -rf views/* ~/bin/buildtool/

test:
	#mysql -u root anb < build/test.sql
	go test -v ./...

clean:
	rm -f ~/bin/*

# Development helpers
fmt:
	go fmt ./...

vet:
	go vet ./...

# Build all binaries
build-all: model router request doc watch

.PHONY: all model router request doc watch run install install-linux test clean fmt vet build-all model-linux
