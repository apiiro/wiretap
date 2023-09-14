FROM golang:1.20 AS build

WORKDIR /wiretap
COPY ./src/go.mod ./src/go.sum ./
RUN go mod download -x

# Build Wiretap
COPY ./src /wiretap

RUN make OUTPUT=./wiretap

FROM alpine:3.18

COPY --from=build /wiretap/wiretap /wiretap/wiretap
COPY --from=build /wiretap/start.sh /wiretap/start.sh

RUN addgroup -S wiretap && adduser -S -G wiretap wiretap
RUN chown -R wiretap:wiretap /wiretap
USER wiretap

WORKDIR /wiretap

# Run webserver for testing
ENTRYPOINT ["/wiretap/start.sh"]
