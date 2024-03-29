#!/usr/bin/env bash
set -ex

go tool dist list

rm -rf dist

GOOS=linux   GOARCH=amd64 go build -buildmode exe -o ./dist/xtemplate-amd64-linux/xtemplate       ./cmd
GOOS=darwin  GOARCH=amd64 go build -buildmode exe -o ./dist/xtemplate-amd64-darwin/xtemplate      ./cmd
GOOS=windows GOARCH=amd64 go build -buildmode exe -o ./dist/xtemplate-amd64-windows/xtemplate.exe ./cmd

cd dist

printf '%s\n' * | while read D; do
    cp ../README.md ../LICENSE "$D"
    tar czvf "$D.tar.gz" "$D/"
    zip -r9 "$D.zip" "$D/"
    rm -r "$D"
done

cd -

ls -lh dist/*
