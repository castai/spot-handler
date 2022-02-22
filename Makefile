build:
	GOOS=linux go build -ldflags "-s -w" -o bin/spot-handler .
	docker build -t castai/spot-handler:$(VERSION) .

push:
	docker push castai/spot-handler:$(VERSION) .

release: push
