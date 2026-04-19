# Feature Systems

## Overview

The GitHub CLI is organized into distinct feature systems, each handling a specific domain of GitHub operations.

## Feature System Map

| Section | Title | Root Command |
|---------|-------|--------------|
| 4.1 | Pull Request Management | `gh pr` |
| 4.2 | Issue Management | `gh issue` |
| 4.3 | Repository Management | `gh repo` |
| 4.4 | Codespaces System | `gh codespace` |
| 4.5 | Browse Command | `gh browse` |
| 4.6 | Extension Management | `gh extension` |
| 4.7 | GitHub Actions Run Management | `gh run` |
| 4.8 | Gist Management | `gh gist` |
| 4.9 | Release Management | `gh release` |
| 4.10 | Attestation System | `gh attestation` |
| 4.11 | Direct API Command | `gh api` |
| 4.12 | Agent Task System | `gh agent-task` |

## Key Feature Systems

### Pull Request & Issue Management (4.1-4.2)

These systems share significant infrastructure in `pkg/cmd/pr/shared/`. Key components include:

- **CreateOptions**: Aggregates flags and factory dependencies for PR/issue creation
- **IssueMetadataState**: Shared state tracking labels, assignees, reviewers, milestones
- **Prompt interface**: Used for interactive prompts (title, body, metadata)
- **PRFinder**: Resolves PRs by number, URL, or branch name

`gh pr create` resolves push targets through `NewCreateContext()`, returning typed refs that drive `handlePush()`.

### Repository Management (4.3)

`gh repo` handles creation, forking, cloning, listing, viewing, and archiving. Three creation paths:

- Creating from scratch
- Creating from templates
- Creating from local repositories

The system includes `cloneWithRetry()` with exponential backoff for post-creation operations.

### GitHub Actions Run Management (4.7)

Key components managing workflow runs:

- **RunLogCache**: Caches run log ZIP files on disk by run ID and start time
- **ViewOptions**: Controls log display, verbose output, web viewing, exit status
- **ListOptions**: Supports filtering by branch, actor, status, event, commit

Functionality includes streaming logs, polling run status, and rendering job/step summaries.

### Extension Management (4.6)

Extensions are repositories prefixed with `gh-`. Three extension types:

| Kind | Storage | Install Source |
|------|---------|-----------------|
| BinaryKind | `~/.local/share/gh/extensions/<name>/` + `manifest.yaml` | GitHub release assets |
| GitKind | `~/.local/share/gh/extensions/<name>/` (git clone) | Repository default branch |
| LocalKind | Symlink to local directory | Local path |

`Manager.Dispatch()` looks up the extension by name, then exec's its binary, forwarding all remaining args.

### Codespaces System (4.4)

Provides create, list, delete, SSH access, file copy, port management, Jupyter integration, logs, and rebuild functionality.

### Browse Command (4.5)

Constructs GitHub URLs and opens them in browsers. Handles repository sections (settings, releases, actions, wiki, projects), specific files with line numbers, branches, commits, issues, and pull requests.

## Common Architectural Patterns

### Command Structure Pattern

Every feature system follows consistent conventions:

- **Options struct**: Aggregates flags and factory dependencies
- **Constructor**: `NewCmd<Name>()` function
- **Run function**: Contains execution logic
- **Flag registration**: Uses Cobra flag methods

### Dependency Injection

All options structs receive services from `cmdutil.Factory`:

- `HttpClient`: Authenticated HTTP client
- `IOStreams`: Standard input/output/error streams
- `Config`: User configuration
- `GitClient`: Local git operations
- `Prompter`: Interactive prompts
- `Browser`: Browser integration
- `BaseRepo`: Current repository context

### Interactive vs. Non-Interactive Modes

Commands check `opts.IO.CanPrompt()` to determine behavior. Missing values are collected via `Prompter` prompts in interactive mode, while non-interactive mode requires all required values supplied as flags.
