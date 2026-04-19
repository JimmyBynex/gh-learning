# Build, Release and Installation

## Overview

The GitHub CLI is a Go binary compiled from `cmd/gh/main.go`. Releases use GoReleaser for multi-platform builds. Local build uses `make` delegating to `script/build.go`.

## Makefile Targets

- **`bin/gh`**: Compiles the binary
- **`manpages`**: Generates man pages into `share/man/man1/`
- **`completions`**: Generates shell completions (bash, fish, zsh)
- **`test`**: Runs `go test ./...`
- **`install`**: Installs to `${prefix}` (default `/usr/local`)

## Version Embedding

```
-X github.com/cli/cli/v2/internal/build.Version={{.Version}}
-X github.com/cli/cli/v2/internal/build.Date={{time "2006-01-02"}}
```

## Platform Matrix

- **macOS (darwin)**: amd64, arm64
- **Linux**: 386, arm, amd64, arm64
- **Windows**: 386, amd64, arm64

## Installation Methods

- **apt**: Packages at `https://cli.github.com/packages`
- **RPM**: `https://cli.github.com/packages/rpm/gh-cli.repo`
- **Source**: Requires Go 1.25+, `make install`
