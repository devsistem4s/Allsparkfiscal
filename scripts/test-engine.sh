#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
fixtures=$(mktemp -d)
trap 'rm -rf "$fixtures"' EXIT
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$fixtures/key.pem" -out "$fixtures/cert.pem" -days 1 -subj '/C=BR/O=AllSpark Synthetic Test/CN=TESTE:11222333000181' -addext 'subjectAltName=otherName:2.16.76.1.3.3;UTF8:11222333000181' >/dev/null 2>&1
openssl pkcs12 -export -out "$fixtures/test.pfx" -inkey "$fixtures/key.pem" -in "$fixtures/cert.pem" -passout pass:synthetic-only >/dev/null 2>&1
chmod 644 "$fixtures/test.pfx"
docker run --rm --network none --user "$(id -u):$(id -g)" -v "$fixtures:/fixtures:ro" -v "$PWD/examples:/examples:ro" -v "$PWD/engines/nfephp/tests:/app/tests:ro" -v "$PWD/engines/nfephp/src:/app/src:ro" allspark-fiscal-engine:local php tests/smoke.php
