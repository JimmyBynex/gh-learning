# GitHub CLI Architecture

## Overview

The GitHub CLI (`gh`) is the official command-line interface for GitHub, providing access to pull requests, issues, repositories, workflows, and other GitHub features directly from the terminal.

**Module Identity**: `github.com/cli/cli/v2` (Go 1.25.7)

## Layered Architecture

The codebase follows a five-layer architectural pattern:

1. **User Interface Layer** - Command parsing, help text, input validation via Cobra
2. **Command Domain Layer** - Business logic for GitHub operations and workflow orchestration
3. **Core Services Layer** - Authentication, API communication, Git operations, dependency injection
4. **Data Model Layer** - Type definitions for GitHub entities with JSON/GraphQL tags
5. **External Systems Layer** - GitHub APIs, local Git repositories, external services

## Key Architectural Patterns

- **Layered Architecture**: Clear separation of concerns across five distinct layers
- **Command Pattern**: Cobra commands with `RunE` functions encapsulate operations
- **Factory Pattern**: `cmdutil.Factory` manages centralized dependency creation and injection
- **Strategy Pattern**: Pluggable components (Browser, Prompter, HTTPClient) enable runtime behavior selection
- **Template Method**: Command execution hooks (PersistentPreRunE, PreRunE, RunE) define algorithm skeletons
- **Adapter Pattern**: `api.Client` wraps go-gh clients for unified REST/GraphQL interface
- **Plugin Architecture**: Extension system enables extensibility without core modifications

## Command Organization

Commands are structured using Cobra with the following hierarchy:

- **Root command**: `gh` (created via `NewCmdRoot()`)
- **Command groups**: Organized by category (core, actions, extensions)
- **Subcommands**: Nested under parent commands (e.g., `pr create`, `issue list`)

All commands follow a consistent constructor pattern: `func NewCmd<Name>(f *cmdutil.Factory) *cobra.Command`

## Cross-Cutting Concerns

**Authentication**: The system uses a `PersistentPreRunE` hook to check authentication status before command execution. Context-aware help suggests appropriate authentication methods based on environment.

**Error Handling**: Standard Unix exit codes—0 (success), 1 (general failure), 2 (cancelled), 4 (authentication required).

**Environment Variables**: The CLI respects multiple configuration variables including `GH_TOKEN`, `GH_DEBUG`, `GH_HOST`, `GH_REPO`, and others for controlling behavior.

## Major Dependencies

**CLI Framework**: `spf13/cobra` (v1.10.2), `spf13/pflag` (v1.0.10)

**GitHub Integration**: `cli/go-gh/v2` (v2.13.0), `cli/oauth` (v1.2.2), `shurcooL/githubv4`

**Interactive UI**: `AlecAivazis/survey/v2`, `charmbracelet/huh`, `charmbracelet/glamour`

**Attestation**: `sigstore/sigstore-go`, `theupdateframework/go-tuf/v2`, `in-toto/attestation`

**Networking**: `gorilla/websocket`, `microsoft/dev-tunnels`, `google.golang.org/grpc`

## Factory-Based Dependency Injection

The `cmdutil.Factory` serves as the integration point, providing commands with:
- Direct field access (IOStreams, Browser, GitClient)
- Lazy-initialized methods (HttpClient(), Config(), BaseRepo(), Prompter())

This pattern enables testability through mock implementations while maintaining consistent configuration across all dependencies.

## Command Execution Flow

1. User invokes command (e.g., `gh pr view 123`)
2. `main()` delegates to `ghcmd.Main()`
3. Root command's `PersistentPreRunE` checks authentication
4. Cobra matches command name to registered commands
5. Command's `RunE` function executes with factory-provided dependencies
6. API interactions occur via REST or GraphQL
7. Results are formatted and displayed to output streams
