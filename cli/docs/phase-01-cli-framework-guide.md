# Phase 1 学习指南：CLI 框架与 IOStreams

---

## Section 1：全局心智模型（先读这里，再动手）

### 1.1 这个 Phase 做了什么

Phase 1 搭建了整个 gh CLI 的骨架：从 `main()` 入口开始，构建一个持有 IOStreams、Config stub、HttpClient stub 的 Factory，再用这个 Factory 创建 Cobra 根命令，注册 `--version` / `--help` 标志和隐藏的 `version` 子命令，最后把 os.Args 交给 Cobra 执行，并把执行结果翻译成 0/1/2/4 四种退出码返回给操作系统。整个过程不涉及任何真实的 GitHub API 调用，只是把框架立起来、让 `gh --version` 能跑通。

---

### 1.2 在架构中的位置

```
用户输入
   │
   ▼
[main.go] → ghcmd.Main()        ← ★ Phase 1
                │
                ▼
         [factory.New()]          ← ★ Phase 1
                │
                ├── IOStreams     ← ★ Phase 1
                ├── Config stub  ← ★ Phase 1（占位，Phase 2 替换）
                ├── HttpClient stub ← ★ Phase 1（占位）
                └── GitClient nil   ← ★ Phase 1（占位，Phase 6 填入）
                │
                ▼
         [root.NewCmdRoot(f)]     ← ★ Phase 1
                │
                ├── version（隐藏命令）← ★ Phase 1
                └── auth（命令组）   ← ★ Phase 1 注册，Phase 2 实现
```

Phase 1 覆盖了从最顶层的 `main.go` 一直到 Cobra 命令树的构建，是后续所有 Phase 的运行基础。Phase 2 将在 `PersistentPreRunE` 中加入认证检查；Phase 3 将用真实 Config 替换 stub；Phase 6 将填入真实 GitClient。

---

### 1.3 控制流图

一次 `gh --version` 的完整调用链：

```
main()
  └─→ ghcmd.Main()
         ├─→ factory.New(build.Version)      // 构建依赖容器
         ├─→ root.NewCmdRoot(f, ver)         // 构建 Cobra 命令树
         └─→ rootCmd.ExecuteC()              // Cobra 解析 os.Args
                  └─→ rootRun(opts)          // --version flag 触发
                           └─→ fmt.Fprint(opts.Out, opts.VersionInfo)
                                    // 输出: "gh version DEV\nhttps://..."

一次 `gh unknowncmd` 的错误路径：

main()
  └─→ ghcmd.Main()
         └─→ rootCmd.ExecuteC() → err = "unknown command unknowncmd"
                  └─→ printError(stderr, err, cmd)
                           ├─→ fmt.Fprintln(out, err)
                           └─→ fmt.Fprintln(out, cmd.UsageString())
         └─→ return exitError (1)
```

---

### 1.4 数据流图

```
build.Version (string "DEV")
    │
    ▼
factory.New(appVersion string) → *cmdutil.Factory {
    AppVersion:     "DEV",
    ExecutableName: "gh",
    IOStreams:       *iostreams.IOStreams { In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr },
    Config:          func() (cmdutil.Config, error)  // 返回 *stubConfig{}
    HttpClient:      func() (*http.Client, error)    // 返回 &http.Client{}
    GitClient:       nil (any)
}
    │
    ▼
root.NewCmdRoot(f *cmdutil.Factory, ver "DEV") → (*cobra.Command, error)
    │  内部构建：
    │  opts = &RootOptions {
    │      Out:         f.IOStreams.Out  (io.Writer = os.Stdout)
    │      VersionInfo: "gh version DEV\nhttps://..."
    │      ShowVersion: func() bool { v, _ := cmd.Flags().GetBool("version"); return v }
    │      ShowHelp:    cmd.Help  (func() error)
    │  }
    │
    ▼
cobra.ExecuteC() → (cmd *cobra.Command, err error)
    │  成功: err = nil → return exitOK (0)
    │  失败: err ≠ nil → 分类判断
    │      errors.Is(err, cmdutil.SilentError) → exitError (1)
    │      cmdutil.IsUserCancellation(err)     → exitCancel (2)
    │      errors.As(err, &authError)          → exitAuth (4)
    │      其他                                → exitError (1) + printError()
    │
    ▼
os.Exit(int(exitCode))
```

---

### 1.5 与前一个 Phase 的连接

这是 Phase 1，没有前置 Phase。所有代码从零开始构建。

---

## Section 2：逐步实现

所有命令均在以下目录运行：

```
cd D:\A\code\claude\gh-learning\cli\src\phase-01-cli-framework
```

---

### 2.1 数据结构

#### IOStreams（pkg/iostreams/iostreams.go）

为什么不直接用 `os.Stdout`？因为测试时必须替换成内存缓冲区，否则测试无法捕获输出。`IOStreams` 把三个流封装在一起，同时携带 TTY 检测状态（是否终端、是否能交互）。TTY override 机制让测试可以精确控制这些状态，而不依赖实际终端环境。

替代方案：有些 CLI 用全局变量 `var Stdout io.Writer = os.Stdout`，但这在并发测试中不安全，也无法携带 TTY 状态。

【现在手敲】

