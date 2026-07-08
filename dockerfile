FROM golang:1.26-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /agones-lifecycle .

FROM alpine:3.20

COPY --from=build /agones-lifecycle /agones-lifecycle

ENTRYPOINT ["/agones-lifecycle"]