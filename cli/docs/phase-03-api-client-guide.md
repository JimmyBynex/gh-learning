# Phase 3 学习指南：API 客户端

---

## Section 1：全局视图

### 1.1 这个 Phase 做了什么

Phase 3 让 `gh api /user` 真正能运行并输出 GitHub 用户的 JSON。具体来说，它做了四件事：

1. **实现带认证的 HTTP 客户端**（`api/http_client.go`）：使用 **RoundTripper 链**将认证 token、User-Agent 和 API 版本头统一注入每个 HTTP 请求，而不依赖调用方手动设置请求头。

2. **实现 `Client` 封装器**（`api/client.go`）：在 `*http.Client` 之上提供两个高层方法——`REST()` 和 `GraphQL()`，封装了 URL 拼接、JSON 序列化/反序列化、HTTP 状态码检查和错误类型转换。

3. **实现 `gh api` 命令**（`pkg/cmd/api/api.go`）：提供统一的命令入口，根据 endpoint 参数自动判断走 REST 还是 GraphQL 路径，并将结果美化输出（indent JSON）。

4. **修改 `factory.go`**：将 `HttpClient` 从 Phase 2 的占位 stub 升级为真正使用认证 token 的实现，采用懒加载闭包模式，在 `HttpClient()` 被调用时才读取 Config 中的 token。

Phase 2 让 CLI 能"认识用户"（保存和读取 token），Phase 3 让 CLI 能"代表用户说话"（用 token 向 GitHub API 发请求）——这是所有后续命令（repo、issue、pr）的通信基础。

---

### 1.2 模块图

```
                    ┌─────────────────────────────────────┐
                    │         Phase 3 新增 / 修改           │
                    └─────────────────────────────────────┘

   pkg/cmd/api/api.go          ← ★ 新增：gh api 命令
         │
         │ 使用
         ▼
      api/client.go            ← ★ 新增：Client, REST(), GraphQL()
         │
         │ 持有
         ▼
      api/http_client.go       ← ★ 新增：NewHTTPClient(), RoundTripper 链
         │
         │ 包装
         ▼
   net/http.DefaultTransport   ← 标准库，实际发 TCP 请求

   internal/factory/factory.go ← ★ 修改：HttpClient 改用真实认证 token
         │
         │ 调用
         ├── api.NewHTTPClient()
         └── cmdutil.Config.AuthToken()

   pkg/cmd/root/root.go        ← ★ 修改：注册 api 命令
         │
         └── apiCmd.NewCmdAPI(f)
```

标记 ★ 的部分是 Phase 3 新增或修改的内容。`api` 包是全新的包，承载 HTTP 客户端和 API 调用逻辑；`pkg/cmd/api` 是命令实现包，与 `api` 包解耦——命令层只持有 `*http.Client`，真正的 API 调用逻辑在 `api` 包中。

---

### 1.3 控制流图

**`gh api /user`（REST 路径）**

```
gh api /user
     │
     ▼
NewCmdAPI(f) → cobra.Command
     │
     │  args[0] = "/user"
     │  IsGraphQL = (strings.ToLower("/user") == "graphql") = false
     ▼
apiRun(opts)
     │
     ├── opts.HttpClient()            ← 调用 factory 闭包
     │       │
     │       ├── f.Config()           ← 读 hosts.yml
     │       └── cfg.AuthToken("github.com") → token
     │               │
     │               └── api.NewHTTPClient(token, appVersion)
     │                       │
     │                       └── 组装 RoundTripper 链（见数据流图）
     │
     ├── api.NewClientFromHTTP(httpClient)
     │
     └── runREST(opts, client)
             │
             ▼
         client.REST("github.com", "GET", "/user", nil, &data)
             │
             ├── url = "https://api.github.com/user"
             ├── http.NewRequest("GET", url, nil)
             ├── req.Header.Set("Accept", "application/vnd.github+json")
             │
             └── c.http.Do(req)
                     │
                     │  请求经过 RoundTripper 链（从外到内）：
                     ├── authTransport.RoundTrip()     → 注入 Authorization
                     ├── userAgentTransport.RoundTrip() → 注入 User-Agent
                     ├── apiVersionTransport.RoundTrip() → 注入 X-GitHub-Api-Version
                     └── http.DefaultTransport.RoundTrip() → 实际发 TCP 请求
                             │
                             ▼
                     GitHub API 响应 { "login": "alice", ... }
                             │
                             ▼
                     json.NewDecoder(resp.Body).Decode(&data)
                             │
                             ▼
                     printJSON(opts.IO.Out, data)
                             │
                             ▼
                     stdout: 格式化 JSON 输出
```

**`gh api graphql -f query='{ viewer { login } }'`（GraphQL 路径）**

```
gh api graphql -f query='{ viewer { login } }'
     │
     │  args[0] = "graphql"
     │  IsGraphQL = true
     │  opts.Fields = {"query": "{ viewer { login } }"}
     ▼
apiRun(opts) → runGraphQL(opts, client)
     │
     ├── query = opts.Fields["query"]
     ├── variables = {} (除 "query" 外的 fields)
     │
     └── client.GraphQL("github.com", query, variables, &data)
             │
             ├── payload = { "query": "{ viewer { login } }" }
             ├── json.Marshal(payload) → []byte
             ├── url = "https://api.github.com/graphql"
             ├── http.NewRequest("POST", url, bytes.NewReader(b))
             │
             └── 同样经过 RoundTripper 链 → GitHub GraphQL API
                     │
                     ▼
             响应: { "data": { "viewer": { "login": "alice" } } }
                     │
                     ├── 检查 gqlResp.Errors (若有 → GraphQLError)
                     └── json.Unmarshal(gqlResp.Data, &data)
```

---

### 1.4 数据流图：HTTP 请求的 RoundTripper 链

理解 RoundTripper 链是 Phase 3 最核心的概念。`http.Client` 的 `Transport` 字段接受任何实现了 `http.RoundTripper` 接口的对象：

```go
type RoundTripper interface {
    RoundTrip(*Request) (*Response, error)
}
```

通过嵌套（洋葱结构），每一层都可以在传递给内层之前修改请求：