```go
package iostreams

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/term"
)

// IOStreams holds the standard input/output/error streams used throughout the CLI.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	// TTY state (override mechanism)
	stdinTTYOverride  bool
	stdinIsTTY        bool
	stdoutTTYOverride bool
	stdoutIsTTY       bool

	colorEnabled bool

	neverPrompt bool
}

// ColorEnabled reports whether color output is enabled.
func (s *IOStreams) ColorEnabled() bool {
	return s.colorEnabled
}

// CanPrompt reports whether interactive prompts are allowed.
func (s *IOStreams) CanPrompt() bool {
	if s.neverPrompt {
		return false
	}
	return s.IsStdinTTY() && s.IsStdoutTTY()
}

// IsStdinTTY reports whether standard input is a terminal.
func (s *IOStreams) IsStdinTTY() bool {
	if s.stdinTTYOverride {
		return s.stdinIsTTY
	}
	f, ok := s.In.(*os.File)
	return ok && isTerminal(f)
}

// IsStdoutTTY reports whether standard output is a terminal.
func (s *IOStreams) IsStdoutTTY() bool {
	if s.stdoutTTYOverride {
		return s.stdoutIsTTY
	}
	f, ok := s.Out.(*os.File)
	return ok && isTerminal(f)
}

// SetStdinTTY overrides whether stdin is treated as a terminal.
func (s *IOStreams) SetStdinTTY(v bool) {
	s.stdinTTYOverride = true
	s.stdinIsTTY = v
}

// SetStdoutTTY overrides whether stdout is treated as a terminal.
func (s *IOStreams) SetStdoutTTY(v bool) {
	s.stdoutTTYOverride = true
	s.stdoutIsTTY = v
}

// SetColorEnabled sets whether color output is enabled.
func (s *IOStreams) SetColorEnabled(v bool) {
	s.colorEnabled = v
}

// SetNeverPrompt sets whether interactive prompts are suppressed.
func (s *IOStreams) SetNeverPrompt(v bool) {
	s.neverPrompt = v
}

// System returns an IOStreams connected to the real stdin/stdout/stderr.
func System() *IOStreams {
	return &IOStreams{
		In:           os.Stdin,
		Out:          os.Stdout,
		ErrOut:       os.Stderr,
		colorEnabled: isTerminal(os.Stdout),
	}
}

// Test returns an IOStreams backed by byte buffers for use in tests.
// It returns the IOStreams and in/out/errOut buffers respectively.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	ios := &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
	}
	ios.SetStdinTTY(false)
	ios.SetStdoutTTY(false)
	return ios, in, out, errOut
}

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
```

【验证】
运行：`go test ./pkg/iostreams/ -v -run TestTest_returnsBuffers`
期望输出：
```
=== RUN   TestTest_returnsBuffers
--- PASS: TestTest_returnsBuffers (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/iostreams
```

---

#### Factory（pkg/cmdutil/factory.go）

为什么用 struct 而不是一堆全局变量？struct 可以在测试中独立实例化，不同测试互不干扰。`Config` 和 `HttpClient` 为什么是函数而不是直接的值？因为这两个依赖可能需要延迟初始化（读文件、建连接），用函数可以按需求值，也方便在测试中替换实现。

替代方案：依赖注入框架（如 dig、wire），但对 CLI 来说过重；直接传参到每个命令函数，参数列表会爆炸。

【现在手敲】

```go
package cmdutil

import (
	"net/http"

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
	// GitClient provides git operations. Nil until Phase 6 wires in git.Client.
	GitClient any
}
```

【验证】
运行：`go build ./pkg/cmdutil/`
期望输出：
```
（无输出，表示编译成功）
```

---

#### Config 接口（pkg/cmdutil/config.go）

为什么现在就定义接口？Go 的接口是隐式实现的，先定义接口让 Phase 1 可以使用 stub，Phase 2 可以用真实实现，不需要改调用方代码。`AuthToken`、`Login`、`Logout` 三个方法是 Phase 2 的扩展需求，在这里提前声明好，避免后续修改接口时破坏现有代码。

替代方案：用具体类型，但那样 stub 和真实实现就不能互换；用空接口 `any`，但那样失去编译期类型检查。

【现在手敲】

```go
package cmdutil

// Config defines the interface for reading and writing CLI configuration.
type Config interface {
	// Get returns the value for the given key under the given hostname.
	Get(hostname, key string) (string, error)
	// Set stores the value for the given key under the given hostname.
	Set(hostname, key, value string) error
	// Write persists configuration to disk.
	Write() error
	// Hosts returns all configured hostnames.
	Hosts() []string
	// AuthToken returns the OAuth token for the given hostname.
	AuthToken(hostname string) (string, error)
	// Login stores the authentication credentials for the given hostname.
	Login(hostname, username, token string) error
	// Logout removes the authentication credentials for the given hostname.
	Logout(hostname string) error
}
```

【验证】
运行：`go build ./pkg/cmdutil/`
期望输出：
```
（无输出，表示编译成功）
```

---

#### exitCode 类型（internal/ghcmd/cmd.go 节选）

为什么不直接用 `int`？自定义类型 `type exitCode int` 让编译器帮你区分退出码和普通整数，防止把普通整数意外传给 `os.Exit`。四个具名常量 `exitOK/exitError/exitCancel/exitAuth` 让代码可读——看到 `exitAuth` 就知道是认证失败，不需要去查"4 是什么意思"。

替代方案：直接用字面量 `os.Exit(1)`，散落在各处难以维护；用 `const` 但不用自定义类型，失去类型安全。

【现在手敲】

这段代码是 `internal/ghcmd/cmd.go` 的一部分，完整文件见下文 2.2 节。此处展示类型定义：

```go
type exitCode int

const (
	exitOK     exitCode = 0
	exitError  exitCode = 1
	exitCancel exitCode = 2
	exitAuth   exitCode = 4
)
```

【验证】
运行：`go build ./internal/ghcmd/`
期望输出：
```
（无输出，表示编译成功）
```

---

#### RootOptions / VersionOptions（为什么提取执行逻辑）

为什么把运行逻辑从 `RunE` lambda 中提取到 `xxxRun(opts)` 函数？因为 Cobra 的 `RunE` 是一个闭包，如果直接在里面写逻辑，测试时必须构造整个 Cobra 命令。把逻辑提到 `rootRun(opts *RootOptions)` 后，测试只需构造 `RootOptions`，独立验证分支逻辑，无需触碰 Cobra。`ShowVersion` 和 `ShowHelp` 是函数字段，而不是 bool 值，这让测试可以注入 mock 函数，不依赖实际的 flag 解析结果。

替代方案：直接在 `RunE` 里写逻辑，简单但不可测；把 opts 设为包级变量，破坏并发安全。

【现在手敲】

VersionOptions 在 `pkg/cmd/version/version.go` 中定义：

