# Phase 5 学习指南：Issue 命令

---

## Section 1：全局视图

### 1.1 这个 Phase 做了什么

Phase 5 在 Phase 4（仓库命令）的基础上，新增了三条 Issue 相关命令：`gh issue list`、`gh issue view`、`gh issue create`。具体来说，它做了四件事：

1. **实现 `api/queries_issue.go`**：定义 `Issue`、`Label`、`User` struct，以及三个 API 函数——`ListIssues`（列出仓库 Issue）、`GetIssue`（查单个 Issue）、`CreateIssue`（新建 Issue）。前两个使用 GraphQL，最后一个使用 REST。

2. **实现 `pkg/cmd/issue/list`**：`gh issue list --repo owner/name [--state open|closed|all] [--limit N]` 命令，支持三种过滤状态；`--state all` 需要分两次 GraphQL 查询再合并。

3. **实现 `pkg/cmd/issue/view`**：`gh issue view <number> --repo owner/name` 命令，按 Issue 编号查看详情，格式化输出标题、状态、作者、标签和正文。

4. **实现 `pkg/cmd/issue/create`**：`gh issue create --repo owner/name --title "..." [--body "..."]` 命令，通过 REST POST 新建 Issue，`--title` 是必填 flag，由 cobra 在执行前自动验证。

Phase 4 只有**读操作**（`repo view`、`repo list`），Phase 5 则同时引入了**读操作**（list、view）和**写操作**（create）。写操作选择了 REST 而非 GraphQL，原因在于 GitHub GraphQL 的 `createIssue` mutation 要求传入 `repositoryId`，必须先额外发一次查询获取 ID；而 REST `POST /repos/{owner}/{repo}/issues` 只需 `owner` 和 `name`，直接可用。这个决策体现了"选择适合场景的 API 风格"而非"统一使用一种协议"。

---

### 1.2 模块依赖图

```
                    ┌─────────────────────────────────────┐
                    │         Phase 5 新增 / 修改           │
                    └─────────────────────────────────────┘

   pkg/cmd/root/root.go          ← ★ 修改：注册 issue 命令组
         │
         └── issueCmd.NewCmdIssue(f)
                   │
                   ├── list.NewCmdIssueList(f)    ← ★ 新增：gh issue list
                   │         │
                   │         ├── internal/ghrepo   (FromFullName)
                   │         └── api/queries_issue (ListIssues)
                   │
                   ├── view.NewCmdIssueView(f)    ← ★ 新增：gh issue view
                   │         │
                   │         ├── internal/ghrepo   (FromFullName)
                   │         └── api/queries_issue (GetIssue)
                   │
                   └── create.NewCmdIssueCreate(f) ← ★ 新增：gh issue create
                             │
                             ├── internal/ghrepo   (FromFullName)
                             └── api/queries_issue (CreateIssue)

   api/queries_issue.go         ← ★ 新增：Issue/Label/User struct，三个查询函数
         │
         ├── GraphQL ──► api/client.go → GitHub GraphQL API  (list, view)
         └── REST    ──► api/client.go → GitHub REST API      (create)

   internal/ghrepo/             ← Phase 4 已有，Phase 5 直接复用
   api/client.go                ← Phase 3 已有：GraphQL(), REST()
```

标记 ★ 的部分是 Phase 5 新增或修改的内容。`api/queries_issue` 是唯一新增的 API 层文件，三个命令都依赖它，体现了"数据访问逻辑集中在 api/ 层"的设计原则。

---

### 1.3 控制流图

**`gh issue list --repo cli/cli --state open`**

```
gh issue list --repo cli/cli --state open
     │
     ▼
NewCmdIssueList(f) → cobra.Command
     │
     │  opts.Repo  = "cli/cli"
     │  opts.State = "open"（默认值）
     │  opts.Limit = 30（默认值）
     ▼
listRun(opts)
     │
     ├── ghrepo.FromFullName("cli/cli") → repo{owner:"cli", name:"cli", host:"github.com"}
     │
     ├── opts.HttpClient() → *http.Client（带 Token 认证）
     │
     ├── api.NewClientFromHTTP(httpClient) → *api.Client
     │
     ├── strings.ToLower("open") → "open"
     │   switch → graphqlState = "OPEN"
     │
     └── api.ListIssues(client, repo, "OPEN", 30)
             │
             ├── 构造 GraphQL query（ListIssues）
             ├── variables = {"owner":"cli", "name":"cli", "states":["OPEN"], "first":30}
             └── client.GraphQL("github.com", query, variables, &result)
                     │
                     ▼
             GitHub GraphQL API → {"data":{"repository":{"issues":{"nodes":[...]}}}}
                     │
                     └── []api.Issue
                             │
                             ▼
             for each issue: fmt.Fprintf(opts.IO.Out, "#%-5d  %s\n", ...)
                     │
                     └── stdout: "#1      Bug report"
```

**`gh issue list --repo cli/cli --state all`（两次查询合并）**

```
gh issue list --repo cli/cli --state all
     │
     ▼
listRun(opts)
     │
     ├── switch "all" → graphqlState = ""（空字符串触发双查询分支）
     │
     ├── api.ListIssues(client, repo, "OPEN", limit)   ← 第一次查询
     │         └── []Issue（open 列表）
     │
     ├── api.ListIssues(client, repo, "CLOSED", limit)  ← 第二次查询
     │         └── []Issue（closed 列表）
     │
     └── issues = append(open, closed...)               ← 合并结果
             │
             ▼
     for each issue: 输出到 stdout
```

