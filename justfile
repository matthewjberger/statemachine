set windows-shell := ["powershell.exe"]

# Lists all available recipes
@just:
    just --list

# Runs all tests
test:
    go test ./...

# go vet + gofmt -l, fails if anything is unformatted (Windows)
[windows]
check:
    go vet ./...
    $unformatted = (gofmt -l . | Out-String).Trim(); if ($unformatted) { Write-Host $unformatted; exit 1 }

# go vet + gofmt -l, fails if anything is unformatted (Unix)
[unix]
check:
    go vet ./...
    unformatted=$(gofmt -l .); if [ -n "$unformatted" ]; then echo "$unformatted"; exit 1; fi

# gofmt -w
format:
    gofmt -w .

# go mod tidy
tidy:
    go mod tidy

# Shows what `go mod tidy` would change
tidy-check:
    go mod tidy -diff

# Deps with available updates
outdated:
    go list -m -u all

# check + test (run before pushing)
ci: check test

# Full read-only audit
audit: check tidy-check outdated test

# Runs the robot demo
run-robot:
    go run ./examples/robot

# Runs the traffic light demo
run-traffic:
    go run ./examples/traffic

# Displays Go tool version
@versions:
    go version
