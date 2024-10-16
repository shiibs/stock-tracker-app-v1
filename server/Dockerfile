FROM golang:1.23.0-alpine3.19

WORKDIR /src/app

RUN go install github.com/air-verse/air@latest

COPY . .

RUN go mod tidy
