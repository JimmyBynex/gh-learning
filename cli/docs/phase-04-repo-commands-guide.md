# Phase 4 学习指南：仓库命令

---

## Section 1：全局视图

### 1.1 这个 Phase 做了什么

Phase 4 在 Phase 3（API 客户端）的基础上，新增了两条真实可用的仓库命令：`gh repo view` 和 `gh repo list`。具体来说，它做了四件事：

1. **实现 `internal/ghrepo` 包**：定义 `ghrepo.Interface`，统一表示"一个 GitHub 仓库引用"的契约（RepoOwner / RepoName / RepoHost）。提供 `FromFullName`（解析 `owner/name`）和 `FromURL`（解析 URL）两个工厂函数。

2. **实现 `api/queries_repo.go`**：定义 `Repository` struct（实现 `ghrepo.Interface`）和两个查询函数——`GetRepository`（按 owner/name 查单个仓库）和 `ListRepositories`（列出某用户的仓库列表）。两个函数都通过 Phase 3 实现的 `client.GraphQL()` 发起 GraphQL 查询。

3. **实现 `pkg/cmd/repo/view`**：`gh repo view <owner/name>` 命令，解析参数 → 调用 `GetRepository` → 格式化输出仓库详情。

4. **实现 `pkg/cmd/repo/list`**：`gh repo list [<user>]` 命令，支持 `--limit` flag；若未指定用户，先通过 REST API `GET /user` 获取当前认证用户的 login，再列出其仓库。

Phase 3 让 CLI 能"发 API 请求"，Phase 4 让 CLI 能"查询仓库数据"——这是所有仓库相关操作（clone、fork、create）的基础。`ghrepo.Interface` 的引入使得"表示一个仓库"这件事与具体实现解耦，无论是从命令行参数解析的仓库引用，还是从 API 返回的 `Repository` 对象，都能以统一的方式传递和使用。

---

### 1.2 模块依赖图

```
                    ┌─────────────────────────────────────┐
                    │         Phase 4 新增 / 修改           │
                    └─────────────────────────────────────┘

   pkg/cmd/root/root.go          ← ★ 修改：注册 repo 命令组
         │
         └── repoCmd.NewCmdRepo(f)
                   │
                   ├── view.NewCmdRepoView(f)   ← ★ 新增：gh repo view
                   │         │
                   │         ├── internal/ghrepo  (FromFullName)
                   │         └── api/queries_repo (GetRepository)
                   │
                   └── list.NewCmdRepoList(f)   ← ★ 新增：gh repo list
                             │
                             ├── api/queries_repo (ListRepositories)
                             └── api/client       (REST - fetchCurrentUser)

   internal/ghrepo/ghrepo.go    ← ★ 新增：Interface, FromFullName, FromURL
         ▲
         │ 实现 Interface
   api/queries_repo.go          ← ★ 新增：Repository struct, GetRepository, ListRepositories
         │
         │ 调用
         ▼
   api/client.go                ← Phase 3 已有：GraphQL(), REST()
         │
         ▼
   api/http_client.go           ← Phase 3 已有：RoundTripper 链
```

标记 ★ 的部分是 Phase 4 新增或修改的内容。注意 `internal/ghrepo` 是纯粹的类型定义包，不依赖任何项目内部的其他包；`api/queries_repo` 依赖 `internal/ghrepo`（接收 Interface 参数），同时被命令层使用——形成清晰的单向依赖。

---

### 1.3 控制流图

**`gh repo view cli/cli`**

```
gh repo view cli/cli
     │
     ▼
NewCmdRepoView(f) → cobra.Command
     │
     │  args[0] = "cli/cli"
     │  opts.RepoArg = "cli/cli"
     ▼
viewRun(opts)
     │
     ├── ghrepo.FromFullName("cli/cli")
     │       │
     │       └── strings.Split("cli/cli", "/") → ["cli", "cli"]
     │               └── ghRepo{owner:"cli", name:"cli", host:"github.com"}
     │
     ├── opts.HttpClient()              ← 调用 factory 闭包，获取带认证的 *http.Client
     │
     ├── api.NewClientFromHTTP(httpClient)
     │
     └── api.GetRepository(client, repo)
             │
             ├── 构造 GraphQL query（GetRepository）
             ├── variables = {"owner":"cli", "name":"cli"}
             └── client.GraphQL("github.com", query, variables, &result)
                     │
                     ▼
             GitHub GraphQL API → {"data":{"repository":{...}}}
                     │
                     └── result.Repository → *api.Repository
                             │
                             ▼
             printRepository(opts.IO, repository)
                     │
                     └── stdout: name, description, stars, forks, visibility, url
```

**`gh repo list`（未指定用户）**

```
gh repo list
     │
     ▼
NewCmdRepoList(f) → cobra.Command
     │
     │  args = []（无位置参数）
     │  opts.Login = ""
     │  opts.Limit = 30（默认值）
     ▼
listRun(opts)
     │
     ├── login == ""，进入"获取当前用户"分支
     │       │
     │       ├── opts.Config() → cfg
     │       ├── cfg.AuthToken("github.com") → token（非空则继续）
     │       ├── opts.HttpClient() → httpClient
     │       ├── api.NewClientFromHTTP(httpClient)
     │       └── fetchCurrentUser(client)
     │               │
     │               └── client.REST("github.com", "GET", "user", nil, &result)
     │                       │
     │                       ▼
     │               GET https://api.github.com/user
     │                       │
     │                       └── result.Login = "alice"
     │
     ├── login = "alice"
     ├── opts.HttpClient() → httpClient（再次获取）
     ├── api.NewClientFromHTTP(httpClient)
     └── api.ListRepositories(client, "alice", 30)
             │
             ├── 构造 GraphQL query（ListRepositories）
             ├── variables = {"login":"alice", "first":30}
             └── client.GraphQL("github.com", query, variables, &result)
                     │
                     ▼
             GitHub GraphQL API → {"data":{"repositoryOwner":{"repositories":{"nodes":[...]}}}}
                     │
                     └── []api.Repository
                             │
                             ▼
             for each repo: fmt.Fprintf(opts.IO.Out, "%-40s\t%s\n", ...)
                     │
                     └── stdout: "alice/myrepo                            \tpublic"
```

