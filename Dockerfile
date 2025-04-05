FROM docker.io/library/golang:alpine AS builder

ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN	apk add --no-cache \
	bash \
	ca-certificates

COPY . /go/src/github.com/ricardobranco777/regview

RUN set -x \
	&& apk add --no-cache --virtual .build-deps \
		gcc \
		git \
		libc-dev \
		libgcc \
		make \
	&& cd /go/src/github.com/ricardobranco777/regview \
	&& make all

FROM alpine:latest

COPY --from=builder /go/bin/regview /usr/bin/regview
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs

WORKDIR /src

ENTRYPOINT [ "regview" ]
CMD [ "--help" ]
