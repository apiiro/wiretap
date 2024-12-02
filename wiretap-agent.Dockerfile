FROM golang:1.21 AS build

ARG VERSION

WORKDIR /wiretap
COPY ./src/go.mod ./src/go.sum ./
RUN go mod download -x

# Build Wiretap
COPY ./src /wiretap

RUN make OUTPUT=./wiretap VERSION=${VERSION}

FROM alpine:3.20

RUN apk update && \
    apk upgrade && \
    apk --no-cache add curl

COPY --from=build /wiretap/wiretap /wiretap/wiretap
COPY --from=build /wiretap/start.sh /wiretap/start.sh

RUN addgroup -S wiretap && adduser -S -G wiretap wiretap
RUN chown -R wiretap:wiretap /wiretap
USER wiretap

WORKDIR /wiretap

ENTRYPOINT ["/wiretap/start.sh"]
