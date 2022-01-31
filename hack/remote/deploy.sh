USER=matas

IMAGE_TAG=v0.0.5
GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=${IMAGE_TAG}" -o bin/castai-spot-handler .

DOCKER_IMAGE_REPO=handlertestregistry.azurecr.io/$USER/castai-spot-handler
docker buildx build --platform linux/amd64 --push -t $DOCKER_IMAGE_REPO:$IMAGE_TAG .