---

### 1.4 数据流图

**从用户输入到格式化输出的完整数据流转**

```
用户输入: gh repo view cli/cli
         │
         ▼
  [参数解析层]
  cobra 将 "cli/cli" 绑定到 opts.RepoArg
         │
         ▼
  [仓库引用解析层]  internal/ghrepo
  FromFullName("cli/cli")
         │
         └── ghRepo{owner:"cli", name:"cli", host:"github.com"}
             实现 Interface：RepoOwner()→"cli", RepoName()→"cli", RepoHost()→"github.com"
         │
         ▼
  [GraphQL 查询层]  api/queries_repo.go
  GetRepository(client, repo ghrepo.Interface)
         │
         ├── 从 repo.RepoOwner() 和 repo.RepoName() 提取变量
         ├── 向 repo.RepoHost() 对应的端点发送请求
         └── 响应解析：
             {
               "data": {                          ← 外层：GraphQL 标准包装
                 "repository": {                  ← 内层：查询字段名
                   "name": "cli",
                   "stargazerCount": 35000,
                   ...
                 }
               }
             }
             │
             ├── 两层嵌套 struct 解析：
             │   var result struct {
             │       Repository Repository `json:"repository"`
             │   }
             │   client.GraphQL(..., &result)   ← GraphQL() 已剥离 "data" 层
             └── &result.Repository             ← *api.Repository
         │
         ▼
  [格式化输出层]  printRepository()
  *api.Repository → 逐字段 fmt.Fprintf
         │
         └── stdout（tab 分隔，便于对齐）：
             name:           cli/cli
             description:    GitHub CLI
             stars:          35000
             forks:          2000
             visibility:     public
             default branch: trunk
             url:            https://github.com/cli/cli
```

**`gh repo list` 的两步 API 调用数据流**

```
用户输入: gh repo list（无参数）
         │
         ▼
  [第一步] REST API：确认当前用户身份
  client.REST("github.com", "GET", "user", nil, &result)
         │
         └── GET https://api.github.com/user
             响应: {"login": "alice", ...}
             result.Login = "alice"
         │
         ▼
  [第二步] GraphQL API：列出仓库
  ListRepositories(client, "alice", 30)
         │
         └── 三层嵌套解析：
             var result struct {
                 RepositoryOwner struct {
                     Repositories struct {
                         Nodes []Repository
                     }
                 }
             }
             result.RepositoryOwner.Repositories.Nodes → []Repository
         │
         ▼
  格式化输出（每行一个仓库）：
  "alice/myrepo                            \tpublic"
```

---

### 1.5 与前三个 Phase 的连接

| 组件                           | 之前的状态              | Phase 4 如何使用                                 |
| ---------------------------- | ------------------ | -------------------------------------------- |
| `api.Client.GraphQL()`       | Phase 3 实现         | `GetRepository` 和 `ListRepositories` 直接调用    |
| `api.Client.REST()`          | Phase 3 实现         | `fetchCurrentUser` 通过 REST 获取当前用户            |
| `cmdutil.Factory.HttpClient` | Phase 3 升级为真实认证客户端 | view 和 list 命令通过 `opts.HttpClient()` 获取      |
| `cmdutil.Factory.Config`     | Phase 2 实现         | `listRun` 检查 token 是否存在                      |
| `cobra.Command` 层次           | Phase 1 建立根命令      | Phase 4 追加 `root → repo → [view, list]` 三层结构 |

---

## Section 2：Implementation Walkthrough

### 2.1 创建 Phase 4 目录和 go.mod

**为什么从 Phase 3 复制而不是从头开始？**

Phase 4 在 Phase 3 的全部代码基础上新增功能，`go.mod`、`api/`（除 queries_repo.go）、`pkg/cmdutil/`、`internal/config/` 等都不需要修改。直接复制整个 Phase 3 目录，然后在此基础上添加新文件，是保持每个 Phase 代码自包含的最简方式。

【现在手敲】

```bash
# 从项目根目录执行
cp -r cli/src/phase-03-api-client cli/src/phase-04-repo-commands

# 创建新增目录
mkdir -p cli/src/phase-04-repo-commands/internal/ghrepo
mkdir -p cli/src/phase-04-repo-commands/pkg/cmd/repo/view
mkdir -p cli/src/phase-04-repo-commands/pkg/cmd/repo/list
```

【验证】

```bash
ls cli/src/phase-04-repo-commands/internal/ghrepo/
ls cli/src/phase-04-repo-commands/pkg/cmd/repo/
```

期望输出：`ghrepo/` 目录存在；`repo/` 目录下存在 `view/` 和 `list/` 子目录。

**关键点解释**

`go.mod` 中的 module 名 `github.com/learngh/gh-impl` 不变，所有 Phase 共享同一个 module 路径，避免跨 Phase 导入路径变动带来的混乱。

---

### 2.2 实现 ghrepo.Interface 和基础类型

**为什么用 Interface 而不是直接用 struct？**

如果只用一个 struct（比如 `type Repo struct{ Owner, Name, Host string }`），则 API 返回的 `Repository` 对象（包含大量字段：Stars、Forks、Description 等）无法直接被"接受仓库引用"的函数消费，必须手动提取 Owner/Name/Host 字段重新构造。

