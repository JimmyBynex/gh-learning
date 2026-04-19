# Design Document: gh CLI 学习实现

## 3a. 项目概述

**一句话描述**: `gh` 是 GitHub 官方命令行工具，将 pull request、issue、仓库等 GitHub 概念封装为终端命令，与本地 git 工作流无缝集成。

**核心问题**: 开发者需要在终端中操作 GitHub（查看 PR、创建 issue、管理仓库），而不必切换到浏览器，也不必手写 curl 命令。

**关键架构决策**:
- 使用 **Cobra** 作为命令框架，提供统一的子命令、flag、帮助文本体系
- 使用 **Factory 模式** 集中管理依赖（HTTP client、配置、IO），让每个命令只声明需要什么，不关心如何构建
- **Options struct** 模式：每个命令将所有输入（flag + 依赖）聚合到一个 struct，使测试无需启动完整 CLI

**实现语言**: Go（与原项目一致）

---

## 3b. 架构图

```
用户输入
   │
   ▼
[main.go] → ghcmd.Main()
                │
                ▼
         [factory.New()]          ← 构建所有依赖
                │
                ├── IOStreams     (stdin/stdout/stderr 封装)
                ├── Config       (读取 ~/.config/gh/hosts.yml)
                ├── HttpClient   (带 token 的 HTTP client)
                └── GitClient    (封装 git 子进程)
                │
                ▼
         [root.NewCmdRoot(f)]     ← 注册所有子命令
                │
         ┌──────┴──────────────────────────┐
         │             │                   │
    [auth cmds]   [repo cmds]   [pr/issue cmds]
    auth/login    repo/view     pr/list
    auth/status   repo/list     pr/view
                              issue/list
                              issue/view
                              issue/create
                │
                ▼
         [api.Client]             ← REST / GraphQL
                │
                ▼
         [GitHub API]             ← api.github.com
```

**数据流**:
```
用户: gh pr list
  → Cobra 解析: command=pr, subcommand=list
  → NewCmdPrList(f) 创建 Options, 绑定 factory 依赖
  → RunE 调用 api.Client.GraphQL() 查询 PR 列表
  → 结果反序列化为 []PullRequest struct
  → IOStreams 格式化输出到 stdout
```

---

## 3c. 功能列表

### core（实现）

```
- CLI 框架
  what: 提供 `gh` 二进制，支持子命令、--help、--version
  why:  所有功能的入口，必须先有框架才能添加命令
  src:  refs/source/cmd/gh/main.go
        refs/source/internal/ghcmd/cmd.go
        refs/source/pkg/cmd/root/root.go
        refs/source/pkg/cmdutil/factory.go
        refs/source/pkg/cmd/factory/default.go
        refs/source/pkg/iostreams/iostreams.go

- 配置与认证
  what: 读写 ~/.config/gh/hosts.yml，存储 GitHub token；
        gh auth login（含 OAuth Device Flow：打开浏览器 → 显示 device code → 轮询 token）；
        gh auth login --with-token（从 stdin 读 token，CI 场景）；
        gh auth status（显示已登录 hostname 和用户名）
  why:  所有 API 调用都需要 token；OAuth Device Flow 是 CLI 工具鉴权的标准模式，
        学完后对"CLI 如何安全登录"有完整认知
  src:  refs/source/internal/config/config.go
        refs/source/internal/config/stub.go
        refs/source/pkg/cmd/auth/login/login.go
        refs/source/pkg/cmd/auth/status/status.go
        refs/source/pkg/cmd/auth/shared/

- API 客户端
  what: 封装带认证的 HTTP client，提供 REST 和 GraphQL 两种调用方式
  why:  所有命令的底层通信层，统一错误处理和认证头注入
  src:  refs/source/api/client.go
        refs/source/api/http_client.go

- 仓库命令 (gh repo)
  what: gh repo view 查看仓库详情，gh repo list 列出用户仓库
  why:  最简单的读操作，验证 API 客户端和输出格式是否正确
  src:  refs/source/pkg/cmd/repo/view/view.go
        refs/source/pkg/cmd/repo/list/list.go
        refs/source/api/queries_repo.go

- Issue 命令 (gh issue)
  what: gh issue list / view / create
  why:  代表"读 + 写"两种操作模式，list/view 是 GraphQL 查询，create 是 mutation
  src:  refs/source/pkg/cmd/issue/list/list.go
        refs/source/pkg/cmd/issue/view/view.go
        refs/source/pkg/cmd/issue/create/create.go
        refs/source/api/queries_issue.go
        refs/source/pkg/cmd/pr/shared/

- Pull Request 命令 (gh pr)
  what: gh pr list / view / create
  why:  PR 是 GitHub 工作流核心，create 需要结合本地 git 状态（当前分支）
  src:  refs/source/pkg/cmd/pr/list/list.go
        refs/source/pkg/cmd/pr/view/view.go
        refs/source/pkg/cmd/pr/create/create.go
        refs/source/api/queries_pr.go
        refs/source/pkg/cmd/pr/shared/
        refs/source/git/client.go
```

