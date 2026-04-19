# Phase 6 学习指南：Pull Request 命令

---

## Section 1：全局视图

### 1.1 这个 Phase 做了什么

Phase 6 是整个学习项目的最后一个 Phase。它在 Phase 5（Issue 命令）的基础上，新增了三条 Pull Request 相关命令：`gh pr list`、`gh pr view`、`gh pr create`。

与 Phase 5 的纯"API + 命令"新增不同，Phase 6 还引入了一个全新的包 `git/`——封装了对本地 git 仓库的操作。这是因为 `gh pr create` 需要从本地 git 仓库中读取两类信息：

1. **当前分支**（`git rev-parse --abbrev-ref HEAD`）：作为 PR 的 head branch。
2. **远端 remote URL**（`git remote -v`）：从中推断出目标 GitHub 仓库（owner/name），避免用户每次都要手动传 `--repo`。

这一设计与 Phase 5 的 `issue create` 形成鲜明对比：`issue create` 只需要用户明确提供 `--repo`，不需要感知本地 git 状态；而 `pr create` 在不传 `--repo` 时，能够自动感知当前 git 仓库的 origin 远端，推断出要操作的 GitHub 仓库。

Phase 6 共涉及以下改动：

1. **新增 `git/client.go`**：`Client` struct 封装 `exec.Command` 调用 git 二进制；实现 `CurrentBranch` 和 `Remotes`；`parseRemotes` 解析 `git remote -v` 输出并去重。
2. **新增 `api/queries_pr.go`**：定义 `PullRequest` struct 及三个 API 函数 `ListPullRequests`、`GetPullRequest`（GraphQL）和 `CreatePullRequest`（REST）。
3. **新增 `pkg/cmd/pr/list`、`pkg/cmd/pr/view`、`pkg/cmd/pr/create`**：三个子命令；`create` 是最复杂的，需要读取 git 状态并做两步 API 调用。
4. **修改 `pkg/cmdutil/factory.go`**：在 `Factory` struct 中新增 `GitClient *git.Client` 字段。
5. **修改 `internal/factory/factory.go`**：在 `factory.New()` 中初始化 `f.GitClient = &git.Client{}`。
6. **修改 `pkg/cmd/root/root.go`**：注册 `prCmd.NewCmdPR(f)`。

---

### 1.2 模块依赖图

```
                    ┌─────────────────────────────────────┐
                    │         Phase 6 新增 / 修改           │
                    └─────────────────────────────────────┘

   pkg/cmd/root/root.go          ← ★ 修改：注册 pr 命令组
         │
         └── prCmd.NewCmdPR(f)
                   │
                   ├── list.NewCmdPRList(f)    ← ★ 新增：gh pr list
                   │         │
                   │         ├── internal/ghrepo   (FromFullName)
                   │         └── api/queries_pr    (ListPullRequests)
                   │
                   ├── view.NewCmdPRView(f)    ← ★ 新增：gh pr view
                   │         │
                   │         ├── internal/ghrepo   (FromFullName)
                   │         └── api/queries_pr    (GetPullRequest)
                   │
                   └── create.NewCmdPRCreate(f) ← ★ 新增：gh pr create
                             │
                             ├── gitClienter 接口（抽象 *git.Client）
                             │         └── git/client.go ← ★ 新增
                             │                 ├── CurrentBranch（当前分支）
                             │                 └── Remotes（远端列表）
                             │
                             ├── internal/ghrepo   (FromURL / FromFullName)
                             └── api/queries_pr    (GetRepository + CreatePullRequest)

   git/client.go               ← ★ 新增：封装 exec.Command 调用 git 二进制
   api/queries_pr.go           ← ★ 新增：PullRequest struct，三个查询函数

   pkg/cmdutil/factory.go      ← ★ 修改：添加 GitClient *git.Client 字段
   internal/factory/factory.go ← ★ 修改：初始化 f.GitClient = &git.Client{}

   internal/ghrepo/            ← Phase 4 已有，Phase 6 直接复用
   api/client.go               ← Phase 3 已有：GraphQL(), REST()
   api/queries_repo.go         ← Phase 4 已有：GetRepository()（被 pr create 复用）
```

标记 ★ 的部分是 Phase 6 新增或修改的内容。`git/` 包是全项目中唯一一个专门封装本地 shell 命令的包，体现了"将外部进程调用集中在专用层"的设计原则。

---

### 1.3 控制流图

**`gh pr list --repo cli/cli --state open`**

```
gh pr list --repo cli/cli --state open
     │
     ▼
NewCmdPRList(f) → cobra.Command
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
     └── api.ListPullRequests(client, repo, "OPEN", 30)
             │
             ├── 构造 GraphQL query（ListPullRequests）
             ├── variables = {"owner":"cli", "name":"cli", "states":["OPEN"], "first":30}
             └── client.GraphQL("github.com", query, variables, &result)
                     │
                     ▼
             GitHub GraphQL API → {"data":{"repository":{"pullRequests":{"nodes":[...]}}}}
                     │
                     └── []api.PullRequest
                             │
                             ▼
             for each pr: fmt.Fprintf(opts.IO.Out, "#%-5d  %s%s\t(%s -> %s)\n", ...)
                     │
                     └── stdout: "#5      Cool PR\t(feature -> main)"
```

**`gh pr list --repo cli/cli --state all`（三次查询合并）**

```
gh pr list --repo cli/cli --state all
     │
     ▼
listRun(opts)
     │
     ├── switch "all" → graphqlState = ""（空字符串触发三查询分支）
     │
     ├── api.ListPullRequests(client, repo, "OPEN", limit)    ← 第一次查询
     │         └── []PullRequest（open 列表）
     │
     ├── api.ListPullRequests(client, repo, "CLOSED", limit)  ← 第二次查询
     │         └── []PullRequest（closed 列表）
     │
     ├── api.ListPullRequests(client, repo, "MERGED", limit)  ← 第三次查询
     │         └── []PullRequest（merged 列表）
     │
     └── prs = append(open, closed..., merged...)             ← 合并结果
             │
             ▼
     for each pr: 输出到 stdout
```

注意：Phase 5 的 `issue list --state all` 只需要两次查询（OPEN + CLOSED），而 PR 有三种终态（OPEN、CLOSED、MERGED），因此 `--state all` 需要三次查询。

**`gh pr view 42 --repo cli/cli`**

