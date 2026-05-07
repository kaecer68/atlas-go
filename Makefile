.PHONY: build dev restart clean lint test ci check-format

BINARY := /tmp/atlas-new
PIDFILE := /tmp/atlas-new.pid

build:
	go build -o $(BINARY) ./cmd/atlas

dev: build restart
	@echo "Dev server running at http://localhost:8080"

restart:
	@-kill $$(pgrep -f "$(BINARY)") 2>/dev/null
	@sleep 1
	@cd $(PWD) && nohup $(BINARY) -api > /tmp/atlas-server.log 2>&1 &
	@sleep 2
	@curl -s http://localhost:8080/health > /dev/null && echo "✅ Server ready" || echo "❌ Server failed to start"

clean:
	@-kill $$(pgrep -f "$(BINARY)") 2>/dev/null
	@rm -f $(BINARY) $(PIDFILE)

lint:
	@test -z "$$(gofmt -l .)" || (echo "❌ gofmt issues:" && gofmt -l . && exit 1)
	@go vet ./...

test:
	go test ./...

ci: lint test build check-format gen-check
	@echo "✅ CI checks passed"

gen-check:
	@echo "🔍 Checking generated files..."
	@go generate ./internal/domain/...
	@test -z "$$(git diff -- web/static/js/shared/field_names.js)" || \
		(echo "❌ field_names.js outdated. Run: go generate ./internal/domain/..." && exit 1)
	@echo "  field_names.js: up to date"

check-format:
	@echo "🔍 Checking persistence format..."
	@go run ./cmd/check-persistence-format -dir data/state