```go
// VersionOptions holds the dependencies for the version command.
type VersionOptions struct {
	Out        io.Writer
	VersionStr string
}
```

RootOptions 在 `pkg/cmd/root/root.go` 中定义：

```go
// RootOptions holds the dependencies for the root command's run logic.
type RootOptions struct {
	Out         io.Writer
	VersionInfo string
	ShowVersion func() bool
	ShowHelp    func() error
}
```

【验证】
运行：`go build ./pkg/cmd/...`
期望输出：
```
（无输出，表示编译成功）
```

---

#### error 类型（pkg/cmdutil/errors.go）

为什么需要三种不同的 error？因为不同错误需要不同处理：`SilentError` 表示命令已经自己打印了错误，主循环只需退出，不要重复打印；`CancelError` 表示用户主动取消（如 Ctrl+C），退出码不同（2）；`FlagError` 携带完整错误信息，同时触发 usage 打印。用 sentinel error（`errors.New`）而不是字符串比较，是因为 `errors.Is` 可以穿透 wrap 链。

替代方案：用 `os.Exit()` 直接在命令内退出，但那样无法测试，也无法统一处理；用 int 返回值，但 Go 惯例用 error。

【现在手敲】

```go
package cmdutil

import (
	"errors"
	"fmt"
)

// SilentError is an error that signals the CLI should exit with an error code
// but should not print an error message (the command already printed one).
var SilentError = errors.New("silent error")

// CancelError signals that the user cancelled an interactive prompt.
var CancelError = errors.New("cancel error")

// FlagError wraps errors that originate from invalid flag usage.
type FlagError struct {
	Err error
}

func (f *FlagError) Error() string {
	return f.Err.Error()
}

func (f *FlagError) Unwrap() error {
	return f.Err
}

// NewFlagErrorf creates a FlagError with a formatted message.
func NewFlagErrorf(format string, args ...any) *FlagError {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}

// IsUserCancellation reports whether err represents a user cancellation.
func IsUserCancellation(err error) bool {
	return errors.Is(err, CancelError)
}
```

【验证】
运行：`go build ./pkg/cmdutil/`
期望输出：
```
（无输出，表示编译成功）
```

---

### 2.2 核心逻辑

#### factory.New()（internal/factory/factory.go）

依赖构建顺序：先创建 `IOStreams`（无依赖），再填充 `Factory` 的其他字段。`Config` 和 `HttpClient` 是闭包，延迟到调用时才执行。`stubConfig` 实现了完整的 `cmdutil.Config` 接口，所有方法返回 error，这样后续 Phase 替换时不会有编译错误。注意 `GitClient: nil` 明确写出来，而不是依赖零值，是为了让读者知道这是有意为之，不是遗漏。

【现在手敲】

```go
package factory

import (
	"errors"
	"net/http"

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

// New constructs a Factory with sensible defaults for the given version.
// Auth commands override the Config getter with the real config.NewConfig().
func New(appVersion string) *cmdutil.Factory {
	ios := iostreams.System()

	f := &cmdutil.Factory{
		AppVersion:     appVersion,
		ExecutableName: "gh",
		IOStreams:       ios,
		Config: func() (cmdutil.Config, error) {
			return &stubConfig{}, nil
		},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{}, nil
		},
		GitClient: nil,
	}

	return f
}
```

【验证】
运行：`go test ./internal/factory/ -v`
期望输出：
```
=== RUN   TestNew_nonNil
--- PASS: TestNew_nonNil (0.00s)
=== RUN   TestNew_appVersion
--- PASS: TestNew_appVersion (0.00s)
=== RUN   TestNew_executableName
--- PASS: TestNew_executableName (0.00s)
=== RUN   TestNew_ioStreams
--- PASS: TestNew_ioStreams (0.00s)
=== RUN   TestNew_config_returnsStub
--- PASS: TestNew_config_returnsStub (0.00s)
=== RUN   TestNew_config_stub_methods_return_errors
--- PASS: TestNew_config_stub_methods_return_errors (0.00s)
=== RUN   TestNew_httpClient_returnsNonNil
--- PASS: TestNew_httpClient_returnsNonNil (0.00s)
=== RUN   TestNew_gitClient_nil
--- PASS: TestNew_gitClient_nil (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/factory
```

---

#### NewCmdRoot()（pkg/cmd/root/root.go）

Cobra 命令的四个关键字段：`Use`（命令格式字符串）、`Short`（一行帮助）、`Long`（详细帮助）、`Annotations`（附加元数据，这里存放 versionInfo 供测试读取）。`SilenceErrors = true` 让 Cobra 不自动打印错误，由 `ghcmd.Main` 统一处理；`SilenceUsage = true` 让 Cobra 不在每次错误时自动打印用法，由 `printError` 按需打印。

`PersistentPreRunE` 现在是空实现，Phase 2 将在这里加入认证检查。`cmd.AddCommand(auth.NewCmdAuth(f))` 在 Phase 1 就注册了 auth 命令组，因为代码是提前写好的，Phase 2 指南会详细解释 auth 的内部实现。

【现在手敲】