```
调用方 c.http.Do(req)
         │
         ▼
 ┌─────────────────────────────────────────────────────────────┐
 │  authTransport（最外层）                                      │
 │  if Authorization == "" → req.Clone() → 注入 Bearer token   │
 │                                                              │
 │  ┌───────────────────────────────────────────────────────┐  │
 │  │  userAgentTransport                                    │  │
 │  │  if User-Agent == "" → req.Clone() → 注入 User-Agent  │  │
 │  │                                                        │  │
 │  │  ┌─────────────────────────────────────────────────┐  │  │
 │  │  │  apiVersionTransport（最内层）                   │  │  │
 │  │  │  if X-GitHub-Api-Version == "" →               │  │  │
 │  │  │      req.Clone() → 注入版本头                   │  │  │
 │  │  │                                                 │  │  │
 │  │  │  ┌───────────────────────────────────────────┐  │  │  │
 │  │  │  │  http.DefaultTransport                    │  │  │  │
 │  │  │  │  实际发 TCP 连接、TLS 握手、HTTP 请求      │  │  │  │
 │  │  │  └───────────────────────────────────────────┘  │  │  │
 │  │  └─────────────────────────────────────────────────┘  │  │
 │  └───────────────────────────────────────────────────────┘  │
 └─────────────────────────────────────────────────────────────┘
         │
         ▼
      *http.Response
```

**组装顺序**（`NewHTTPClient` 中）：

```go
tr := http.RoundTripper(http.DefaultTransport)  // 最内层：真正发请求
tr = &apiVersionTransport{inner: tr}             // 第3层：版本头
tr = &userAgentTransport{agent: ..., inner: tr}  // 第2层：User-Agent
if token != "" {
    tr = &authTransport{token: token, inner: tr} // 第1层：认证（最外）
}
```

注意：`inner` 指向内层，所以**最后赋值的是最外层**（最先被调用）。这个顺序很重要：`authTransport` 作为最外层，能看到调用方原始 Request，判断是否已有 `Authorization` 头——如果调用方（比如 authflow）已经自己设置了 token，`authTransport` 会跳过注入，避免覆盖。

---

### 1.5 与前两个 Phase 的连接

**Phase 1 提供的基础设施**

| 组件 | Phase 1 中的状态 | Phase 3 如何使用 |
|------|-----------------|-----------------|
| `IOStreams` | 完整实现 | `apiRun` 通过 `opts.IO.Out` 输出 JSON |
| `Factory.HttpClient` | 返回 `nil, nil` 的占位 stub | Phase 3 重写为真正创建认证客户端 |
| `cobra.Command` 框架 | 完整实现 | `NewCmdAPI` 遵循相同的 `NewCmdXxx(f *Factory)` 模式 |
| `root.NewCmdRoot()` | 已注册 auth 命令 | Phase 3 追加注册 api 命令 |

**Phase 2 提供的认证能力**

Phase 2 实现了 `cfg.AuthToken(hostname)` 方法，Phase 3 的 `factory.go` 正是通过这个方法取得 token，再传给 `api.NewHTTPClient(token, appVersion)` 创建带认证的客户端。如果 Phase 2 没有实现 token 的存储和读取，Phase 3 只能创建匿名客户端（无法访问需要认证的 API）。

---

## Section 2：Implementation Walkthrough

### 2.1 创建 api 包骨架

**为什么需要独立的 `api` 包？**

HTTP 客户端逻辑（RoundTripper 链、请求头注入）和 API 调用逻辑（REST/GraphQL 封装）是**横切关注点**——未来的 `repo`、`issue`、`pr` 命令都会复用它们。将这些逻辑放在独立的 `api` 包中，而不是嵌入某个命令，符合"高内聚、低耦合"原则。

`api` 包与 `pkg/cmd/api`（命令包）是**两个不同的包**：
- `api`（`github.com/learngh/gh-impl/api`）：纯粹的 HTTP/API 逻辑，不依赖 cobra 或 cmdutil
- `pkg/cmd/api`（`github.com/learngh/gh-impl/pkg/cmd/api`）：命令实现，依赖 cobra 和 Factory

这样的分层确保 API 客户端可以被任何命令使用，而不产生循环依赖。

【现在手敲】

首先创建目录结构：

```bash
# 在 phase-03-api-client 根目录下
mkdir -p api
mkdir -p pkg/cmd/api
```

确认 `go.mod` 中的 module 名称（phase-03 复用同一个 go.mod，不需要额外创建）：

```
module github.com/learngh/gh-impl

go 1.21

require (
    github.com/spf13/cobra v1.8.0
    golang.org/x/term v0.17.0
    gopkg.in/yaml.v3 v3.0.1
)
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
ls api/ pkg/cmd/api/
```

期望输出：`api/` 和 `pkg/cmd/api/` 目录存在。

---

### 2.2 实现 authTransport

**为什么要 clone request？**

HTTP `RoundTripper` 的合约（contract）规定：**不能修改传入的 `*http.Request`**。`http.Client` 内部可能在多次重定向时复用同一个 Request 对象，如果 Transport 直接修改传入的 Request，会造成竞态条件（race condition）。`req.Clone(req.Context())` 创建一个浅拷贝（Header map 被深拷贝），安全地修改副本而不影响原始对象。

**非干扰设计：检查 Authorization 是否已设置**

`authTransport` 在注入 token 之前先检查 `req.Header.Get("Authorization") == ""`。这个设计解决了一个实际问题：`authflow`（Phase 2 的认证流程）在轮询 token 时，会向 `github.com/login/oauth/access_token` 发请求，这个请求可能携带不同的 Authorization 凭证（client_id/client_secret）。如果 `authTransport` 无条件覆盖，会破坏 authflow 的请求。通过这个检查，authTransport 实现了"有则不覆盖"的非干扰语义。

【现在手敲】

```go
// 文件：api/http_client.go
package api

import (
	"fmt"
	"net/http"
)

const apiVersionValue = "2022-11-28"

type authTransport struct {
	token string
	inner http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" && req.Header.Get("Authorization") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.inner.RoundTrip(req)
}
```

**关键点解释**

- `req = req.Clone(req.Context())`：注意是赋值给局部变量 `req`，覆盖了参数 `req`，但原始调用方持有的 Request 指针不受影响。
- `"Bearer " + t.token`：GitHub API 使用 Bearer token 认证方案（RFC 6750），与 Basic 认证（`"Basic " + base64(...)`）区分。
- 两个条件缺一不可：`t.token != ""`（没有 token 就不注入）AND `req.Header.Get("Authorization") == ""`（已有 Authorization 就不覆盖）。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go vet ./api/...
```

期望输出：无输出（exit code 0）。

---

### 2.3 实现 userAgentTransport

**为什么需要 User-Agent？**

GitHub API 要求客户端提供 `User-Agent` 头，用于流量识别和限速策略。官方 `gh` CLI 使用 `"GitHub CLI <version>"` 格式，以便 GitHub 服务端区分不同版本的客户端行为。同样，只在未设置时注入，避免覆盖调用方显式设置的自定义 User-Agent。

【现在手敲】

将以下代码追加到 `api/http_client.go`：

```go
type userAgentTransport struct {
	agent string
	inner http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.agent)
	}
	return t.inner.RoundTrip(req)
}
```

**关键点解释**

`agent` 字段在 `NewHTTPClient` 中被设置为 `fmt.Sprintf("GitHub CLI %s", appVersion)`，格式化后的字符串例如 `"GitHub CLI 2.0.0"`。字符串在构造时就格式化完成，`RoundTrip` 调用时不再重复格式化，是轻量的性能优化。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go build ./api/...
```

