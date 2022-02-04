IMAGE_TAG=v0.0.1-local
GITHUB_SHA=local
GITHUB_REF=local

GOOS=linux GOARCH=amd64 go build -ldflags "-X main.GitCommit=$GITHUB_SHA -X main.GitRef=$GITHUB_REF -X main.Version=${IMAGE_TAG:-commit-$GITHUB_SHA}" -o bin/castai-spot-handler .

# test ACR
DOCKER_IMAGE_REPO=$REGISTRY_NAME.azurecr.io/$USER/castai-spot-handler

# buildx needed to build on ARM M1 machines
docker buildx build --platform linux/amd64 --push -t $DOCKER_IMAGE_REPO:$IMAGE_TAG .