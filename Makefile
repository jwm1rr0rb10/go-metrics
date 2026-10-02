NAMES=go-metrics

.PHONY: tags test lint bench vuln

test:
	go test -race -count=1 -cover ./...

lint:
	golangci-lint run ./...

bench:
	go test -run='^$$' -bench=. -benchmem ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tags:
	@bash -c ' \
		version=$$(cat "$(CURDIR)/version" 2>/dev/null || echo "0.0.0") && \
		echo "→ work with current directory and $$version" && \
		tag=v$$version && \
		echo "→ tag: $$tag" && \
		if [[ ! $$(git tag -l "$$tag") ]]; then \
			git tag -a "$$tag" -m "Release $$version" && \
			git push origin "$$tag" -o ci.skip && \
			echo "✅ Tagged and pushed $$tag"; \
		else \
			echo "⚠️  Tag $$tag already exists"; \
		fi \
	'