使用 Interface 后：
- `ghRepo`（内部解析结果）实现 Interface
- `api.Repository`（API 返回对象）也实现 Interface
- 任何接受 `ghrepo.Interface` 的函数（如 `GetRepository`）都能同时接受这两种类型

这是 Go 中"面向接口编程"的典型应用：**接口由使用者定义（`ghrepo` 定义了它需要什么），实现者只需满足接口，无需声明"我实现了某接口"**。

【现在手敲】

```go
// 文件：internal/ghrepo/ghrepo.go
package ghrepo

import (
	"fmt"
	"net/url"
	"strings"
)

// Interface represents a GitHub repository reference.
type Interface interface {
	RepoName() string
	RepoOwner() string
	RepoHost() string
}

type ghRepo struct {
	owner string
	name  string
	host  string
}

func (r ghRepo) RepoOwner() string { return r.owner }
func (r ghRepo) RepoName() string  { return r.name }
func (r ghRepo) RepoHost() string  { return r.host }

// New creates a repo reference for github.com.
func New(owner, name string) Interface {
	return ghRepo{owner: owner, name: name, host: "github.com"}
}

// NewWithHost creates a repo reference for a specific host.
func NewWithHost(owner, name, host string) Interface {
	return ghRepo{owner: owner, name: name, host: host}
}

// FullName returns "owner/name".
func FullName(r Interface) string {
	return fmt.Sprintf("%s/%s", r.RepoOwner(), r.RepoName())
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./internal/ghrepo/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`ghRepo` 是**未导出的 struct**（小写 g），但 `New` 和 `NewWithHost` 返回**导出的 Interface**。这是 Go 中常见的"工厂函数 + 未导出实现"模式：外部包只能通过 Interface 与 `ghRepo` 交互，无法直接访问其字段，保证了封装性。

---

### 2.3 实现 FromFullName

**为什么要处理三段式 `host/owner/name`？**

GitHub Enterprise 等自托管实例的 URL 格式为 `github.mycompany.com/owner/name`。`gh` 支持在命令行直接输入三段式引用，所以 `FromFullName` 需要同时处理两种格式：`owner/name`（默认 github.com）和 `host/owner/name`（自定义 host）。

【现在手敲】

在 `internal/ghrepo/ghrepo.go` 中追加以下函数：

```go
// FromFullName parses "owner/name" or "host/owner/name".
// Returns error if the string is not a valid repo reference.
func FromFullName(nwo string) (Interface, error) {
	parts := strings.Split(nwo, "/")
	switch len(parts) {
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid repository: %q", nwo)
		}
		return New(parts[0], parts[1]), nil
	case 3:
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid repository: %q", nwo)
		}
		return NewWithHost(parts[1], parts[2], parts[0]), nil
	default:
		return nil, fmt.Errorf("expected \"owner/name\" or \"host/owner/name\", got %q", nwo)
	}
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./internal/ghrepo/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

注意三段式时的参数顺序：`strings.Split("github.com/owner/name", "/")` 得到 `["github.com", "owner", "name"]`，所以调用 `NewWithHost(parts[1], parts[2], parts[0])`，即 `NewWithHost(owner, name, host)`——**host 是最后一个参数**，不要搞反。

---

### 2.4 实现 FromURL

**什么时候需要从 URL 解析？**

当用户粘贴了完整的 GitHub URL（如 `https://github.com/cli/cli`）或者从 `git remote -v` 输出中提取 URL（如 `git@github.com:cli/cli.git`）时，需要 `FromURL` 将其转换为 `ghrepo.Interface`。Phase 4 目前不直接使用此函数，但 `ghrepo` 包提供它是为了后续命令（`gh repo clone` 等）的复用。

【现在手敲】

在 `internal/ghrepo/ghrepo.go` 中追加以下函数：

```go
// FromURL parses a GitHub repository URL.
// Supports https://github.com/owner/name and git@github.com:owner/name.git
func FromURL(u *url.URL) (Interface, error) {
	if u.Hostname() == "" {
		return nil, fmt.Errorf("no hostname in URL")
	}
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid GitHub URL: %s", u)
	}
	return NewWithHost(parts[0], parts[1], u.Hostname()), nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./internal/ghrepo/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

- `strings.TrimPrefix(u.Path, "/")` 去掉路径开头的斜杠（`/cli/cli` → `cli/cli`）
- `strings.TrimSuffix(path, ".git")` 去掉 `.git` 后缀（`cli/cli.git` → `cli/cli`）
- `strings.SplitN(path, "/", 2)` 的第三个参数 `2` 确保最多切两段——如果仓库名中含有 `/`（理论上不合法，但防御性处理），不会被错误切开

---

### 2.5 编写 ghrepo 测试

**测试策略：覆盖边界情况**

`FromFullName` 的边界情况包括：缺少 owner（`/repo`）、缺少 name（`owner/`）、完全无斜杠（`invalid`）、三段式。每个错误路径都应该有对应的测试用例。

【现在手敲】

```go
// 文件：internal/ghrepo/ghrepo_test.go
package ghrepo

import (
	"net/url"
	"testing"
)

