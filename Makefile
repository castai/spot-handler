build:
	GOOS=linux go build -ldflags "-s -w" -o bin/azure-spot-handler .
	docker build -t castai/azure-spot-handler:$(VERSION) .

push:
	# buildx needed to build on ARM M1 machines
    docker buildx build --platform linux/amd64 --push -t castai/azure-spot-handler:$(VERSION) .

release: push