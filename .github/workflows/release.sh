#!/usr/bin/env bash
set -ex

go tool dist list

rm -rf dist

GITVER="$(git describe --exact-match --tags --match="v*" 2> /dev/null || git rev-parse --short HEAD)"
VERSION="$(go list -f '{{.Version}}' -m github.com/infogulch/xtemplate@$GITVER)"
LDFLAGS="-X 'github.com/infogulch/xtemplate/app.version=$VERSION'"

echo "VERSION=$VERSION" >> $GITHUB_ENV
echo "LDFLAGS=$LDFLAGS" >> $GITHUB_ENV

GOOS=linux   GOARCH=amd64 go build -ldflags="$LDFLAGS" -buildmode exe -o ./dist/xtemplate-amd64-linux/xtemplate       ./cmd
GOOS=darwin  GOARCH=amd64 go build -ldflags="$LDFLAGS" -buildmode exe -o ./dist/xtemplate-amd64-darwin/xtemplate      ./cmd
GOOS=windows GOARCH=amd64 go build -ldflags="$LDFLAGS" -buildmode exe -o ./dist/xtemplate-amd64-windows/xtemplate.exe ./cmd

cd dist

printf '%s\n' * | while read D; do
    cp ../README.md ../LICENSE "$D"
    tar czvf "${D}_$VERSION.tar.gz" "$D/"
    zip -r9 "${D}_$VERSION.zip" "$D/"
    rm -r "$D"
done

cd -

ls -lh dist/*