func TestFromFullName(t *testing.T) {
	tests := []struct {
		input   string
		owner   string
		name    string
		host    string
		wantErr bool
	}{
		{"cli/cli", "cli", "cli", "github.com", false},
		{"owner/repo", "owner", "repo", "github.com", false},
		{"github.com/owner/repo", "owner", "repo", "github.com", false},
		{"invalid", "", "", "", true},
		{"/repo", "", "", "", true},
		{"owner/", "", "", "", true},
	}
	for _, tc := range tests {
		got, err := FromFullName(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("FromFullName(%q): expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("FromFullName(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got.RepoOwner() != tc.owner || got.RepoName() != tc.name || got.RepoHost() != tc.host {
			t.Errorf("FromFullName(%q) = {%s/%s @ %s}, want {%s/%s @ %s}",
				tc.input, got.RepoOwner(), got.RepoName(), got.RepoHost(),
				tc.owner, tc.name, tc.host)
		}
	}
}

func TestFullName(t *testing.T) {
	r := New("owner", "repo")
	if got := FullName(r); got != "owner/repo" {
		t.Errorf("FullName = %q, want owner/repo", got)
	}
}

func TestFromURL(t *testing.T) {
	u, _ := url.Parse("https://github.com/cli/cli")
	r, err := FromURL(u)
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	if r.RepoOwner() != "cli" || r.RepoName() != "cli" {
		t.Errorf("FromURL = %s/%s, want cli/cli", r.RepoOwner(), r.RepoName())
	}
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go test ./internal/ghrepo/...
```

期望输出：`ok  	github.com/learngh/gh-impl/internal/ghrepo`

**关键点解释**

使用 table-driven tests（测试表格）是 Go 社区的惯用做法：将多个测试用例放在一个切片中，用同一个测试逻辑遍历，减少代码重复，并且新增用例只需添加一行数据。

---

### 2.6 实现 Repository struct

**为什么 Repository 实现了 ghrepo.Interface？**

`api.Repository` 是 API 响应的完整数据模型，包含 Stars、Forks、Description 等丰富字段。同时，它通过实现三个方法（`RepoOwner`、`RepoName`、`RepoHost`）满足 `ghrepo.Interface`。

这样设计的好处：`GetRepository` 函数的签名是 `func GetRepository(client *Client, repo ghrepo.Interface)`——任何实现了 Interface 的对象（`ghRepo` 或 `Repository`）都能作为参数传入。例如，你可以先用 `GetRepository` 获取一个仓库，然后把返回的 `*Repository` 直接传给另一个接受 `ghrepo.Interface` 的函数（比如未来的 `CloneRepository(client, repo ghrepo.Interface)`），而不需要手动提取 owner/name 重新构造引用。

【现在手敲】

```go
// 文件：api/queries_repo.go
package api

import (
	"fmt"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

// Repository represents a GitHub repository.
type Repository struct {
	ID               string
	Name             string
	NameWithOwner    string
	Owner            struct{ Login string }
	Description      string
	IsPrivate        bool
	IsFork           bool
	StargazerCount   int
	ForkCount        int
	DefaultBranchRef struct{ Name string }
	URL              string
}

// RepoOwner implements ghrepo.Interface.
func (r Repository) RepoOwner() string { return r.Owner.Login }

// RepoName implements ghrepo.Interface.
func (r Repository) RepoName() string { return r.Name }

// RepoHost implements ghrepo.Interface.
func (r Repository) RepoHost() string { return "github.com" }
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./api/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`Owner struct{ Login string }` 是**匿名 struct 字段**。GraphQL 响应中 `owner` 是一个嵌套对象 `{"login": "cli"}`，Go 的 JSON 解析器会将其映射到嵌套 struct。这比定义独立的 `type Owner struct{ Login string }` 更简洁，适用于只在这一个地方使用的嵌套结构。

`RepoHost()` 硬编码返回 `"github.com"`——Phase 4 只支持 github.com，未来支持 GitHub Enterprise 时可以通过在 `Repository` 中增加 host 字段来解决。

---

### 2.7 实现 GetRepository

**GraphQL 嵌套响应解析：为什么需要两层 struct？**

GraphQL 的标准响应格式是：
```json
{
  "data": {                    ← 第一层：GraphQL 协议层（由 client.GraphQL() 剥离）
    "repository": {            ← 第二层：查询字段名（需要在本函数处理）
      "name": "cli",
      ...
    }
  }
}
```

`client.GraphQL()` 已经剥离了最外层的 `"data"` 包装（Phase 3 实现），将内容反序列化到 `v` 参数中。因此在 `GetRepository` 中，我们声明一个带 `Repository` 字段的匿名 struct 来匹配 `"repository"` 这一层，最终拿到 `result.Repository`。

【现在手敲】

在 `api/queries_repo.go` 中追加 `GetRepository` 函数：

```go
// GetRepository fetches a single repository by owner/name.
func GetRepository(client *Client, repo ghrepo.Interface) (*Repository, error) {
	var result struct {
		Repository Repository `json:"repository"`
	}
	query := `
query GetRepository($owner: String!, $name: String!) {
    repository(owner: $owner, name: $name) {
        id
        name
        nameWithOwner
        owner { login }
        description
        isPrivate
        isFork
        stargazerCount
        forkCount
        defaultBranchRef { name }
        url
    }
}`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	return &result.Repository, nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./api/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

- `variables` 使用 `map[string]interface{}`：GraphQL 变量是动态 key-value，使用 map 可以在 JSON 序列化时自动生成正确格式 `{"owner":"cli","name":"cli"}`。
- `client.GraphQL(repo.RepoHost(), ...)` 将 host 作为参数传入，而不是硬编码 `"github.com"`——这让 `GetRepository` 天然支持 GitHub Enterprise（只要 `repo.RepoHost()` 返回正确的 host）。
- 错误用 `fmt.Errorf("...: %w", err)` 包装：`%w` 保留原始错误（可通过 `errors.Is/As` 解包），同时在外层添加上下文信息。

---

### 2.8 实现 ListRepositories

**分页参数：为什么用 `first` 而不是 `after`？**

GraphQL Cursor-based 分页通常使用 `first`（每页数量）和 `after`（游标，指向上一页最后一项）。Phase 4 使用简单的 `first: $first` 策略——只取前 N 条，不实现多页翻页。这是因为 Phase 4 的目标是演示基础 GraphQL 查询，完整的分页实现（处理 `pageInfo.endCursor` 和循环请求）会在后续 Phase 引入。

**limit 的边界检查**

```go
if limit <= 0 || limit > 100 {
    limit = 30
}
```

GitHub GraphQL API 对 `first` 的最大值有限制（通常 100），超出会报错。这里做了防御性检查，确保传入的 limit 合法。

【现在手敲】

在 `api/queries_repo.go` 中追加 `ListRepositories` 函数：

```go
// ListRepositories lists repositories for a user.
func ListRepositories(client *Client, login string, limit int) ([]Repository, error) {
	var result struct {
		RepositoryOwner struct {
			Repositories struct {
				Nodes []Repository
			}
		}
	}
	query := `
query ListRepositories($login: String!, $first: Int!) {
    repositoryOwner(login: $login) {
        repositories(first: $first, orderBy: {field: PUSHED_AT, direction: DESC}) {
            nodes {
                id
                name
                nameWithOwner
                owner { login }
                description
                isPrivate
                isFork
                stargazerCount
                forkCount
                defaultBranchRef { name }
                url
            }
        }
    }
}`
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	variables := map[string]interface{}{
		"login": login,
		"first": limit,
	}
	if err := client.GraphQL("github.com", query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	return result.RepositoryOwner.Repositories.Nodes, nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./api/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`result` 的三层嵌套 struct 完全对应 GraphQL 响应结构：

```
result
  └── RepositoryOwner     ← 对应 "repositoryOwner"
        └── Repositories  ← 对应 "repositories"
              └── Nodes   ← 对应 "nodes"（[]Repository）
```

注意 Go struct 字段名使用 PascalCase（`RepositoryOwner`），而 JSON key 是 camelCase（`repositoryOwner`）。**Go 的 JSON 解析默认支持大小写不敏感匹配**，所以不需要显式写 `json:"repositoryOwner"` tag（写上也无妨）。

---

### 2.9 编写 queries_repo 测试

**测试策略：httptest.Server 模拟 API**

与 Phase 3 的测试相同，使用 `httptest.NewServer` 启动一个本地 HTTP server 返回预设 JSON，通过 `rewriteTransport` 将所有请求重定向到该 server，从而在不发真实网络请求的情况下测试 API 调用逻辑。

【现在手敲】

```go
// 文件：api/queries_repo_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

func TestGetRepository(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"id":            "R_123",
					"name":          "cli",
					"nameWithOwner": "cli/cli",
					"owner":         map[string]string{"login": "cli"},
					"description":   "GitHub CLI",
					"isPrivate":     false,
					"isFork":        false,
					"stargazerCount": 1000,
					"forkCount":     200,
					"defaultBranchRef": map[string]string{"name": "main"},
					"url":           "https://github.com/cli/cli",
				},
			},
		})
	}))
	defer srv.Close()

	// rewriteTransport is defined in client_test.go (same package).
	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	httpClient := &http.Client{Transport: transport}
	client := NewClientFromHTTP(httpClient)

	repo := ghrepo.New("cli", "cli")
	result, err := GetRepository(client, repo)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if result.Name != "cli" {
		t.Errorf("Name = %q, want cli", result.Name)
	}
	if result.StargazerCount != 1000 {
		t.Errorf("StargazerCount = %d, want 1000", result.StargazerCount)
	}
}

func TestListRepositories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repositoryOwner": map[string]interface{}{
					"repositories": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"id": "R_1", "name": "repo1", "nameWithOwner": "alice/repo1",
								"owner": map[string]string{"login": "alice"},
								"description": "Repo 1", "isPrivate": false, "isFork": false,
								"stargazerCount": 5, "forkCount": 0,
								"defaultBranchRef": map[string]string{"name": "main"},
								"url": "https://github.com/alice/repo1",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	httpClient := &http.Client{Transport: transport}
	client := NewClientFromHTTP(httpClient)

	repos, err := ListRepositories(client, "alice", 10)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].Name != "repo1" {
		t.Errorf("repos[0].Name = %q, want repo1", repos[0].Name)
	}
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go test ./api/...
```

期望输出：`ok  	github.com/learngh/gh-impl/api`

**关键点解释**

`rewriteTransport` 在 `api/client_test.go` 中已经定义（Phase 3 引入），`queries_repo_test.go` 在同一 package（`package api`）中，可以直接复用它，不需要重新定义。这是 Go 中**测试文件共享同一 package 内其他测试辅助类型**的正常用法。

---

### 2.10 实现 NewCmdRepoView

**参数解析：`cobra.MaximumNArgs(1)` 的含义**

`cobra.MaximumNArgs(1)` 表示该命令最多接受 1 个位置参数。如果用户传入 0 个参数（`gh repo view`），`args` 为空，`opts.RepoArg` 保持空字符串，后续由 `viewRun` 内部判断并报错（目前 Phase 4 未实现"从当前目录 git remote 推断仓库"的功能，所以报错提示用户提供参数）。

【现在手敲】

```go
// 文件：pkg/cmd/repo/view/view.go
package view

import (
	"fmt"
	"net/http"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ViewOptions holds all inputs for the repo view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	RepoArg    string // positional: owner/name
}

// NewCmdRepoView creates the `gh repo view` command.
func NewCmdRepoView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "view [<repository>]",
		Short: "View repository information",
		Long:  "Display details about a GitHub repository.\n\nWith no argument, shows the repository for the current directory.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			return viewRun(opts)
		},
	}
	return cmd
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./pkg/cmd/repo/view/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`ViewOptions` 中 `HttpClient` 的类型是 `func() (*http.Client, error)` 而不是 `*http.Client`。这是**懒加载闭包**模式：`*http.Client` 的创建需要读取 Config（可能失败），延迟到 `viewRun` 真正需要发请求时再调用，而不是在命令解析时就初始化。

---

### 2.11 实现 viewRun 和 printRepository

**为什么 printRepository 接受 `*iostreams.IOStreams` 而不是 `io.Writer`？**

`IOStreams` 封装了 stdout、stderr 和 isTerminal 信息，未来可以根据 `io.IsTerminal()` 决定是否输出颜色或分页（Phase 3 已设计好这个扩展点）。现在虽然只用 `io.Out`，但接口保持一致，方便后续功能扩展。

【现在手敲】

在 `pkg/cmd/repo/view/view.go` 中追加以下函数：

```go
func viewRun(opts *ViewOptions) error {
	if opts.RepoArg == "" {
		return fmt.Errorf("repository argument required (e.g. gh repo view owner/name)")
	}

	repo, err := ghrepo.FromFullName(opts.RepoArg)
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	repository, err := api.GetRepository(client, repo)
	if err != nil {
		return err
	}

	printRepository(opts.IO, repository)
	return nil
}

func printRepository(io *iostreams.IOStreams, r *api.Repository) {
	w := io.Out
	fmt.Fprintf(w, "name:\t%s\n", r.NameWithOwner)
	if r.Description != "" {
		fmt.Fprintf(w, "description:\t%s\n", r.Description)
	}
	fmt.Fprintf(w, "stars:\t%d\n", r.StargazerCount)
	fmt.Fprintf(w, "forks:\t%d\n", r.ForkCount)
	visibility := "public"
	if r.IsPrivate {
		visibility = "private"
	}
	fmt.Fprintf(w, "visibility:\t%s\n", visibility)
	if r.DefaultBranchRef.Name != "" {
		fmt.Fprintf(w, "default branch:\t%s\n", r.DefaultBranchRef.Name)
	}
	fmt.Fprintf(w, "url:\t%s\n", r.URL)
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./pkg/cmd/repo/view/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`printRepository` 使用 `\t`（tab）分隔 key 和 value，而不是固定宽度的空格。这样配合 `tabwriter`（未来可以引入）可以自动对齐多行输出。目前虽然没有用 tabwriter，但格式上已经为其预留了位置。

`if r.Description != ""` 和 `if r.DefaultBranchRef.Name != ""`：可选字段只在非空时输出，避免输出空行，保持输出整洁。

---

### 2.12 编写 view 测试

**rewriteTransport 在 view 包中的重新定义**

`api` 包的 `client_test.go` 中的 `rewriteTransport` 是 **`package api` 内的测试辅助类型**，外部包无法访问。`view_test.go` 在 `package view` 中，需要自己定义同名的 `rewriteTransport`。这是 Go 测试中的常见做法——每个需要 HTTP 重定向的测试包各自维护这个辅助类型。

【现在手敲】

```go
// 文件：pkg/cmd/repo/view/view_test.go
package view

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestViewRun_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"id":              "R_1",
					"name":            "cli",
					"nameWithOwner":   "cli/cli",
					"owner":           map[string]string{"login": "cli"},
					"description":     "GitHub CLI",
					"isPrivate":       false,
					"isFork":          false,
					"stargazerCount":  35000,
					"forkCount":       2000,
					"defaultBranchRef": map[string]string{"name": "trunk"},
					"url":             "https://github.com/cli/cli",
				},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()

	opts := &ViewOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
		RepoArg: "cli/cli",
	}

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "cli/cli") {
		t.Errorf("output missing repo name: %q", output)
	}
	if !strings.Contains(output, "35000") {
		t.Errorf("output missing star count: %q", output)
	}
	if !strings.Contains(output, "GitHub CLI") {
		t.Errorf("output missing description: %q", output)
	}
}

func TestViewRun_MissingArg(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ViewOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		RepoArg:    "",
	}
	if err := viewRun(opts); err == nil {
		t.Fatal("expected error for missing repo arg")
	}
}

// rewriteTransport redirects all requests to a base URL for testing.
type rewriteTransport struct {
	base  string
	inner http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(rt.base, "http://")
	return rt.inner.RoundTrip(clone)
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go test ./pkg/cmd/repo/view/...
```

期望输出：`ok  	github.com/learngh/gh-impl/pkg/cmd/repo/view`

**关键点解释**

`iostreams.Test()` 返回四个值：`(ios *IOStreams, in *bytes.Buffer, out *bytes.Buffer, err *bytes.Buffer)`。测试中通过 `out.String()` 检查命令输出，而不需要解析真实的文件描述符，完全在内存中进行。

---

### 2.13 实现 NewCmdRepoList

**`--limit` flag：`IntVarP` 与 `IntVar` 的区别**

`IntVarP` 同时注册长 flag（`--limit`）和短 flag（`-L`）。短 flag 使用大写字母 `L` 而不是小写 `l`，是因为 `-l` 在 Unix 传统中常用于 "long format"，大写 `L` 避免冲突，且与真实 `gh` CLI 保持一致。

【现在手敲】

```go
// 文件：pkg/cmd/repo/list/list.go
package list

import (
	"fmt"
	"net/http"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ListOptions holds all inputs for the repo list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (cmdutil.Config, error)
	Login      string // GitHub username; if empty, use authenticated user
	Limit      int
}

// NewCmdRepoList creates the `gh repo list` command.
func NewCmdRepoList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Limit:      30,
	}

	cmd := &cobra.Command{
		Use:   "list [<user>]",
		Short: "List repositories",
		Long:  "List GitHub repositories.\n\nWith no argument, lists repositories for the authenticated user.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Login = args[0]
			}
			return listRun(opts)
		},
	}
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of repositories to list")
	return cmd
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./pkg/cmd/repo/list/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`opts.Limit` 在两处被赋值：
1. `ListOptions` struct 字面量中 `Limit: 30`（初始化默认值）
2. `cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, ...)`（将 flag 的默认值也设为 30）

两处都写 30 是必要的：struct 初始化确保 `opts.Limit` 在 flag 注册前就是有效值；`IntVarP` 的第四个参数确保用户未传 `--limit` 时 flag 系统也能给出正确的默认值显示在 `--help` 输出中。

---

### 2.14 实现 listRun 和 fetchCurrentUser

**listRun 的两步逻辑：为什么需要两次 API 调用？**

GitHub GraphQL API 的 `repositoryOwner` 查询需要一个明确的 `login`（用户名）参数，没有"当前认证用户的仓库"这样的隐式 API（GraphQL 有 `viewer` 字段可以获取当前用户信息，但 `viewer` 下的 repositories 字段结构与 `repositoryOwner` 稍有不同）。

最简单的实现是：**先用 REST API `GET /user` 获取当前用户名，再用 GraphQL 查询该用户的仓库**。

**fetchCurrentUser 复用了 Phase 3 的 `client.REST`**

这证明了 Phase 3 API client 设计的通用性：`REST()` 方法不仅能用于 `gh api` 命令，也能被其他命令的内部逻辑使用——`fetchCurrentUser` 是一个内部辅助函数，不暴露给用户，但复用了相同的 HTTP 客户端基础设施。

【现在手敲】

在 `pkg/cmd/repo/list/list.go` 中追加以下函数：

```go
func listRun(opts *ListOptions) error {
	login := opts.Login
	if login == "" {
		// Use the authenticated user's login from config.
		cfg, err := opts.Config()
		if err != nil {
			return fmt.Errorf("could not load config: %w", err)
		}
		tok, err := cfg.AuthToken("github.com")
		if err != nil || tok == "" {
			return fmt.Errorf("not authenticated: run `gh auth login`")
		}
		// Fetch the current user's login via the API.
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		client := api.NewClientFromHTTP(httpClient)
		login, err = fetchCurrentUser(client)
		if err != nil {
			return err
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	repos, err := api.ListRepositories(client, login, opts.Limit)
	if err != nil {
		return err
	}

	for _, r := range repos {
		visibility := "public"
		if r.IsPrivate {
			visibility = "private"
		}
		fmt.Fprintf(opts.IO.Out, "%-40s\t%s\n", r.NameWithOwner, visibility)
	}
	return nil
}

// fetchCurrentUser returns the authenticated user's login name.
func fetchCurrentUser(client *api.Client) (string, error) {
	var result struct {
		Login string `json:"login"`
	}
	if err := client.REST("github.com", "GET", "user", nil, &result); err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	return result.Login, nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go vet ./pkg/cmd/repo/list/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`fmt.Fprintf(opts.IO.Out, "%-40s\t%s\n", r.NameWithOwner, visibility)` 中：
- `%-40s`：左对齐，最小宽度 40 字符（右侧用空格补齐），让仓库名列整齐对齐
- `\t%s\n`：tab 分隔 visibility 列，再换行

注意 `listRun` 中 `opts.HttpClient()` 被调用了两次（获取当前用户时一次，调用 ListRepositories 时一次）。这是因为 `HttpClient` 是一个懒加载闭包，每次调用都会重新执行（但由于使用了相同的底层 Config，两次调用会得到行为一致的客户端）。

---

### 2.15 编写 list 测试

**memConfig：实现 cmdutil.Config 接口的最小测试替身**

`listRun` 依赖 `cmdutil.Config` 的 `AuthToken()` 方法。在测试中，我们不想读取真实的配置文件，所以定义一个内存实现 `memConfig`，只需满足 `Config` 接口的所有方法（大多数方法直接返回零值）。

【现在手敲】

```go
// 文件：pkg/cmd/repo/list/list_test.go
package list

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

type memConfig struct {
	token string
}

func (c *memConfig) Get(hostname, key string) (string, error)    { return "", nil }
func (c *memConfig) Set(hostname, key, value string) error       { return nil }
func (c *memConfig) Write() error                                 { return nil }
func (c *memConfig) Hosts() []string                              { return nil }
func (c *memConfig) AuthToken(hostname string) (string, error)    { return c.token, nil }
func (c *memConfig) Login(hostname, username, token string) error { return nil }
func (c *memConfig) Logout(hostname string) error                 { return nil }

var _ cmdutil.Config = (*memConfig)(nil)

func TestListRun_WithLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repositoryOwner": map[string]interface{}{
					"repositories": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"id": "R_1", "name": "myrepo", "nameWithOwner": "alice/myrepo",
								"owner": map[string]string{"login": "alice"},
								"description": "", "isPrivate": false, "isFork": false,
								"stargazerCount": 0, "forkCount": 0,
								"defaultBranchRef": map[string]string{"name": "main"},
								"url": "https://github.com/alice/myrepo",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &ListOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{
				Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport},
			}, nil
		},
		Config: func() (cmdutil.Config, error) { return &memConfig{token: "tok"}, nil },
		Login:  "alice",
		Limit:  10,
	}

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "alice/myrepo") {
		t.Errorf("output missing repo: %q", out.String())
	}
}

// rewriteTransport redirects all requests to a base URL for testing.
type rewriteTransport struct {
	base  string
	inner http.RoundTripper
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(rt.base, "http://")
	return rt.inner.RoundTrip(clone)
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go test ./pkg/cmd/repo/list/...
```

期望输出：`ok  	github.com/learngh/gh-impl/pkg/cmd/repo/list`

**关键点解释**

```go
var _ cmdutil.Config = (*memConfig)(nil)
```

这行代码是一个**编译期接口检查**：通过将 `(*memConfig)(nil)` 赋值给 `cmdutil.Config` 类型的空白标识符，如果 `memConfig` 没有实现 `Config` 的所有方法，编译器会立即报错。这是 Go 中验证接口实现的惯用手法，不会产生任何运行时开销。

`TestListRun_WithLogin` 直接设置 `opts.Login = "alice"` 跳过了"获取当前用户"的分支，专注于测试"列出指定用户仓库"的主路径。两步逻辑中的第一步（`fetchCurrentUser`）已被 `opts.Login` 非空条件跳过，因此测试 server 只需要处理 GraphQL 请求，不需要同时处理 REST `/user` 请求。

---

### 2.16 实现 NewCmdRepo 并注册到 root

**`cmd.AddCommand` 的层次：root → repo → [view, list]**

`gh` 的命令树是三级结构：
- 根命令（`gh`）：`root.NewCmdRoot`
- 命令组（`gh repo`）：`repo.NewCmdRepo`
- 子命令（`gh repo view`、`gh repo list`）：`view.NewCmdRepoView`、`list.NewCmdRepoList`

`NewCmdRepo` 是一个**命令组（command group）**，它本身不执行任何操作（没有 `RunE`），只作为 `view` 和 `list` 的父节点，cobra 会自动为其生成显示子命令列表的帮助信息。

【现在手敲】

创建命令组文件：

```go
// 文件：pkg/cmd/repo/repo.go
package repo

import (
	"github.com/learngh/gh-impl/pkg/cmd/repo/list"
	"github.com/learngh/gh-impl/pkg/cmd/repo/view"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdRepo creates the `gh repo` command group.
func NewCmdRepo(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <subcommand>",
		Short: "Manage repositories",
		Long:  "Work with GitHub repositories.",
	}
	cmd.AddCommand(view.NewCmdRepoView(f))
	cmd.AddCommand(list.NewCmdRepoList(f))
	return cmd
}
```

然后修改 `pkg/cmd/root/root.go`，追加 import 和命令注册：

```go
// 在 root.go 的 import 块中追加：
import (
    // ... 已有导入 ...
    repoCmd "github.com/learngh/gh-impl/pkg/cmd/repo"
)

// 在 NewCmdRoot 函数中，cmd.AddCommand(apiCmd.NewCmdAPI(f)) 之后追加：

// Repo command group.
cmd.AddCommand(repoCmd.NewCmdRepo(f))
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go build ./...
```

期望输出：无错误（exit code 0）。

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go run ./cmd/gh/main.go repo --help
```

期望输出：显示 `repo` 命令的帮助，包含 `view` 和 `list` 两个子命令。

**关键点解释**

`repoCmd` 是 import alias——因为 `repo` 这个名字过于通用，可能与其他包名冲突，使用 alias `repoCmd` 使代码意图更清晰。这与 Phase 3 中 `apiCmd "github.com/learngh/gh-impl/pkg/cmd/api"` 的模式一致。

`NewCmdRepo` 没有设置 `RunE`：当用户只输入 `gh repo`（不带子命令）时，cobra 默认显示帮助信息，列出所有可用子命令。这是命令组的标准行为。

---

### 2.17 运行所有测试

**最终验证：确保所有新增代码通过测试，且未破坏已有功能**

【现在手敲】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go test ./...
```

【验证】

期望输出（顺序可能不同）：

```
ok  	github.com/learngh/gh-impl/api
ok  	github.com/learngh/gh-impl/internal/ghrepo
ok  	github.com/learngh/gh-impl/pkg/cmd/repo/view
ok  	github.com/learngh/gh-impl/pkg/cmd/repo/list
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/login
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/logout
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/status
```

所有包显示 `ok`，无 `FAIL`。

也可以单独验证构建：

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-04-repo-commands
go build -v ./...
```

期望输出：列出所有编译的包，无错误。

**关键点解释**

`go test ./...` 递归测试当前目录下所有包。如果某个包没有测试文件，它会显示 `? github.com/learngh/gh-impl/xxx [no test files]`，这是正常的，不是失败。只有 `FAIL` 才表示测试失败。

---

## 附录：Phase 4 文件结构总览

```
phase-04-repo-commands/
├── go.mod                              # 与 Phase 3 相同，module github.com/learngh/gh-impl
├── go.sum
├── cmd/
│   └── gh/
│       └── main.go                    # 入口文件（不变）
├── internal/
│   ├── config/                        # Phase 2 实现（不变）
│   └── ghrepo/                        # ★ 新增
│       ├── ghrepo.go                  #   Interface, New, FromFullName, FromURL
│       └── ghrepo_test.go             #   单元测试
├── api/
│   ├── client.go                      # Phase 3 实现（不变）
│   ├── client_test.go                 # Phase 3 实现（不变）
│   ├── http_client.go                 # Phase 3 实现（不变）
│   ├── http_client_test.go            # Phase 3 实现（不变）
│   ├── queries_repo.go                # ★ 新增：Repository, GetRepository, ListRepositories
│   └── queries_repo_test.go           # ★ 新增：API 测试
├── pkg/
│   ├── cmd/
│   │   ├── root/
│   │   │   └── root.go               # ★ 修改：注册 repoCmd
│   │   ├── auth/                      # Phase 2 实现（不变）
│   │   ├── api/                       # Phase 3 实现（不变）
│   │   └── repo/                      # ★ 新增
│   │       ├── repo.go               #   NewCmdRepo（命令组）
│   │       ├── view/
│   │       │   ├── view.go           #   NewCmdRepoView, viewRun, printRepository
│   │       │   └── view_test.go      #   命令测试
│   │       └── list/
│   │           ├── list.go           #   NewCmdRepoList, listRun, fetchCurrentUser
│   │           └── list_test.go      #   命令测试
│   ├── cmdutil/                       # Phase 1 实现（不变）
│   └── iostreams/                     # Phase 1 实现（不变）
└── internal/
    └── factory/
        └── factory.go                 # Phase 3 修改（不变）
```

标记 ★ 的是 Phase 4 新增或修改的文件。
