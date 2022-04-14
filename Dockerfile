FROM alpine:3.13
COPY bin/spot-handler /usr/local/bin/spot-handler
CMD ["spot-handler"]

