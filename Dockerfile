FROM golang:1.27-alpine AS builder

WORKDIR /srv

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o dgx-backend main.go

FROM alpine:3.24

WORKDIR /srv

COPY --from=builder /srv/dgx-backend .

EXPOSE 8080

CMD ["./dgx-backend"]