期望输出：无输出（exit code 0）。

---

### 2.4 实现 apiVersionTransport

**为什么需要 X-GitHub-Api-Version？**

GitHub REST API 从 2022 年起支持通过 `X-GitHub-Api-Version` 头来锁定 API 版本。这解决了 API 版本演进的向后兼容问题：即使 GitHub 推出新版本 API，指定旧版本的客户端行为不会改变。`2022-11-28` 是当前稳定的基准版本，`gh` 官方 CLI 使用这个版本。

【现在手敲】

将以下代码追加到 `api/http_client.go`：

```go
type apiVersionTransport struct {
	inner http.RoundTripper
}

func (t *apiVersionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("X-GitHub-Api-Version") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("X-GitHub-Api-Version", apiVersionValue)
	}
	return t.inner.RoundTrip(req)
}
```

**关键点解释**

`apiVersionTransport` 没有存储版本字符串的字段，而是直接引用包级常量 `apiVersionValue`。这是因为 API 版本在整个程序生命周期内是不变的（不像 token 或 User-Agent 随实例不同而变化），用常量更清晰地表达"这是一个固定值"的语义。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go vet ./api/...
```

期望输出：无输出（exit code 0）。

---

### 2.5 实现 NewHTTPClient

**RoundTripper 链的组装顺序和原因**

这是 Phase 3 最关键的设计决策。先看代码，再解释为什么：

【现在手敲】

将以下代码追加到 `api/http_client.go`：

```go
// NewHTTPClient returns an *http.Client pre-configured with:
//   - Authorization: Bearer <token>  (if token non-empty)
//   - User-Agent: GitHub CLI <appVersion>
//   - X-GitHub-Api-Version: 2022-11-28
func NewHTTPClient(token, appVersion string) *http.Client {
	tr := http.RoundTripper(http.DefaultTransport)
	tr = &apiVersionTransport{inner: tr}
	tr = &userAgentTransport{agent: fmt.Sprintf("GitHub CLI %s", appVersion), inner: tr}
	if token != "" {
		tr = &authTransport{token: token, inner: tr}
	}
	return &http.Client{Transport: tr}
}
```

**关键点解释：链的顺序为什么重要**

组装时，每个新的 transport 包裹前一个（`inner: tr`），形成洋葱结构：

```
组装顺序（赋值顺序）：
  1. tr = DefaultTransport        （最内层，实际发请求）
  2. tr = apiVersionTransport     （现在 tr 是第2层）
  3. tr = userAgentTransport      （现在 tr 是第3层）
  4. tr = authTransport           （现在 tr 是最外层）

调用顺序（RoundTrip 调用链）：
  authTransport → userAgentTransport → apiVersionTransport → DefaultTransport
```

**为什么 `authTransport` 必须是最外层？**

`authTransport` 需要检查传入 Request 上是否**已经有** `Authorization` 头。如果把它放在内层，则调用它时，外层 transport 可能已经修改了 Header（比如如果有一个假设的"加密 transport"在外层）。放在最外层，它能看到调用方 `c.http.Do(req)` 传入的原始 Request，从而正确判断是否需要注入 token。

**为什么 token 为空时不添加 `authTransport`？**

减少链的长度，避免每次请求都执行无意义的条件检查。更重要的是语义清晰：匿名请求就是匿名请求，链里不应该有一个永远不注入任何东西的 authTransport。

完整的 `api/http_client.go` 最终内容：

```go
package api

import (
	"fmt"
	"net/http"
)

const apiVersionValue = "2022-11-28"

type authTransport struct {
	token string
	inner http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" && req.Header.Get("Authorization") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.inner.RoundTrip(req)
}

type userAgentTransport struct {
	agent string
	inner http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.agent)
	}
	return t.inner.RoundTrip(req)
}

type apiVersionTransport struct {
	inner http.RoundTripper
}

func (t *apiVersionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("X-GitHub-Api-Version") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("X-GitHub-Api-Version", apiVersionValue)
	}
	return t.inner.RoundTrip(req)
}

