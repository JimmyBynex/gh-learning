# Core Subsystems

## Overview

Four foundational subsystems underpin all `gh` CLI feature commands:

1. **GitHub API Client**
2. **Authentication System**
3. **Git Integration**
4. **API Data Models**

These subsystems are instantiated via `factory.New()` and delivered to commands through `cmdutil.Factory`.

## GitHub API Client

The `api.Client` struct holds an authenticated HTTP client and exposes five primary methods:

- **GraphQL(hostname, query, vars, data)** - Raw query strings
- **Query(hostname, name, query, vars)** - Typed struct queries
- **Mutate(hostname, name, mutation, vars)** - Typed struct mutations
- **REST(hostname, method, path, body, data)** - Standard requests
- **RESTWithNext(hostname, method, path, body, data)** - Pagination support

All methods call `clientOptions()` to construct request configuration, setting API version headers and delegating to underlying go-gh clients.

Error types include `api.HTTPError` and `api.GraphQLError`. The `HTTPError` carries a `scopesSuggestion` string that compares current token scopes against endpoint requirements.

## Authentication and OAuth Flow

Four `gh auth` subcommands implement authentication:

- **`gh auth login`** - Interactive OAuth device flow
- **`gh auth status`** - Display authentication status
- **`gh auth refresh`** - Refresh or upgrade token scopes
- **`gh auth logout`** - Remove stored credentials

Interactive logins open a browser, display a device code, and poll until a token is returned. Tokens are persisted to the system credential store by default, or to `~/.config/gh/hosts.yml` when `--insecure-storage` is used.

After successful login, `GitCredentialFlow` configures `gh auth git-credential` as the git credential helper.

## Git Integration

`git.Client` wraps subprocess calls to the local `git` binary. Each operation constructs a `*Command` via `c.Command(ctx, args...)`, optionally prepending `-C <RepoDir>` for directory-specific execution.

Key operations include:
- **CurrentBranch()** - `git symbolic-ref --quiet HEAD`
- **Remotes()** - `git remote -v` plus config parsing
- **ReadBranchConfig()** - Branch-specific configuration
- **Commits()** - `git log --cherry`
- **ShowRefs()** - `git show-ref --verify`

`AuthenticatedCommand()` injects `gh auth git-credential` as the helper via `-c credential.<pattern>.helper=!gh auth git-credential`.

## API Data Models

API responses deserialize into typed Go structs across three files:

- **PullRequest** (`api/queries_pr.go`) - Full PR state including refs, commits, checks, reviews
- **Repository** (`api/queries_repo.go`) - Repo metadata, visibility, merge settings
- **Issue** (`api/queries_issue.go`) - Issue state, labels, assignees, milestones

`RepoMetadata()` fetches assignees, labels, milestones, teams, and projects in parallel using `errgroup`. Helper methods convert user-supplied names into GraphQL node IDs.

## Subsystem Dependencies

The dependency map shows:
- Factory instantiates all subsystems
- API Client requires authenticated HTTP Client
- Git Client operates independently on local repository
- All data models feed into API Client responses
- Authentication system provides tokens for API Client
