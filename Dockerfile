FROM golang:alpine AS build-env

RUN apk add --update --no-cache ca-certificates git

WORKDIR /build

ADD ./go.mod .
ADD ./go.sum .

RUN go mod download

ADD ./main.go .

RUN set -ex && \
  CGO_ENABLED=0 go build \
        -v -a \
        -ldflags '-extldflags "-static"' \
        -o /usr/bin/shelly

FROM busybox

ENV BROKER ""
ENV PIN ""

LABEL maintainer="Romuald Bulyshko <opensource@bulyshko.com>" \
    description="Shelly MQTT HomeKit bridge"

RUN mkdir -p /shelly
VOLUME /shelly

COPY --from=build-env /usr/bin/shelly /usr/local/bin/shelly

ENTRYPOINT /usr/local/bin/shelly -broker ${BROKER} -pin ${PIN} -data /shelly
