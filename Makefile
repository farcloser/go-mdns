lint:
	golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 --sort-results

lint-fix:
	golangci-lint run --fix

tidy:
	go mod tidy
