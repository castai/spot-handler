FROM alpine:3.13
COPY bin/castai-spot-handler /usr/local/bin/castai-spot-handler
CMD ["castai-spot-handler"]
