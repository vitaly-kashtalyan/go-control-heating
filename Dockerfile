FROM golang:1.18-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux go build -o main .


FROM alpine:3.19
WORKDIR /root
COPY --from=build /app/main .
CMD ["./main"]