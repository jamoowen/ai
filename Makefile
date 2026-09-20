.PHONY: export fmt fmt-check

export:
	set -a; source .env;

fmt:
	go tool gofumpt -w .

fmt-check:
	@files="$$(go tool gofumpt -l .)" || exit $$?; \
	if [ -n "$$files" ]; then \
		echo "gofumpt required for:"; \
		echo "$$files"; \
		exit 1; \
	fi
