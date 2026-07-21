# Pin the Go minor version to match the toolchain pinned in go.mod and
# .config/mise/config.toml. Floating to a newer Go (via golang:1-alpine) can
# change net/http behavior; e.g. Go 1.26 changed ServeMux's trailing-slash
# redirect status from 301 to 307, which diverges from the other build targets
# and breaks the hurl integration tests.
FROM golang:1.25-alpine AS build

RUN apk add --no-cache build-base

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

ARG LDFLAGS

COPY app ./app/
COPY cmd ./cmd/
COPY providers ./providers/
COPY *.go ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags="${LDFLAGS} -X 'github.com/infogulch/xtemplate/app.defaultListenAddress=0.0.0.0:80'" -o /build/xtemplate ./cmd

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
COPY ./test/migrations /app/migrations/
COPY ./test/config.json /app/

USER root:root
RUN mkdir /app/dataw

VOLUME /app/dataw

RUN ["/app/xtemplate", "--version"]

WORKDIR /app

CMD ["--loglevel", "-4", "--config-file", "config.json"]

###

FROM dist AS final
