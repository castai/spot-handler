build:
	GOOS=linux go build -ldflags "-s -w" -o bin/spot-handler .
	docker build -t us-docker.pkg.dev/castai-hub/library/spot-handler:$(VERSION) .

push:
	docker push us-docker.pkg.dev/castai-hub/library/spot-handler:$(VERSION) .

release: push
