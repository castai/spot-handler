build:
	GOOS=linux go build -ldflags "-s -w" -o bin/azure-spot-handler .
	docker build -t castai/azure-spot-handler:$(VERSION) .

push:
	docker push castai/azure-spot-handler:$(VERSION) .

release: push