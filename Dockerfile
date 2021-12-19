FROM golang:1.17-alpine3.13 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN go build -o main .

FROM alpine:3.13
WORKDIR /root
COPY --from=build /app/main .
CMD ["./main"]