# GitHub CLI (`gh`) Overview

The page documents the `gh` CLI repository (`github.com/cli/cli/v2`), GitHub's official command-line tool for managing repositories, pull requests, issues, and related GitHub functionality.

## Key Purpose

"`gh` is the official GitHub command-line tool. It exposes GitHub concepts — pull requests, issues, repositories, workflows, gists, releases, codespaces, and more — as first-class terminal commands that integrate with a local `git` workflow."

## Architecture Highlights

The codebase follows a layered architecture with:
- **Entry point**: `cmd/gh/main.go`
- **Command logic**: `pkg/cmd/` directory structure
- **Shared infrastructure**: `internal/` packages including API client, authentication, and Git integration
- **Framework**: Built on Spf13/Cobra for command organization

## Supported Environments

- **Platforms**: GitHub.com, GitHub Enterprise Cloud, GitHub Enterprise Server 2.20+
- **Operating Systems**: macOS, Windows, Linux
- **Architectures**: 386, amd64, arm64, armv6

## Major Command Groups

The CLI organizes commands into logical groups:
- **Core commands** (pr, issue, repo, codespace, browse, release)
- **Actions commands** (run, workflow, cache)
- **Extension commands** (dynamically loaded)

## Key Technologies

The project uses significant dependencies including Cobra (CLI framework), GraphQL clients for API access, interactive prompt libraries (survey, huh), and Sigstore for attestation verification.