```go
package root

import (
	"fmt"
	"io"

	"github.com/learngh/gh-impl/pkg/cmd/auth"
	"github.com/learngh/gh-impl/pkg/cmd/version"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// AuthError wraps an authentication error so callers can distinguish it from
// generic errors and exit with the auth exit code.
type AuthError struct {
	err error
}

func (ae *AuthError) Error() string {
	return ae.err.Error()
}

// NewAuthError creates an AuthError wrapping the given error.
func NewAuthError(err error) *AuthError {
	return &AuthError{err: err}
}

// RootOptions holds the dependencies for the root command's run logic.
type RootOptions struct {
	Out         io.Writer
	VersionInfo string
	ShowVersion func() bool
	ShowHelp    func() error
}

// NewCmdRoot builds and returns the root cobra command.
func NewCmdRoot(f *cmdutil.Factory, ver string) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "gh <command> <subcommand> [flags]",
		Short: "GitHub CLI",
		Long:  "GitHub CLI\n\nWork seamlessly with GitHub from the command line.",
		Annotations: map[string]string{
			"versionInfo": version.Format(ver),
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Phase 2 will add auth checks here.
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
运行：`go test ./pkg/cmd/root/ -v -run TestNewCmdRoot_returnsCommand`
期望输出：
```
=== RUN   TestNewCmdRoot_returnsCommand
--- PASS: TestNewCmdRoot_returnsCommand (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/root
```

---

#### rootRun()：--version 和 --help 的分支逻辑

`rootRun` 只做一件事：根据 `ShowVersion()` 的返回值决定打印版本还是打印帮助。`ShowVersion` 是一个函数字段而非 bool，这样测试可以直接注入 `func() bool { return true }` 而不需要解析 flag。`ShowHelp` 同理，注入 mock 函数可以测试帮助分支而不依赖 Cobra 的帮助格式。

这段逻辑已包含在上面 `pkg/cmd/root/root.go` 的完整文件中。

【现在手敲】

rootRun 函数体（包含在 root.go 末尾）：

```go
func rootRun(opts *RootOptions) error {
	if opts.ShowVersion() {
		fmt.Fprint(opts.Out, opts.VersionInfo)
		return nil
	}
	return opts.ShowHelp()
}
```

【验证】
运行：`go test ./pkg/cmd/root/ -v -run TestNewCmdRoot_versionOutput`
期望输出：
```
=== RUN   TestNewCmdRoot_versionOutput
--- PASS: TestNewCmdRoot_versionOutput (0.01s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/root
```

---

#### versionRun()：为什么从 opts 读 VersionStr（pkg/cmd/version/version.go）

`versionRun` 从 `opts.VersionStr` 读取版本字符串，而不是直接引用 `build.Version`。原因：命令层（`pkg/cmd/version`）不应该直接依赖内部构建信息（`internal/build`），依赖方向应该是外层调用者把版本号传进来。这让 `version` 包可以独立测试，传入任意版本字符串而不需要修改全局变量。

`Format()` 函数把原始版本字符串（如 "v2.40.0"）格式化成标准输出格式，剥去 `v` 前缀，拼接 release URL。

【现在手敲】

```go
package version

import (
	"fmt"
	"io"
	"strings"

	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// VersionOptions holds the dependencies for the version command.
type VersionOptions struct {
	Out        io.Writer
	VersionStr string
}

// NewCmdVersion returns a hidden cobra command that prints the version string.
func NewCmdVersion(f *cmdutil.Factory, ver string) *cobra.Command {
	opts := &VersionOptions{
		Out:        f.IOStreams.Out,
		VersionStr: Format(ver),
	}
	return &cobra.Command{
		Use:    "version",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return versionRun(opts)
		},
	}
}

func versionRun(opts *VersionOptions) error {
	fmt.Fprint(opts.Out, opts.VersionStr)
	return nil
}

// Format formats a version string into the standard gh version output.
func Format(ver string) string {
	ver = strings.TrimPrefix(ver, "v")
	return fmt.Sprintf("gh version %s\nhttps://github.com/cli/cli/releases/tag/v%s\n", ver, ver)
}
```

【验证】
运行：`go test ./pkg/cmd/version/ -v`
期望输出：
```
=== RUN   TestFormat_devVersion
--- PASS: TestFormat_devVersion (0.00s)
=== RUN   TestFormat_tagged
--- PASS: TestFormat_tagged (0.00s)
=== RUN   TestFormat_noVPrefix
--- PASS: TestFormat_noVPrefix (0.00s)
=== RUN   TestNewCmdVersion_properties
--- PASS: TestNewCmdVersion_properties (0.00s)
=== RUN   TestNewCmdVersion_output
--- PASS: TestNewCmdVersion_output (0.02s)
=== RUN   TestNewCmdVersion_devBuild
--- PASS: TestNewCmdVersion_devBuild (0.02s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/version
```

---

#### Main()：error 分类与退出码映射（internal/ghcmd/cmd.go）

`Main()` 是整个 CLI 的中枢。它的职责：构建依赖（factory）、构建命令树（root）、执行命令（ExecuteC）、翻译错误为退出码。`ExecuteC()` 比 `Execute()` 多返回执行的那个 `*cobra.Command`，让 `printError` 能打印该命令的 usage，而不是根命令的 usage。

错误分类的优先级：`SilentError` 最先检查（命令已自行处理），然后是 `CancelError`（用户取消），然后是 `AuthError`（认证失败），最后才是未知错误（打印后退出）。

`build.Version` 来自 `internal/build/build.go`，默认值是 `"DEV"`，生产构建通过 ldflags 覆盖。

【现在手敲】

```go
package ghcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/learngh/gh-impl/internal/build"
	"github.com/learngh/gh-impl/internal/factory"
	"github.com/learngh/gh-impl/pkg/cmd/root"
	"github.com/learngh/gh-impl/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type exitCode int

const (
	exitOK     exitCode = 0
	exitError  exitCode = 1
	exitCancel exitCode = 2
	exitAuth   exitCode = 4
)

// Main is the primary entry point for the gh CLI. It returns an exit code that
// the caller (main.go) passes directly to os.Exit.
func Main() exitCode {
	buildVersion := build.Version
	cmdFactory := factory.New(buildVersion)
	stderr := cmdFactory.IOStreams.ErrOut

	rootCmd, err := root.NewCmdRoot(cmdFactory, buildVersion)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create root command: %s\n", err)
		return exitError
	}

	rootCmd.SetArgs(os.Args[1:])

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		var authError *root.AuthError
		if errors.Is(err, cmdutil.SilentError) {
			return exitError
		} else if cmdutil.IsUserCancellation(err) {
			return exitCancel
		} else if errors.As(err, &authError) {
			return exitAuth
		}
		printError(stderr, err, cmd)
		return exitError
	}
	return exitOK
}

// printError writes err to out. When the error looks like an unknown command or
// a flag error, the command's usage string is also printed.
func printError(out io.Writer, err error, cmd *cobra.Command) {
	fmt.Fprintln(out, err)

	var flagError *cmdutil.FlagError
	if errors.As(err, &flagError) || strings.HasPrefix(err.Error(), "unknown command ") {
		if !strings.HasSuffix(err.Error(), "\n") {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, cmd.UsageString())
	}
}
```

【验证】
运行：`go test ./internal/ghcmd/ -v`
期望输出：
```
=== RUN   TestPrintError_plainError
--- PASS: TestPrintError_plainError (0.00s)
=== RUN   TestPrintError_flagError_printsUsage
--- PASS: TestPrintError_flagError_printsUsage (0.00s)
=== RUN   TestPrintError_unknownCommand_printsUsage
--- PASS: TestPrintError_unknownCommand_printsUsage (0.00s)
=== RUN   TestPrintError_flagError_noTrailingNewline
--- PASS: TestPrintError_flagError_noTrailingNewline (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/ghcmd
```

---

#### printError()：何时打印 usage

`printError` 只在两种情况下打印 usage：`FlagError`（无效 flag）和 "unknown command " 前缀的错误。普通错误（如网络失败、API 返回错误）不打印 usage，因为 usage 对用户毫无帮助。

`strings.HasPrefix(err.Error(), "unknown command ")` 是对 Cobra 内部错误消息格式的依赖，这是有意为之——Cobra 不提供类型化的 "unknown command" error，所以只能字符串匹配。

这段逻辑已包含在上面 `internal/ghcmd/cmd.go` 的完整文件中。

【现在手敲】

printError 函数体（包含在 cmd.go 末尾）：

```go
func printError(out io.Writer, err error, cmd *cobra.Command) {
	fmt.Fprintln(out, err)

	var flagError *cmdutil.FlagError
	if errors.As(err, &flagError) || strings.HasPrefix(err.Error(), "unknown command ") {
		if !strings.HasSuffix(err.Error(), "\n") {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, cmd.UsageString())
	}
}
```

【验证】
运行：`go test ./internal/ghcmd/ -v -run TestPrintError_flagError_printsUsage`
期望输出：
```
=== RUN   TestPrintError_flagError_printsUsage
--- PASS: TestPrintError_flagError_printsUsage (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/ghcmd
```

---

### 2.3 接线（Wiring）

#### main.go → ghcmd.Main() → factory.New() → root.NewCmdRoot() → cobra.Execute() 全链路

这是整个程序的入口文件，极其简单：调用 `ghcmd.Main()` 获取退出码，然后用 `os.Exit` 退出。为什么不直接在 `main()` 里写逻辑？因为 `main()` 不能被测试调用，把所有逻辑放在 `ghcmd.Main()` 里让集成测试可以在不启动子进程的情况下测试大部分逻辑。

`os.Exit(int(code))` 的类型转换：`exitCode` 是自定义类型，`os.Exit` 要求 `int`，显式转换让读者看到这里有类型边界。

【现在手敲】

```go
package main

import (
	"os"

	"github.com/learngh/gh-impl/internal/ghcmd"
)

func main() {
	code := ghcmd.Main()
	os.Exit(int(code))
}
```

【验证】
运行：`go build -o bin/gh.exe ./cmd/gh/ && bin/gh.exe --version`
期望输出：
```
gh version DEV
https://github.com/cli/cli/releases/tag/vDEV
```

---

#### build.go（internal/build/build.go）

`Version` 和 `Date` 是包级变量，默认值分别是 `"DEV"` 和 `""`。生产构建时通过 `-ldflags "-X github.com/learngh/gh-impl/internal/build.Version=2.40.0"` 覆盖。这是 Go 生态中注入构建信息的标准做法，不需要在源码中硬编码版本号。

【现在手敲】

```go
package build

// Version is the current version of gh. It is set at build time via ldflags.
var Version = "DEV"

// Date is the build date of gh. It is set at build time via ldflags.
var Date = ""
```

【验证】
运行：`go build -o bin/gh.exe ./cmd/gh/ && bin/gh.exe --version`
期望输出：
```
gh version DEV
https://github.com/cli/cli/releases/tag/vDEV
```

---

### 2.4 错误路径

#### SilentError：已打印错误，只退出

当命令内部已经打印了错误信息（例如，HTTP 请求失败后命令自己 `fmt.Fprintf(ios.ErrOut, "error: %s\n", err)` 然后 `return cmdutil.SilentError`），`Main()` 检测到 `SilentError` 后直接返回 `exitError(1)`，不再打印任何内容，避免重复输出。

`errors.Is(err, cmdutil.SilentError)` 可以穿透 wrap 链，即使命令用 `fmt.Errorf("...: %w", cmdutil.SilentError)` 包装也能识别。

为什么选择 0/1/2/4 而不是 0/1/2/3？因为 exit code 4 是 `gh` 历史上为认证失败保留的值，与 gh CLI 官方保持一致。

【现在手敲】

SilentError 处理逻辑（已包含在 `internal/ghcmd/cmd.go` 中，此处展示关键分支）：

```go
if cmd, err := rootCmd.ExecuteC(); err != nil {
    var authError *root.AuthError
    if errors.Is(err, cmdutil.SilentError) {
        return exitError
    } else if cmdutil.IsUserCancellation(err) {
        return exitCancel
    } else if errors.As(err, &authError) {
        return exitAuth
    }
    printError(stderr, err, cmd)
    return exitError
}
return exitOK
```

【验证】
运行：`go test ./internal/ghcmd/ -v -run TestPrintError_plainError`
期望输出：
```
=== RUN   TestPrintError_plainError
--- PASS: TestPrintError_plainError (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/ghcmd
```

---

#### CancelError：用户取消

`CancelError` 用于交互式提示被取消（如用户在选择菜单时按 Ctrl+C）。退出码 2 与 shell 的 SIGINT 约定一致，让脚本可以区分"命令成功退出"和"用户取消"。

`cmdutil.IsUserCancellation(err)` 内部是 `errors.Is(err, CancelError)`，提供语义清晰的 API。

这段处理逻辑已包含在上面展示的 `ExecuteC()` 错误分类代码块中。

【现在手敲】

CancelError 定义（已包含在 `pkg/cmdutil/errors.go` 中，此处展示定义和辅助函数）：

```go
// CancelError signals that the user cancelled an interactive prompt.
var CancelError = errors.New("cancel error")

// IsUserCancellation reports whether err represents a user cancellation.
func IsUserCancellation(err error) bool {
	return errors.Is(err, CancelError)
}
```

【验证】
运行：`go build ./pkg/cmdutil/`
期望输出：
```
（无输出，表示编译成功）
```

---

#### AuthError：认证失败

`AuthError` 是一个结构体类型（不是 sentinel），因为它需要携带具体的错误信息（如 "not authenticated with github.com"）。`errors.As(err, &authError)` 用类型断言匹配，能穿透 wrap 链。退出码 4 让 CI 脚本可以区分认证失败和其他错误。

`NewAuthError` 构造函数提供统一的创建方式，Phase 2 的 auth 检查将在 `PersistentPreRunE` 中调用它。

【现在手敲】

AuthError 定义（已包含在 `pkg/cmd/root/root.go` 中，此处展示类型定义和构造函数）：

```go
// AuthError wraps an authentication error so callers can distinguish it from
// generic errors and exit with the auth exit code.
type AuthError struct {
	err error
}

func (ae *AuthError) Error() string {
	return ae.err.Error()
}

// NewAuthError creates an AuthError wrapping the given error.
func NewAuthError(err error) *AuthError {
	return &AuthError{err: err}
}
```

【验证】
运行：`go test ./pkg/cmd/root/ -v -run TestAuthError_message`
期望输出：
```
=== RUN   TestAuthError_message
--- PASS: TestAuthError_message (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/root
```

---

#### FlagError + unknown command：打印 usage

这两种错误都意味着用户"用错了"CLI，所以在打印错误之后还要打印 usage，帮助用户纠正。`printError` 检查两个条件：`errors.As(err, &flagError)` 匹配 FlagError，`strings.HasPrefix(err.Error(), "unknown command ")` 匹配 Cobra 的 unknown command 错误。

打印 usage 前有个换行检查：如果错误消息本身不以换行结尾，先加一个空行再打印 usage，保持视觉间距。

这段处理逻辑已包含在上面展示的 `printError` 函数中。

【现在手敲】

printError 完整函数（已包含在 `internal/ghcmd/cmd.go` 中）：

```go
func printError(out io.Writer, err error, cmd *cobra.Command) {
	fmt.Fprintln(out, err)

	var flagError *cmdutil.FlagError
	if errors.As(err, &flagError) || strings.HasPrefix(err.Error(), "unknown command ") {
		if !strings.HasSuffix(err.Error(), "\n") {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, cmd.UsageString())
	}
}
```

【验证】
运行：`go test ./internal/ghcmd/ -v -run TestPrintError_unknownCommand_printsUsage`
期望输出：
```
=== RUN   TestPrintError_unknownCommand_printsUsage
--- PASS: TestPrintError_unknownCommand_printsUsage (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/ghcmd
```

---

#### 未知错误：只打印错误

对于既不是 SilentError、也不是 CancelError、也不是 AuthError、也不是 FlagError、也不是 unknown command 的错误，`printError` 只打印错误消息本身（通过 `fmt.Fprintln(out, err)`），不打印 usage。因为 usage 对"内部错误"没有帮助。

【现在手敲】

printError 的普通错误路径（已包含在上面的完整函数中，此处重申完整调用链）：

```go
// 在 Main() 中：
printError(stderr, err, cmd)
return exitError

// printError 对普通错误：
func printError(out io.Writer, err error, cmd *cobra.Command) {
	fmt.Fprintln(out, err)
	// FlagError 和 unknown command 的 if 分支不满足，直接返回
}
```

【验证】
运行：`go test ./internal/ghcmd/ -v -run TestPrintError_plainError`
期望输出：
```
=== RUN   TestPrintError_plainError
--- PASS: TestPrintError_plainError (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/ghcmd
```

---

### 2.5 测试

#### IOStreams 单元测试（pkg/iostreams/iostreams_test.go）

测试文件在独立包 `iostreams_test` 中（注意不是 `iostreams`），这是 Go 的 black-box 测试约定，只测试导出的 API，不能访问私有字段。`Test()` 函数是测试的基础设施，每个测试用例都从它获取隔离的 IOStreams 实例。

为什么测试 TTY override 而不是真实 TTY？因为测试环境不保证有终端，用 override 让测试结果确定性。

【现在手敲】

```go
package iostreams_test

import (
	"strings"
	"testing"

	"github.com/learngh/gh-impl/pkg/iostreams"
)

func TestTest_returnsBuffers(t *testing.T) {
	ios, in, out, errOut := iostreams.Test()
	if ios == nil {
		t.Fatal("expected non-nil IOStreams")
	}
	if in == nil || out == nil || errOut == nil {
		t.Fatal("expected non-nil buffers")
	}
}

func TestTest_isNotTTY(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.IsStdinTTY() {
		t.Error("expected stdin to not be a TTY in test mode")
	}
	if ios.IsStdoutTTY() {
		t.Error("expected stdout to not be a TTY in test mode")
	}
}

func TestTest_canPrompt_false(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.CanPrompt() {
		t.Error("expected CanPrompt() to return false in test mode")
	}
}

func TestSetNeverPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetNeverPrompt(true)
	if ios.CanPrompt() {
		t.Error("expected CanPrompt() false when NeverPrompt is set")
	}
}

func TestSetStdinTTY_override(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	if !ios.IsStdinTTY() {
		t.Error("expected IsStdinTTY() true after override")
	}
	ios.SetStdinTTY(false)
	if ios.IsStdinTTY() {
		t.Error("expected IsStdinTTY() false after override set to false")
	}
}

func TestSetStdoutTTY_override(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	if !ios.IsStdoutTTY() {
		t.Error("expected IsStdoutTTY() true after override")
	}
}

func TestColorEnabled(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	if ios.ColorEnabled() {
		t.Error("expected color disabled by default in test mode")
	}
	ios.SetColorEnabled(true)
	if !ios.ColorEnabled() {
		t.Error("expected color enabled after SetColorEnabled(true)")
	}
}

func TestSystem_fields(t *testing.T) {
	ios := iostreams.System()
	if ios.In == nil {
		t.Error("expected non-nil In")
	}
	if ios.Out == nil {
		t.Error("expected non-nil Out")
	}
	if ios.ErrOut == nil {
		t.Error("expected non-nil ErrOut")
	}
}

func TestIOStreams_readWrite(t *testing.T) {
	ios, in, out, errOut := iostreams.Test()

	in.WriteString("hello")
	buf := make([]byte, 5)
	n, err := ios.In.Read(buf)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("got %q, want %q", string(buf[:n]), "hello")
	}

	ios.Out.Write([]byte("world"))
	if out.String() != "world" {
		t.Errorf("got %q, want %q", out.String(), "world")
	}

	ios.ErrOut.Write([]byte("error!"))
	if !strings.Contains(errOut.String(), "error!") {
		t.Errorf("errOut did not contain expected string")
	}
}
```

【验证】
运行：`go test ./pkg/iostreams/ -v`
期望输出：
```
=== RUN   TestTest_returnsBuffers
--- PASS: TestTest_returnsBuffers (0.00s)
=== RUN   TestTest_isNotTTY
--- PASS: TestTest_isNotTTY (0.00s)
=== RUN   TestTest_canPrompt_false
--- PASS: TestTest_canPrompt_false (0.00s)
=== RUN   TestSetNeverPrompt
--- PASS: TestSetNeverPrompt (0.00s)
=== RUN   TestSetStdinTTY_override
--- PASS: TestSetStdinTTY_override (0.00s)
=== RUN   TestSetStdoutTTY_override
--- PASS: TestSetStdoutTTY_override (0.00s)
=== RUN   TestColorEnabled
--- PASS: TestColorEnabled (0.00s)
=== RUN   TestSystem_fields
--- PASS: TestSystem_fields (0.00s)
=== RUN   TestIOStreams_readWrite
--- PASS: TestIOStreams_readWrite (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/iostreams
```

---

#### factory 单元测试（internal/factory/factory_test.go）

factory 测试验证 `New()` 的所有字段契约：版本字符串、可执行文件名、IOStreams 非 nil、Config getter 返回 stub（stub 方法均返回 error）、HttpClient getter 返回非 nil 客户端、GitClient 在 Phase 1 为 nil。

测试在 `factory_test` 包（black-box），只能通过导出 API 验证。

【现在手敲】

```go
package factory_test

import (
	"testing"

	"github.com/learngh/gh-impl/internal/factory"
)

func TestNew_nonNil(t *testing.T) {
	f := factory.New("1.0.0")
	if f == nil {
		t.Fatal("expected non-nil Factory")
	}
}

func TestNew_appVersion(t *testing.T) {
	f := factory.New("2.40.0")
	if f.AppVersion != "2.40.0" {
		t.Errorf("AppVersion = %q, want %q", f.AppVersion, "2.40.0")
	}
}

func TestNew_executableName(t *testing.T) {
	f := factory.New("1.0.0")
	if f.ExecutableName != "gh" {
		t.Errorf("ExecutableName = %q, want %q", f.ExecutableName, "gh")
	}
}

func TestNew_ioStreams(t *testing.T) {
	f := factory.New("1.0.0")
	if f.IOStreams == nil {
		t.Error("expected non-nil IOStreams")
	}
}

func TestNew_config_returnsStub(t *testing.T) {
	f := factory.New("1.0.0")
	if f.Config == nil {
		t.Fatal("Config getter is nil")
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("Config() returned nil config")
	}
}

func TestNew_config_stub_methods_return_errors(t *testing.T) {
	f := factory.New("1.0.0")
	cfg, _ := f.Config()

	if _, err := cfg.Get("github.com", "token"); err == nil {
		t.Error("stub Config.Get should return error")
	}
	if err := cfg.Set("github.com", "token", "val"); err == nil {
		t.Error("stub Config.Set should return error")
	}
	if err := cfg.Write(); err == nil {
		t.Error("stub Config.Write should return error")
	}
}

func TestNew_httpClient_returnsNonNil(t *testing.T) {
	f := factory.New("1.0.0")
	if f.HttpClient == nil {
		t.Fatal("HttpClient getter is nil")
	}
	client, err := f.HttpClient()
	if err != nil {
		t.Fatalf("HttpClient() returned unexpected error: %v", err)
	}
	if client == nil {
		t.Error("HttpClient() returned nil client")
	}
}

func TestNew_gitClient_nil(t *testing.T) {
	f := factory.New("1.0.0")
	if f.GitClient != nil {
		t.Error("expected GitClient to be nil in Phase 1")
	}
}
```

【验证】
运行：`go test ./internal/factory/ -v`
期望输出：
```
=== RUN   TestNew_nonNil
--- PASS: TestNew_nonNil (0.00s)
=== RUN   TestNew_appVersion
--- PASS: TestNew_appVersion (0.00s)
=== RUN   TestNew_executableName
--- PASS: TestNew_executableName (0.00s)
=== RUN   TestNew_ioStreams
--- PASS: TestNew_ioStreams (0.00s)
=== RUN   TestNew_config_returnsStub
--- PASS: TestNew_config_returnsStub (0.00s)
=== RUN   TestNew_config_stub_methods_return_errors
--- PASS: TestNew_config_stub_methods_return_errors (0.00s)
=== RUN   TestNew_httpClient_returnsNonNil
--- PASS: TestNew_httpClient_returnsNonNil (0.00s)
=== RUN   TestNew_gitClient_nil
--- PASS: TestNew_gitClient_nil (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/factory
```

---

#### root 单元测试（pkg/cmd/root/root_test.go）

root 测试覆盖：命令元数据（Use、Short、Annotations）、Silence 配置（SilenceErrors/SilenceUsage 必须为 true）、子命令注册（version 子命令存在）、标志注册（--help、--version）、完整输出测试（`--version` 输出版本、`--help` 输出帮助）、错误路径（unknown command 返回 error）、AuthError 类型（消息透传）。

这些测试构造 `*cmdutil.Factory` 时直接用字面量，不调用 `factory.New()`，因为测试只需 IOStreams，不需要真实 Config 或 HttpClient。

【现在手敲】

root_test.go 的内容对应以下测试（完整测试文件按项目中的实现）：

```
TestNewCmdRoot_returnsCommand       - NewCmdRoot 返回非 nil 命令
TestNewCmdRoot_useString            - Use 字段等于 "gh <command> <subcommand> [flags]"
TestNewCmdRoot_shortDescription     - Short 字段等于 "GitHub CLI"
TestNewCmdRoot_versionAnnotation    - Annotations["versionInfo"] 包含 "gh version"
TestNewCmdRoot_silenceErrors        - SilenceErrors 和 SilenceUsage 均为 true
TestNewCmdRoot_hasVersionSubcommand - 子命令列表中包含 "version"
TestNewCmdRoot_helpFlag             - --help 标志已注册
TestNewCmdRoot_versionFlag          - --version 标志已注册
TestNewCmdRoot_versionOutput        - 执行 --version 后 Out 包含版本字符串
TestNewCmdRoot_helpOutput           - 执行 --help 后 Out 包含 "GitHub CLI"
TestNewCmdRoot_unknownCommand       - 执行未知子命令返回 error
TestAuthError_message               - AuthError.Error() 返回内部 error 的消息
```

【验证】
运行：`go test ./pkg/cmd/root/ -v`
期望输出：
```
=== RUN   TestNewCmdRoot_returnsCommand
--- PASS: TestNewCmdRoot_returnsCommand (0.00s)
=== RUN   TestNewCmdRoot_useString
--- PASS: TestNewCmdRoot_useString (0.00s)
=== RUN   TestNewCmdRoot_shortDescription
--- PASS: TestNewCmdRoot_shortDescription (0.00s)
=== RUN   TestNewCmdRoot_versionAnnotation
--- PASS: TestNewCmdRoot_versionAnnotation (0.00s)
=== RUN   TestNewCmdRoot_silenceErrors
--- PASS: TestNewCmdRoot_silenceErrors (0.00s)
=== RUN   TestNewCmdRoot_hasVersionSubcommand
--- PASS: TestNewCmdRoot_hasVersionSubcommand (0.00s)
=== RUN   TestNewCmdRoot_helpFlag
--- PASS: TestNewCmdRoot_helpFlag (0.00s)
=== RUN   TestNewCmdRoot_versionFlag
--- PASS: TestNewCmdRoot_versionFlag (0.00s)
=== RUN   TestNewCmdRoot_versionOutput
--- PASS: TestNewCmdRoot_versionOutput (0.01s)
=== RUN   TestNewCmdRoot_helpOutput
--- PASS: TestNewCmdRoot_helpOutput (0.01s)
=== RUN   TestNewCmdRoot_unknownCommand
--- PASS: TestNewCmdRoot_unknownCommand (0.01s)
=== RUN   TestAuthError_message
--- PASS: TestAuthError_message (0.00s)
PASS
ok  	github.com/learngh/gh-impl/pkg/cmd/root
```

---

#### ghcmd 单元测试（internal/ghcmd/cmd_test.go）

ghcmd 测试专注于 `printError` 的行为：普通错误只打印错误消息；FlagError 额外打印 usage；unknown command 错误额外打印 usage；FlagError 消息不以换行结尾时正确插入空行。

`Main()` 本身不做单元测试（因为它依赖 `os.Args` 和真实 Factory），而是通过集成测试 `TestIntegration_Version` 覆盖。

【现在手敲】

ghcmd_test.go 的内容对应以下测试：

```
TestPrintError_plainError                  - 普通 error 只打印消息，不打印 usage
TestPrintError_flagError_printsUsage       - FlagError 打印消息后打印 usage
TestPrintError_unknownCommand_printsUsage  - "unknown command xxx" 打印消息后打印 usage
TestPrintError_flagError_noTrailingNewline - 消息无换行时插入空行再打印 usage
```

【验证】
运行：`go test ./internal/ghcmd/ -v`
期望输出：
```
=== RUN   TestPrintError_plainError
--- PASS: TestPrintError_plainError (0.00s)
=== RUN   TestPrintError_flagError_printsUsage
--- PASS: TestPrintError_flagError_printsUsage (0.00s)
=== RUN   TestPrintError_unknownCommand_printsUsage
--- PASS: TestPrintError_unknownCommand_printsUsage (0.00s)
=== RUN   TestPrintError_flagError_noTrailingNewline
--- PASS: TestPrintError_flagError_noTrailingNewline (0.00s)
PASS
ok  	github.com/learngh/gh-impl/internal/ghcmd
```

---

#### 所有测试一次性验证

【现在手敲】

（无需手敲新代码，这是对已写代码的全量验证命令）

【验证】
运行：`go test ./...`
期望输出：
```
ok  	github.com/learngh/gh-impl/internal/factory
ok  	github.com/learngh/gh-impl/internal/ghcmd
ok  	github.com/learngh/gh-impl/pkg/cmd/root
ok  	github.com/learngh/gh-impl/pkg/cmd/version
ok  	github.com/learngh/gh-impl/pkg/iostreams
```

---

## Section 3：运行效果

完成所有代码后，运行以下命令验证最终效果：

```bash
go build -o bin/gh.exe ./cmd/gh/
bin/gh.exe --version
```

期望输出：
```
gh version DEV
https://github.com/cli/cli/releases/tag/vDEV
```

```bash
bin/gh.exe --help
```

期望输出（部分）：
```
GitHub CLI

Work seamlessly with GitHub from the command line.

Usage:
  gh <command> <subcommand> [flags]

Available Commands:
  auth        Authenticate gh and git with GitHub
  version     

Flags:
      --help      Show help for command
      --version   Show gh version

Use "gh <command> --help" for more information about a command.
```