**`gh issue view 42 --repo cli/cli`**

```
gh issue view 42 --repo cli/cli
     │
     ▼
NewCmdIssueView(f) → cobra.Command（Args: cobra.ExactArgs(1)）
     │
     │  args[0] = "42"
     │  opts.IssueArg = "42"
     ▼
viewRun(opts)
     │
     ├── ghrepo.FromFullName("cli/cli") → repo
     │
     ├── strconv.Atoi("42") → number = 42
     │       └── 若参数非数字 → return fmt.Errorf("invalid issue number: ...")
     │
     ├── opts.HttpClient() → httpClient
     │
     ├── api.NewClientFromHTTP(httpClient)
     │
     └── api.GetIssue(client, repo, 42)
             │
             ├── 构造 GraphQL query（GetIssue）
             ├── variables = {"owner":"cli", "name":"cli", "number":42}
             └── client.GraphQL("github.com", query, variables, &result)
                     │
                     ▼
             GitHub GraphQL API → {"data":{"repository":{"issue":{...}}}}
                     │
                     └── &result.Repository.Issue → *api.Issue
                             │
                             ▼
             printIssue(opts.IO, issue)
                     │
                     └── stdout:
                         #42 Feature request
                         state:  open
                         author: bob
                         (labels: 若有)
                         (body: 若非空)
                         url:    https://github.com/cli/cli/issues/42
```

**`gh issue create --repo cli/cli --title "Bug" --body "Details"`**

```
gh issue create --repo cli/cli --title "Bug" --body "Details"
     │
     ▼
NewCmdIssueCreate(f) → cobra.Command
     │  MarkFlagRequired("title") ← cobra 在 RunE 前自动验证 --title 已提供
     │
     │  opts.Repo  = "cli/cli"
     │  opts.Title = "Bug"
     │  opts.Body  = "Details"
     ▼
createRun(opts)
     │
     ├── ghrepo.FromFullName("cli/cli") → repo
     │
     ├── opts.HttpClient() → httpClient
     │
     ├── api.NewClientFromHTTP(httpClient)
     │
     ├── params = map[string]interface{}{"title":"Bug", "body":"Details"}
     │
     └── api.CreateIssue(client, repo, params)
             │
             ├── json.Marshal(params) → JSON body
             ├── path = "repos/cli/cli/issues"
             └── client.REST("github.com", "POST", path, body, &raw)
                     │
                     ▼
             POST https://api.github.com/repos/cli/cli/issues
             响应 201 Created: {"number":99,"title":"Bug","html_url":"..."}
                     │
                     └── issueRESTResponse{Number:99, Title:"Bug", URL:"..."}
                             │
                             └── &Issue{Number:99, Title:"Bug", URL:"..."}
                                     │
                                     ▼
             fmt.Fprintf(opts.IO.Out, "Created issue #%d: %s\n%s\n", ...)
                     │
                     └── stdout: "Created issue #99: Bug\nhttps://..."
```

---

### 1.4 关键设计对比：GraphQL vs REST

| 操作 | 协议 | 原因 |
|------|------|------|
| `ListIssues` | GraphQL | 需要精确指定返回字段（number/title/state/labels/assignees 等），GraphQL 按需取字段，避免过量传输 |
| `GetIssue` | GraphQL | 同上，单个 Issue 的详情字段较多，GraphQL 的字段选择优势明显 |
| `CreateIssue` | REST | GitHub GraphQL `createIssue` mutation 需要 `repositoryId`（Node ID），必须先额外查一次；REST `POST /repos/{owner}/{repo}/issues` 只需 owner/name，更直接 |

**REST 响应的 snake_case 问题**

GitHub GraphQL API 返回 camelCase 字段（如 `url`），而 GitHub REST API 返回 snake_case 字段（如 `html_url`）。通用的 `Issue` struct 没有 `json:"html_url"` tag，直接解码 REST 响应会导致 `URL` 字段为零值。

解决方案是引入专用的 `issueRESTResponse` struct：

```go
type issueRESTResponse struct {
    Number int    `json:"number"`
    Title  string `json:"title"`
    URL    string `json:"html_url"`   // ← REST 专用 tag
}
```

解码完成后，再将其字段复制到通用的 `*Issue`，保持 API 层内部的 struct 设计干净。

---

## Section 2：Implementation Walkthrough

### 2.1 创建 Phase 5 目录

**为什么从 Phase 4 复制？**

Phase 5 在 Phase 4 的全部代码上追加功能。`go.mod`、`api/`（除 queries_issue.go）、`internal/`、`pkg/cmdutil/` 等文件完全不变。直接复制 Phase 4 目录，然后新增文件，是保持每个 Phase 自包含的最简方式。

【现在手敲】

```bash
# 从项目根目录执行
cp -r cli/src/phase-04-repo-commands cli/src/phase-05-issue-commands

# 创建 issue 命令相关目录
mkdir -p cli/src/phase-05-issue-commands/pkg/cmd/issue/list
mkdir -p cli/src/phase-05-issue-commands/pkg/cmd/issue/view
mkdir -p cli/src/phase-05-issue-commands/pkg/cmd/issue/create
```

【验证】

