FROM golang:1.27-alpine AS builder

WORKDIR /srv

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o dgx-backend main.go

FROM alpine:3.24

RUN adduser -D -H -u 10001 dgx

WORKDIR /srv

COPY --from=builder /srv/dgx-backend .

RUN mkdir data tmp && chown dgx:dgx data tmp

USER dgx

EXPOSE 8080

CMD ["./dgx-backend"]