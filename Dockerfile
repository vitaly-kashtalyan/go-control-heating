FROM --platform=linux/amd64 golang:1.18-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOOS=linux GOARCH=amd64 go build -o main .


FROM --platform=linux/amd64 alpine:3.19
WORKDIR /root
COPY --from=build /app/main .
CMD ["./main"]