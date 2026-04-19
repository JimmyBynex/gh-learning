# Phase 2 学习指南：配置与认证

---

## Section 1：全局心智模型

### 1.1 这个 Phase 做了什么

Phase 2 让 `gh auth login` 和 `gh auth status` 真正能运行。具体来说，它做了四件事：

1. **扩展 Config 接口**：在 Phase 1 的四个方法（`Get/Set/Write/Hosts`）基础上，增加 `AuthToken`、`Login`、`Logout` 三个认证专用方法，明确 Config 不只是通用的键值存储，还是认证凭证的管理器。

2. **实现文件配置**（`internal/config/config.go`）：把 `~/.config/gh/hosts.yml` 读进内存，封装为线程安全的 `fileConfig` struct，提供所有 Config 接口方法的真实实现，并在写入时保证原子性（同一把锁下修改内存并立即落盘）。

3. **实现 OAuth Device Authorization Flow**（`internal/authflow/flow.go`）：完整实现 RFC 8628 标准——向 GitHub 请求设备码，展示给用户，等待用户在浏览器授权，轮询直到拿到 token，最后调用 `/user` API 取得用户名。

4. **实现两个 auth 命令**：`gh auth login`（支持 `--with-token` 直接提供 token，以及交互式 Device Flow）和 `gh auth status`（遍历已保存的 hostname，打印登录状态）。

Phase 1 建立了骨架，Phase 2 让骨架第一次具备了"认识用户"的能力——这是所有后续 API 命令的前提。

---

### 1.2 在架构中的位置

```
用户输入
   │
   ▼
[main.go] → ghcmd.Main()
                │
                ▼
         [factory.New()]
                │
                ├── IOStreams          ← Phase 1
                ├── Config stub        ← Phase 1 占位
                │       │
                │       └── [NewCmdAuth() 替换为真实 Config] ← ★ Phase 2
                │               └── config.NewConfig()
                │                       └── ~/.config/gh/hosts.yml
                ├── HttpClient stub    ← Phase 1 占位
                └── GitClient nil      ← Phase 6 填入
                │
                ▼
         [root.NewCmdRoot(f)]
                │
                ├── auth              ← ★ Phase 2
                │   ├── login         ← ★ Phase 2
                │   └── status        ← ★ Phase 2
                └── version           ← Phase 1
```

标记 ★ 的部分是 Phase 2 新增或替换的内容。`factory.New()` 仍然创建 Config stub，但 `NewCmdAuth()` 在进入 auth 子命令树之前会把 stub 替换为真实的 `config.NewConfig()`。

---

### 1.3 控制流图

Phase 2 支持三条主要执行路径：

**路径 A：`gh auth login --with-token`（token 直接写入）**

```
gh auth login --with-token
       │
       ▼
  NewCmdLogin()
  读取 opts.WithToken = true
       │
       ▼
  RunE: bufio.NewReader(opts.IO.In).ReadString('\n')
  → opts.Token = strings.TrimSpace(tok)
       │
       ▼
  loginRun(opts)
       │
       ├── opts.Config() → config.NewConfig() → 读 hosts.yml
       ├── opts.HttpClient() → *http.Client
       │
       ▼
  authflow.FetchUsername(httpClient, apiBase, opts.Token)
  → GET https://api.github.com/user
  → Bearer <token>
  → 解析 JSON { "login": "alice" }
       │
       ▼
  cfg.Login(hostname, username, token)
  → 锁内更新内存 + writeUnlocked() → 写 hosts.yml
       │
       ▼
  fmt.Fprintf(opts.IO.Out, "Logged in to %s as %s\n", ...)
```

**路径 B：`gh auth login`（Device Flow 交互式）**

```
gh auth login
       │
       ▼
  NewCmdLogin()
  opts.WithToken = false（默认）
       │
       ▼
  loginRun(opts)
       │
       ▼
  authflow.DeviceFlow(httpClient, hostname, ios)
       │
       ├── requestDeviceCode()
       │   → POST https://github.com/login/device/code
       │   → 返回 { device_code, user_code, verification_uri, interval }
       │
       ├── 打印 user_code，等待用户按 Enter
       │
       └── pollForToken() [循环]
           → POST https://github.com/login/oauth/access_token
           → switch tr.Error:
               "authorization_pending" → 继续等待
               "slow_down"            → 增加 interval 后继续
               "expired_token"        → 返回错误，退出
               ""（成功）             → 返回 access_token
       │
       ▼
  FetchUsername(httpClient, apiBase, token)
  → GET https://api.github.com/user
       │
       ▼
  cfg.Login(hostname, username, token) → 写 hosts.yml
```

**路径 C：`gh auth status`（查看状态）**

```
gh auth status
       │
       ▼
  NewCmdStatus()
       │
       ▼
  statusRun(opts)
       │
       ├── opts.Config() → config.NewConfig() → 读 hosts.yml
       │
       ▼
  cfg.Hosts() → []string（已登录的 hostname 列表）
       │
       ├── 如果 len(hosts) == 0:
       │   → fmt.Fprintf(ErrOut, "You are not logged in...")
       │   → return cmdutil.SilentError
       │
       └── 遍历 hosts:
           cfg.AuthToken(hostname) → token
           cfg.Get(hostname, "user") → username
           fmt.Fprintf(Out, "%s\n  Logged in to %s as %s\n\n", ...)
```

---

### 1.4 数据流图

**磁盘 YAML 格式**（`~/.config/gh/hosts.yml`）：

```yaml
github.com:
    oauth_token: ghp_xxxxxxxxxxxxxxxxxxxx
    user: alice
github.enterprise.com:
    oauth_token: ghp_yyyyyyyyyyyyyyyyyy
    user: bob
```

**内存结构**（Go 类型）：

```
fileConfig {
    mu:    sync.Mutex               // 保护并发读写
    path:  string                   // "/home/alice/.config/gh/hosts.yml"
    hosts: map[string]*hostEntry {
        "github.com": &hostEntry {
            OAuthToken: "ghp_xxx",  // yaml tag: oauth_token
            User:       "alice",    // yaml tag: user
        },
        "github.enterprise.com": &hostEntry {
            OAuthToken: "ghp_yyy",
            User:       "bob",
        },
    }
}
```

**Login 数据流（具体类型）**：

