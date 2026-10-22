# gip

[English](README.md) | 简体中文

`gip` 用于管理 Go 工具链。它把多个 Go 版本并行安装到本地，并运行模块所要求的那个版本对应的 `go` 命令。

与 Go 1.21 引入的 `GOTOOLCHAIN` 机制不同，`gip` 不要求机器上已经装有较新的 Go 工具链：它自己下载所需要的版本。因此它可以在系统 Go 早于 1.21 的机器上工作，同时把工具链的选择权完全交给用户。

## 安装

```console
$ go install github.com/go-comm/gip/cmd/gip@latest
```

刚发版时校验库可能还不知道这个新版本，安装会报 `invalid version: unknown revision`。版本本身没问题，直接装即可：

```console
$ GONOSUMDB=github.com/go-comm/* go install github.com/go-comm/gip/cmd/gip@v0.2.0
```

## 使用

`gip` 是 `go` 的即插即用替代品：凡是不属于 gip 自己的命令，都会原样交给所选版本的 `go` 命令执行。

```console
$ gip install 1.22.0        # 下载并安装一个版本
$ gip use 1.22.0            # 把它设为当前版本
$ gip ls                    # 列出已安装的版本
$ gip test ./...            # 用正确的工具链执行 go test ./...
$ gip build -o bin/app ./cmd/...
$ gip mod tidy
```

### gip 自己的命令

gip 自己负责管理工具链：

| 命令                                 | 含义                     |
| ------------------------------------ | ------------------------ |
| `gip`                                | 把工具链与当前项目对齐   |
| `gip auto`                           | 同上，完整写法           |
| `gip exec <命令> [参数]`             | 用该工具链运行一条命令   |
| `gip doctor`                         | 体检环境配置             |
| `gip toolchain install <version>`    | 下载并安装一个版本       |
| `gip toolchain use <version>`        | 把某个版本设为当前版本   |
| `gip toolchain ls`                   | 列出已安装的版本         |
| `gip toolchain current`              | 打印当前版本             |
| `gip toolchain uninstall <version>`  | 卸载一个已安装的版本     |
| `gip toolchain env`                  | 打印 gip 管理的路径      |

其中不与 go 命令重名的，也可以省略命名空间直接使用：`gip use`、`gip ls`、`gip current` 和 `gip uninstall`。

`gip ls --all` 还会列出可以安装的版本。

有两个名字同时属于两边，靠参数来区分：

* `gip install` 的参数是版本号时安装工具链，否则执行 `go install`。因此 `gip install 1.22.0` 安装一个版本，而 `gip install golang.org/x/tools/cmd/stringer@latest` 安装一个包。
* `gip env` 不带参数时打印 gip 管理的路径，带参数时执行 `go env`。因此 `gip env GOROOT` 会报告所选版本的 GOROOT。

`gip version` 先打印 gip 自身的版本，再打印当前会使用的版本。

其余命令都属于 go 命令，会被原样转发：

```console
$ gip test ./...
$ gip vet ./...
$ gip fmt ./...
$ gip mod tidy
$ gip get ./...
$ gip run ./cmd/app
$ gip doc fmt
$ gip help build
```

gip 自己的参数可以在命令行任意位置出现；go 命令的参数则原样保留并传递给 go。

参数可以写成单横线或双横线，这一点与 go 命令的 flag 包一致：

```console
$ gip -go=1.20 run ./cmd/app
$ gip --go=1.20 run ./cmd/app
$ gip -go 1.20 run ./cmd/app
```

需要注意的是，Windows PowerShell 5.1 会把带等号的参数拆开，因此在该版本中必须加引号：

```console
> gip "-go=1.20" run ./cmd/app
```

PowerShell 7 以及其他 shell 会原样传递。

### 与项目对齐

不带命令，或者使用 `gip auto` 时，gip 会查看当前目录的 `go.mod` 或 `go.work` 文件，并把工具链与之对齐：

```console
$ cd ~/code/myproject
$ gip
switched to go1.21.13
/home/user/code/myproject/go.mod asks for go1.21.0
```

模块要求的版本若尚未安装会被下载，其专属目录会被准备好，当前版本也会切换过去。由于 `current` 下的链接同时被更新，之后 PATH 上指向 `~/.gip/current/goroot/bin` 的 `go` 就是正确的那个。