```
gh pr view 42 --repo cli/cli
     │
     ▼
NewCmdPRView(f) → cobra.Command（Args: cobra.ExactArgs(1)）
     │
     │  args[0] = "42"
     │  opts.PRArg = "42"
     ▼
viewRun(opts)
     │
     ├── ghrepo.FromFullName("cli/cli") → repo
     │
     ├── strconv.Atoi("42") → number = 42
     │       └── 若参数非数字 → return fmt.Errorf("invalid pull request number: ...")
     │
     ├── opts.HttpClient() → httpClient
     │
     ├── api.NewClientFromHTTP(httpClient)
     │
     └── api.GetPullRequest(client, repo, 42)
             │
             ├── 构造 GraphQL query（GetPullRequest）
             ├── variables = {"owner":"cli", "name":"cli", "number":42}
             └── client.GraphQL("github.com", query, variables, &result)
                     │
                     ▼
             GitHub GraphQL API → {"data":{"repository":{"pullRequest":{...}}}}
                     │
                     └── &result.Repository.PullRequest → *api.PullRequest
                             │
                             ▼
             printPR(opts.IO, pr)
                     │
                     └── stdout:
                         #42 Fix bug
                         state:  merged
                         author: bob
                         branch: fix-branch -> main
                         (body: 若非空)
                         url:    https://github.com/cli/cli/pull/42
```

**`gh pr create --title "Feature" [--repo cli/cli | 自动推断]`**

```
gh pr create --title "Feature"
     │
     ▼
NewCmdPRCreate(f) → cobra.Command
     │  MarkFlagRequired("title") ← cobra 在 RunE 前自动验证 --title 已提供
     │
     │  opts.Title     = "Feature"
     │  opts.GitClient = f.GitClient（*git.Client）
     ▼
createRun(opts)
     │
     ├── opts.GitClient.CurrentBranch(ctx)
     │       └── exec "git rev-parse --abbrev-ref HEAD" → "feature"
     │
     ├── [分支判断：opts.Repo 是否为空]
     │
     │   ──────────── 路径 A：--repo 已提供 ────────────
     │   ├── ghrepo.FromFullName(opts.Repo) → repo
     │
     │   ──────────── 路径 B：--repo 未提供，从 git remote 推断 ────────────
     │   ├── opts.GitClient.Remotes(ctx)
     │   │       └── exec "git remote -v"
     │   │           → parseRemotes(...) → [{Name:"origin", FetchURL:&url.URL{...}}]
     │   │
     │   ├── 找名为 "origin" 的 remote，取其 FetchURL
     │   │       └── "https://github.com/cli/cli.git"
     │   │
     │   └── ghrepo.FromURL(fetchURL) → repo{owner:"cli", name:"cli", host:"github.com"}
     │
     ├── opts.HttpClient() → httpClient
     ├── api.NewClientFromHTTP(httpClient) → client
     │
     ├── api.GetRepository(client, repo)      ← 第一次 API 调用（GraphQL）
     │       └── apiRepo.DefaultBranchRef.Name = "main"（默认 base 分支）
     │
     ├── base = opts.Base || apiRepo.DefaultBranchRef.Name || "main"
     │
     ├── params = {"title":"Feature", "body":"", "head":"feature", "base":"main", "draft":false}
     │
     └── api.CreatePullRequest(client, apiRepo, params)  ← 第二次 API 调用（REST POST）
             │
             ├── json.Marshal(params) → JSON body
             ├── path = "repos/cli/cli/pulls"
             └── client.REST("github.com", "POST", path, body, &raw)
                     │
                     ▼
             POST https://api.github.com/repos/cli/cli/pulls
             响应 201 Created:
             {"number":33,"title":"Feature","html_url":"...","head":{"ref":"feature"},"base":{"ref":"main"}}
                     │
                     └── raw.Head.Ref → "feature"，raw.Base.Ref → "main"（嵌套解码）
                             │
                             └── &PullRequest{Number:33, Title:"Feature", HeadRefName:"feature", ...}
                                     │
                                     ▼
             fmt.Fprintf(opts.IO.Out, "Created pull request #%d: %s\n%s\n", ...)
                     │
                     └── stdout: "Created pull request #33: Feature\nhttps://..."
```

---

### 1.4 设计对比：`pr create` vs `issue create`

| 维度 | `issue create` | `pr create` |
|------|---------------|-------------|
| 仓库来源 | 只能通过 `--repo` 显式传入 | `--repo` 或从 git remote 自动推断 |
| 依赖 git | 不需要 | 需要（读 CurrentBranch 和 Remotes）|
| API 调用次数 | 1 次（REST POST） | 2 次（GraphQL GetRepository + REST POST）|
| 第一次 API 的用途 | 无 | 获取仓库默认分支名（作为默认 base）|
| head branch | 无此概念 | 自动从当前 git 分支读取 |
| 接口抽象 | 直接依赖具体类型 | 定义 `gitClienter` 接口，测试可 mock |
| REST 响应解码 | 专用 `issueRESTResponse` struct | 专用匿名 `raw` struct，嵌套解码 head/base |

**为什么 `pr create` 需要两步 API 调用？**

第一步 `GetRepository` 的目的是获取 `defaultBranchRef.Name`（通常是 `"main"` 或 `"master"`）。当用户没有通过 `--base` 明确指定 base 分支时，PR 应当 merge 到仓库的默认分支。这个信息无法从本地 git 仓库获取（本地不一定有全部远端分支信息），必须通过 API 查询。

---

## Section 2：实现步骤

### 2.1 创建 Phase 6 目录

**为什么从 Phase 5 复制？**

Phase 6 在 Phase 5 的全部代码上追加功能。直接复制 Phase 5 目录，然后新增和修改文件，是保持每个 Phase 自包含的最简方式。

【现在手敲】

```bash
# 从项目根目录执行
cp -r cli/src/phase-05-issue-commands cli/src/phase-06-pr-commands

# 创建 git 包目录和 pr 命令相关目录
mkdir -p cli/src/phase-06-pr-commands/git
mkdir -p cli/src/phase-06-pr-commands/pkg/cmd/pr/list
mkdir -p cli/src/phase-06-pr-commands/pkg/cmd/pr/view
mkdir -p cli/src/phase-06-pr-commands/pkg/cmd/pr/create
```

【验证】

```bash
ls cli/src/phase-06-pr-commands/git/
ls cli/src/phase-06-pr-commands/pkg/cmd/pr/
```

期望输出：`git/` 目录存在，`pkg/cmd/pr/` 下有 `list/`、`view/`、`create/` 三个子目录。

**关键点解释**

`git/` 包与 `api/` 包在层级上平行，都是基础设施层。`api/` 封装 HTTP 请求，`git/` 封装 shell 命令执行。二者都不依赖 `pkg/cmd/`，而是被 `pkg/cmd/` 依赖。

---

### 2.2 实现 git.Client 和 run() 方法

**为什么封装 exec.Command？**

直接在命令逻辑中调用 `exec.Command("git", ...)` 会导致两个问题：一是 git 命令散落在各处，难以统一管理工作目录（`RepoDir`）和标准错误处理；二是测试时无法替换 git 二进制。`Client` struct 将这些关注点集中在一处，并通过 `context.Context` 支持超时取消。

【现在手敲】

