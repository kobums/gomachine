#all: router request model
all: install

router: router.go
	go build -o bin/router router.go

request: request.go
	go build -o bin/request request.go

model: model.go
	go build -o bin/model model.go

watch: watch.go
	go build -o bin/watch watch.go

run: model.go
	go run model.go

install: model
	# router request model watch
	rm -rf ~/bin/buildtool*
	mkdir -p ~/bin/buildtool
	# cp bin/router ~/bin/buildtool-router
	# cp bin/request ~/bin/buildtool-request
	cp bin/model ~/bin/buildtool-model
	# cp bin/watch ~/bin/buildtool-watch
	# env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-s' -o bin/router.linux router.go
	# env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-s' -o bin/request.linux request.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-s' -o bin/model.linux model.go
	# cp bin/router.linux ~/bin/buildtool-router.linux
	# cp bin/request.linux ~/bin/buildtool-request.linux
	cp bin/model.linux ~/bin/buildtool-model.linux
	cp -rf views/* ~/bin/buildtool/

test:
	#mysql -u root anb < build/test.sql
	go test -v ./...

clean:
	rm -f ~/bin/*
