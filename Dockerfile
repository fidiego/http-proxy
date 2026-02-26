FROM golang:1.25 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /http-proxy ./cmd/http-proxy

FROM gcr.io/distroless/static-debian12
COPY --from=build /http-proxy /http-proxy

ENTRYPOINT ["/http-proxy"]