// NewHTTPClient returns an *http.Client pre-configured with:
//   - Authorization: Bearer <token>  (if token non-empty)
//   - User-Agent: GitHub CLI <appVersion>
//   - X-GitHub-Api-Version: 2022-11-28
func NewHTTPClient(token, appVersion string) *http.Client {
	tr := http.RoundTripper(http.DefaultTransport)
	tr = &apiVersionTransport{inner: tr}
	tr = &userAgentTransport{agent: fmt.Sprintf("GitHub CLI %s", appVersion), inner: tr}
	if token != "" {
		tr = &authTransport{token: token, inner: tr}
	}
	return &http.Client{Transport: tr}
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go build ./api/...
```

期望输出：无输出（exit code 0）。

---

### 2.6 实现 Client 和 REST 方法

**为什么用 `Client` 包装 `*http.Client`？**

直接使用 `*http.Client` 调用 GitHub API 需要每次手动：拼接 URL、设置 Accept 头、检查状态码、解析错误体、反序列化 JSON。这些逻辑在每个命令里重复写会导致大量冗余。`Client.REST()` 将这一完整流程封装为一次调用，同时通过 `data interface{}` 参数接受任意目标类型，保持了足够的灵活性。

**`apiBaseURL` 的作用**

GitHub.com 使用 `https://api.github.com`，GitHub Enterprise 使用 `https://<hostname>/api/v3`。`apiBaseURL` 集中处理这个差异，所有 REST 和 GraphQL 调用都通过它构造 URL，保证行为一致。

【现在手敲】

创建 `api/client.go`：

```go
// 文件：api/client.go
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client wraps an *http.Client to provide REST and GraphQL helpers for GitHub APIs.
type Client struct {
	http *http.Client
}

// NewClientFromHTTP creates an API Client from an existing *http.Client.
func NewClientFromHTTP(httpClient *http.Client) *Client {
	return &Client{http: httpClient}
}

// GraphQLErrorItem is a single error item in a GraphQL response.
type GraphQLErrorItem struct {
	Message   string
	Locations []struct{ Line, Column int }
	Path      []string
}

// GraphQLError is returned when the GraphQL response contains errors.
type GraphQLError struct {
	Message string
	Errors  []GraphQLErrorItem
}

func (e GraphQLError) Error() string {
	return e.Message
}

// HTTPError is returned when the API responds with a non-2xx status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// apiBaseURL returns the base API URL for a hostname.
func apiBaseURL(hostname string) string {
	if hostname == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + hostname + "/api/v3"
}

// REST performs a REST API request and JSON-decodes the response into data.
// path may include or omit a leading slash.
func (c *Client) REST(hostname, method, path string, body io.Reader, data interface{}) error {
	url := apiBaseURL(hostname) + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseHTTPError(resp)
	}

	if data != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(data)
	}
	return nil
}

// parseHTTPError reads the response body and constructs an HTTPError.
func parseHTTPError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var apiMsg struct {
		Message string `json:"message"`
	}
	if len(b) > 0 {
		_ = json.Unmarshal(b, &apiMsg)
	}
	return HTTPError{StatusCode: resp.StatusCode, Message: apiMsg.Message}
}
```

**关键点解释**

- `strings.TrimPrefix(path, "/")` 后再加 `"/"` 前缀：无论调用方传入 `"/user"` 还是 `"user"`，结果都是 `"https://api.github.com/user"`，防止双斜杠或缺斜杠的 URL。
- `resp.StatusCode != http.StatusNoContent`：HTTP 204 No Content 表示成功但没有响应体，不应该尝试 JSON decode（否则 `json.Decoder` 会返回 `io.EOF` 错误）。
- `parseHTTPError` 中 `json.Unmarshal` 错误被忽略（`_ =`）：API 响应体可能不是 JSON（如 502 网关错误返回 HTML），这时 `apiMsg.Message` 保持空字符串，`HTTPError.Error()` 只输出状态码，不崩溃。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go vet ./api/...
```

期望输出：无输出（exit code 0）。

---

### 2.7 实现 GraphQL 方法

**REST vs GraphQL 的实现差异**

| 维度 | REST | GraphQL |
|------|------|---------|
| URL | `apiBaseURL + "/" + path` | `apiBaseURL + "/graphql"` |
| HTTP 方法 | 由调用方指定（GET/POST/PATCH/DELETE） | 固定 POST |
| 请求体 | 可选（GET 没有 body） | 必须（JSON payload 含 query 和 variables） |
| 错误判断 | HTTP 状态码不在 2xx 范围 | HTTP 状态码 + 响应体中的 `errors` 字段 |
| 响应结构 | 直接 JSON 对象 | `{ "data": {...}, "errors": [...] }` 包装层 |

GraphQL 的错误处理更复杂：即使 HTTP 状态码是 200，响应体中也可能有 `errors` 字段表示查询失败（如字段不存在、权限不足）。所以 GraphQL 方法需要先检查 HTTP 状态码，再检查 `errors` 字段。

【现在手敲】

将以下代码追加到 `api/client.go`：

```go
// GraphQL performs a GraphQL query against the API and decodes the response data into data.
func (c *Client) GraphQL(hostname, query string, variables map[string]interface{}, data interface{}) error {
	payload := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables,omitempty"`
	}{Query: query, Variables: variables}

	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := apiBaseURL(hostname) + "/graphql"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseHTTPError(resp)
	}

	var gqlResp struct {
		Data   json.RawMessage    `json:"data"`
		Errors []GraphQLErrorItem `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return err
	}

	if len(gqlResp.Errors) > 0 {
		return GraphQLError{
			Message: gqlResp.Errors[0].Message,
			Errors:  gqlResp.Errors,
		}
	}

	if data != nil && gqlResp.Data != nil {
		return json.Unmarshal(gqlResp.Data, data)
	}
	return nil
}
```

**关键点解释**

- `json.RawMessage` 类型：先把 `data` 字段捕获为原始 JSON 字节（不解析），等确认没有 errors 后再把 `gqlResp.Data` 解析到调用方指定的类型 `data`。这避免了需要知道 `data` 的具体类型才能解析整个响应体的问题。
- `variables,omitempty`：如果 variables 为 nil 或空 map，序列化时省略 `variables` 字段，减少请求体大小，符合 GraphQL 规范。
- `GraphQLError.Message` 取第一个 error 的 message：GraphQL 可能返回多个 error，但 `Error()` 接口只返回一个字符串。保存完整的 `Errors []GraphQLErrorItem` 方便调用方自行遍历所有错误。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go build ./api/...
```

期望输出：无输出（exit code 0）。

---

### 2.8 实现 HTTPError 和 GraphQLError（已在 2.6/2.7 完成）

`HTTPError` 和 `GraphQLError` 已在 2.6 节（`client.go` 的前半部分）实现。这里单独梳理它们的设计决策。

**为什么使用自定义错误类型而不是 `fmt.Errorf`？**

使用 `fmt.Errorf("HTTP 401: requires authentication")` 会把错误信息固化为字符串，调用方无法提取 status code 做程序化判断（例如：401 → 提示用户登录，404 → 提示资源不存在）。自定义 `HTTPError` struct 让调用方可以通过类型断言提取结构化信息：

```go
if httpErr, ok := err.(HTTPError); ok {
    if httpErr.StatusCode == 401 {
        // 提示用户运行 gh auth login
    }
}
```

**Go 错误接口**：任何实现了 `Error() string` 方法的类型都满足 `error` 接口。`HTTPError` 和 `GraphQLError` 都实现了 `Error()`，所以可以直接作为 `error` 返回，调用方可以用标准的 `if err != nil` 模式处理，也可以用类型断言获取详细信息——两者兼得。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go test ./api/... -run TestHTTPError -v
go test ./api/... -run TestGraphQLError -v
```

期望输出（示例）：

```
=== RUN   TestHTTPError_Error
--- PASS: TestHTTPError_Error (0.00s)
PASS
=== RUN   TestGraphQLError_Error
--- PASS: TestGraphQLError_Error (0.00s)
PASS
```

---

### 2.9 实现 NewCmdAPI

**命令的职责边界**

`NewCmdAPI` 遵循与其他命令相同的 `NewCmdXxx(f *Factory)` 模式：
1. 创建 `APIOptions` struct，持有所有依赖和输入
2. 从 `Factory` 注入依赖（IO、Config、HttpClient）
3. 定义 cobra.Command，在 `RunE` 中填充 opts 并调用 `apiRun`

命令层（`NewCmdAPI`）不做任何业务逻辑，只负责参数解析和依赖注入。实际的 HTTP 调用在 `apiRun`、`runREST`、`runGraphQL` 中完成。

**IsGraphQL 的判断逻辑**

当用户运行 `gh api graphql`，`args[0]` 就是字符串 `"graphql"`。通过 `strings.ToLower(args[0]) == "graphql"` 判断，避免大小写问题（`gh api GraphQL` 也能工作）。这比增加一个 `--graphql` flag 更简洁，同时保持了 URL path 和 "graphql" 的一致性。

【现在手敲】

创建 `pkg/cmd/api/api.go`：

```go
// 文件：pkg/cmd/api/api.go
// Package api provides the `gh api` command.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	apiPkg "github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

// APIOptions holds dependencies and inputs for `gh api`.
type APIOptions struct {
	IO          *iostreams.IOStreams
	Config      func() (cmdutil.Config, error)
	HttpClient  func() (*http.Client, error)
	Hostname    string
	Method      string
	RequestPath string
	IsGraphQL   bool
	Fields      map[string]string // -f key=value pairs
}

// NewCmdAPI returns the `gh api` cobra command.
func NewCmdAPI(f *cmdutil.Factory) *cobra.Command {
	opts := &APIOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Hostname:   "github.com",
		Method:     "GET",
		Fields:     map[string]string{},
	}

	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated GitHub API request",
		Long: `Make an authenticated GitHub API request.

For REST endpoints, supply a path like /user or repos/owner/repo.
For GraphQL, use the "graphql" endpoint with -f query=<gql>.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RequestPath = args[0]
			opts.IsGraphQL = strings.ToLower(args[0]) == "graphql"
			return apiRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Method, "method", "X", "GET", "The HTTP method for the request")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "github.com", "The GitHub hostname for the request")

	// -f key=value flags for GraphQL variables / request fields.
	cmd.Flags().StringToStringVarP(&opts.Fields, "field", "f", map[string]string{}, "Add a key=value field to the request")

	return cmd
}
```

**关键点解释**

- `cobra.ExactArgs(1)`：强制要求恰好一个位置参数（endpoint）。少了或多了都会报错并显示帮助文本。
- `StringToStringVarP`：cobra 内置的 `map[string]string` flag 类型，支持 `-f key=value` 多次传入，自动解析为 map。使用短标志 `-f` 与 GitHub CLI 原版保持一致。
- 方法默认值为 `"GET"`：对 REST API 来说 GET 是最常见的操作，符合最少惊喜原则（Principle of Least Surprise）。GraphQL 总是 POST，这个默认值对 GraphQL 路径没有影响（`runGraphQL` 内部固定使用 POST）。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go build ./pkg/cmd/api/...
```

