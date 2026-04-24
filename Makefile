.PHONY: build serve cgi worker reset update test e2e e2e-s3 e2e-install dev web-install web-build theme-css docs docs-build godoc db-reset cross dist-cgi docker-cgi docker-cgi-test clean

BIN := dist/murlog
CMD := ./cmd/murlog
VERSION := $(shell cat version.txt 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -s -w -X github.com/murlog-org/murlog.Version=$(VERSION) -X github.com/murlog-org/murlog.Commit=$(COMMIT)

build:
	@mkdir -p $(dir $(BIN))
	go build -ldflags '$(LDFLAGS)' -o $(BIN) $(CMD)

serve: build
	MURLOG_WEB_DIR=../web/dist ./$(BIN) serve

cgi: build
	./$(BIN) cgi

worker: build
	./$(BIN) worker

reset: build
	./$(BIN) reset

update: build
	./$(BIN) update

test:
	go test ./...

bench:
	go test ./store/sqlite/ -bench=. -benchmem -timeout 120s

seed-db:
	go run scripts/seeddb.go -o seed.db

web-test:
	cd web && npx vitest run

e2e-install:
	cd tests/e2e && npm install && npx playwright install --with-deps chromium

e2e: db-reset build web-build
	cd tests/e2e && npx playwright test

e2e-s3: db-reset build web-build
	@echo "=== Starting MinIO ==="
	cd media/s3/testdata && docker compose up -d && sleep 3
	@echo "=== Running e2e with S3 backend ==="
	@MURLOG_STORAGE_TYPE=s3 \
	MURLOG_S3_BUCKET=murlog-test \
	MURLOG_S3_REGION=us-east-1 \
	MURLOG_S3_ENDPOINT=http://localhost:9000 \
	MURLOG_S3_ACCESS_KEY=minioadmin \
	MURLOG_S3_SECRET_KEY=minioadmin \
	MURLOG_S3_PUBLIC_URL=http://localhost:9000/murlog-test \
	sh -c 'cd tests/e2e && npx playwright test media.spec.ts'; \
	EXIT=$$?; \
	cd media/s3/testdata && docker compose down; \
	exit $$EXIT

web-install:
	cd web && npm install

theme-css:
	@sed -e 's/body\.spa/body.public/g' \
	     -e '1s/murlog SPA — design tokens + components/murlog default theme/' \
	     -e '2s/SPA デザイントークン + コンポーネント。/デフォルトテーマ。--mur-* トークンを借用し SPA と体験を統一する。/' \
	     -e '3s/テーマ・公開ページに影響しない。/SPA に影響しない。/' \
	     web/src/style.css > web/public/themes/default/style.css

web-build: theme-css
	cd web && npm run build

dev: build
	MURLOG_PROTOCOL=http ./$(BIN) serve & SERVER_PID=$$!; \
	trap 'kill $$SERVER_PID 2>/dev/null' EXIT; \
	cd web && npx vite

docs:
	cd docs && npx vitepress dev --port 5174

docs-build:
	cd docs && npx vitepress build

godoc:
	@echo "godoc: http://localhost:6060/pkg/github.com/alarky/murlog/"
	godoc -http=:6060

DIST := dist/release
GOFLAGS := -trimpath -ldflags '$(LDFLAGS)'
cross: web-build
	mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 go build $(GOFLAGS) -o $(DIST)/murlog-linux-amd64     $(CMD)
	GOOS=linux   GOARCH=arm64 go build $(GOFLAGS) -o $(DIST)/murlog-linux-arm64     $(CMD)
	GOOS=freebsd GOARCH=amd64 go build $(GOFLAGS) -o $(DIST)/murlog-freebsd-amd64   $(CMD)

dist-cgi: cross
	@echo "=== Building CGI deploy zips ==="
	rm -f $(DIST)/murlog-cgi-*.zip
	$(call make-cgi-zip,linux,amd64)
	$(call make-cgi-zip,linux,arm64)
	$(call make-cgi-zip,freebsd,amd64)
	@echo "=== Done ==="
	@ls -lh $(DIST)/murlog-cgi-*.zip

define make-cgi-zip
	$(eval STAGING := $(DIST)/staging-$(1)-$(2))
	rm -rf $(STAGING)
	mkdir -p $(STAGING)/dist
	cp $(DIST)/murlog-$(1)-$(2) $(STAGING)/murlog.bin
	chmod 755 $(STAGING)/murlog.bin
	printf '#!/bin/sh\nexec env GOMAXPROCS=1 "$$(dirname "$$0")/murlog.bin" cgi\n' > $(STAGING)/murlog.cgi
	chmod 755 $(STAGING)/murlog.cgi
	cp deploy/cgi/htaccess $(STAGING)/.htaccess
	cp deploy/cgi/500.html $(STAGING)/500.html
	cp -r web/dist/* $(STAGING)/dist/
	rm -f $(STAGING)/dist/.vite/manifest.json
	rm -rf $(STAGING)/dist/.vite
	cd $(STAGING) && zip -r ../murlog-cgi-$(1)-$(2).zip .
	rm -rf $(STAGING)
endef

DOCKER_GOARCH ?= $(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/' | sed 's/arm64/arm64/')

docker-cgi: web-build
	GOOS=linux GOARCH=$(DOCKER_GOARCH) go build $(GOFLAGS) -o docker/murlog.cgi $(CMD)
	rm -rf docker/dist
	cp -r web/dist docker/dist
	cp deploy/cgi/htaccess docker/htaccess
	cd docker && docker compose build

docker-cgi-test: docker-cgi
	cd docker && docker compose up -d
	@echo "Waiting for Apache..."
	@sleep 2
	cd tests/e2e && BASE_URL=http://127.0.0.1:8888 npx playwright test; \
	EXIT=$$?; \
	cd ../../docker && docker compose down; \
	exit $$EXIT

db-reset:
	rm -rf data

clean:
	rm -f $(BIN)
	rm -rf data web/dist
