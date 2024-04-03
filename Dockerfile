FROM golang:1-alpine AS builder

RUN apk add --no-cache build-base

ARG LDFLAGS

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

WORKDIR /build/app
COPY go.mod go.sum /build/
COPY app/go.mod app/go.sum /build/app/
COPY providers/nats/go.mod providers/nats/go.sum /build/providers/nats/
RUN go mod download

COPY . /build/
RUN CGO_ENABLED=1 \
    GOFLAGS='-tags="sqlite_json"' \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags="${LDFLAGS}" -o /dist/xtemplate ./cmd
RUN ldd /dist/xtemplate | tr -s [:blank:] '\n' | grep ^/ | xargs -I % install -D % /dist/%
RUN ln -s ld-musl-x86_64.so.1 /dist/lib/libc.musl-x86_64.so.1

###

FROM scratch

COPY --from=builder /etc/passwd /etc/group /etc/
COPY --from=builder /dist/lib /lib/
COPY --from=builder /dist/xtemplate /app/xtemplate

WORKDIR /app
VOLUME /app/data
USER appuser:appuser
EXPOSE 80

ENTRYPOINT ["/app/xtemplate"]

CMD ["-template-path", "/app/templates", "-watch-template", "false", "-log", "0", "-listen", ":80"]
