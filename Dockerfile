FROM golang:alpine

WORKDIR /build
RUN apk add upx
ADD . .
RUN --mount=type=cache,target=/go/pkg/mod \
	  --mount=type=cache,target=/root/.cache/go-build \ 
		go build -o /groupcache-demo -ldflags="-s -w" . \
		&& upx /groupcache-demo


FROM alpine

COPY --from=0 /groupcache-demo /
LABEL source_repository="https://github.com/sapcc/databus23/k8sgroupcache"

ENTRYPOINT ["/groupcache-demo"]


