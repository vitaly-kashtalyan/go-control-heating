FROM golang:1.15-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN go build -o main .

FROM alpine:latest
WORKDIR /root
COPY --from=build /app/main .
CMD ["./main"]