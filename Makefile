NAMES=go-metrics

.PHONY: tags

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
.PHONY: init
init:
	@for name in $(NAMES); do \
  	  echo $$name && \
	  cd $$name && \
	  go mod init github.com/jwm1rr0rb10/go-metrics/$$name || true && go mod tidy || true && go get -u ./... && cd ..; \
  	done
