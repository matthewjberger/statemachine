set windows-shell := ["powershell.exe"]

# Lists all available recipes
@just:
    just --list

# Library: runs all tests
test:
    go test ./...

# Library: go vet + gofmt -l (fails if anything is unformatted)
check:
    go vet ./...
    $unformatted = (gofmt -l . | Out-String).Trim(); if ($unformatted) { Write-Host $unformatted; exit 1 }

# Library: gofmt -w
format:
    gofmt -w .

# Library: go mod tidy
tidy:
    go mod tidy

# Library: shows what `go mod tidy` would change
tidy-check:
    go mod tidy -diff

# Library: deps with available updates
outdated:
    go list -m -u all

# Library: check + test (run before pushing)
ci: check test

# Library: full read-only audit
audit: check tidy-check outdated test

# CLI: builds cmd/statemachine into ./statemachine.exe
build:
    go build -buildvcs=false -o statemachine.exe ./cmd/statemachine

# CLI: installs cmd/statemachine into $GOBIN (or $GOPATH/bin)
install:
    go install -buildvcs=false ./cmd/statemachine

# Examples: regenerates every example's _gen.go from its .sm
gen: install
    go generate ./examples/...

# Examples: runs the robot demo
run-robot:
    go run -buildvcs=false ./examples/robot

# Examples: runs the traffic light demo
run-traffic:
    go run -buildvcs=false ./examples/traffic

# Removes generated artifacts
clean:
    Remove-Item -Force -ErrorAction SilentlyContinue statemachine.exe

# Displays Go tool version
@versions:
    go version