### skip（跳过）

```
- codespace: 云开发环境，依赖复杂的 tunnel/SSH 基础设施
  reason: 平台适配器，超出核心学习范围

- extension: 插件系统，需要动态加载和 exec
  reason: 平台适配器

- attestation: Sigstore 签名验证，安全专项工具
  reason: 超出核心范围

- workflow/run/cache: GitHub Actions 专用命令
  reason: 超出核心范围，不影响主流程理解

- gist/release/project: 次要资源
  reason: 模式与 issue 相同，实现后无额外学习价值

- browse: 只是打开浏览器
  reason: 无 API 交互

- Windows/macOS 特定适配: 代码签名、MSI 打包
  reason: 平台兼容性补丁
```

---

## 3d. Phase 分解

```
Phase 1: CLI 框架与 IOStreams
  goal:       运行 `./gh --version` 输出版本号；`./gh --help` 显示帮助
  features:   CLI 框架
  depends-on: 无

Phase 2: 配置与认证
  goal:       运行 `./gh auth login` 完成完整 OAuth Device Flow（输出 device code，
              打开浏览器，轮询直到用户授权，token 写入配置文件）；
              `./gh auth login --with-token` 从 stdin 读 token 直接写入（CI 场景）；
              `./gh auth status` 显示已登录的 hostname 和用户名
  features:   配置与认证
  depends-on: Phase 1: IOStreams, Factory, NewCmdRoot

Phase 3: API 客户端
  goal:       运行 `./gh api /user` 输出当前 GitHub 用户的 JSON；
              支持 REST GET 和 GraphQL 查询
  features:   API 客户端
  depends-on: Phase 2: Config（提供 token）, Factory.HttpClient

Phase 4: 仓库命令
  goal:       运行 `./gh repo view cli/cli` 显示仓库描述和统计；
              `./gh repo list` 列出当前用户的仓库
  features:   仓库命令
  depends-on: Phase 3: api.Client.REST(), api.Client.GraphQL()

Phase 5: Issue 命令
  goal:       运行 `./gh issue list -R cli/cli` 列出 issue；
              `./gh issue view 1 -R cli/cli` 查看详情；
              `./gh issue create -R cli/cli --title "test"` 创建 issue
  features:   Issue 命令
  depends-on: Phase 4: Repository struct, RepoFromArgs()

Phase 6: Pull Request 命令
  goal:       运行 `./gh pr list -R cli/cli` 列出 PR；
              `./gh pr view 1 -R cli/cli` 查看详情；
              `./gh pr create` 基于当前分支创建 PR
  features:   Pull Request 命令
  depends-on: Phase 5: Issue 模式（Options struct, GraphQL query 模式）;
              Phase 1: GitClient（读取当前分支）
```

---

## 3e. 接口定义

### Phase 1 接口

```go
// pkg/iostreams/iostreams.go
type IOStreams struct {
    In     io.Reader
    Out    io.Writer
    ErrOut io.Writer
}
func (s *IOStreams) CanPrompt() bool
func (s *IOStreams) ColorEnabled() bool
func System() *IOStreams
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer)

// pkg/cmdutil/factory.go
type Factory struct {
    AppVersion     string
    ExecutableName string
    IOStreams       *iostreams.IOStreams
    Config         func() (Config, error)
    HttpClient     func() (*http.Client, error)
    GitClient      *git.Client
}

// internal/config/config.go
type Config interface {
    Get(hostname, key string) (string, error)
    Set(hostname, key, value string) error
    Write() error
    Hosts() []string
}

// pkg/cmd/root/root.go
func NewCmdRoot(f *cmdutil.Factory, version string) *cobra.Command

// internal/ghcmd/cmd.go
func Main() exitCode

// internal/build/build.go
var Version string
var Date string
```

### Phase 2 接口

```go
// pkg/cmd/auth/login
func NewCmdLogin(f *cmdutil.Factory) *cobra.Command
// 支持 --hostname, --with-token (从 stdin 读 token)

// pkg/cmd/auth/status
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command
// 输出每个 hostname 的登录状态和用户名

// internal/config/config.go 补充
func (c *fileConfig) AuthToken(hostname string) (string, error)
func (c *fileConfig) Login(hostname, username, token string) error
func (c *fileConfig) Logout(hostname string) error
```

### Phase 3 接口

```go
// api/client.go
type Client struct{ http *http.Client }
func NewClientFromHTTP(httpClient *http.Client) *Client
func (c *Client) REST(hostname, method, path string, body io.Reader, data interface{}) error
func (c *Client) GraphQL(hostname, query string, variables map[string]interface{}, data interface{}) error

// api/http_client.go
func NewHTTPClient(token string, appVersion string) *http.Client

// pkg/cmd/api/api.go
func NewCmdAPI(f *cmdutil.Factory) *cobra.Command
// 支持 gh api /path 和 gh api graphql

// HTTPError and GraphQLError types
type HTTPError struct {
    StatusCode int
    Message    string
}
func (e HTTPError) Error() string

type GraphQLError struct {
    Message string
    Errors  []GraphQLErrorItem
}
func (e GraphQLError) Error() string
```