期望输出：无输出（exit code 0）。

---

### 2.10 实现 apiRun / runREST / runGraphQL

**三层调用的分工**

- `apiRun`：入口分发器，获取 httpClient，创建 `api.Client`，判断走哪条路径
- `runREST`：调用 `client.REST()`，打印 JSON
- `runGraphQL`：验证 query 字段存在，组装 variables map，调用 `client.GraphQL()`，打印 JSON

分三个函数而不是在 `RunE` 里写一个大函数，原因：每个函数有明确的单一职责，测试时可以跳过 cobra 命令解析直接测试 `apiRun(opts)`。

【现在手敲】

将以下代码追加到 `pkg/cmd/api/api.go`：

```go
// apiRun executes the API request.
func apiRun(opts *APIOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := apiPkg.NewClientFromHTTP(httpClient)

	if opts.IsGraphQL {
		return runGraphQL(opts, client)
	}
	return runREST(opts, client)
}

func runREST(opts *APIOptions, client *apiPkg.Client) error {
	var data interface{}
	if err := client.REST(opts.Hostname, opts.Method, opts.RequestPath, nil, &data); err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	return printJSON(opts.IO.Out, data)
}

func runGraphQL(opts *APIOptions, client *apiPkg.Client) error {
	query, ok := opts.Fields["query"]
	if !ok || query == "" {
		return fmt.Errorf("graphql requires -f query=<gql query>")
	}

	// Remaining fields become variables.
	variables := map[string]interface{}{}
	for k, v := range opts.Fields {
		if k != "query" {
			variables[k] = v
		}
	}
	if len(variables) == 0 {
		variables = nil
	}

	var data interface{}
	if err := client.GraphQL(opts.Hostname, query, variables, &data); err != nil {
		return fmt.Errorf("graphql call failed: %w", err)
	}
	return printJSON(opts.IO.Out, data)
}

func printJSON(w io.Writer, data interface{}) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(b))
	return nil
}
```

**关键点解释**

- `fmt.Errorf("api call failed: %w", err)`：`%w` 动词将原始 error 包装（wrap）进新的 error 中。调用方可以通过 `errors.As(err, &httpErr)` 提取原始的 `HTTPError`，同时错误信息也包含了上下文（"api call failed:"）。
- `runGraphQL` 中 `variables = nil` 而不是空 map：如果没有额外的 fields，把 variables 设为 nil，这样在 `GraphQL()` 方法的 `payload` 序列化时，`variables,omitempty` 会省略该字段，减少请求体冗余。
- `printJSON` 使用 `json.MarshalIndent`：`"  "`（两个空格）缩进的格式化 JSON 对人类友好，便于在终端阅读。使用 `opts.IO.Out` 而不是直接 `os.Stdout`，保持了可测试性。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go build ./pkg/cmd/api/...
```

期望输出：无输出（exit code 0）。

---

### 2.11 修改 factory.go

**懒加载（Lazy Initialization）模式**

Phase 2 的 `factory.go` 中 `HttpClient` 是一个返回 stub 的闭包（或者直接返回空客户端）。Phase 3 需要把它升级为真正读取认证 token 的闭包。

关键设计：`f.HttpClient` 被赋值为**闭包**（`func() (*http.Client, error)`），而不是直接调用 `api.NewHTTPClient(...)`。原因：

1. **延迟执行**：`factory.New()` 在程序启动时调用，此时配置文件可能还没有被读取（auth 命令会替换 Config 实现）。闭包捕获了 `f`（指针），在 `HttpClient()` 真正被调用时，`f.Config` 已经是最终版本。
2. **每次调用都读取最新 token**：如果用户在同一次进程中先登录再调用 API，闭包能拿到最新写入的 token（不过 CLI 工具通常一次只执行一个命令，这个场景较少出现）。
3. **错误处理**：如果 Config 读取失败，可以优雅降级（使用空 token 的匿名客户端），不让 Config 错误阻止所有 HTTP 请求。

【现在手敲】

修改 `internal/factory/factory.go`，将 `HttpClient` 字段从占位 stub 改为真实实现：

```go
// 文件：internal/factory/factory.go
package factory