```bash
ls cli/src/phase-05-issue-commands/pkg/cmd/issue/
```

期望输出：`list/`、`view/`、`create/` 三个子目录均存在。

**关键点解释**

目录结构 `pkg/cmd/issue/list`、`pkg/cmd/issue/view`、`pkg/cmd/issue/create` 与 Phase 4 的 `pkg/cmd/repo/view`、`pkg/cmd/repo/list` 保持相同层级。每个子命令一个独立的包，包名与目录名一致（`package list`、`package view`、`package create`），避免命名冲突。

---

### 2.2 定义 Issue、Label、User struct

**为什么 CreatedAt/UpdatedAt 用 `time.Time` 而不是 `string`？**

GitHub API 返回 ISO 8601 格式的时间字符串（如 `"2024-01-01T00:00:00Z"`）。使用 `time.Time` 后，`encoding/json` 会自动将其解析为 Go 的时间类型，后续可以直接进行时间运算、格式化（`time.Since`、`Format`）等操作，无需手动解析字符串。

**为什么 `Author`、`Labels`、`Assignees` 使用嵌套 struct 而非具名类型别名？**

GraphQL 的 `author { login }`、`labels(first: 10) { nodes { name } }`、`assignees(first: 5) { nodes { login } }` 在 JSON 中分别是 `{"login":"..."}` 和 `{"nodes":[...]}` 结构。使用匿名嵌套 struct（`struct{ Login string }` 和 `struct{ Nodes []Label }`）直接对应 JSON 结构，无需引入额外的具名类型，代码更紧凑。

【现在手敲】