```
string(hostname)  ──────────────────────────────────────────┐
string(username)  ──────────────────┐                       │
string(token)     ──┐               │                       │
                    │               │                       │
                    ▼               ▼                       ▼
              cfg.Login(hostname, username, token) error
                    │
                    ▼  [持锁]
              c.hosts[hostname] = &hostEntry{
                  OAuthToken: token,    // string → yaml: oauth_token
                  User:       username, // string → yaml: user
              }
                    │
                    ▼
              c.writeUnlocked()  // 锁内调用，不二次加锁
                    │
                    ▼
              yaml.Marshal(c.hosts)  // map[string]*hostEntry → []byte
                    │
                    ▼
              os.WriteFile(c.path, data, 0o600)  // []byte → 磁盘文件
```

YAML 解析方向（读取时）：

```
os.ReadFile(path) → []byte
    │
    ▼
yaml.Unmarshal(data, &hosts)
    │
    ▼
map[string]*hostEntry（内存）
// yaml tag "oauth_token" → OAuthToken string
// yaml tag "user"        → User string
```

---

### 1.5 与 Phase 1 的连接

Phase 2 直接使用了 Phase 1 定义的四个核心组件，以下是每个组件的内联解释：

**`*iostreams.IOStreams`**

Phase 1 定义的输出抽象层。包含三个字段：
- `Out io.Writer`：正常输出（打印登录成功消息、状态信息）
- `ErrOut io.Writer`：错误输出（打印"未登录"的警告）
- `In io.Reader`：标准输入（Device Flow 读取用户按 Enter，`--with-token` 读取 token）

测试时通过 `iostreams.Test()` 注入 `bytes.Buffer`，替换真实的 os.Stdout/os.Stderr/os.Stdin，使测试可以验证输出内容而不依赖终端。

Phase 2 中 `authflow.DeviceFlow()` 接收 `*iostreams.IOStreams` 参数，调用 `ios.Out` 和 `ios.In`；`statusRun()` 同时使用 `opts.IO.Out`（成功输出）和 `opts.IO.ErrOut`（未登录警告）。

**`*cmdutil.Factory`**

Phase 1 定义的依赖容器（不是依赖注入框架，就是一个持有函数指针的 struct）：

```go
type Factory struct {
    IOStreams   *iostreams.IOStreams
    Config     func() (Config, error)      // 延迟初始化
    HttpClient func() (*http.Client, error) // 延迟初始化
    GitClient  interface{}
}
```

Phase 2 的关键技巧：`NewCmdAuth()` 里执行 `authFactory := *f`（值拷贝），然后替换 `authFactory.Config`。这样 auth 子树用真实 Config，其他命令（如 version）继续用 Phase 1 的 stub。值拷贝确保不影响原始 Factory。

**`cmdutil.Config` 接口（4 个基础方法）**

Phase 1 定义了接口但只提供了返回 error 的 stub 实现：

```go
// Phase 1 stub（factory.go 中）：
Config: func() (cmdutil.Config, error) {
    return nil, fmt.Errorf("not implemented")
},
```

Phase 1 的 stub 保证了编译通过，但调用任何 Config 方法都会得到错误。Phase 2 通过 `config.NewConfig()` 提供真实实现——`fileConfig` 实现了接口的全部 7 个方法（4 个基础 + 3 个认证专用）。Phase 2 新增的 3 个认证方法（`AuthToken`、`Login`、`Logout`）也在 `cmdutil.Config` 接口中定义，由 `pkg/cmdutil/config.go` 声明。

**`root.NewCmdRoot()`**

Phase 1 已经在根命令中注册了 auth 子命令：

```go
// Phase 1 的 root.go 中：
cmd.AddCommand(auth.NewCmdAuth(f))
```

Phase 2 实现的 `auth.NewCmdAuth(f)` 正是被这行代码调用。命令树从 root 开始，`gh auth` 指向 `NewCmdAuth` 返回的 cobra.Command，`gh auth login` 和 `gh auth status` 是其子命令。Phase 1 的命令注册代码不需要改动——Phase 2 只需要提供正确的实现。

---

## Section 2：Implementation Walkthrough

### 2.1 数据结构

**`cmdutil.Config` 接口（完整版，Phase 2 新增认证方法）**

Phase 1 已有 `Get/Set/Write/Hosts` 四个方法满足通用配置读写需求。Phase 2 新增 `AuthToken/Login/Logout` 三个专用方法。

为什么不用 `Get("github.com", "oauth_token")` 而专门加 `AuthToken`？原因有三：
1. 类型安全：`AuthToken` 明确返回 `(string, error)`，调用方不需要知道 key 名称是 `"oauth_token"` 还是别的字符串。
2. 语义清晰：接口表达"这是一个能管理认证凭证的配置"，而不是"这是一个通用 map"。
3. `Login` 需要同时写 token 和 username 并落盘——如果拆成两次 `Set` + 一次 `Write`，中间可能发生并发修改；单个 `Login` 方法在锁内原子完成全部操作。

【现在手敲】

```go
// 文件：pkg/cmdutil/config.go
package cmdutil

// Config defines the interface for reading and writing CLI configuration.
type Config interface {
	Get(hostname, key string) (string, error)
	Set(hostname, key, value string) error
	Write() error
	Hosts() []string
	AuthToken(hostname string) (string, error)
	Login(hostname, username, token string) error
	Logout(hostname string) error
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go vet ./pkg/cmdutil/...`

期望输出：
```
（无输出，exit code 0）
```

---

**`hostEntry` + `fileConfig`（核心数据结构）**

`hostEntry` 是每个 hostname 对应的配置条目，使用 yaml struct tag 控制序列化字段名。

`fileConfig` 是 `Config` 接口的磁盘实现。`sync.Mutex` 的原因：`Hosts()`、`AuthToken()`、`Get()` 是读操作，`Login()`、`Logout()` 是写操作，多个 goroutine（比如并发的 HTTP 请求处理）可能同时访问。即使当前 CLI 是单线程的，Go race detector 会在测试中检测到无锁并发访问，加锁是正确做法。

注意 `fileConfig` 没有导出（小写 f），外部只能通过 `cmdutil.Config` 接口操作它，这是接口隔离原则的体现。

【现在手敲】

```go
// 文件：internal/config/config.go（前半部分）
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"gopkg.in/yaml.v3"
)

type hostEntry struct {
	OAuthToken string `yaml:"oauth_token,omitempty"`
	User       string `yaml:"user,omitempty"`
}

type fileConfig struct {
	mu    sync.Mutex
	hosts map[string]*hostEntry
	path  string
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go build ./internal/config/...`