切换到要求另一个版本的项目再执行一次 `gip`，即可切换过去，两个方向都支持。

### 如何选择版本

版本按以下顺序确定：

1. 显式的 `--go <version>` 参数优先级最高，精确选中该版本。
2. 当前目录及其父目录中的 `go.work` 文件优先于 `go.mod` 文件。
3. 在同一个文件内，`toolchain` 指令优先于 `go` 指令，除非它指定的是更旧的版本。
4. `toolchain` 指令指定的是某一个确切的版本，而 `go` 指令表示的是*最低*要求：同一系列中不低于它的版本都满足要求。
5. `toolchain default` 表示"使用默认版本"，此时忽略 `go` 指令，也不会触发下载。

当前版本不参与这个选择：模块要求更旧的工具链就会得到更旧的版本，即使当前选中的是更新的版本。`gip use` 只决定在模块没有提出要求时使用哪个版本。

如果选中的版本尚未安装，`gip` 会自动下载。也可以加上 `--no-download` 让它在缺失时直接报错，这在 CI 中很有用。

如果既没有 `go.mod` 也没有 `go.work`，并且没有用 `gip use` 选定过版本，则使用已安装的最新版本。

### 版本号写法

版本号不写 `go` 前缀，例如 `1.22.0`。

* 完整版本号（如 `1.22.0`）精确选中该版本。
* 不带补丁号的版本号（如 `1.22`）选中该系列中最新的版本，因此 `gip install 1.22` 会安装最新的 1.22.x。

判断哪个版本满足要求的规则与 Go 标准库保持一致：

```
1.21 < 1.21rc1 < 1.21.0
```

语言版本 `1.21` 排在候选版和正式版*之下*；而在 Go 1.21 之前缺少补丁号等同于 `0`，所以 `1.20` 与 `1.20.0` 是同一个版本。

## 目录结构

所有文件都在 `~/.gip` 下；如果设置了 `$GIP_HOME`，则以它为准：

```
~/.gip/
├── versions/
│   ├── 1.19.10/          <- 该版本的 GOROOT
│   └── 1.22.0/
├── gobin/
│   ├── 1.19.10/          <- 该版本的 GOBIN
│   └── 1.22.0/
├── gocache/
│   ├── 1.19.10/          <- 该版本的 GOCACHE
│   └── 1.22.0/
└── current/
    ├── gobin  -> ../gobin/1.22.0
    └── goroot -> ../versions/1.22.0
```

`GOROOT` 始终指向 `versions/<version>`，绝不会指向 `current` 目录本身——那个目录里只有链接，没有 `src`、`pkg`、`lib` 目录。例外是其中的 `goroot` 链接：它指向一个完整的 GOROOT，正是给需要 GOROOT 的工具用的。这些链接是给那些把 `~/.gip/current/goroot/bin` 加进 `PATH` 的人提供便利。

由 gip 执行的每一条 go 命令，都会固定所选版本的 `GOROOT`、`GOBIN`、`GOCACHE`，并把该版本的 bin 目录放到 `PATH` 前面。构建缓存按版本隔离，因为跨工具链版本共享缓存可能让产物出现难以察觉的差异。

`current` 下的两个链接是一起切换的：它们先以临时名字就位，再重命名到目标位置；如果重命名失败，之前的版本会被恢复，因此始终保持可用。

## 第三方工具

lint 工具、代码生成器、IDE 插件都会自己调用 go 命令：它们在 PATH 里找 `go`，并使用它报告的 `GOROOT`。有两种方式让它们与 gip 配合。

### gip exec

`gip exec` 用所选版本的环境运行一条命令：

```console
$ gip exec golangci-lint run ./...
$ gip exec gopls check ./...
```

该命令会在 PATH 中找到这个版本的 go 命令，并使用与之配套的 `GOROOT`、`GOBIN`、`GOCACHE`。此时 `GOTOOLCHAIN` 被设为 `local`，go 命令不会在你背后自行切换工具链。

### 把当前版本放进 PATH

不经 `gip exec` 启动的工具会在 PATH 中找 go 命令。把当前版本的 bin 目录放在 PATH 最前面：

```
~/.gip/current/goroot/bin
```

在项目里执行 `gip` 会切换当前版本，之后启动的工具就会使用该项目要求的版本。

### GOROOT 链接

