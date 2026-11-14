.PHONY: init up down test test-engine integration recovery load docs upstreams
init:
	python3 scripts/init-local.py
up:
	docker compose up -d --build --scale worker=4
down:
	docker compose stop
test:
	go test -race ./...
	go vet ./...
test-engine:
	sh scripts/test-engine.sh
integration:
	python3 scripts/integration.py
recovery:
	python3 scripts/test-recovery.py
load:
	python3 scripts/load-test.py --requests 1000 --rate 50 --concurrency 20
docs:
	python3 scripts/build-openapi.py
upstreams:
	python3 scripts/fetch-upstreams.py