import (
	"errors"
	"net/http"

	apiPkg "github.com/learngh/gh-impl/api"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

// stubConfig is a minimal Config implementation used as a placeholder until
// the real config is injected (e.g. by auth commands).
type stubConfig struct{}

func (c *stubConfig) Get(_, _ string) (string, error) {
	return "", errors.New("config not implemented")
}

func (c *stubConfig) Set(_, _, _ string) error {
	return errors.New("config not implemented")
}

func (c *stubConfig) Write() error {
	return errors.New("config not implemented")
}

func (c *stubConfig) Hosts() []string {
	return nil
}

func (c *stubConfig) AuthToken(_ string) (string, error) {
	return "", errors.New("config not implemented")
}

func (c *stubConfig) Login(_, _, _ string) error {
	return errors.New("config not implemented")
}

func (c *stubConfig) Logout(_ string) error {
	return errors.New("config not implemented")
}

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
		// Best-effort: get token for github.com.
		// Auth commands inject their own Authorization header so they are unaffected.
		token, _ := cfg.AuthToken("github.com")
		return apiPkg.NewHTTPClient(token, f.AppVersion), nil
	}

	f.GitClient = nil
	return f
}
```

**关键点解释**

- `cfg.AuthToken("github.com")` 的错误被丢弃（`token, _`）：如果用户未登录，`AuthToken` 返回 `("", error)`，这时 `token` 是空字符串，`NewHTTPClient("", ...)` 返回无认证的客户端。这是期望行为——匿名请求会被 GitHub API 拒绝（401），错误信息会正确传达给用户。
- `f.Config` 而不是在 `New()` 里直接调用：闭包捕获的是 `f`（Factory 指针），`f.Config` 在闭包被调用时才解引用。这允许 `NewCmdAuth` 替换 `f.Config` 为真实的 config 实现，`HttpClient()` 闭包自动使用最新版本的 Config。
- `f.HttpClient` 在 `f.Config` 赋值**之后**再赋值：这确保闭包捕获的 `f.Config` 是正确的字段（尽管 Go 的闭包捕获的是变量本身而不是值，这里其实是捕获 `f`，通过 `f.Config` 间接访问）。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go build ./internal/factory/...
```

期望输出：无输出（exit code 0）。

---

### 2.12 修改 root.go

**注册 api 命令**

`root.go` 的改动只有两处：增加 import 和调用 `cmd.AddCommand`。这体现了良好的架构设计——命令注册与命令实现完全分离，每个 Phase 只需要在 `root.go` 中追加一行 `AddCommand`，不影响其他命令。

【现在手敲】

修改 `pkg/cmd/root/root.go`，在 import 中添加 api 命令包，并注册命令：

```go
// 文件：pkg/cmd/root/root.go
package root

import (
	"fmt"
	"io"

	"github.com/learngh/gh-impl/pkg/cmd/auth"
	apiCmd "github.com/learngh/gh-impl/pkg/cmd/api"
	"github.com/learngh/gh-impl/pkg/cmd/version"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// ... （保持 AuthError、RootOptions 不变）

// NewCmdRoot builds and returns the root cobra command.
func NewCmdRoot(f *cmdutil.Factory, ver string) (*cobra.Command, error) {
	// ... （保持 cobra.Command 定义不变）

	// Hidden "version" sub-command for `gh version`.
	cmd.AddCommand(version.NewCmdVersion(f, ver))

	// Auth command group.
	cmd.AddCommand(auth.NewCmdAuth(f))

	// API command.
	cmd.AddCommand(apiCmd.NewCmdAPI(f))

	return cmd, nil
}
```

**关键点解释**

- `apiCmd "github.com/learngh/gh-impl/pkg/cmd/api"`：import alias 是必要的，因为当前包内已经有局部变量 `cmd`（cobra.Command 指针），如果 import 的包名也叫 `api` 还好（不冲突），但为了清晰区分 `api` 包（HTTP 客户端）和 `pkg/cmd/api` 包（命令），使用 `apiCmd` 作为 alias。

完整的 `root.go` 内容（展示关键修改点）：

```go
package root

import (
	"fmt"
	"io"

	"github.com/learngh/gh-impl/pkg/cmd/auth"
	apiCmd "github.com/learngh/gh-impl/pkg/cmd/api"
	"github.com/learngh/gh-impl/pkg/cmd/version"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type AuthError struct {
	err error
}

func (ae *AuthError) Error() string {
	return ae.err.Error()
}

func NewAuthError(err error) *AuthError {
	return &AuthError{err: err}
}

type RootOptions struct {
	Out         io.Writer
	VersionInfo string
	ShowVersion func() bool
	ShowHelp    func() error
}

func NewCmdRoot(f *cmdutil.Factory, ver string) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "gh <command> <subcommand> [flags]",
		Short: "GitHub CLI",
		Long:  "GitHub CLI\n\nWork seamlessly with GitHub from the command line.",
		Annotations: map[string]string{
			"versionInfo": version.Format(ver),
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.PersistentFlags().Bool("help", false, "Show help for command")
	cmd.Flags().Bool("version", false, "Show gh version")

	opts := &RootOptions{
		Out:         f.IOStreams.Out,
		VersionInfo: version.Format(ver),
		ShowVersion: func() bool {
			v, _ := cmd.Flags().GetBool("version")
			return v
		},
		ShowHelp: cmd.Help,
	}

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return rootRun(opts)
	}

	cmd.AddCommand(version.NewCmdVersion(f, ver))
	cmd.AddCommand(auth.NewCmdAuth(f))
	cmd.AddCommand(apiCmd.NewCmdAPI(f))  // ← Phase 3 新增

	return cmd, nil
}

func rootRun(opts *RootOptions) error {
	if opts.ShowVersion() {
		fmt.Fprint(opts.Out, opts.VersionInfo)
		return nil
	}
	return opts.ShowHelp()
}
```

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go build ./...
```

期望输出：无输出（exit code 0），整个项目可以编译。

---

### 2.13 编写 HTTP 客户端测试

**测试策略：使用 `httptest.NewServer`**

测试 `NewHTTPClient` 的最佳方式是启动一个真实的本地 HTTP 服务器（`httptest.NewServer`），让真实的 HTTP 请求经过 RoundTripper 链，在服务端检查请求头。这比 mock 更可靠，因为它测试的是真实的网络行为，而不是对 mock 的假设。

【现在手敲】

创建 `api/http_client_test.go`：

```go
// 文件：api/http_client_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHTTPClient_setsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("", "2.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotUA != "GitHub CLI 2.0.0" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "GitHub CLI 2.0.0")
	}
}

