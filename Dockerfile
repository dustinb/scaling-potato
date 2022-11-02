FROM golang AS golang

WORKDIR /app

COPY go.mod /app/
COPY go.sum /app/
COPY worker.go /app/

RUN go mod vendor
RUN go build -o worker worker.go

FROM alpine:latest
RUN apk add --no-cache libc6-compat
RUN apk add curl
WORKDIR /app
COPY --from=golang /app/worker /app/

CMD [ "/app/worker" ]