期望输出：
```
（无输出，exit code 0）
```

---

### 2.2 核心逻辑

**`NewConfig()` / `newConfigFromPath()`（从磁盘读 YAML）**

`NewConfig()` 是公开入口，调用私有的 `newConfigFromPath()` 并传入标准路径。分为两层的原因：测试时可以直接调用 `newConfigFromPath(tmpDir + "/hosts.yml")`，不依赖真实的 `~/.config/gh/`。

缺失文件不报错：`errors.Is(err, os.ErrNotExist)` 判断文件不存在——第一次使用时 hosts.yml 不存在，这是正常状态，不应该报错，直接返回空配置。其他错误（权限问题、磁盘损坏）才真正报错。

`nil` entry 修复：YAML 中可能出现 `github.com:` 后面没有内容的情况，`yaml.Unmarshal` 会把对应 value 设为 nil pointer，所以需要遍历修复。

【现在手敲】

```go
func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gh")
}

func hostsFilePath() string {
	return filepath.Join(ConfigDir(), "hosts.yml")
}

func NewConfig() (cmdutil.Config, error) {
	return newConfigFromPath(hostsFilePath())
}

func newConfigFromPath(path string) (cmdutil.Config, error) {
	hosts := make(map[string]*hostEntry)

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &hosts); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	for k, v := range hosts {
		if v == nil {
			hosts[k] = &hostEntry{}
		}
	}

	return &fileConfig{hosts: hosts, path: path}, nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/config/ -run TestConfig_ParsesExistingYAML -v`

期望输出：
```
=== RUN   TestConfig_ParsesExistingYAML
--- PASS: TestConfig_ParsesExistingYAML (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/config
```

---

**`Login()`（原子性写入）**

`Login()` 在同一把锁内完成内存更新和磁盘写入。关键点：它调用的是 `writeUnlocked()`（不加锁的版本），因为锁已经在 `Login()` 开头获取了。如果调用 `Write()`（会再次加锁），就会发生死锁。

`writeUnlocked()` 的 `MkdirAll` 确保首次运行时目录不存在也能正常写入，`0o700` 只有所有者可以访问目录，`0o600` 只有所有者可以读写文件——保护 token 不被其他用户读取。

【现在手敲】

```go
func (c *fileConfig) Login(hostname, username, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hosts[hostname] = &hostEntry{
		OAuthToken: token,
		User:       username,
	}
	return c.writeUnlocked()
}

func (c *fileConfig) Write() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeUnlocked()
}

func (c *fileConfig) writeUnlocked() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := yaml.Marshal(c.hosts)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(c.path, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/config/ -run TestConfig_WriteAndRead -v`

期望输出：
```
=== RUN   TestConfig_WriteAndRead
--- PASS: TestConfig_WriteAndRead (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/config
```

---

**`AuthToken()` / `Hosts()`（读操作，锁保护）**

读操作同样需要锁，原因是 Go 的 map 不是并发安全的——即使只读，如果同时有写操作，就会 panic（Go race detector 会捕获这个问题）。`Hosts()` 返回的是新建的切片，不是 map 的引用，调用方无法通过返回值修改内部状态。

`AuthToken()` 同时检查 entry 存在性和 token 非空，这样空 token 的 entry 也会报错（不会返回空字符串给调用方作为有效 token 使用）。

【现在手敲】

```go
func (c *fileConfig) Hosts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	names := make([]string, 0, len(c.hosts))
	for h := range c.hosts {
		names = append(names, h)
	}
	return names
}

func (c *fileConfig) AuthToken(hostname string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.hosts[hostname]
	if !ok || entry.OAuthToken == "" {
		return "", fmt.Errorf("not logged in to %s", hostname)
	}
	return entry.OAuthToken, nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/config/ -run TestConfig_AuthToken -v`

期望输出：
```
=== RUN   TestConfig_AuthToken_Missing
--- PASS: TestConfig_AuthToken_Missing (0.00s)
=== RUN   TestConfig_AuthToken_Present
--- PASS: TestConfig_AuthToken_Present (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/config
```

---

**`requestDeviceCode()`（POST /login/device/code）**

这是 Device Flow 的第一步。POST 请求使用 `application/x-www-form-urlencoded` 格式（不是 JSON），因为 GitHub OAuth 端点遵循 RFC 6749 传统格式。`Accept: application/json` 头告诉 GitHub 以 JSON 格式返回响应（否则会返回 query string 格式）。

`url.Values` 是 `map[string][]string`，`.Encode()` 自动进行 URL 编码。`strings.NewReader()` 把字符串转为 `io.Reader`，满足 `http.NewRequest` 的 body 参数类型要求。

【现在手敲】

```go
// 文件：internal/authflow/flow.go
package authflow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

const (
	clientID = "178c6fc778ccc68e1d6a"
	scope    = "repo,read:org,gist"
)

type DeviceFlowResult struct {
	Token    string
	Username string
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	Interval    int    `json:"interval"`
}

type userResponse struct {
	Login string `json:"login"`
}

func requestDeviceCode(httpClient *http.Client, ghBaseURL string) (*deviceCodeResponse, error) {
	endpoint := ghBaseURL + "/login/device/code"
	body := url.Values{
		"client_id": {clientID},
		"scope":     {scope},
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var dc deviceCodeResponse
	if err := json.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("decoding device code response: %w", err)
	}
	return &dc, nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -run TestRequestDeviceCode_HTTPError -v`