func TestNewHTTPClient_setsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("mytoken", "1.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer mytoken" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer mytoken")
	}
}

func TestNewHTTPClient_noAuthWhenTokenEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("", "1.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "" {
		t.Errorf("Authorization should be empty without token, got %q", gotAuth)
	}
}

func TestNewHTTPClient_setsAPIVersion(t *testing.T) {
	var gotVer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotVer = r.Header.Get("X-GitHub-Api-Version")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("", "1.0.0")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if gotVer != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", gotVer, "2022-11-28")
	}
}

func TestNewHTTPClient_existingAuthNotOverwritten(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewHTTPClient("factorytoken", "1.0.0")
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer manualtoken")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer manualtoken" {
		t.Errorf("Authorization = %q, want manual token to win", gotAuth)
	}
}
```

**关键点解释**

- `TestNewHTTPClient_existingAuthNotOverwritten`：这个测试直接验证了 `authTransport` 的"非干扰"设计——即使 Factory 持有 token，调用方手动在 Request 上设置的 Authorization 头会优先生效。这模拟了 `authflow` 使用自己的凭证发请求的场景。
- 使用 handler 内的闭包变量（`gotUA`、`gotAuth`）捕获请求头：handler 在 `httptest.Server` 的 goroutine 中运行，使用闭包捕获变量是线程安全的（测试在 `client.Get()` 返回后才读取变量，此时 handler 已经执行完毕）。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go test ./api/... -run TestNewHTTPClient -v
```

期望输出：

```
=== RUN   TestNewHTTPClient_setsUserAgent
--- PASS: TestNewHTTPClient_setsUserAgent (0.00s)
=== RUN   TestNewHTTPClient_setsAuthHeader
--- PASS: TestNewHTTPClient_setsAuthHeader (0.00s)
=== RUN   TestNewHTTPClient_noAuthWhenTokenEmpty
--- PASS: TestNewHTTPClient_noAuthWhenTokenEmpty (0.00s)
=== RUN   TestNewHTTPClient_setsAPIVersion
--- PASS: TestNewHTTPClient_setsAPIVersion (0.00s)
=== RUN   TestNewHTTPClient_existingAuthNotOverwritten
--- PASS: TestNewHTTPClient_existingAuthNotOverwritten (0.00s)
PASS
ok  	github.com/learngh/gh-impl/api
```

---

### 2.14 编写 Client 测试

**`rewriteTransport`：测试专用的 URL 重写 Transport**

`Client.REST()` 会拼接 `https://api.github.com/user` 这样的 URL，但测试服务器运行在本地 `http://127.0.0.1:PORT`。`rewriteTransport` 在发请求前将 Host 替换为测试服务器的地址，同时保留 Path，让测试可以验证路径拼接逻辑，又不需要真正访问 GitHub。

这个模式（自定义 Transport 做 URL 重写）在 Go HTTP 测试中非常常见，值得掌握。

【现在手敲】

创建 `api/client_test.go`：

```go
// 文件：api/client_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestREST_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/user" {
			t.Errorf("path = %q, want /user", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"login": "alice"})
	}))
	defer srv.Close()

	client := NewClientFromHTTP(srv.Client())
	var data struct{ Login string }
	// Direct test using the actual REST helper via a rewrite transport.
	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)
	if err := c.REST("github.com", http.MethodGet, "/user", nil, &data); err != nil {
		t.Fatalf("REST: %v", err)
	}
	if data.Login != "alice" {
		t.Errorf("Login = %q, want alice", data.Login)
	}
	_ = client
}

func TestREST_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "requires authentication"})
	}))
	defer srv.Close()

	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)
	err := c.REST("github.com", http.MethodGet, "/user", nil, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	var httpErr HTTPError
	if e, ok := err.(HTTPError); ok {
		httpErr = e
	} else {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", httpErr.StatusCode)
	}
	if httpErr.Message != "requires authentication" {
		t.Errorf("Message = %q", httpErr.Message)
	}
}

func TestHTTPError_Error(t *testing.T) {
	e := HTTPError{StatusCode: 404, Message: "Not Found"}
	if e.Error() != "HTTP 404: Not Found" {
		t.Errorf("Error() = %q", e.Error())
	}
	e2 := HTTPError{StatusCode: 500}
	if e2.Error() != "HTTP 500" {
		t.Errorf("Error() = %q", e2.Error())
	}
}

func TestGraphQL_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Errorf("path = %q, want /graphql", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]string{"login": "alice"},
			},
		})
	}))
	defer srv.Close()

	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)

	var data struct {
		Viewer struct{ Login string }
	}
	err := c.GraphQL("github.com", `query { viewer { login } }`, nil, &data)
	if err != nil {
		t.Fatalf("GraphQL: %v", err)
	}
	if data.Viewer.Login != "alice" {
		t.Errorf("Login = %q, want alice", data.Viewer.Login)
	}
}

func TestGraphQL_errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{
				{"message": "Field 'foo' doesn't exist"},
			},
		})
	}))
	defer srv.Close()

	httpCl := &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}
	c := NewClientFromHTTP(httpCl)

	err := c.GraphQL("github.com", `query { foo }`, nil, nil)
	if err == nil {
		t.Fatal("expected GraphQL error")
	}
	gqlErr, ok := err.(GraphQLError)
	if !ok {
		t.Fatalf("expected GraphQLError, got %T", err)
	}
	if gqlErr.Message != "Field 'foo' doesn't exist" {
		t.Errorf("Message = %q", gqlErr.Message)
	}
}

func TestGraphQLError_Error(t *testing.T) {
	e := GraphQLError{Message: "some graphql error"}
	if e.Error() != "some graphql error" {
		t.Errorf("Error() = %q", e.Error())
	}
}

// rewriteTransport redirects all requests to a test server base URL.
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

**关键点解释**

- `TestGraphQL_errors`：测试服务端返回 HTTP 200 但响应体包含 `errors` 字段的情况，这是 GraphQL 特有的错误模式。HTTP 层面成功（200），但应用层面失败，必须通过检查 `gqlResp.Errors` 来发现。
- `err.(HTTPError)` 和 `err.(GraphQLError)` 类型断言：直接使用类型断言（而不是 `errors.As`），因为这些错误类型**没有**被 `fmt.Errorf("%w", ...)` 包装，是直接返回的自定义类型。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go test ./api/... -v
```

期望输出：所有测试 PASS。

---

### 2.15 编写 api 命令测试

**Options struct 模式的测试优势**