```go
// 文件：api/queries_issue.go
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

// Label represents a GitHub label.
type Label struct {
	Name string
}

// User represents a GitHub user reference.
type User struct {
	Login string
}

// Issue represents a GitHub issue.
type Issue struct {
	Number    int
	Title     string
	State     string
	Body      string
	Author    struct{ Login string }
	Labels    struct{ Nodes []Label }
	Assignees struct{ Nodes []User }
	CreatedAt time.Time
	UpdatedAt time.Time
	URL       string
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go vet ./api/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`Issue.Author` 使用匿名 struct `struct{ Login string }` 而不是具名的 `User` 类型，是因为 `Author` 在 GraphQL schema 中返回的是 `Actor` 接口（包含 `login` 字段），与 `Assignees` 的 `User` 节点类型不同。虽然字段结构相同，但语义不同，保持独立有助于日后扩展（例如为 `Author` 添加 `AvatarURL` 字段）。

---

### 2.3 实现 ListIssues

**GraphQL `IssueState` 枚举和 `states` 参数**

GitHub GraphQL schema 将 Issue 状态定义为枚举 `IssueState`，只有两个合法值：`OPEN` 和 `CLOSED`（均为大写）。查询参数 `states` 是数组类型 `[IssueState!]`，可以同时传入两个值，也可只传一个。`ListIssues` 函数接收单个 `string` 形式的状态（`"OPEN"` 或 `"CLOSED"`），上层的 `listRun` 负责处理 "all" 逻辑（分两次调用）。

【现在手敲】

在 `api/queries_issue.go` 中追加：

```go
// ListIssues fetches issues for a repository.
func ListIssues(client *Client, repo ghrepo.Interface, state string, limit int) ([]Issue, error) {
	if state == "" {
		state = "OPEN"
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var result struct {
		Repository struct {
			Issues struct {
				Nodes []Issue
			}
		}
	}
	query := `
query ListIssues($owner: String!, $name: String!, $states: [IssueState!], $first: Int!) {
	repository(owner: $owner, name: $name) {
		issues(states: $states, first: $first, orderBy: {field: CREATED_AT, direction: DESC}) {
			nodes {
				number
				title
				state
				body
				author { login }
				labels(first: 10) { nodes { name } }
				assignees(first: 5) { nodes { login } }
				createdAt
				updatedAt
				url
			}
		}
	}
}`
	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"name":   repo.RepoName(),
		"states": []string{state},
		"first":  limit,
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}
	return result.Repository.Issues.Nodes, nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./api/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

- `states` 变量传入 `[]string{state}`（数组），对应 GraphQL 的 `[IssueState!]` 类型。即使只查一种状态，也要包裹成数组。
- `orderBy: {field: CREATED_AT, direction: DESC}` 确保最新创建的 Issue 排在前面，与 `gh` 官方 CLI 行为一致。
- `limit <= 0 || limit > 100` 的边界检查：GraphQL API 通常限制 `first` 最大为 100，超出会报错；小于等于 0 无意义，统一回退到默认值 30。

---

### 2.4 实现 GetIssue

**按 Issue number 查询单个 Issue**

GraphQL `repository.issue(number: $number)` 直接接受 `Int!` 类型的 Issue 编号，返回单个 Issue 对象（非数组）。与 `ListIssues` 的三层嵌套不同，这里只有两层：`result.Repository.Issue`。

【现在手敲】

在 `api/queries_issue.go` 中追加：

```go
// GetIssue fetches a single issue by number.
func GetIssue(client *Client, repo ghrepo.Interface, number int) (*Issue, error) {
	var result struct {
		Repository struct {
			Issue Issue
		}
	}
	query := `
query GetIssue($owner: String!, $name: String!, $number: Int!) {
	repository(owner: $owner, name: $name) {
		issue(number: $number) {
			number
			title
			state
			body
			author { login }
			labels(first: 10) { nodes { name } }
			assignees(first: 5) { nodes { login } }
			createdAt
			updatedAt
			url
		}
	}
}`
	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"name":   repo.RepoName(),
		"number": number,
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to get issue: %w", err)
	}
	return &result.Repository.Issue, nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./api/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

返回 `&result.Repository.Issue` 而非 `result.Repository.Issue` 的原因：调用方需要判断 Issue 是否不存在（例如 number 不合法时 API 返回 null），返回指针使得未来可以通过 nil 检查处理这种情况，而不需要为"零值 Issue"设计特殊标记。

---

### 2.5 实现 CreateIssue（REST POST）

**为什么不用 GraphQL mutation？**

GitHub GraphQL 提供 `createIssue(input: CreateIssueInput!)` mutation，但 `CreateIssueInput` 要求 `repositoryId: ID!`——这是仓库的 Node ID（如 `R_kgDOA...`），而不是 `owner/name`。要获取这个 ID，必须先发一次 GraphQL 查询：`repository(owner: $owner, name: $name) { id }`。这意味着"创建一个 Issue"变成了两次网络请求。

相比之下，REST `POST /repos/{owner}/{repo}/issues` 只需 owner 和 name，直接可用，一次请求完成创建。对于写操作，简单性优先。

**issueRESTResponse 的设计**

REST API 返回 snake_case 字段（如 `html_url`），而通用的 `Issue` struct 的字段使用 Go 惯例（`URL string`，JSON 解码时匹配 `"url"`）。如果直接用 `Issue` 解码 REST 响应，`URL` 字段会因为找不到 `"url"` key 而保持零值（空字符串）。

引入 `issueRESTResponse` 作为 REST 解码的中间 struct，加上正确的 json tag `json:"html_url"`，解码后再转换为 `*Issue`。这样通用的 `Issue` struct 不需要为 REST 的 snake_case 添加任何 tag，保持干净。

【现在手敲】

在 `api/queries_issue.go` 中追加：

```go
// issueRESTResponse captures the fields returned by the REST create-issue endpoint.
type issueRESTResponse struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"html_url"`
}

// CreateIssue creates a new issue via REST POST /repos/{owner}/{repo}/issues.
func CreateIssue(client *Client, repo ghrepo.Interface, params map[string]interface{}) (*Issue, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("repos/%s/%s/issues", repo.RepoOwner(), repo.RepoName())
	var raw issueRESTResponse
	if err := client.REST(repo.RepoHost(), "POST", path, bytes.NewReader(b), &raw); err != nil {
		return nil, fmt.Errorf("failed to create issue: %w", err)
	}
	return &Issue{Number: raw.Number, Title: raw.Title, URL: raw.URL}, nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./api/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

- `params` 使用 `map[string]interface{}` 而非固定 struct，是为了保持灵活性：调用方（`createRun`）按需传入 `title`、`body`，未来还可以扩展 `labels`、`assignees` 等，无需修改 `CreateIssue` 函数签名。
- `bytes.NewReader(b)` 将 JSON 字节转为 `io.Reader`，与 `client.REST` 的接口约定匹配。
- `issueRESTResponse` 是**未导出类型**（小写 i），是 `api` 包内部的实现细节，调用方无需感知。

---

### 2.6 编写 queries_issue 测试

**测试策略：启动本地 HTTP 服务器模拟 GitHub API**

与 Phase 4 的 API 测试相同，使用 `httptest.NewServer` 启动本地 HTTP 服务器，通过 `rewriteTransport` 将请求重定向到本地服务器，避免真实网络请求。三个函数各一个测试用例，覆盖核心返回值验证。

【现在手敲】

```go
// 文件：api/queries_issue_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

func TestListIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"issues": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 1, "title": "Bug report", "state": "OPEN",
								"body": "Something broke", "author": map[string]string{"login": "alice"},
								"labels":    map[string]interface{}{"nodes": []interface{}{}},
								"assignees": map[string]interface{}{"nodes": []interface{}{}},
								"createdAt": "2024-01-01T00:00:00Z",
								"updatedAt": "2024-01-02T00:00:00Z",
								"url":       "https://github.com/cli/cli/issues/1",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	issues, err := ListIssues(client, repo, "OPEN", 10)
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("len = %d, want 1", len(issues))
	}
	if issues[0].Title != "Bug report" {
		t.Errorf("Title = %q, want 'Bug report'", issues[0].Title)
	}
}

func TestGetIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"issue": map[string]interface{}{
						"number": 42, "title": "Feature request", "state": "OPEN",
						"body": "Please add X", "author": map[string]string{"login": "bob"},
						"labels":    map[string]interface{}{"nodes": []interface{}{}},
						"assignees": map[string]interface{}{"nodes": []interface{}{}},
						"createdAt": "2024-01-01T00:00:00Z",
						"updatedAt": "2024-01-01T00:00:00Z",
						"url":       "https://github.com/cli/cli/issues/42",
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	issue, err := GetIssue(client, repo, 42)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Number != 42 {
		t.Errorf("Number = %d, want 42", issue.Number)
	}
	if issue.Title != "Feature request" {
		t.Errorf("Title = %q", issue.Title)
	}
}

func TestCreateIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   99,
			"title":    "New issue",
			"html_url": "https://github.com/cli/cli/issues/99",
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	issue, err := CreateIssue(client, repo, map[string]interface{}{
		"title": "New issue",
		"body":  "Details",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if issue.Number != 99 {
		t.Errorf("Number = %d, want 99", issue.Number)
	}
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go test ./api/... -run TestListIssues -v
go test ./api/... -run TestGetIssue -v
go test ./api/... -run TestCreateIssue -v
```

期望输出：三个测试各输出 `PASS`。

**关键点解释**

- `TestCreateIssue` 的模拟服务器验证了请求方法是 `POST`（`if r.Method != "POST"`），同时返回 `http.StatusCreated`（201）。注意响应中的 key 是 `html_url`，不是 `url`——这正是测试 `issueRESTResponse` 的 json tag 是否正确的关键。
- `rewriteTransport` 在 `api` 包的测试文件中已存在（来自 Phase 3 或 4 的 API 测试），无需重新定义。

---

### 2.7 实现 NewCmdIssueList

**`--repo`、`--state`、`--limit` 三个 flags 的设计**

与 `gh repo list` 的 `--limit` 相同，这里也使用 `IntVarP` 绑定到 `opts.Limit`。`--state` 使用用户友好的小写格式（`open`/`closed`/`all`），而不是 GraphQL 的枚举值，转换在 `listRun` 内部进行。

`--repo` 在当前实现中是必填的（`listRun` 里检查空值返回错误），但没有调用 `MarkFlagRequired`——这是有意为之：未来可以从 git remote 自动检测当前仓库，届时 `--repo` 就不再必须显式提供，提前加 `MarkFlagRequired` 会增加迁移成本。

【现在手敲】

```go
// 文件：pkg/cmd/issue/list/list.go
package list

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ListOptions holds all inputs for the issue list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string // --repo flag: owner/name
	State      string // --state flag: open|closed|all
	Limit      int    // --limit flag
}

// NewCmdIssueList creates the `gh issue list` command.
func NewCmdIssueList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		State:      "open",
		Limit:      30,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues in a repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: open, closed, all")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of issues to fetch")
	return cmd
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./pkg/cmd/issue/list/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`opts` 在 `NewCmdIssueList` 闭包中创建，`State` 和 `Limit` 设置了默认值（`"open"` 和 `30`）。这些默认值同时也作为 `StringVarP`/`IntVarP` 的第三个参数传入，确保 flag 的默认值说明（`--help` 输出）与实际行为一致。

---

### 2.8 实现 listRun（state 映射和 all 状态处理）

**`--state all` 为什么需要两次查询**

GitHub GraphQL `IssueState` 枚举只定义了 `OPEN` 和 `CLOSED`，没有 `ALL`。虽然可以传入 `states: [OPEN, CLOSED]`，但实际使用中发现这等价于"两次查询然后合并"，而且分开查询更清晰：每次查询可以独立指定 `--limit`，避免合并后条数翻倍的歧义。

当前实现选择了"分两次调用 `ListIssues`，再 `append` 合并"的方式，逻辑直观，代价是两次网络请求。

【现在手敲】

在 `pkg/cmd/issue/list/list.go` 中追加：

```go
func listRun(opts *ListOptions) error {
	if opts.Repo == "" {
		return fmt.Errorf("repository required: use --repo owner/name")
	}
	repo, err := ghrepo.FromFullName(opts.Repo)
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	// Map user-facing state to GraphQL IssueState enum.
	var graphqlState string
	switch strings.ToLower(opts.State) {
	case "open":
		graphqlState = "OPEN"
	case "closed":
		graphqlState = "CLOSED"
	case "all":
		graphqlState = "" // handled below
	default:
		return fmt.Errorf("invalid state %q: use open, closed, or all", opts.State)
	}

	var issues []api.Issue
	if graphqlState == "" {
		// Fetch both open and closed.
		open, err := api.ListIssues(client, repo, "OPEN", opts.Limit)
		if err != nil {
			return err
		}
		closed, err := api.ListIssues(client, repo, "CLOSED", opts.Limit)
		if err != nil {
			return err
		}
		issues = append(open, closed...)
	} else {
		issues, err = api.ListIssues(client, repo, graphqlState, opts.Limit)
		if err != nil {
			return err
		}
	}

	if len(issues) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "No issues found.\n")
		return nil
	}
	for _, issue := range issues {
		fmt.Fprintf(opts.IO.Out, "#%-5d  %s\n", issue.Number, issue.Title)
	}
	return nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./pkg/cmd/issue/list/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

- `strings.ToLower(opts.State)` 使 `--state OPEN`、`--state Open`、`--state open` 均合法，提升用户体验。
- "No issues found." 输出到 `opts.IO.ErrOut`（stderr）而不是 `Out`（stdout）。这是 `gh` CLI 的惯例：提示性信息走 stderr，实际数据走 stdout，便于管道（`gh issue list | grep bug`）中过滤数据而不混入提示。
- `#%-5d` 格式：`#` 是字面量井号，`%-5d` 是左对齐、宽度为 5 的整数，使 Issue 编号列对齐（`#1    `、`#100  `）。

---

### 2.9 编写 list 测试

**测试覆盖三个场景**：正常流程（有效 state + 返回数据）、无效 state、缺少 --repo。

【现在手敲】