期望输出：
```
=== RUN   TestRequestDeviceCode_HTTPError
--- PASS: TestRequestDeviceCode_HTTPError (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**`pollForToken()`（轮询等待用户授权）**

这是 Device Flow 的核心循环。`switch tr.Error` 处理四种情况：
- `""`（空字符串）且 `AccessToken != ""`：成功，返回 token
- `"authorization_pending"`：用户还没操作，继续等待
- `"slow_down"`：GitHub 要求降低轮询频率，使用响应中的新 interval 或增加 5 秒
- `"expired_token"`：设备码已过期（通常 15 分钟），返回错误让用户重新开始
- 其他错误：未知 OAuth 错误，直接返回

`time.Sleep(interval)` 在循环末尾，第一次轮询前没有等待（GitHub 刚发完设备码，立即轮询一次通常就是 pending，但不浪费时间）。

【现在手敲】

```go
func pollForToken(httpClient *http.Client, ghBaseURL, deviceCode string, intervalSecs int) (string, error) {
	endpoint := ghBaseURL + "/login/oauth/access_token"
	body := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	interval := time.Duration(intervalSecs) * time.Second

	for {
		req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return "", readErr
		}

		var tr tokenResponse
		if err := json.Unmarshal(data, &tr); err != nil {
			return "", fmt.Errorf("decoding token response: %w", err)
		}

		switch tr.Error {
		case "":
			if tr.AccessToken != "" {
				return tr.AccessToken, nil
			}
			return "", fmt.Errorf("no access token in response")
		case "authorization_pending":
			// continue
		case "slow_down":
			if tr.Interval > 0 {
				interval = time.Duration(tr.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return "", fmt.Errorf("device code expired; please try again")
		default:
			return "", fmt.Errorf("OAuth error: %s", tr.Error)
		}

		time.Sleep(interval)
	}
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -run TestPollForToken_SlowDown -v`

期望输出：
```
=== RUN   TestPollForToken_SlowDown
--- PASS: TestPollForToken_SlowDown (1.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**`FetchUsername()`（GET /user，验证 token 有效性）**

`FetchUsername()` 同时完成两件事：验证 token 有效（401 会报错）和获取用户名。`Authorization: Bearer <token>` 是 GitHub API 推荐的认证头格式（相比旧式的 `token <token>`）。

HTTP 非 200 状态码立即报错，不尝试解析 body——这是防御性编程，避免把错误响应当成正常数据解析。

【现在手敲】

```go
func FetchUsername(httpClient *http.Client, apiBaseURL, token string) (string, error) {
	endpoint := apiBaseURL + "/user"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("/user returned HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var u userResponse
	if err := json.Unmarshal(data, &u); err != nil {
		return "", fmt.Errorf("decoding user response: %w", err)
	}
	if u.Login == "" {
		return "", fmt.Errorf("empty login in /user response")
	}
	return u.Login, nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -run TestFetchUsername_Unauthorized -v`

期望输出：
```
=== RUN   TestFetchUsername_Unauthorized
--- PASS: TestFetchUsername_Unauthorized (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**`DeviceFlow()` / `deviceFlow()`（组合三步，公共 vs 可测试）**

两个函数分工明确：
- `DeviceFlow()`：公开 API，接收 hostname 字符串，自动构造 URL（`"https://"+hostname`），供生产代码调用。
- `deviceFlow()`：私有实现，接收 `ghBaseURL` 和 `apiBaseURL` 两个完整 URL，供测试注入 `httptest.Server` 的地址。

这种模式让测试不需要真实网络连接——测试可以直接调用 `deviceFlow(client, ts.URL, ts.URL, ios)` 指向本地测试服务器。

`bufio.NewReader(ios.In).ReadString('\n')` 等待用户按 Enter——测试时 `ios.In` 是注入的 `strings.NewReader("\n")`，自动通过等待。

【现在手敲】

```go
func DeviceFlow(httpClient *http.Client, hostname string, ios *iostreams.IOStreams) (*DeviceFlowResult, error) {
	return deviceFlow(httpClient, "https://"+hostname, "https://api.github.com", ios)
}

func deviceFlow(httpClient *http.Client, ghBaseURL, apiBaseURL string, ios *iostreams.IOStreams) (*DeviceFlowResult, error) {
	dcResp, err := requestDeviceCode(httpClient, ghBaseURL)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}

	fmt.Fprintf(ios.Out, "! First copy your one-time code: %s\n", dcResp.UserCode)
	fmt.Fprintf(ios.Out, "Press Enter to open GitHub in your browser... ")
	_, _ = bufio.NewReader(ios.In).ReadString('\n')
	fmt.Fprintf(ios.Out, "\nOpening %s\n", dcResp.VerificationURI)

	intervalSecs := dcResp.Interval
	if intervalSecs <= 0 {
		intervalSecs = 5
	}

	token, err := pollForToken(httpClient, ghBaseURL, dcResp.DeviceCode, intervalSecs)
	if err != nil {
		return nil, err
	}

	username, err := FetchUsername(httpClient, apiBaseURL, token)
	if err != nil {
		return nil, fmt.Errorf("fetching username: %w", err)
	}

	fmt.Fprintf(ios.Out, "Logged in as %s\n", username)
	return &DeviceFlowResult{Token: token, Username: username}, nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -run TestDeviceFlow_EndToEnd -v`

期望输出：
```
=== RUN   TestDeviceFlow_EndToEnd
--- PASS: TestDeviceFlow_EndToEnd (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**`loginRun()`（with-token 分支 vs Device Flow 分支）**

`loginRun()` 是 auth login 的核心逻辑。两个分支共享同一套 config 加载和 http client 初始化代码：
- `--with-token` 分支：token 已经在 RunE 中读入 `opts.Token`，直接调用 `FetchUsername()` 验证并保存。
- Device Flow 分支：委托给 `authflow.DeviceFlow()`，该函数内部已经打印提示和保存 token——不，实际上 `DeviceFlow()` 返回结果后，`loginRun` 还需要调用 `cfg.Login()` 保存。

注意：`DeviceFlow()` 内部只打印 "Logged in as X"，不保存到配置文件——保存逻辑在 `loginRun` 中，保持了 authflow 包对 config 包的零依赖（单向依赖，不循环）。

【现在手敲】

```go
// 文件：pkg/cmd/auth/login/login.go
package login

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/learngh/gh-impl/internal/authflow"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

type LoginOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (cmdutil.Config, error)
	HttpClient func() (*http.Client, error)
	Hostname   string
	Token      string
	WithToken  bool
}

func loginRun(opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	if opts.WithToken {
		apiBase := apiBaseURL(opts.Hostname)
		username, err := authflow.FetchUsername(httpClient, apiBase, opts.Token)
		if err != nil {
			return fmt.Errorf("authenticating with token: %w", err)
		}
		if err := cfg.Login(opts.Hostname, username, opts.Token); err != nil {
			return fmt.Errorf("saving credentials: %w", err)
		}
		fmt.Fprintf(opts.IO.Out, "Logged in to %s as %s\n", opts.Hostname, username)
		return nil
	}

	result, err := authflow.DeviceFlow(httpClient, opts.Hostname, opts.IO)
	if err != nil {
		return err
	}
	if err := cfg.Login(opts.Hostname, result.Username, result.Token); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}
	return nil
}

func apiBaseURL(hostname string) string {
	if hostname == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + hostname + "/api/v3"
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./pkg/cmd/auth/login/ -run TestLoginRun_WithToken_Success -v`

期望输出：
```
=== RUN   TestLoginRun_WithToken_Success
--- PASS: TestLoginRun_WithToken_Success (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/login
```

---

**`statusRun()`（遍历 hosts，打印每个 hostname 的状态）**

`statusRun()` 的关键设计决策：未登录时输出到 `ErrOut`（stderr），并返回 `cmdutil.SilentError`。`SilentError` 是一个哨兵错误值，`ghcmd.Main()` 检测到它时设置非零退出码但不打印错误消息（cobra 默认会打印 RunE 返回的错误）。

这样设计的原因：`gh auth status` 在未登录时打印的提示"You are not logged in..."已经足够清楚，不需要再让 cobra 打印一行"Error: ..."，那会让输出显得混乱。

【现在手敲】

```go
// 文件：pkg/cmd/auth/status/status.go
package status

import (
	"fmt"
	"net/http"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/learngh/gh-impl/pkg/iostreams"
	"github.com/spf13/cobra"
)

type StatusOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (cmdutil.Config, error)
	HttpClient func() (*http.Client, error)
}

func statusRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hosts := cfg.Hosts()
	if len(hosts) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "You are not logged in to any GitHub hosts. Run `gh auth login` to authenticate.\n")
		return cmdutil.SilentError
	}

	for _, hostname := range hosts {
		token, err := cfg.AuthToken(hostname)
		if err != nil || token == "" {
			fmt.Fprintf(opts.IO.Out, "%s\n  x Not logged in\n\n", hostname)
			continue
		}

		username, _ := cfg.Get(hostname, "user")
		if username == "" {
			username = "(unknown)"
		}

		fmt.Fprintf(opts.IO.Out, "%s\n  Logged in to %s as %s\n\n", hostname, hostname, username)
	}

	return nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./pkg/cmd/auth/status/ -run TestStatusRun_NoHosts -v`

期望输出：
```
=== RUN   TestStatusRun_NoHosts
--- PASS: TestStatusRun_NoHosts (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/status
```

---

### 2.3 接线（Wiring）

**`NewCmdAuth()`：authFactory 值拷贝、Config getter 替换**

`authFactory := *f` 是值拷贝（不是指针赋值）。因为 `cmdutil.Factory` 是 struct（不是 interface），值拷贝会复制所有字段，包括 `Config`、`HttpClient`、`IOStreams` 等函数指针。然后替换 `authFactory.Config` 只影响这个局部副本，原始 `f` 不受影响。

为什么需要替换？`factory.New()` 提供的 Config 是 stub（直接返回 error），auth 子命令需要真实的 Config 实现。但其他命令（如 version）不需要 Config，让它们继续用 stub 是合理的。

【现在手敲】

```go
// 文件：pkg/cmd/auth/auth.go
package auth

import (
	"github.com/learngh/gh-impl/internal/config"
	"github.com/learngh/gh-impl/pkg/cmd/auth/login"
	"github.com/learngh/gh-impl/pkg/cmd/auth/status"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	authFactory := *f
	authFactory.Config = func() (cmdutil.Config, error) {
		return config.NewConfig()
	}

	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate gh and git with GitHub",
		Long:  "Authenticate gh and git with GitHub.",
	}

	cmd.AddCommand(login.NewCmdLogin(&authFactory))
	cmd.AddCommand(status.NewCmdStatus(&authFactory))

	return cmd
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go build ./pkg/cmd/auth/...`

期望输出：
```
（无输出，exit code 0）
```

---

**`NewCmdLogin()` / `NewCmdStatus()`：Options struct、flag 注册（-h 冲突解决方案）**

`Options` struct 集中持有所有依赖（IO、Config、HttpClient）和命令行 flag 的目标变量（Hostname、Token、WithToken）。这个模式让 `loginRun(opts)` 的参数简洁，同时测试时可以直接构造 `LoginOptions` 而不需要解析命令行。

`-h` flag 冲突：cobra 默认把 `-h` 和 `--help` 注册为帮助 flag。如果命令需要 `--hostname` 并绑定 `-h`，和 cobra 内置的 `-h` 会冲突。解决方案：先用 `cmd.Flags().Bool("help", false, "Show help for login")` 覆盖 cobra 的 help flag，再隐藏它，最后用 `StringVarP` 注册 `-h` 给 hostname。

【现在手敲】

```go
func NewCmdLogin(f *cmdutil.Factory) *cobra.Command {
	opts := &LoginOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with a GitHub host",
		Long: `Authenticate with a GitHub host.

The default authentication mode is an interactive OAuth device flow.
Alternatively, pass in a token on standard input by using --with-token.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.WithToken {
				reader := bufio.NewReader(opts.IO.In)
				tok, err := reader.ReadString('\n')
				if err != nil && !errors.Is(err, io.EOF) {
					return fmt.Errorf("reading token from stdin: %w", err)
				}
				opts.Token = strings.TrimSpace(tok)
				if opts.Token == "" {
					return fmt.Errorf("--with-token: no token provided on stdin")
				}
			}
			return loginRun(opts)
		},
	}

	cmd.Flags().Bool("help", false, "Show help for login")
	cmd.Flags().Lookup("help").Hidden = true

	cmd.Flags().StringVarP(&opts.Hostname, "hostname", "h", "github.com", "The hostname of the GitHub instance to authenticate with")
	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "Read token from standard input")

	return cmd
}

