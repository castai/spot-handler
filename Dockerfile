FROM alpine:3.13
ARG TARGETARCH amd64
COPY bin/spot-handler-$TARGETARCH /usr/local/bin/spot-handler
CMD ["spot-handler"]
