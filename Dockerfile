FROM golang:alpine as builder

ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN	apk add --no-cache \
	bash \
	ca-certificates

COPY . /go/src/github.com/ricardobranco777/regview

RUN set -x \
	&& apk add --no-cache --virtual .build-deps \
		make \
	&& cd /go/src/github.com/ricardobranco777/regview \
	&& make static \
	&& mv regview /usr/bin/regview \
	&& apk del .build-deps \
	&& rm -rf /go \
	&& echo "Build complete."

FROM alpine:latest

COPY --from=builder /usr/bin/regview /usr/bin/regview
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs

WORKDIR /src

ENTRYPOINT [ "regview" ]
CMD [ "--help" ]