```go
// 文件：git/client.go
package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
)

// Remote represents a git remote.
type Remote struct {
	Name     string
	FetchURL *url.URL
	PushURL  *url.URL
}

// Client runs git commands in a repository directory.
type Client struct {
	RepoDir string
	GitPath string // path to git binary; if empty, uses "git" from PATH
	Stderr  io.Writer
}

// gitPath returns the git binary path, defaulting to "git".
func (c *Client) gitPath() string {
	if c.GitPath != "" {
		return c.GitPath
	}
	return "git"
}

// run executes a git command and returns its stdout.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.gitPath(), args...)
	if c.RepoDir != "" {
		cmd.Dir = c.RepoDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if c.Stderr != nil {
		cmd.Stderr = c.Stderr
	} else {
		cmd.Stderr = &stderr
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./git/
```

此时只编译 git 包骨架，无报错即可继续。

**关键点解释**

- `exec.CommandContext`：相比 `exec.Command`，可以通过 `ctx.Done()` 中断正在运行的 git 命令，避免命令挂起时 CLI 卡死。
- `bytes.Buffer` 分别捕获 stdout 和 stderr：正常输出通过返回值传递，错误信息通过 `fmt.Errorf` 包装，调用方无需解析 stderr。
- `strings.TrimSpace`：去掉 git 输出末尾的换行符，使调用方得到干净的字符串。

---

### 2.3 实现 CurrentBranch

**为什么用 `git rev-parse --abbrev-ref HEAD`？**

`git branch --show-current` 是更现代的命令，但在 git 2.22 之前不可用。`rev-parse --abbrev-ref HEAD` 输出当前分支的简短名称（如 `main`），且在所有主流 git 版本中都可用。

**为什么检查输出是否为 `"HEAD"`？**

当仓库处于 detached HEAD 状态（例如 `git checkout v1.0` 后），`rev-parse --abbrev-ref HEAD` 输出字面量 `"HEAD"` 而不是分支名。此时没有当前分支，不能创建 PR，需要明确报错。

【现在手敲】

在 `git/client.go` 末尾追加：

```go
// CurrentBranch returns the name of the current git branch.
func (c *Client) CurrentBranch(ctx context.Context) (string, error) {
	branch, err := c.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("could not determine current branch: %w", err)
	}
	if branch == "HEAD" {
		return "", fmt.Errorf("not on a branch (detached HEAD state)")
	}
	return branch, nil
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go vet ./git/
```

**关键点解释**

错误包装使用 `%w` 而非 `%v`，使调用方可以用 `errors.Is` / `errors.As` 判断错误类型，保留完整错误链。

---

### 2.4 实现 Remotes 和 parseRemotes

**`git remote -v` 输出格式**

```
origin	https://github.com/owner/repo.git (fetch)
origin	https://github.com/owner/repo.git (push)
upstream	https://github.com/upstream/repo.git (fetch)
upstream	https://github.com/upstream/repo.git (push)
```

每个 remote 出现**两次**——一次标注 `(fetch)`，一次标注 `(push)`。`parseRemotes` 需要按 name 去重，同时分别记录 FetchURL 和 PushURL。

【现在手敲】

在 `git/client.go` 末尾追加：

```go
// Remotes returns the configured git remotes.
func (c *Client) Remotes(ctx context.Context) ([]Remote, error) {
	out, err := c.run(ctx, "remote", "-v")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return parseRemotes(out), nil
}

// parseRemotes parses the output of `git remote -v`.
// Each remote appears twice (fetch and push); we deduplicate by name.
func parseRemotes(output string) []Remote {
	seen := map[string]*Remote{}
	var order []string

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "origin\thttps://github.com/owner/repo.git (fetch)"
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		name := parts[0]
		rawURL := parts[1]
		kind := strings.Trim(parts[2], "()")

		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		r, ok := seen[name]
		if !ok {
			r = &Remote{Name: name}
			seen[name] = r
			order = append(order, name)
		}
		switch kind {
		case "fetch":
			r.FetchURL = u
		case "push":
			r.PushURL = u
		}
	}

	remotes := make([]Remote, 0, len(order))
	for _, name := range order {
		remotes = append(remotes, *seen[name])
	}
	return remotes
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./git/
```

**关键点解释**

- `seen` map + `order` slice：map 用于 O(1) 查找已见过的 remote，slice 用于保持 remote 的原始顺序（`git remote -v` 的输出顺序有意义，`origin` 通常排在最前）。
- `strings.Fields`：按任意空白字符（包括 tab）分割，比 `strings.Split(line, "\t")` 更健壮。
- `url.Parse` 失败时 `continue`：忽略格式异常的行，不让单行错误中断整个解析。
- `git remote → GitHub repo 推断链`：`git remote -v` → 取 origin FetchURL → `ghrepo.FromURL()` 解析 `host`/`owner`/`name` → 构造 `ghrepo.Interface`。`ghrepo.FromURL` 能处理 HTTPS URL（`https://github.com/owner/repo.git`）和 SSH URL（`git@github.com:owner/repo.git`）两种格式。

---

### 2.5 编写 git 包测试

**为什么需要 `initTestRepo`？**

`CurrentBranch` 和 `Remotes` 的测试需要真实的 git 仓库。`initTestRepo` 用 `t.TempDir()` 创建隔离的临时目录，在测试结束后自动清理，无副作用。

`parseRemotes` 是纯函数（只依赖输入字符串），可以直接用单元测试，无需真实 git 仓库，是最简单的测试。

【现在手敲】

```go
// 文件：git/client_test.go
package git

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// initTestRepo creates a temporary git repo with a commit and an "origin" remote,
// returning a Client pointed at it.
func initTestRepo(t *testing.T) *Client {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Set minimal git config so commits work in CI.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "init")
	run("remote", "add", "origin", "https://github.com/owner/repo.git")

	return &Client{RepoDir: dir}
}

func TestCurrentBranch(t *testing.T) {
	c := initTestRepo(t)
	branch, err := c.CurrentBranch(context.Background())
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
}

func TestRemotes(t *testing.T) {
	c := initTestRepo(t)
	remotes, err := c.Remotes(context.Background())
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if len(remotes) != 1 {
		t.Fatalf("len(remotes) = %d, want 1", len(remotes))
	}
	if remotes[0].Name != "origin" {
		t.Errorf("remote name = %q, want origin", remotes[0].Name)
	}
	if remotes[0].FetchURL == nil {
		t.Error("FetchURL is nil")
	}
}

func TestParseRemotes(t *testing.T) {
	input := "origin\thttps://github.com/owner/repo.git (fetch)\norigin\thttps://github.com/owner/repo.git (push)\n"
	remotes := parseRemotes(input)
	if len(remotes) != 1 {
		t.Fatalf("len = %d, want 1", len(remotes))
	}
	if remotes[0].Name != "origin" {
		t.Errorf("name = %q", remotes[0].Name)
	}
	if remotes[0].FetchURL.Host != "github.com" {
		t.Errorf("host = %q", remotes[0].FetchURL.Host)
	}
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go test ./git/ -v
```

期望：三个测试全部 PASS。`TestParseRemotes` 验证去重逻辑——两行 `origin` 输入只产生一个 `Remote`。

**关键点解释**

