
build-server:
	go build -o bin/meetings-server server/cmd/main.go

build-connect:
	go build -o bin/meetings-connect ./clients/connect/cmd/...

build-filters:
	go build -o bin/meetings-filters ./clients/filters/cmd/...

build-lights:
	go build -o bin/meetings-lights ./clients/actions/cmd/lights.go

build-alerts:
	go build -o bin/meetings-alerts ./clients/actions/cmd/alerts.go

build-all: build-server build-connect build-filters build-lights build-alerts
