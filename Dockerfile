FROM golang:1.21 AS build

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . ./

ENV GOFLAGS='-tags="sqlite_json"'
ENV CGO_ENABLED=1
ENV GOOS=linux
RUN go build -o xtemplate ./cmd

###

FROM scratch

WORKDIR /app

COPY --from=build /build/xtemplate /app/xtemplate

VOLUME /app/data
EXPOSE 80

ENTRYPOINT ["/app/xtemplate", '-template-root', '/app/templates', '-log', '0']
