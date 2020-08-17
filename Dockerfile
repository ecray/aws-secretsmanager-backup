FROM golang:1.14-alpine as builder

WORKDIR /go/src/app
COPY main.go .

RUN apk --no-cache add git  && \
    go get -d -v ./... && \
    go install -v ./... && \
    CGO_ENABLED=0 GOOS=linux go build -o run main.go


FROM alpine:3.12

ARG USER=asm

RUN apk --no-cache add ca-certificates shadow && \
    groupadd -r ${USER} && useradd --no-log-init -r -g ${USER} ${USER}

COPY --from=builder  /go/src/app/run /srv/run

USER ${USER}

CMD ["/srv/run"]