```go
// 文件：pkg/cmd/issue/list/list_test.go
package list

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestListRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"issues": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 3, "title": "Test issue", "state": "OPEN",
								"body": "", "author": map[string]string{"login": "user"},
								"labels":    map[string]interface{}{"nodes": []interface{}{}},
								"assignees": map[string]interface{}{"nodes": []interface{}{}},
								"createdAt": "2024-01-01T00:00:00Z",
								"updatedAt": "2024-01-01T00:00:00Z",
								"url":       "https://github.com/cli/cli/issues/3",
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
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Repo:  "cli/cli",
		State: "open",
		Limit: 10,
	}

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "Test issue") {
		t.Errorf("output = %q, want to contain 'Test issue'", out.String())
	}
}

func TestListRun_InvalidState(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ListOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "cli/cli",
		State:      "invalid",
		Limit:      10,
	}
	if err := listRun(opts); err == nil {
		t.Fatal("expected error for invalid state")
	}
}

func TestListRun_NoRepo(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ListOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "",
		State:      "open",
		Limit:      10,
	}
	if err := listRun(opts); err == nil {
		t.Fatal("expected error for missing repo")
	}
}

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
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go test ./pkg/cmd/issue/list/... -v
```

期望输出：三个测试（`TestListRun`、`TestListRun_InvalidState`、`TestListRun_NoRepo`）均输出 `PASS`。

**关键点解释**

`rewriteTransport` 在每个命令子包的测试文件中各自定义一份（`list_test.go`、`view_test.go`、`create_test.go`），而不是抽取到共享的测试工具包。这是刻意的权衡：测试文件的自包含性（无需导入测试辅助包）比消除重复代码更重要；而且每个 `rewriteTransport` 的逻辑完全相同，复制成本极低。

---

### 2.10 实现 NewCmdIssueView（ExactArgs(1) 和 number 解析）

**为什么用 `ExactArgs(1)` 而不是在 `viewRun` 里检查 `len(args)`？**

`cobra.ExactArgs(1)` 在 cobra 框架层面声明"此命令恰好需要 1 个位置参数"。如果用户未提供或提供多个参数，cobra 会在调用 `RunE` 之前就打印使用说明并返回错误，错误信息统一（`"accepts 1 arg(s), received 0"`），用户体验更好。在 `viewRun` 里额外检查 `len(args)` 则是冗余的，而且错误时机不对（`RunE` 收到的 `args` 已经通过了 cobra 验证）。

**为什么 `IssueArg` 存储为 `string` 而不是 `int`？**

cobra 的 `args` 参数始终是 `[]string`，在 cobra 层面无法声明"某个位置参数是整数"。将解析（`strconv.Atoi`）推迟到 `viewRun` 内部，可以在 `opts` 层保持一致性（所有来自命令行的原始输入都是字符串），同时提供更清晰的错误信息（`"invalid issue number: \"abc\""`）。

【现在手敲】

```go
// 文件：pkg/cmd/issue/view/view.go
package view

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ViewOptions holds all inputs for the issue view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	IssueArg   string // number as string from positional arg
}

// NewCmdIssueView creates the `gh issue view` command.
func NewCmdIssueView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "View details of an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.IssueArg = args[0]
			return viewRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	return cmd
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./pkg/cmd/issue/view/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

`opts.IssueArg = args[0]` 在 `RunE` 闭包内赋值，而不是在 `NewCmdIssueView` 的外层赋值。这是因为 `args` 只在命令执行时（`RunE` 调用时）才可用，无法在命令构建时提前绑定。位置参数的处理方式与 flag 不同：flag 通过 `StringVarP` 直接绑定到 `opts` 字段，而位置参数只能在 `RunE` 闭包中手动赋值。

---

### 2.11 实现 viewRun 和 printIssue

**`printIssue` 的格式化设计**

`printIssue` 按行输出字段：先是标题行（`#42 Feature request`），然后是 `key:\tvalue` 格式的元数据（使用 tab 分隔，终端会将 tab 对齐），最后是正文（Body）和 URL。Labels 和 Body 是可选的——只有非空时才输出，避免空行干扰阅读。

【现在手敲】

在 `pkg/cmd/issue/view/view.go` 中追加：

