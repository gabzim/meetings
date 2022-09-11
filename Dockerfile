FROM golang:1.19-alpine AS build_base
RUN apk add --no-cache git
WORKDIR /tmp/meetings-api

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY server/ server/
RUN go build -o ./out/meetings-api ./server/cmd/...

FROM alpine:3.16
RUN apk add ca-certificates
COPY --from=build_base /tmp/meetings-api/out/meetings-api /app/meetings-api

EXPOSE 8080
CMD ["/app/meetings-api"]