需要完整 GOROOT 的工具（比如 VS Code 的 Go 扩展）不能用 `current` 目录本身：里面是 `gobin` 和 `goroot` 两个链接，都不是带 `src`、`pkg`、`lib` 子目录的 GOROOT 目录。

`current` 目录下的 `goroot` 链接指向当前版本的**完整** GOROOT：

```
~/.gip/current/goroot -> ../versions/1.22.0
```

**它的路径不随版本变化**，所以配置里填一次就能一直用。它与当前版本一起切换。

```console
$ gip toolchain env
GIP_HOME   /home/user/.gip
CURRENT    1.22.0
GOROOT     /home/user/.gip/versions/1.22.0
GOBIN      /home/user/.gip/gobin/1.22.0
GOCACHE    /home/user/.gip/gocache/1.22.0
GOROOT_LINK /home/user/.gip/current/goroot
GO_BIN      /home/user/.gip/current/goroot/bin
MIRROR     https://go.dev/dl
```

### 环境变量里的 GOROOT

环境中的 `GOROOT` 会覆盖任何 go 命令的工具链，不管那个命令来自哪里。工具于是会去用另一个 Go 安装的标准库来构建。

`GOTOOLCHAIN` 同理：取值不是 `local` 时，go 命令会自行挑选另一个工具链。

`gip doctor` 会报告这两类问题并给出修复方法。发现问题时退出码为 1，便于在脚本中使用：

```console
$ gip doctor
GIP_HOME   /home/user/.gip
CURRENT    1.22.0

! GOROOT is set to /usr/local/go in the environment.
  It overrides the toolchain of every go command, so a tool
  would build against /usr/local/go instead of /home/user/.gip/versions/1.22.0.
  Remove it: unset GOROOT

! /home/user/.gip/current/goroot/bin is not in the PATH.
  A tool that calls go would find another one.
  Add it in front of the PATH:
    export PATH="/home/user/.gip/current/goroot/bin:$PATH"

2 problem(s) found.
```

用 `gip install` 安装的工具会落在该版本的 GOBIN 里，即 `~/.gip/gobin/<version>`。这个目录已经在 `gip exec` 构造的 PATH 中，所以这样的工具能被直接找到。

## 环境变量

| 变量          | 含义                                                       |
| ------------- | ---------------------------------------------------------- |
| `GIP_HOME`    | 安装目录，默认为 `~/.gip`                                  |
| `GIP_MIRROR`  | 下载站点，默认为 `https://go.dev/dl`                       |
| `GOWORK`      | 查找 `go.work` 文件时会被遵从                               |

镜像站必须提供相同的清单格式，例如 `https://golang.google.cn/dl`：

```console
$ GIP_MIRROR=https://golang.google.cn/dl gip install 1.22.0
```

## 下载

版本来自官方清单 `https://go.dev/dl/?mode=json&include=all`，其中包含每个归档文件的 SHA-256 校验和。归档先下载到临时文件，校验通过之后才会解压，因此下载中断不会留下损坏的安装。解压同样在临时目录中进行，完成后再重命名到最终位置。

两个终端同时请求同一个缺失的版本时，会通过锁文件串行化，因此该版本只会下载一次。

## 平台说明

Unix 上的归档是 `.tar.gz` 文件，Windows 上是 `.zip` 文件；下载器会根据清单自动选择。

在 Windows 上创建符号链接需要默认并不授予的权限，因此 `gip` 在该平台会退回到使用目录联接（junction）。目录联接保存的是绝对路径，这是与上面目录结构唯一的差异。

## 开发

```console
$ go test ./...
```

本项目除标准库外没有任何依赖。

### 版本号

`gip version` 从构建信息里取版本号：

* 用 `go install github.com/go-comm/gip/cmd/gip@v0.2.0` 安装的二进制报告 `v0.2.0`。
* 从仓库构建的二进制报告它所基于的那个 commit 的版本：commit 上带 tag 就报告该 tag，
  在最新 tag 之后则是形如 `v0.2.1-0.20261022133637-62a53e2482f2` 的伪版本。工作区有未提交改动时加上 `+dirty` 后缀。
* `go run` 完全不带版本信息，报告的是源码树的版本号 `v0.2.0`。

也可以在构建时指定：

```console
$ go build -ldflags "-X main.versionOverride=v1.2.3" ./cmd/gip
```