```go
func viewRun(opts *ViewOptions) error {
	if opts.Repo == "" {
		return fmt.Errorf("repository required: use --repo owner/name")
	}
	repo, err := ghrepo.FromFullName(opts.Repo)
	if err != nil {
		return err
	}

	number, err := strconv.Atoi(opts.IssueArg)
	if err != nil {
		return fmt.Errorf("invalid issue number: %q", opts.IssueArg)
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	issue, err := api.GetIssue(client, repo, number)
	if err != nil {
		return err
	}

	printIssue(opts.IO, issue)
	return nil
}

func printIssue(io *iostreams.IOStreams, issue *api.Issue) {
	w := io.Out
	fmt.Fprintf(w, "#%d %s\n", issue.Number, issue.Title)
	fmt.Fprintf(w, "state:\t%s\n", strings.ToLower(issue.State))
	fmt.Fprintf(w, "author:\t%s\n", issue.Author.Login)
	if len(issue.Labels.Nodes) > 0 {
		labels := make([]string, len(issue.Labels.Nodes))
		for i, l := range issue.Labels.Nodes {
			labels[i] = l.Name
		}
		fmt.Fprintf(w, "labels:\t%s\n", strings.Join(labels, ", "))
	}
	if issue.Body != "" {
		fmt.Fprintf(w, "\n%s\n", issue.Body)
	}
	fmt.Fprintf(w, "\nurl:\t%s\n", issue.URL)
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./pkg/cmd/issue/view/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

- `strings.ToLower(issue.State)` 将 GraphQL 返回的 `"OPEN"`、`"CLOSED"` 转为用户友好的小写格式，与 `gh` 官方 CLI 输出风格一致。
- `strings.Join(labels, ", ")` 将多个标签合并为逗号分隔的一行（如 `"bug, enhancement"`），而不是每个标签单独一行，节省输出空间。
- `url` 字段前加了 `\n`（空行），与正文 Body 之间有视觉分隔，提升可读性。

---

### 2.12 编写 view 测试

【现在手敲】

```go
// 文件：pkg/cmd/issue/view/view_test.go
package view

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestViewRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"issue": map[string]interface{}{
						"number": 7, "title": "Example issue", "state": "OPEN",
						"body": "Issue body text", "author": map[string]string{"login": "dave"},
						"labels":    map[string]interface{}{"nodes": []interface{}{}},
						"assignees": map[string]interface{}{"nodes": []interface{}{}},
						"createdAt": "2024-01-01T00:00:00Z",
						"updatedAt": "2024-01-01T00:00:00Z",
						"url":       "https://github.com/cli/cli/issues/7",
					},
				},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &ViewOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Repo:     "cli/cli",
		IssueArg: "7",
	}

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Example issue") {
		t.Errorf("output missing title: %q", output)
	}
	if !strings.Contains(output, "dave") {
		t.Errorf("output missing author: %q", output)
	}
}

func TestViewRun_InvalidNumber(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ViewOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "cli/cli",
		IssueArg:   "notanumber",
	}
	if err := viewRun(opts); err == nil {
		t.Fatal("expected error for invalid number")
	}
}

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
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go test ./pkg/cmd/issue/view/... -v
```

期望输出：`TestViewRun` 和 `TestViewRun_InvalidNumber` 均输出 `PASS`。

**关键点解释**

`TestViewRun_InvalidNumber` 直接将 `IssueArg` 设为 `"notanumber"`，绕过 cobra 的 `ExactArgs(1)` 检查，直接调用 `viewRun`。这样可以单独测试 `strconv.Atoi` 的错误路径，而不需要构建完整的 cobra 命令执行链。

---

### 2.13 实现 NewCmdIssueCreate（MarkFlagRequired）

**`MarkFlagRequired("title")` 的工作原理**

调用 `cmd.MarkFlagRequired("title")` 后，cobra 会在调用 `RunE` 之前检查该 flag 是否已设置。如果用户未提供 `--title`，cobra 直接返回错误 `"required flag(s) \"title\" not set"`，不会进入 `createRun`。因此 `createRun` 无需再次检查 `opts.Title == ""`——避免了防御性冗余代码。

注意 `_ = cmd.MarkFlagRequired("title")`：`MarkFlagRequired` 返回 error，但当 flag 名拼写正确时永远不会出错（flag 已在上面注册），所以用 `_` 忽略返回值是安全的，也符合 Go 惯例。

【现在手敲】

```go
// 文件：pkg/cmd/issue/create/create.go
package create

import (
	"fmt"
	"net/http"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CreateOptions holds all inputs for the issue create command.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	Title      string
	Body       string
}

// NewCmdIssueCreate creates the `gh issue create` command.
func NewCmdIssueCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Supply a title (required)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./pkg/cmd/issue/create/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

短标志（shorthand）选择：`-R` 用于 `--repo`，`-t` 用于 `--title`，`-b` 用于 `--body`，`-L` 用于 `--limit`，`-s` 用于 `--state`。这些短标志与 `gh` 官方 CLI 保持一致，降低学习者的记忆成本。

---

### 2.14 实现 createRun（params map 和输出创建结果）

**params map 的设计**

将 `title` 和 `body` 打包成 `map[string]interface{}` 传给 `CreateIssue`，而不是直接传 `opts.Title` 和 `opts.Body` 作为独立参数。好处是扩展性：未来添加 `--assignee`、`--label` 等 flag 时，只需在 `createRun` 中往 `params` map 里加 key，`CreateIssue` 的签名不变。

【现在手敲】

在 `pkg/cmd/issue/create/create.go` 中追加：

```go
func createRun(opts *CreateOptions) error {
	if opts.Repo == "" {
		return fmt.Errorf("repository required: use --repo owner/name")
	}
	repo, err := ghrepo.FromFullName(opts.Repo)
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	params := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Body,
	}
	issue, err := api.CreateIssue(client, repo, params)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "Created issue #%d: %s\n%s\n", issue.Number, issue.Title, issue.URL)
	return nil
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./pkg/cmd/issue/create/...
```

期望输出：无输出（exit code 0）。

**关键点解释**

输出格式 `"Created issue #%d: %s\n%s\n"` 将 Issue 编号、标题和 URL 合并为两行输出。URL 来自 `issueRESTResponse.URL`（对应 JSON 的 `html_url`），是可以直接在浏览器打开的网页链接，方便用户立即查看新创建的 Issue。

---

### 2.15 编写 create 测试

【现在手敲】