func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	return &cobra.Command{
		Use:   "status",
		Short: "View authentication status",
		Long:  "Verifies and displays information about your authentication state.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusRun(opts)
		},
	}
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./pkg/cmd/auth/login/ -run TestNewCmdLogin_WithToken_ReadsStdin -v`

期望输出：
```
=== RUN   TestNewCmdLogin_WithToken_ReadsStdin
--- PASS: TestNewCmdLogin_WithToken_ReadsStdin (0.02s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/login
```

---

### 2.4 错误路径

**网络错误（requestDeviceCode 失败 → 错误包装）**

`requestDeviceCode()` 的网络错误通过 `%w` 包装向上传播：

```
httpClient.Do(req) 失败
  → requestDeviceCode 返回原始 net/http 错误
  → deviceFlow: fmt.Errorf("requesting device code: %w", err)
  → DeviceFlow 返回包装后的错误
  → loginRun 返回（不再包装）
  → cobra 打印: "Error: requesting device code: dial tcp: ..."
```

`%w`（而不是 `%v`）保留了错误链，调用方可以用 `errors.Is()` 检查原始错误类型（比如检测是否是网络超时）。

【现在手敲】

```go
// 此代码片段展示错误包装模式，位于 internal/authflow/flow.go 的 deviceFlow() 函数中：
func deviceFlow(httpClient *http.Client, ghBaseURL, apiBaseURL string, ios *iostreams.IOStreams) (*DeviceFlowResult, error) {
	dcResp, err := requestDeviceCode(httpClient, ghBaseURL)
	if err != nil {
		return nil, fmt.Errorf("requesting device code: %w", err)
	}
	// ... 其余逻辑
	username, err := FetchUsername(httpClient, apiBaseURL, token)
	if err != nil {
		return nil, fmt.Errorf("fetching username: %w", err)
	}
	// ...
	return &DeviceFlowResult{Token: token, Username: username}, nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -run TestRequestDeviceCode_HTTPError -v`

期望输出：
```
=== RUN   TestRequestDeviceCode_HTTPError
--- PASS: TestRequestDeviceCode_HTTPError (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**无效 token（/user 返回 401 → FetchUsername 返回 error）**

当用户提供了无效 token（已撤销、拼写错误等），`FetchUsername()` 调用 `/user` 会得到 HTTP 401。错误传播路径：

```
FetchUsername: fmt.Errorf("/user returned HTTP %d", 401)
  → loginRun (with-token 分支):
      fmt.Errorf("authenticating with token: %w", err)
  → cobra 打印:
      "Error: authenticating with token: /user returned HTTP 401"
```

用户看到的错误消息清楚指明了 token 无效，不是网络问题。

【现在手敲】

```go
// 此代码片段展示 FetchUsername 中的 401 处理，位于 internal/authflow/flow.go：
func FetchUsername(httpClient *http.Client, apiBaseURL, token string) (string, error) {
	// ... 构造请求 ...
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("/user returned HTTP %d", resp.StatusCode)
	}
	// ... 解析响应 ...
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./pkg/cmd/auth/login/ -run TestLoginRun_WithToken_BadToken -v`

期望输出：
```
=== RUN   TestLoginRun_WithToken_BadToken
--- PASS: TestLoginRun_WithToken_BadToken (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/login
```

---

**device code 过期（expired_token → 返回 "device code expired; please try again"）**

设备码默认 15 分钟有效，用户如果没有及时在浏览器授权，轮询会收到 `"expired_token"` 错误。`pollForToken()` 立即退出循环并返回明确的错误消息，引导用户重新运行命令：

```go
case "expired_token":
    return "", fmt.Errorf("device code expired; please try again")
```

错误消息直接告知用户操作（"please try again"），避免显示原始错误码造成困惑。

【现在手敲】

```go
// 此代码片段展示 pollForToken 中的 expired_token 处理，位于 internal/authflow/flow.go：
switch tr.Error {
case "":
	if tr.AccessToken != "" {
		return tr.AccessToken, nil
	}
	return "", fmt.Errorf("no access token in response")
case "authorization_pending":
	// continue
case "slow_down":
	if tr.Interval > 0 {
		interval = time.Duration(tr.Interval) * time.Second
	} else {
		interval += 5 * time.Second
	}
case "expired_token":
	return "", fmt.Errorf("device code expired; please try again")
default:
	return "", fmt.Errorf("OAuth error: %s", tr.Error)
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -run TestDeviceFlow_ExpiredToken -v`

期望输出：
```
=== RUN   TestDeviceFlow_ExpiredToken
--- PASS: TestDeviceFlow_ExpiredToken (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**slow_down（增加轮询间隔后继续）**

`"slow_down"` 是 GitHub 告诉客户端"你轮询太快了"的信号。响应中可能包含建议的新 `interval` 值；如果没有，则增加 5 秒作为保守策略。注意不是 `return`，是继续循环（`switch` 结束后执行 `time.Sleep(interval)` 然后继续 `for`）：

```go
case "slow_down":
    if tr.Interval > 0 {
        interval = time.Duration(tr.Interval) * time.Second
    } else {
        interval += 5 * time.Second
    }
// fall through to time.Sleep(interval) and continue loop
```

【现在手敲】

```go
// 完整的 pollForToken 函数，展示 slow_down 路径，位于 internal/authflow/flow.go：
func pollForToken(httpClient *http.Client, ghBaseURL, deviceCode string, intervalSecs int) (string, error) {
	endpoint := ghBaseURL + "/login/oauth/access_token"
	body := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	interval := time.Duration(intervalSecs) * time.Second

	for {
		req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body.Encode()))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return "", readErr
		}

		var tr tokenResponse
		if err := json.Unmarshal(data, &tr); err != nil {
			return "", fmt.Errorf("decoding token response: %w", err)
		}

		switch tr.Error {
		case "":
			if tr.AccessToken != "" {
				return tr.AccessToken, nil
			}
			return "", fmt.Errorf("no access token in response")
		case "authorization_pending":
			// continue
		case "slow_down":
			if tr.Interval > 0 {
				interval = time.Duration(tr.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return "", fmt.Errorf("device code expired; please try again")
		default:
			return "", fmt.Errorf("OAuth error: %s", tr.Error)
		}

		time.Sleep(interval)
	}
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -run TestPollForToken_SlowDown -v`

期望输出：
```
=== RUN   TestPollForToken_SlowDown
--- PASS: TestPollForToken_SlowDown (1.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**未登录时 auth status 的 stderr 输出 + SilentError**

`statusRun()` 在无 hosts 时向 `ErrOut`（stderr）写入提示，然后返回 `cmdutil.SilentError`。

`SilentError` 在 `pkg/cmdutil/` 中定义为一个哨兵值：

```go
var SilentError = errors.New("SilentError")
```

`ghcmd.Main()` 检测到 SilentError 时调用 `os.Exit(1)`（非零退出码），但不调用 `fmt.Println(err)`——这样用户只看到 statusRun 写入 stderr 的提示，不看到多余的 "Error: SilentError" 行。

【现在手敲】

```go
// 此代码片段展示 statusRun 中的未登录处理，位于 pkg/cmd/auth/status/status.go：
func statusRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hosts := cfg.Hosts()
	if len(hosts) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "You are not logged in to any GitHub hosts. Run `gh auth login` to authenticate.\n")
		return cmdutil.SilentError
	}

	for _, hostname := range hosts {
		token, err := cfg.AuthToken(hostname)
		if err != nil || token == "" {
			fmt.Fprintf(opts.IO.Out, "%s\n  x Not logged in\n\n", hostname)
			continue
		}

		username, _ := cfg.Get(hostname, "user")
		if username == "" {
			username = "(unknown)"
		}

		fmt.Fprintf(opts.IO.Out, "%s\n  Logged in to %s as %s\n\n", hostname, hostname, username)
	}

	return nil
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./pkg/cmd/auth/status/ -run TestStatusRun_NoHosts -v`

期望输出：
```
=== RUN   TestStatusRun_NoHosts
--- PASS: TestStatusRun_NoHosts (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/status
```

---

### 2.5 测试

**internal/config 的 10 个测试**

`config_test.go` 中的每个测试都通过 `newTestConfig(t)` 创建独立的 `fileConfig`，使用 `t.TempDir()` 获取隔离的临时目录，测试之间不共享状态。

测试覆盖了：空配置、Set/Get 正常路径、Get 未知 host、Set/Get 未知 key、写入并读取文件、AuthToken 不存在/存在、Logout、解析现有 YAML。

【现在手敲】

```go
// 文件：internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestConfig(t *testing.T) *fileConfig {
	t.Helper()
	dir := t.TempDir()
	return &fileConfig{
		hosts: make(map[string]*hostEntry),
		path:  filepath.Join(dir, "hosts.yml"),
	}
}

func TestConfig_EmptyByDefault(t *testing.T) {
	cfg := newTestConfig(t)
	if got := cfg.Hosts(); len(got) != 0 {
		t.Errorf("expected 0 hosts, got %v", got)
	}
}

func TestConfig_SetAndGet(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Set("github.com", "oauth_token", "ghp_test"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := cfg.Set("github.com", "user", "alice"); err != nil {
		t.Fatalf("Set user: %v", err)
	}
	got, err := cfg.Get("github.com", "oauth_token")
	if err != nil || got != "ghp_test" {
		t.Errorf("Get oauth_token = %q, err=%v", got, err)
	}
	got, err = cfg.Get("github.com", "user")
	if err != nil || got != "alice" {
		t.Errorf("Get user = %q, err=%v", got, err)
	}
}

func TestConfig_Get_UnknownHost(t *testing.T) {
	cfg := newTestConfig(t)
	_, err := cfg.Get("missing.host", "user")
	if err == nil {
		t.Error("expected error for unknown hostname")
	}
}

func TestConfig_Set_UnknownKey(t *testing.T) {
	cfg := newTestConfig(t)
	err := cfg.Set("github.com", "bad_key", "value")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestConfig_Get_UnknownKey(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Set("github.com", "oauth_token", "tok"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_, err := cfg.Get("github.com", "bad_key")
	if err == nil {
		t.Error("expected error for unknown key in Get")
	}
}

func TestConfig_WriteAndRead(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Login("github.com", "bob", "ghp_abc123"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	data, err := os.ReadFile(cfg.path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "ghp_abc123") {
		t.Errorf("hosts.yml should contain token, got:\n%s", content)
	}
	if !strings.Contains(content, "bob") {
		t.Errorf("hosts.yml should contain username, got:\n%s", content)
	}
}

func TestConfig_AuthToken_Missing(t *testing.T) {
	cfg := newTestConfig(t)
	_, err := cfg.AuthToken("github.com")
	if err == nil {
		t.Error("expected error for missing token")
	}
}

func TestConfig_AuthToken_Present(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Login("github.com", "user1", "tok123"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	tok, err := cfg.AuthToken("github.com")
	if err != nil {
		t.Fatalf("AuthToken: %v", err)
	}
	if tok != "tok123" {
		t.Errorf("AuthToken = %q, want tok123", tok)
	}
}

func TestConfig_Logout(t *testing.T) {
	cfg := newTestConfig(t)
	if err := cfg.Login("github.com", "user1", "tok123"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := cfg.Logout("github.com"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if hosts := cfg.Hosts(); len(hosts) != 0 {
		t.Errorf("expected 0 hosts after logout, got %v", hosts)
	}
}

func TestConfig_ParsesExistingYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts.yml")
	raw := "github.com:\n    oauth_token: ghp_existing\n    user: charlie\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := newConfigFromPath(path)
	if err != nil {
		t.Fatalf("newConfigFromPath: %v", err)
	}
	tok, err := cfg.AuthToken("github.com")
	if err != nil {
		t.Fatalf("AuthToken: %v", err)
	}
	if tok != "ghp_existing" {
		t.Errorf("token = %q, want ghp_existing", tok)
	}
	user, _ := cfg.Get("github.com", "user")
	if user != "charlie" {
		t.Errorf("user = %q, want charlie", user)
	}
}
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/config/ -v`

期望输出：
```
=== RUN   TestConfig_EmptyByDefault
--- PASS: TestConfig_EmptyByDefault (0.00s)
=== RUN   TestConfig_SetAndGet
--- PASS: TestConfig_SetAndGet (0.00s)
=== RUN   TestConfig_Get_UnknownHost
--- PASS: TestConfig_Get_UnknownHost (0.00s)
=== RUN   TestConfig_Set_UnknownKey
--- PASS: TestConfig_Set_UnknownKey (0.00s)
=== RUN   TestConfig_Get_UnknownKey
--- PASS: TestConfig_Get_UnknownKey (0.00s)
=== RUN   TestConfig_WriteAndRead
--- PASS: TestConfig_WriteAndRead (0.00s)
=== RUN   TestConfig_AuthToken_Missing
--- PASS: TestConfig_AuthToken_Missing (0.00s)
=== RUN   TestConfig_AuthToken_Present
--- PASS: TestConfig_AuthToken_Present (0.00s)
=== RUN   TestConfig_Logout
--- PASS: TestConfig_Logout (0.00s)
=== RUN   TestConfig_ParsesExistingYAML
--- PASS: TestConfig_ParsesExistingYAML (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/config
```

---

**internal/authflow 的 6 个测试**

authflow 测试使用 `httptest.NewServer()` 创建本地 HTTP 服务器，模拟 GitHub 的响应，不需要真实网络连接。`deviceFlow()` 私有函数（小写 d）接受 URL 参数，测试直接注入 `ts.URL`。

6 个测试覆盖：完整 Device Flow 成功路径、authorization_pending 然后成功、expired_token 错误、requestDeviceCode HTTP 错误、FetchUsername 401 错误、slow_down 后成功。

【现在手敲】

```go
// authflow 测试使用 httptest.Server 的关键模式（概念示例，实际测试文件已存在）：
// ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//     switch r.URL.Path {
//     case "/login/device/code":
//         json.NewEncoder(w).Encode(deviceCodeResponse{...})
//     case "/login/oauth/access_token":
//         json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok"})
//     case "/user":
//         json.NewEncoder(w).Encode(userResponse{Login: "alice"})
//     }
// }))
// defer ts.Close()
// result, err := deviceFlow(ts.Client(), ts.URL, ts.URL, ios)
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./internal/authflow/ -v`

期望输出：
```
=== RUN   TestDeviceFlow_EndToEnd
--- PASS: TestDeviceFlow_EndToEnd (0.00s)
=== RUN   TestDeviceFlow_AuthorizationPending
--- PASS: TestDeviceFlow_AuthorizationPending (2.00s)
=== RUN   TestDeviceFlow_ExpiredToken
--- PASS: TestDeviceFlow_ExpiredToken (0.00s)
=== RUN   TestRequestDeviceCode_HTTPError
--- PASS: TestRequestDeviceCode_HTTPError (0.00s)
=== RUN   TestFetchUsername_Unauthorized
--- PASS: TestFetchUsername_Unauthorized (0.00s)
=== RUN   TestPollForToken_SlowDown
--- PASS: TestPollForToken_SlowDown (1.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/authflow
```

---

**pkg/cmd/auth/login 的 5 个测试**

login 测试直接构造 `LoginOptions`（不通过 cobra 解析），注入假的 Config 和 HttpClient：

- `TestLoginRun_WithToken_Success`：token 有效，验证输出 "Logged in to ... as ..."
- `TestLoginRun_WithToken_BadToken`：token 无效（401），验证返回错误
- `TestLoginRun_DeviceFlow_Success`：Device Flow 完整路径，验证 cfg.Login 被调用
- `TestAPIBaseURL`：github.com 映射到 api.github.com，其他主机映射到 /api/v3
- `TestNewCmdLogin_WithToken_ReadsStdin`：通过 cobra 执行，验证 stdin 读取

【现在手敲】

```go
// login 测试的关键模式（测试文件已存在于源码中）：
// opts := &LoginOptions{
//     IO:     ios,
//     Config: func() (cmdutil.Config, error) { return fakeConfig, nil },
//     HttpClient: func() (*http.Client, error) { return ts.Client(), nil },
//     Hostname: "github.com",
//     Token: "ghp_test",
//     WithToken: true,
// }
// err := loginRun(opts)
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./pkg/cmd/auth/login/ -v`

期望输出：
```
=== RUN   TestLoginRun_WithToken_Success
--- PASS: TestLoginRun_WithToken_Success (0.00s)
=== RUN   TestLoginRun_WithToken_BadToken
--- PASS: TestLoginRun_WithToken_BadToken (0.00s)
=== RUN   TestLoginRun_DeviceFlow_Success
--- PASS: TestLoginRun_DeviceFlow_Success (0.00s)
=== RUN   TestAPIBaseURL
--- PASS: TestAPIBaseURL (0.00s)
=== RUN   TestNewCmdLogin_WithToken_ReadsStdin
--- PASS: TestNewCmdLogin_WithToken_ReadsStdin (0.02s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/login
```

---

**pkg/cmd/auth/status 的 3 个测试 + 集成测试**

status 测试覆盖三个场景：
- `TestStatusRun_NoHosts`：空 Config，验证 stderr 输出和 SilentError 返回
- `TestStatusRun_WithHost`：单个已登录 host，验证 stdout 格式
- `TestStatusRun_MultipleHosts`：多个 host，验证所有 host 都被打印

集成测试 `TestIntegration_AuthStatus_NoConfig` 通过真实编译好的二进制运行 `gh auth status`，验证：无配置文件时退出码为 1，stderr 包含提示消息。

【现在手敲】

```go
// status 测试的关键模式（测试文件已存在于源码中）：
// ios, _, _, errBuf := iostreams.Test()
// opts := &StatusOptions{
//     IO: ios,
//     Config: func() (cmdutil.Config, error) { return fakeEmptyConfig, nil },
// }
// err := statusRun(opts)
// if !errors.Is(err, cmdutil.SilentError) { t.Error("expected SilentError") }
// if !strings.Contains(errBuf.String(), "not logged in") { t.Error("expected stderr message") }
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./pkg/cmd/auth/status/ -v`

期望输出：
```
=== RUN   TestStatusRun_NoHosts
--- PASS: TestStatusRun_NoHosts (0.00s)
=== RUN   TestStatusRun_WithHost
--- PASS: TestStatusRun_WithHost (0.00s)
=== RUN   TestStatusRun_MultipleHosts
--- PASS: TestStatusRun_MultipleHosts (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/status
```

---

**集成测试**

集成测试编译真实二进制然后作为子进程运行，验证端到端行为：

【现在手敲】

```go
// 集成测试模式（integration/ 目录中已存在）：
// func TestIntegration_AuthStatus_NoConfig(t *testing.T) {
//     // 编译二进制到临时目录
//     // 设置 GH_CONFIG_DIR 指向不存在的目录
//     // 运行 binary auth status
//     // 验证退出码为 1
//     // 验证 stderr 包含 "not logged in"
// }
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./integration/ -run TestIntegration_AuthStatus_NoConfig -v`

期望输出：
```
=== RUN   TestIntegration_AuthStatus_NoConfig
--- PASS: TestIntegration_AuthStatus_NoConfig (0.30s)
PASS
ok  	github.com/learngh/gh-impl/integration
```

---

**全量测试**

【现在手敲】

```bash
# 在项目根目录运行所有测试
go test ./...
```

【验证】

运行：`cd /d/A/code/claude/gh-learning/cli/src/phase-02-config-auth && go test ./...`

期望输出：
```
ok  	github.com/learngh/gh-impl/integration
ok  	github.com/learngh/gh-impl/internal/authflow
ok  	github.com/learngh/gh-impl/internal/config
ok  	github.com/learngh/gh-impl/internal/factory
ok  	github.com/learngh/gh-impl/internal/ghcmd
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/login
ok  	github.com/learngh/gh-impl/pkg/cmd/auth/status
ok  	github.com/learngh/gh-impl/pkg/cmd/root
ok  	github.com/learngh/gh-impl/pkg/cmd/version
ok  	github.com/learngh/gh-impl/pkg/iostreams
```
