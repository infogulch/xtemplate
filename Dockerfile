FROM golang:1-alpine AS builder

RUN apk add --no-cache build-base

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

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 \
    GOFLAGS='-tags="sqlite_json"' \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags="-w -s" -o /dist/xtemplate ./cmd
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