- `t.Helper()`：让测试失败时报告调用 `initTestRepo` 的测试行，而不是 `initTestRepo` 内部的行，更易定位问题。
- 注入 `GIT_AUTHOR_*` 和 `GIT_COMMITTER_*` 环境变量：在 CI 环境中 git 全局配置可能为空，空邮件地址会导致 `git commit` 失败。
- `git init -b main`：明确指定初始分支名为 `main`，避免因 `init.defaultBranch` 配置不同而得到 `master`，使 `TestCurrentBranch` 的断言不依赖环境配置。

---

### 2.6 定义 PullRequest struct

**与 Issue struct 的相似之处**

`PullRequest` 和 `Issue` 有大量共同字段：`Number`、`Title`、`State`、`Body`、`Author`、`URL`、`CreatedAt`。这是因为 GitHub API 对 Issue 和 PR 的基础字段设计是一致的。

**PR 独有的字段**

- `HeadRefName`：PR 的源分支（feature branch）。
- `BaseRefName`：PR 的目标分支（通常是 main）。
- `IsDraft`：是否为草稿 PR。Issue 没有 Draft 概念。

**`prFields` 常量的作用**

`ListPullRequests` 和 `GetPullRequest` 需要查询相同的字段集合。将字段列表抽取为包级常量 `prFields`，在两个查询中复用，避免重复定义。

【现在手敲】

```go
// 文件：api/queries_pr.go
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	Number      int
	Title       string
	State       string
	Body        string
	HeadRefName string
	BaseRefName string
	Author      struct{ Login string }
	URL         string
	CreatedAt   time.Time
	IsDraft     bool
}

const prFields = `
			number
			title
			state
			body
			headRefName
			baseRefName
			author { login }
			url
			createdAt
			isDraft`
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./api/
```

**关键点解释**

GraphQL 字段名使用 camelCase（如 `headRefName`），Go struct 字段名使用 PascalCase（如 `HeadRefName`）。`encoding/json` 解码时做大小写不敏感匹配，因此无需为每个字段添加 `json` tag——这是 Phase 3 建立的约定，在 Phase 6 继续沿用。

---

### 2.7 实现 ListPullRequests

**为什么 `states` 是数组类型？**

GitHub GraphQL 的 `pullRequests(states: [PullRequestState!])` 接受一个状态数组，支持同时查询多种状态。实际代码中每次只传一个状态（`["OPEN"]`、`["CLOSED"]` 或 `["MERGED"]`），`--state all` 的多状态合并在命令层（`listRun`）通过三次独立查询实现，而非通过 GraphQL 数组参数一次获取（以避免分页逻辑复杂化）。

【现在手敲】

在 `api/queries_pr.go` 中追加：

```go
// ListPullRequests fetches pull requests for a repository.
func ListPullRequests(client *Client, repo ghrepo.Interface, state string, limit int) ([]PullRequest, error) {
	if state == "" {
		state = "OPEN"
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var result struct {
		Repository struct {
			PullRequests struct {
				Nodes []PullRequest
			}
		}
	}
	query := `