因为 `APIOptions` 包含 `HttpClient func() (*http.Client, error)`（一个函数），测试时可以直接注入一个返回测试客户端的函数，完全绕过 Factory 和真实 Config，测试代码更简洁、更可控。

【现在手敲】

创建 `pkg/cmd/api/api_test.go`：

```go
// 文件：pkg/cmd/api/api_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
)

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

func TestAPIRun_REST_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"login": "alice"})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &APIOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		Hostname:    "github.com",
		Method:      "GET",
		RequestPath: "/user",
		Fields:      map[string]string{},
	}

	if err := apiRun(opts); err != nil {
		t.Fatalf("apiRun: %v", err)
	}
	if !strings.Contains(out.String(), "alice") {
		t.Errorf("output = %q, want to contain 'alice'", out.String())
	}
}

func TestAPIRun_REST_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "requires authentication"})
	}))
	defer srv.Close()

	ios, _, _, _ := iostreams.Test()
	opts := &APIOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		Hostname:    "github.com",
		Method:      "GET",
		RequestPath: "/user",
		Fields:      map[string]string{},
	}

	err := apiRun(opts)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want to contain '401'", err.Error())
	}
}

func TestAPIRun_GraphQL_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]string{"login": "bob"},
			},
		})
	}))
	defer srv.Close()

	ios, _, out, _ := iostreams.Test()
	opts := &APIOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &rewriteTransport{base: srv.URL, inner: srv.Client().Transport}}, nil
		},
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		Hostname:    "github.com",
		RequestPath: "graphql",
		IsGraphQL:   true,
		Fields:      map[string]string{"query": "{ viewer { login } }"},
	}

	if err := apiRun(opts); err != nil {
		t.Fatalf("apiRun graphql: %v", err)
	}
	if !strings.Contains(out.String(), "bob") {
		t.Errorf("output = %q, want to contain 'bob'", out.String())
	}
}

func TestAPIRun_GraphQL_missingQuery(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &APIOptions{
		IO:          ios,
		HttpClient:  func() (*http.Client, error) { return &http.Client{}, nil },
		Config:      func() (cmdutil.Config, error) { return nil, nil },
		RequestPath: "graphql",
		IsGraphQL:   true,
		Fields:      map[string]string{},
	}
	err := apiRun(opts)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("error = %q, want to mention 'query'", err.Error())
	}
}

func TestNewCmdAPI_hasCorrectUse(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams:   ios,
		Config:     func() (cmdutil.Config, error) { return nil, nil },
		HttpClient: func() (*http.Client, error) { return &http.Client{}, nil },
	}
	cmd := NewCmdAPI(f)
	if cmd.Use != "api <endpoint>" {
		t.Errorf("Use = %q", cmd.Use)
	}
}
```

**关键点解释**

- `iostreams.Test()` 返回四个值：`(*IOStreams, *bytes.Buffer/*stdin*, *bytes.Buffer/*stdout*, *bytes.Buffer/*stderr*)`。测试通过 `out.String()` 检查命令输出，不依赖真实终端。
- `TestAPIRun_GraphQL_missingQuery`：验证防御性检查——GraphQL 路径如果没有 `-f query=...`，应该立即报错，不发出 HTTP 请求。这是用户体验的重要保障。
- `pkg/cmd/api/api_test.go` 中重新定义了 `rewriteTransport`：与 `api/client_test.go` 中的同名类型在**不同包**（`package api` 在 `pkg/cmd/api/` 目录下），所以没有冲突。Go 包隔离机制使得测试辅助类型可以在各个测试包中独立定义。

【验证】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go test ./pkg/cmd/api/... -v
```

期望输出：所有测试 PASS。

---

### 2.16 运行所有测试

【现在手敲】

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go test ./...
```

【验证】

期望输出：

```
ok  	github.com/learngh/gh-impl/api              0.XXXs
ok  	github.com/learngh/gh-impl/internal/config  0.XXXs
ok  	github.com/learngh/gh-impl/internal/factory  [no test files]
ok  	github.com/learngh/gh-impl/pkg/cmd/api      0.XXXs
ok  	github.com/learngh/gh-impl/pkg/cmd/root     [no test files]
...
```

所有测试包应该 PASS，无 FAIL。

可以用 `-race` flag 检测 race condition：

```bash
cd /d/A/code/claude/gh-learning/cli/src/phase-03-api-client
go test -race ./...
```

期望输出：与不加 `-race` 相同，无 DATA RACE 报告。

---

## 附录：Phase 3 关键设计决策总结

### A. RoundTripper 链模式 vs 中间件模式

RoundTripper 链是 Go 标准库 HTTP 的官方扩展点，与 Web 框架的中间件（middleware）模式本质相同，但应用于客户端而非服务端。每个 Transport 只做一件事（单一职责），通过组合实现复杂行为，且每层都可以独立测试。相比于在 `Do(req)` 调用前手动设置 Header，RoundTripper 链的优势在于：对调用方透明，无论调用方用 `client.Get()`、`client.Post()` 还是 `client.Do(req)`，都能自动注入 Header。

### B. clone request 的 Go HTTP 合约

Go 的 HTTP Transport 合约（`net/http` 文档）明确规定：RoundTripper 不得修改请求，不得在 `RoundTrip` 返回后持有请求。`req.Clone(ctx)` 是标准库提供的浅拷贝方法，专门用于这个场景——它深拷贝 Header（Map 类型，直接引用会有并发问题），浅拷贝其余字段（如 URL、Body）。违反这个合约会导致微妙的并发 bug，难以排查。

### C. 懒加载闭包 vs 立即初始化

`Factory.HttpClient` 使用闭包而不是在 `factory.New()` 时立即调用 `api.NewHTTPClient()` 的原因：初始化顺序问题。`factory.New()` 执行时，`f.Config` 可能还是返回 stub 的函数；`auth.NewCmdAuth()` 执行时会替换 `f.Config` 为真实 Config。如果 `HttpClient` 在 `factory.New()` 时就确定，它将永远使用 stub Config 的空 token。闭包捕获 `f`（指针），延迟到 `HttpClient()` 被调用时才读取 `f.Config`，保证拿到最新版本的 Config 和 token。

### D. `data interface{}` 的灵活性

`REST()` 和 `GraphQL()` 接收 `data interface{}` 参数，调用方传入指向具体类型的指针（如 `&myStruct`），`json.NewDecoder(resp.Body).Decode(data)` 通过反射填充字段。这个模式允许同一个 `REST()` 方法服务于任意响应结构——调用方定义自己的 struct 类型，不需要 API 包提前知道每种响应的 schema。这是 Go 中处理异构 JSON 数据的惯用方式。