```go
// 文件：pkg/cmd/issue/create/create_test.go
package create

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestCreateRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   55,
			"title":    "My issue",
			"html_url": "https://github.com/cli/cli/issues/55",
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &CreateOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Repo:  "cli/cli",
		Title: "My issue",
		Body:  "body text",
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !strings.Contains(out.String(), "#55") {
		t.Errorf("output = %q, want to contain #55", out.String())
	}
}

func TestCreateRun_NoRepo(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &CreateOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return http.DefaultClient, nil },
		Repo:       "",
		Title:      "My issue",
	}
	if err := createRun(opts); err == nil {
		t.Fatal("expected error for missing repo")
	}
}

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
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go test ./pkg/cmd/issue/create/... -v
```

期望输出：`TestCreateRun` 和 `TestCreateRun_NoRepo` 均输出 `PASS`。

**关键点解释**

`TestCreateRun` 验证了两点：（1）HTTP 方法是 `POST`（写操作必须用正确的方法）；（2）输出包含 `"#55"`（Issue 编号来自 REST 响应的 `number` 字段）。响应中使用 `html_url` 而非 `url`，直接测试了 `issueRESTResponse` 的 json tag 是否正确工作。

---

### 2.16 实现 NewCmdIssue 并注册到 root

**命令组的组织方式**

`NewCmdIssue` 创建一个没有 `RunE` 的父命令（仅有 `Use`、`Short`、`Long`），然后将三个子命令通过 `AddCommand` 挂载。父命令本身不执行任何操作，用户必须指定子命令（`gh issue list`、`gh issue view`、`gh issue create`），直接运行 `gh issue` 会显示帮助信息。

【现在手敲】

```go
// 文件：pkg/cmd/issue/issue.go
package issue

import (
	"github.com/learngh/gh-impl/pkg/cmd/issue/create"
	"github.com/learngh/gh-impl/pkg/cmd/issue/list"
	"github.com/learngh/gh-impl/pkg/cmd/issue/view"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdIssue creates the `gh issue` command group.
func NewCmdIssue(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <subcommand>",
		Short: "Manage issues",
		Long:  "Work with GitHub issues.",
	}
	cmd.AddCommand(list.NewCmdIssueList(f))
	cmd.AddCommand(view.NewCmdIssueView(f))
	cmd.AddCommand(create.NewCmdIssueCreate(f))
	return cmd
}
```

然后修改 `pkg/cmd/root/root.go`，在 repo 命令后注册 issue 命令组：

```go
// 在 root.go 的 import 块中添加：
issueCmd "github.com/learngh/gh-impl/pkg/cmd/issue"

// 在 NewCmdRoot 函数中，repoCmd 注册之后添加：
// Issue command group.
cmd.AddCommand(issueCmd.NewCmdIssue(f))
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go build ./...
```

期望输出：无输出（exit code 0）——整个项目编译通过。

**关键点解释**

import alias `issueCmd "github.com/learngh/gh-impl/pkg/cmd/issue"` 是为了避免与变量名冲突——包名 `issue` 可能与某个局部变量重名。使用别名 `issueCmd` 使得 `issueCmd.NewCmdIssue(f)` 的调用意图更明确，与 `repoCmd.NewCmdRepo(f)` 的命名风格保持一致。

---

### 2.17 运行所有测试

**最终验证：确保所有 Phase 5 测试通过**

【现在手敲】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-05-issue-commands
go test ./... -v
```

【验证】

期望输出（关键部分）：

```
--- PASS: TestListIssues (0.00s)
--- PASS: TestGetIssue (0.00s)
--- PASS: TestCreateIssue (0.00s)
--- PASS: TestListRun (0.00s)
--- PASS: TestListRun_InvalidState (0.00s)
--- PASS: TestListRun_NoRepo (0.00s)
--- PASS: TestViewRun (0.00s)
--- PASS: TestViewRun_InvalidNumber (0.00s)
--- PASS: TestCreateRun (0.00s)
--- PASS: TestCreateRun_NoRepo (0.00s)
ok  	github.com/learngh/gh-impl/api
ok  	github.com/learngh/gh-impl/pkg/cmd/issue/list
ok  	github.com/learngh/gh-impl/pkg/cmd/issue/view
ok  	github.com/learngh/gh-impl/pkg/cmd/issue/create
```

**关键点解释**

`go test ./...` 递归测试所有包，包括从 Phase 4 继承的 `internal/ghrepo`、`api/queries_repo` 等测试。确保新增代码没有破坏已有功能（回归测试）。如果有测试失败，根据包名和测试函数名定位问题，重新检查对应小节的代码。

---

## 附录：五个 Phase 的知识演进

| Phase | 新增能力 | 关键技术点 |
|-------|---------|-----------|
| Phase 1 | cobra 命令框架 | `cobra.Command`、flag 绑定 |
| Phase 2 | 配置与认证 | token 读写、`cmdutil.Factory` |
| Phase 3 | API 客户端 | `http.RoundTripper`、GraphQL/REST |
| Phase 4 | 仓库命令（只读） | `ghrepo.Interface`、GraphQL 查询 |
| Phase 5 | Issue 命令（读+写） | REST 写操作、`MarkFlagRequired`、双查询合并 |

Phase 5 完成后，CLI 具备了完整的"读-写"能力基础，后续可以在此模式上扩展任意资源（PR、release、gist 等），模式是相同的：`api/queries_*.go` 定义数据结构和 API 调用，`pkg/cmd/<resource>/` 定义命令和业务逻辑。