query ListPullRequests($owner: String!, $name: String!, $states: [PullRequestState!], $first: Int!) {
	repository(owner: $owner, name: $name) {
		pullRequests(states: $states, first: $first, orderBy: {field: CREATED_AT, direction: DESC}) {
			nodes {` + prFields + `
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
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}
	return result.Repository.PullRequests.Nodes, nil
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go vet ./api/
```

**关键点解释**

查询结果通过三层嵌套的匿名 struct 解码：`result.Repository.PullRequests.Nodes`。这与 GraphQL 响应的 JSON 结构 `{"data":{"repository":{"pullRequests":{"nodes":[...]}}}}` 一一对应，是 Phase 3 建立的解码模式在 PR 场景的直接应用。

---

### 2.8 实现 GetPullRequest

**与 GetIssue 的对比**

`GetPullRequest` 和 `GetIssue` 结构完全相同：接收 `number` 参数，构造单查询 GraphQL，返回单个对象。差异仅在于 struct 类型和字段集合。

【现在手敲】

在 `api/queries_pr.go` 中追加：

```go
// GetPullRequest fetches a single pull request by number.
func GetPullRequest(client *Client, repo ghrepo.Interface, number int) (*PullRequest, error) {
	var result struct {
		Repository struct {
			PullRequest PullRequest
		}
	}
	query := `
query GetPullRequest($owner: String!, $name: String!, $number: Int!) {
	repository(owner: $owner, name: $name) {
		pullRequest(number: $number) {` + prFields + `
		}
	}
}`
	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"name":   repo.RepoName(),
		"number": number,
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}
	return &result.Repository.PullRequest, nil
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./api/
```

**关键点解释**

返回 `*PullRequest`（指针）而不是 `PullRequest`（值），使调用方可以检查 `nil`（虽然 GraphQL 返回 404 时实际上会通过 `errors` 字段报错，而非返回 null 对象），同时避免大 struct 的值拷贝。

---

### 2.9 实现 CreatePullRequest

**为什么使用 REST 而非 GraphQL？**

GitHub GraphQL `createPullRequest` mutation 存在和 `createIssue` 类似的问题——需要传入 `repositoryId`（Node ID），必须先额外查询。而 REST `POST /repos/{owner}/{repo}/pulls` 只需 owner/name，更直接。

**嵌套响应解码的设计**

GitHub REST API 返回的 PR JSON 包含嵌套结构：

```json
{
  "number": 33,
  "html_url": "https://github.com/cli/cli/pull/33",
  "head": {"ref": "feature"},
  "base": {"ref": "main"}
}
```

`head.ref` 和 `base.ref` 不能直接映射到 `PullRequest.HeadRefName` 和 `PullRequest.BaseRefName`，需要一个专用的匿名 `raw` struct 来承接嵌套解码，再手动赋值到 `PullRequest`。

【现在手敲】

在 `api/queries_pr.go` 中追加：

```go
// CreatePullRequest creates a new pull request via REST POST.
// params should contain: title, body, head (branch), base (branch).
func CreatePullRequest(client *Client, repo *Repository, params map[string]interface{}) (*PullRequest, error) {
	b, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("repos/%s/%s/pulls", repo.RepoOwner(), repo.RepoName())
	var raw struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		IsDraft bool   `json:"draft"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := client.REST(repo.RepoHost(), "POST", path, bytes.NewReader(b), &raw); err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}
	return &PullRequest{
		Number:      raw.Number,
		Title:       raw.Title,
		URL:         raw.HTMLURL,
		IsDraft:     raw.IsDraft,
		HeadRefName: raw.Head.Ref,
		BaseRefName: raw.Base.Ref,
	}, nil
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./api/
```

**关键点解释**

- 参数接收 `*Repository` 而非 `ghrepo.Interface`：因为 `CreatePullRequest` 调用前已经执行过 `GetRepository`，拿到了 `*api.Repository`；直接传入可以复用，避免重复解构 owner/name。
- `raw.HTMLURL` 对应 `json:"html_url"`：REST API 返回 snake_case 的 `html_url`，而通用 `PullRequest.URL` 字段没有 json tag；专用 `raw` struct 用 `json:"html_url"` tag 正确解码，再赋值给 `PullRequest.URL`。
- `json:"draft"` 不是 `json:"isDraft"`：REST API 用 `draft` 字段表示草稿状态，GraphQL API 用 `isDraft`。这是 GitHub API 两种协议的命名不一致，需要分别处理。

---

### 2.10 编写 queries_pr 测试

**测试策略**

三个函数各一个测试，均使用 `httptest.NewServer` + `rewriteTransport` 模式（与 Phase 5 完全相同）。`TestCreatePullRequest` 额外验证请求 method 为 POST。

【现在手敲】

```go
// 文件：api/queries_pr_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/learngh/gh-impl/internal/ghrepo"
)

func TestListPullRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"pullRequests": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 10, "title": "Add feature", "state": "OPEN",
								"body": "", "headRefName": "feature", "baseRefName": "main",
								"author": map[string]string{"login": "alice"},
								"url":       "https://github.com/cli/cli/pull/10",
								"createdAt": "2024-01-01T00:00:00Z",
								"isDraft":   false,
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

	prs, err := ListPullRequests(client, repo, "OPEN", 10)
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len = %d, want 1", len(prs))
	}
	if prs[0].Title != "Add feature" {
		t.Errorf("Title = %q", prs[0].Title)
	}
}

func TestGetPullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"pullRequest": map[string]interface{}{
						"number": 42, "title": "Fix bug", "state": "MERGED",
						"body": "fixes it", "headRefName": "fix-branch", "baseRefName": "main",
						"author": map[string]string{"login": "bob"},
						"url":       "https://github.com/cli/cli/pull/42",
						"createdAt": "2024-01-01T00:00:00Z",
						"isDraft":   false,
					},
				},
			},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := ghrepo.New("cli", "cli")

	pr, err := GetPullRequest(client, repo, 42)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if pr.Number != 42 || pr.Title != "Fix bug" {
		t.Errorf("got %d %q", pr.Number, pr.Title)
	}
}

func TestCreatePullRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   77,
			"title":    "My PR",
			"html_url": "https://github.com/cli/cli/pull/77",
			"draft":    false,
			"head":     map[string]interface{}{"ref": "feature"},
			"base":     map[string]interface{}{"ref": "main"},
		})
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}
	client := NewClientFromHTTP(&http.Client{Transport: transport})
	repo := &Repository{
		Name:  "cli",
		Owner: struct{ Login string }{Login: "cli"},
	}

	pr, err := CreatePullRequest(client, repo, map[string]interface{}{
		"title": "My PR",
		"head":  "feature",
		"base":  "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Number != 77 {
		t.Errorf("Number = %d, want 77", pr.Number)
	}
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go test ./api/ -run TestListPullRequests -v
cd cli/src/phase-06-pr-commands && go test ./api/ -run TestGetPullRequest -v
cd cli/src/phase-06-pr-commands && go test ./api/ -run TestCreatePullRequest -v
```

**关键点解释**

`rewriteTransport` 在 `api` 包测试中已经存在（来自 Phase 5 的 `queries_issue_test.go`）。因为两个测试文件都在同一个包（`package api`），`rewriteTransport` 不会重复定义——它在 `queries_issue_test.go` 中已经有了，`queries_pr_test.go` 直接复用。

---

### 2.11 实现 NewCmdPRList

**`--state all` 的三次查询设计**

PR 有三种终态：`OPEN`、`CLOSED`（已关闭但未 merge）、`MERGED`（已合并）。GitHub GraphQL 的 `PullRequestState` enum 没有 `ALL` 值，因此 `--state all` 需要对三种状态各执行一次查询，再将结果合并。这与 Phase 5 的 `issue list --state all`（两次查询：OPEN + CLOSED）类似，只是多了 `MERGED`。

**输出格式**

每行显示 PR 编号、标题、Draft 标记和分支信息，体现 PR 与 Issue 不同的核心属性：

```
#5      Cool PR        (feature -> main)
#3      Draft PR [DRAFT]  (wip -> main)
```

【现在手敲】

```go
// 文件：pkg/cmd/pr/list/list.go
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

// ListOptions holds all inputs for the pr list command.
type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	State      string
	Limit      int
}

// NewCmdPRList creates the `gh pr list` command.
func NewCmdPRList(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		State:      "open",
		Limit:      30,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests in a repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: open, closed, merged, all")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	return cmd
}

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

	var graphqlState string
	switch strings.ToLower(opts.State) {
	case "open":
		graphqlState = "OPEN"
	case "closed":
		graphqlState = "CLOSED"
	case "merged":
		graphqlState = "MERGED"
	case "all":
		graphqlState = ""
	default:
		return fmt.Errorf("invalid state %q: use open, closed, merged, or all", opts.State)
	}

	var prs []api.PullRequest
	if graphqlState == "" {
		for _, s := range []string{"OPEN", "CLOSED", "MERGED"} {
			got, err := api.ListPullRequests(client, repo, s, opts.Limit)
			if err != nil {
				return err
			}
			prs = append(prs, got...)
		}
	} else {
		prs, err = api.ListPullRequests(client, repo, graphqlState, opts.Limit)
		if err != nil {
			return err
		}
	}

	if len(prs) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "No pull requests found.\n")
		return nil
	}
	for _, pr := range prs {
		draft := ""
		if pr.IsDraft {
			draft = " [DRAFT]"
		}
		fmt.Fprintf(opts.IO.Out, "#%-5d  %s%s\t(%s -> %s)\n",
			pr.Number, pr.Title, draft, pr.HeadRefName, pr.BaseRefName)
	}
	return nil
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./pkg/cmd/pr/list/
```

**关键点解释**

- 默认状态为 `"open"`（小写），在 `listRun` 内转换为 GraphQL 枚举值 `"OPEN"`（大写），用户侧友好，API 侧正确。
- `"No pull requests found."` 输出到 `ErrOut`（stderr）而非 `Out`（stdout），遵循 Unix 约定：informational messages 走 stderr，data 走 stdout，方便管道处理。

---

### 2.12 编写 list 测试

【现在手敲】

```go
// 文件：pkg/cmd/pr/list/list_test.go
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
					"pullRequests": map[string]interface{}{
						"nodes": []map[string]interface{}{
							{
								"number": 5, "title": "Cool PR", "state": "OPEN",
								"body": "", "headRefName": "feature", "baseRefName": "main",
								"author":    map[string]string{"login": "user"},
								"url":       "https://github.com/cli/cli/pull/5",
								"createdAt": "2024-01-01T00:00:00Z",
								"isDraft":   false,
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
	if !strings.Contains(out.String(), "Cool PR") {
		t.Errorf("output = %q", out.String())
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
cd cli/src/phase-06-pr-commands && go test ./pkg/cmd/pr/list/ -v
```

**关键点解释**

`iostreams.Test()` 返回 `(ios, in, out, errOut)`，其中 `out` 是一个 `*bytes.Buffer`，可以在测试结束后通过 `out.String()` 检查输出内容。这是整个项目统一的命令测试模式，从 Phase 5 开始建立。

---

### 2.13 实现 NewCmdPRView

**与 NewCmdIssueView 的对比**

`view` 命令结构与 `issue view` 几乎相同：`cobra.ExactArgs(1)` 要求恰好一个位置参数，`strconv.Atoi` 解析编号，`GetPullRequest` 查询数据，`printPR` 格式化输出。

新增的 `printPR` 输出 `HeadRefName -> BaseRefName`（分支信息），这是 PR 独有的，`printIssue` 没有这一行。

【现在手敲】

```go
// 文件：pkg/cmd/pr/view/view.go
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

// ViewOptions holds all inputs for the pr view command.
type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Repo       string
	PRArg      string
}

// NewCmdPRView creates the `gh pr view` command.
func NewCmdPRView(f *cmdutil.Factory) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}
	cmd := &cobra.Command{
		Use:   "view <number>",
		Short: "View details of a pull request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.PRArg = args[0]
			return viewRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	return cmd
}

func viewRun(opts *ViewOptions) error {
	if opts.Repo == "" {
		return fmt.Errorf("repository required: use --repo owner/name")
	}
	repo, err := ghrepo.FromFullName(opts.Repo)
	if err != nil {
		return err
	}
	number, err := strconv.Atoi(opts.PRArg)
	if err != nil {
		return fmt.Errorf("invalid pull request number: %q", opts.PRArg)
	}
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)
	pr, err := api.GetPullRequest(client, repo, number)
	if err != nil {
		return err
	}
	printPR(opts.IO, pr)
	return nil
}

func printPR(io *iostreams.IOStreams, pr *api.PullRequest) {
	w := io.Out
	draft := ""
	if pr.IsDraft {
		draft = " [DRAFT]"
	}
	fmt.Fprintf(w, "#%d %s%s\n", pr.Number, pr.Title, draft)
	fmt.Fprintf(w, "state:\t%s\n", strings.ToLower(pr.State))
	fmt.Fprintf(w, "author:\t%s\n", pr.Author.Login)
	fmt.Fprintf(w, "branch:\t%s -> %s\n", pr.HeadRefName, pr.BaseRefName)
	if pr.Body != "" {
		fmt.Fprintf(w, "\n%s\n", pr.Body)
	}
	fmt.Fprintf(w, "\nurl:\t%s\n", pr.URL)
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./pkg/cmd/pr/view/
```

**关键点解释**

`strings.ToLower(pr.State)` 将 GraphQL 返回的大写状态（`"OPEN"`、`"MERGED"`）转为用户友好的小写展示（`"open"`、`"merged"`）。这与 `printIssue` 采用相同的展示约定。

---

### 2.14 编写 view 测试

【现在手敲】

```go
// 文件：pkg/cmd/pr/view/view_test.go
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
					"pullRequest": map[string]interface{}{
						"number": 9, "title": "Test PR", "state": "OPEN",
						"body": "PR body", "headRefName": "feat", "baseRefName": "main",
						"author":    map[string]string{"login": "eve"},
						"url":       "https://github.com/cli/cli/pull/9",
						"createdAt": "2024-01-01T00:00:00Z",
						"isDraft":   false,
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
		Repo:  "cli/cli",
		PRArg: "9",
	}
	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "Test PR") {
		t.Errorf("output missing title: %q", output)
	}
	if !strings.Contains(output, "eve") {
		t.Errorf("output missing author: %q", output)
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
cd cli/src/phase-06-pr-commands && go test ./pkg/cmd/pr/view/ -v
```

**关键点解释**

测试同时检查标题（`"Test PR"`）和作者（`"eve"`），覆盖了 `printPR` 的两个关键输出路径，比只检查一个字段更可靠。

---

### 2.15 实现 gitClienter 接口

**为什么用接口而不是直接用 `*git.Client`？**

如果 `CreateOptions.GitClient` 字段直接声明为 `*git.Client`，测试时就必须提供真实的 git 仓库环境（或者用 `initTestRepo` 创建临时仓库）。这会让 `create` 包的测试依赖 git 的具体实现，引入不必要的复杂性。

通过定义包内接口 `gitClienter`，测试可以注入 `fakeGitClient`——一个只实现了两个方法的简单 struct，返回预设的分支名和 remote 列表，完全受测试控制，不需要任何真实的 git 仓库。

这体现了 Go 的**接口隔离**原则：接口在使用方（`create` 包）中定义，只包含使用方实际需要的方法，而不是在提供方（`git` 包）中定义一个大接口。

【现在手敲】

`gitClienter` 接口已经在 `create.go` 文件的开头定义，在写 `create.go` 时一并完成：

```go
// gitClienter abstracts git operations needed by pr create.
type gitClienter interface {
	CurrentBranch(ctx context.Context) (string, error)
	Remotes(ctx context.Context) ([]git.Remote, error)
}
```

`*git.Client` 实现了 `CurrentBranch` 和 `Remotes` 两个方法，满足 `gitClienter` 接口，无需任何额外声明（Go 隐式接口）。

【验证】

```bash
# 验证接口满足（在任意 _test.go 或 main.go 中检查类型断言）
cd cli/src/phase-06-pr-commands && go vet ./pkg/cmd/pr/create/
```

**关键点解释**

接口的两个方法签名与 `*git.Client` 的实际方法完全一致：`CurrentBranch(ctx context.Context) (string, error)` 和 `Remotes(ctx context.Context) ([]git.Remote, error)`。Go 编译器在赋值时隐式检查满足关系：`opts.GitClient = f.GitClient`（`f.GitClient` 是 `*git.Client`，`opts.GitClient` 类型是 `gitClienter`）。

---

### 2.16 实现 NewCmdPRCreate 和 createRun

**两步逻辑的完整流程**

1. **读取本地 git 状态**：`CurrentBranch`（head branch）和 `Remotes`（推断 repo）。
2. **推断目标仓库**：优先用 `--repo`，否则从 origin FetchURL 解析。
3. **第一次 API 调用**：`GetRepository` 获取 `defaultBranchRef.Name`（默认 base 分支）。
4. **第二次 API 调用**：`CreatePullRequest` 通过 REST POST 创建 PR。

【现在手敲】

```go
// 文件：pkg/cmd/pr/create/create.go
package create

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/git"
	"github.com/learngh/gh-impl/internal/ghrepo"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// gitClienter abstracts git operations needed by pr create.
type gitClienter interface {
	CurrentBranch(ctx context.Context) (string, error)
	Remotes(ctx context.Context) ([]git.Remote, error)
}

// CreateOptions holds all inputs for the pr create command.
type CreateOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	GitClient  gitClienter
	Repo       string
	Title      string
	Body       string
	Base       string
	Draft      bool
}

// NewCmdPRCreate creates the `gh pr create` command.
func NewCmdPRCreate(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient, // *git.Client satisfies gitClienter
	}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		RunE: func(cmd *cobra.Command, args []string) error {
			return createRun(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title (required)")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Body")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "The branch into which you want your code merged")
	cmd.Flags().BoolVarP(&opts.Draft, "draft", "d", false, "Mark pull request as a draft")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func createRun(opts *CreateOptions) error {
	ctx := context.Background()

	if opts.GitClient == nil {
		return fmt.Errorf("not in a git repository")
	}

	headBranch, err := opts.GitClient.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("could not determine head branch: %w", err)
	}

	var repo ghrepo.Interface
	if opts.Repo != "" {
		repo, err = ghrepo.FromFullName(opts.Repo)
		if err != nil {
			return err
		}
	} else {
		remotes, err := opts.GitClient.Remotes(ctx)
		if err != nil || len(remotes) == 0 {
			return fmt.Errorf("no git remotes found; use --repo")
		}
		var fetchURL *url.URL
		for _, r := range remotes {
			if r.Name == "origin" {
				fetchURL = r.FetchURL
				break
			}
		}
		if fetchURL == nil {
			fetchURL = remotes[0].FetchURL
		}
		if fetchURL == nil {
			return fmt.Errorf("remote has no fetch URL; use --repo")
		}
		repo, err = ghrepo.FromURL(fetchURL)
		if err != nil {
			return fmt.Errorf("could not parse remote URL: %w", err)
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	apiRepo, err := api.GetRepository(client, repo)
	if err != nil {
		return err
	}

	base := opts.Base
	if base == "" {
		base = apiRepo.DefaultBranchRef.Name
		if base == "" {
			base = "main"
		}
	}

	params := map[string]interface{}{
		"title": opts.Title,
		"body":  opts.Body,
		"head":  headBranch,
		"base":  base,
		"draft": opts.Draft,
	}

	pr, err := api.CreatePullRequest(client, apiRepo, params)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "Created pull request #%d: %s\n%s\n", pr.Number, pr.Title, pr.URL)
	return nil
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./pkg/cmd/pr/create/
```

**关键点解释**

- origin 优先：在遍历 remotes 时，明确查找名为 `"origin"` 的 remote，找不到才 fallback 到第一个 remote（`remotes[0]`）。大多数仓库的主 remote 叫 `"origin"`，这个约定降低了用户的认知负担。
- `base = "main"` 的三层 fallback：`opts.Base`（用户显式传入）> `apiRepo.DefaultBranchRef.Name`（API 获取）> 硬编码 `"main"`（极端情况兜底）。
- `api.GetRepository` 的复用：这个函数在 Phase 4 的 `repo view` 命令中就已经存在，Phase 6 的 `pr create` 直接复用，体现了"API 层函数跨命令共享"的设计。

---

### 2.17 编写 create 测试

**fakeGitClient 的设计**

`fakeGitClient` 是一个私有 struct，只在测试文件中定义，实现了 `gitClienter` 接口。它不依赖任何真实的 git 二进制，直接返回测试构造时注入的 `branch` 和 `remotes`，完全可控。

**测试服务器的路由**

`TestCreateRun` 的 `httptest.Server` 需要同时处理两类请求：

1. **GraphQL**（`GetRepository`）：任何非 POST 的路径，返回包含 `defaultBranchRef` 的仓库数据。
2. **REST POST `/pulls`**（`CreatePullRequest`）：路径包含 `/pulls`，返回新建 PR 数据。

通过 `r.Method == "POST" && strings.Contains(r.URL.Path, "/pulls")` 区分两种请求，其余请求视为 GraphQL 查询。

【现在手敲】

```go
// 文件：pkg/cmd/pr/create/create_test.go
package create

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/git"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

// fakeGitClient implements gitClienter for tests.
type fakeGitClient struct {
	branch  string
	remotes []git.Remote
}

func (f *fakeGitClient) CurrentBranch(_ context.Context) (string, error) {
	return f.branch, nil
}

func (f *fakeGitClient) Remotes(_ context.Context) ([]git.Remote, error) {
	return f.remotes, nil
}

func TestCreateRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/pulls") {
			// CreatePullRequest REST call
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"number":   33,
				"title":    "Feature branch PR",
				"html_url": "https://github.com/cli/cli/pull/33",
				"draft":    false,
				"head":     map[string]interface{}{"ref": "feature"},
				"base":     map[string]interface{}{"ref": "main"},
			})
			return
		}
		// GraphQL call (GetRepository)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"id":            "R_1",
					"name":          "cli",
					"nameWithOwner": "cli/cli",
					"owner":         map[string]string{"login": "cli"},
					"description":   "",
					"isPrivate":     false,
					"isFork":        false,
					"stargazerCount":  0,
					"forkCount":     0,
					"defaultBranchRef": map[string]string{"name": "main"},
					"url":           "https://github.com/cli/cli",
				},
			},
		})
	}))
	defer srv.Close()

	originURL, _ := url.Parse("https://github.com/cli/cli.git")
	ios, _, out, _ := iostreams.Test()
	opts := &CreateOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		GitClient: &fakeGitClient{
			branch:  "feature",
			remotes: []git.Remote{{Name: "origin", FetchURL: originURL}},
		},
		Title: "Feature branch PR",
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !strings.Contains(out.String(), "#33") {
		t.Errorf("output = %q, want to contain #33", out.String())
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
cd cli/src/phase-06-pr-commands && go test ./pkg/cmd/pr/create/ -v
```

**关键点解释**

`fakeGitClient.remotes` 中注入了一个 `origin` remote，其 `FetchURL` 是 `https://github.com/cli/cli.git`。`createRun` 会通过 `ghrepo.FromURL(fetchURL)` 将其解析为 `{owner:"cli", name:"cli", host:"github.com"}`，这个推断链在测试中得到了验证，无需真实网络请求。

---

### 2.18 修改 Factory 和 factory.New()

**为什么 `GitClient` 的类型是 `*git.Client` 而非 `gitClienter`？**

`Factory` 是全局依赖容器，应该持有具体类型（`*git.Client`），这样各个命令可以根据自身需要将其转换为相应的接口。`pr create` 将 `f.GitClient`（`*git.Client`）赋值给 `opts.GitClient`（`gitClienter`），Go 的隐式接口机制会在赋值时检查类型满足。其他未来的命令如果需要额外的 git 操作，可以定义自己的接口，Factory 提供的 `*git.Client` 同样能满足。

【现在手敲】

修改 `pkg/cmdutil/factory.go`，在 `Factory` struct 中添加 `GitClient` 字段：

```go
// 文件：pkg/cmdutil/factory.go
package cmdutil

import (
	"net/http"

	"github.com/learngh/gh-impl/git"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

// Factory holds the dependencies that commands need to execute.
// All fields are set by factory.New and are safe to read concurrently.
type Factory struct {
	// AppVersion is the CLI version string (e.g. "2.40.0").
	AppVersion string
	// ExecutableName is the name used to invoke the CLI (e.g. "gh").
	ExecutableName string
	// IOStreams provides access to stdin/stdout/stderr.
	IOStreams *iostreams.IOStreams
	// Config is a lazy getter for application configuration.
	Config func() (Config, error)
	// HttpClient is a lazy getter for an authenticated HTTP client.
	HttpClient func() (*http.Client, error)
	// GitClient provides git operations.
	GitClient *git.Client
}
```

修改 `internal/factory/factory.go`，在 `New()` 函数末尾添加 `GitClient` 初始化：

```go
// 在 f.HttpClient = func() {...} 的闭包定义之后添加：
f.GitClient = &git.Client{}
return f
```

完整的 `New()` 函数如下：

```go
// New constructs a Factory. HttpClient is lazy: reads the auth token from
// Config at call time and builds an authenticated *http.Client.
func New(appVersion string) *cmdutil.Factory {
	ios := iostreams.System()

	f := &cmdutil.Factory{
		AppVersion:     appVersion,
		ExecutableName: "gh",
		IOStreams:       ios,
		Config: func() (cmdutil.Config, error) {
			return &stubConfig{}, nil
		},
	}

	f.HttpClient = func() (*http.Client, error) {
		cfg, err := f.Config()
		if err != nil {
			return apiPkg.NewHTTPClient("", f.AppVersion), nil
		}
		token, _ := cfg.AuthToken("github.com")
		return apiPkg.NewHTTPClient(token, f.AppVersion), nil
	}

	f.GitClient = &git.Client{}
	return f
}
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./pkg/cmdutil/ ./internal/factory/
```

**关键点解释**

`&git.Client{}` 创建一个零值 `Client`：`RepoDir` 为空（使用当前工作目录）、`GitPath` 为空（使用 PATH 中的 `git`）、`Stderr` 为 nil（错误输出捕获到内部 buffer）。这是最通用的默认配置，适合 CLI 的实际运行场景。

---

### 2.19 实现 NewCmdPR 并注册到 root

**命令组的组织**

`pkg/cmd/pr/pr.go` 是 PR 命令组的入口，模式与 `pkg/cmd/issue/issue.go` 完全相同：创建父命令 `gh pr`，将三个子命令作为子命令注册。

【现在手敲】

```go
// 文件：pkg/cmd/pr/pr.go
package pr

import (
	"github.com/learngh/gh-impl/pkg/cmd/pr/create"
	"github.com/learngh/gh-impl/pkg/cmd/pr/list"
	"github.com/learngh/gh-impl/pkg/cmd/pr/view"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdPR creates the `gh pr` command group.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <subcommand>",
		Short: "Manage pull requests",
		Long:  "Work with GitHub pull requests.",
	}
	cmd.AddCommand(list.NewCmdPRList(f))
	cmd.AddCommand(view.NewCmdPRView(f))
	cmd.AddCommand(create.NewCmdPRCreate(f))
	return cmd
}
```

修改 `pkg/cmd/root/root.go`，在 issue 命令注册之后添加 pr 命令注册：

```go
// 在 cmd.AddCommand(issueCmd.NewCmdIssue(f)) 之后添加：

// PR command group.
cmd.AddCommand(prCmd.NewCmdPR(f))
```

并在 import 块中添加：

```go
prCmd "github.com/learngh/gh-impl/pkg/cmd/pr"
```

【验证】

```bash
cd cli/src/phase-06-pr-commands && go build ./...
```

期望：整个项目无编译错误。

**关键点解释**

`root.go` 中已经存在 `issueCmd` 的 import 别名模式，`prCmd` 的 import 别名与之一致，保持命名约定统一。命令注册顺序（version → auth → api → repo → issue → pr）反映了功能从基础到复杂的层次关系。

---

### 2.20 运行所有测试

【现在手敲】

```bash
cd cli/src/phase-06-pr-commands && go test ./...
```

【验证】

期望输出类似：

```
ok  	github.com/learngh/gh-impl/api         0.XXXs
ok  	github.com/learngh/gh-impl/git         0.XXXs
ok  	github.com/learngh/gh-impl/internal/ghrepo  0.XXXs
ok  	github.com/learngh/gh-impl/pkg/cmd/pr/create  0.XXXs
ok  	github.com/learngh/gh-impl/pkg/cmd/pr/list    0.XXXs
ok  	github.com/learngh/gh-impl/pkg/cmd/pr/view    0.XXXs
...
```

所有包测试通过，无 FAIL。

如果遇到 `git: command not found` 错误，说明当前环境没有安装 git，`git/` 包的集成测试（`TestCurrentBranch`、`TestRemotes`）会失败。解决方式：安装 git，或者在 CI 中确保 git 可用。`TestParseRemotes` 是纯单元测试，不需要 git 二进制，始终可以运行。

**运行单个包的测试：**

```bash
# 只运行 git 包
go test ./git/ -v

# 只运行 pr create
go test ./pkg/cmd/pr/create/ -v -run TestCreateRun

# 只运行 api 层 PR 相关测试
go test ./api/ -v -run TestListPullRequests
go test ./api/ -v -run TestGetPullRequest
go test ./api/ -v -run TestCreatePullRequest
```

---

## 项目总结：六个 Phase 的核心学习点

至此，整个 `gh` CLI 学习项目已经完成。回顾六个 Phase 各自解决的核心问题：

| Phase | 主题 | 核心学习点 |
|-------|------|-----------|
| Phase 1 | CLI 框架 | cobra 命令树的构建；`Factory` 依赖容器模式；PersistentPreRunE 中间件 |
| Phase 2 | 配置与认证 | 文件系统配置（`os.UserConfigDir`）；keyring 安全存储 token；`gh auth login / logout / status` 流程 |
| Phase 3 | API 客户端 | HTTP 客户端封装；GraphQL 和 REST 两种调用方式；`rewriteTransport` 测试模式 |
| Phase 4 | 仓库命令 | `ghrepo.Interface` 抽象；GraphQL 查询字段选择；`GetRepository` 的复用设计 |
| Phase 5 | Issue 命令 | 读写操作分层（GraphQL 读 / REST 写）；`issueRESTResponse` 处理 snake_case；`MarkFlagRequired` 必填 flag |
| Phase 6 | PR 命令 | `git/` 包封装 shell 命令；`gitClienter` 接口隔离；两步 API 调用（GetRepository + CreatePullRequest）；嵌套 REST 响应解码 |

**贯穿全项目的设计原则：**

1. **依赖注入**：所有命令通过 `Factory` 接收依赖，而非直接构造——使测试可以注入 fake 实现。
2. **接口在使用方定义**：`gitClienter`、`Config` 等接口定义在使用方包内，只包含实际需要的方法，而不是在实现方定义大接口。
3. **API 层集中**：`api/queries_*.go` 负责所有 GitHub API 交互，命令层只调用 API 函数，不直接构造 HTTP 请求。
4. **测试隔离**：`httptest.Server` + `rewriteTransport` 模拟 HTTP；`fakeGitClient` 模拟 git；`iostreams.Test()` 捕获输出——三种模式覆盖了全项目的测试需求。
5. **错误包装**：始终用 `%w` 包装错误，保留完整的错误链，方便调试和 `errors.Is` / `errors.As` 使用。
