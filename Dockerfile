FROM alpine:3.13
COPY bin/azure-spot-handler /usr/local/bin/azure-spot-handler
CMD ["azure-spot-handler"]
