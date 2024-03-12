FROM golang:1.22 AS build

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

ENTRYPOINT ["/app/xtemplate"]

CMD ["-template-root", "/app/templates", "-watch-template", "false", "-log", "0"]
