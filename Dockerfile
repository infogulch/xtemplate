FROM golang:1-alpine AS deps

RUN apk add --no-cache build-base

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download -x

###

FROM deps AS build

ARG LDFLAGS

COPY app ./app/
COPY cmd ./cmd/
COPY providers ./providers/
COPY *.go ./
RUN CGO_ENABLED=1 \
    GOFLAGS='-tags="sqlite_json"' \
    GOOS=linux \
    GOARCH=amd64 \
    go build -x -ldflags="${LDFLAGS} -X 'github.com/infogulch/xtemplate/app.defaultWatchTemplates=false' -X 'github.com/infogulch/xtemplate/app.defaultListenAddress=0.0.0.0:80'" -o /build/xtemplate ./cmd

###

FROM alpine AS dist

ENV USER=appuser
ENV UID=10001
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"
WORKDIR /app
USER $USER:$USER
EXPOSE 80

COPY --from=build /build/xtemplate /app/xtemplate

ENTRYPOINT ["/app/xtemplate"]

###

FROM dist AS test

COPY ./test/templates /app/templates/
COPY ./test/data /app/data/
COPY ./test/config.json /app/

USER root:root
RUN mkdir /app/dataw; chown $USER:$USER /app/dataw
USER $USER:$USER

VOLUME /app/dataw

RUN ["/app/xtemplate", "--version"]

CMD ["--loglevel", "-4", "-d", "DB:sql:sqlite3:file:./dataw/test.sqlite", "-d", "FS:fs:./data", "--config-file", "config.json"]

###

FROM dist as final