### Phase 4 接口

```go
// api/queries_repo.go
type Repository struct {
    ID          string
    Name        string
    NameWithOwner string
    Owner       struct{ Login string }
    Description string
    IsPrivate   bool
    IsFork      bool
    StargazerCount int
    ForkCount   int
    DefaultBranchRef struct{ Name string }
    URL         string
}
func GetRepository(client *Client, repo ghrepo.Interface) (*Repository, error)
func ListRepositories(client *Client, login string, limit int) ([]Repository, error)

// internal/ghrepo/ghrepo.go
type Interface interface {
    RepoName() string
    RepoOwner() string
    RepoHost() string
}
func FromFullName(nwo string) (Interface, error)  // "owner/name" → Interface
func FullName(r Interface) string                 // Interface → "owner/name"

// pkg/cmd/repo/view
func NewCmdRepoView(f *cmdutil.Factory) *cobra.Command

// pkg/cmd/repo/list
func NewCmdRepoList(f *cmdutil.Factory) *cobra.Command
```

### Phase 5 接口

```go
// api/queries_issue.go
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
type Label struct{ Name string }
type User  struct{ Login string }

func ListIssues(client *Client, repo ghrepo.Interface, state string, limit int) ([]Issue, error)
func GetIssue(client *Client, repo ghrepo.Interface, number int) (*Issue, error)
func CreateIssue(client *Client, repo ghrepo.Interface, params map[string]interface{}) (*Issue, error)

// pkg/cmd/issue/list
func NewCmdIssueList(f *cmdutil.Factory) *cobra.Command

// pkg/cmd/issue/view
func NewCmdIssueView(f *cmdutil.Factory) *cobra.Command

// pkg/cmd/issue/create
func NewCmdIssueCreate(f *cmdutil.Factory) *cobra.Command
```

### Phase 6 接口

```go
// api/queries_pr.go
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

func ListPullRequests(client *Client, repo ghrepo.Interface, state string, limit int) ([]PullRequest, error)
func GetPullRequest(client *Client, repo ghrepo.Interface, number int) (*PullRequest, error)
func CreatePullRequest(client *Client, repo *Repository, params map[string]interface{}) (*PullRequest, error)

// git/client.go
type Client struct {
    RepoDir string
    GitPath string
    Stderr  io.Writer
}
func (c *Client) CurrentBranch(ctx context.Context) (string, error)
func (c *Client) Remotes(ctx context.Context) ([]Remote, error)

type Remote struct {
    Name     string
    FetchURL *url.URL
    PushURL  *url.URL
}

// pkg/cmd/pr/list
func NewCmdPRList(f *cmdutil.Factory) *cobra.Command

// pkg/cmd/pr/view
func NewCmdPRView(f *cmdutil.Factory) *cobra.Command

// pkg/cmd/pr/create
func NewCmdPRCreate(f *cmdutil.Factory) *cobra.Command
```

---

## 3f. 项目约定

**目录结构**:
```
gh-impl/
├── cmd/gh/main.go          ← 二进制入口
├── internal/
│   ├── build/              ← 版本信息（编译时注入）
│   ├── config/             ← 配置文件读写
│   └── ghrepo/             ← owner/repo 解析工具
├── api/                    ← GitHub API 客户端 + 数据模型
├── git/                    ← git 子进程封装
├── pkg/
│   ├── cmdutil/            ← Factory struct, 共享工具
│   ├── iostreams/          ← IO 封装
│   └── cmd/                ← 所有命令实现
│       ├── root/
│       ├── auth/
│       ├── api/
│       ├── repo/
│       ├── issue/
│       └── pr/
└── src/                    ← learnOpenSource 输出（按 phase 分）
```

**包命名**: 与目录名一致，小写单词，例如 `package iostreams`, `package cmdutil`

**错误处理**: 
- 返回 `error`，不使用 panic（除非是初始化时的不可恢复错误）
- HTTP 错误封装为自定义类型（`HTTPError`），包含 status code 和 message
- 命令层面捕获错误，写入 `IOStreams.ErrOut`，返回非零 exit code

**代码风格**（来自原项目）:
- 每个命令一个 `Options` struct，聚合所有 flag 和 factory 依赖
- 构造函数签名：`func NewCmdXxx(f *cmdutil.Factory) *cobra.Command`
- 执行逻辑放在独立函数 `func xxxRun(opts *XxxOptions) error`，方便测试
- 使用 `heredoc.Doc()` 编写多行帮助文本
- 输出用 `fmt.Fprintf(opts.IO.Out, ...)` 而非直接 `fmt.Print`

**Go module**: `github.com/yourname/gh-impl`（自定义，不使用原项目 module path）
