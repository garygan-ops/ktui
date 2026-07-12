# ktui

`ktui` 是一个用于查看 [Komari](https://github.com/komari-monitor/komari) 探针监控状态的终端 TUI 工具。它可以让你不用打开浏览器，直接在终端里查看服务器在线状态、资源占用、流量、Ping、节点详情和历史趋势。

`ktui` 不内置默认 Komari 地址。首次启动且未配置 URL 时，会在终端中引导输入 Komari 地址和可选 API key，并保存到配置文件。

## 灵感来源

这个项目的点子来源于 NodeSeek 上这位大佬的帖子：

https://www.nodeseek.com/post-710243-1

感谢原帖提供的思路。当前实现是基于 Komari 公开 API 和个人使用需求写成的 Go 版本 TUI。

## 功能

- `sheet` 卡片视图：按卡片展示每台服务器的 CPU、RAM、磁盘、网络、流量、运行时间和过期信息
- `line` 列表视图：更紧凑地逐行展示服务器概览，宽屏下会展开负载、流量、连接数、进程数、过期时间、系统和标签等列
- 支持节点流量上限展示：已配置流量限额的节点会显示累计流量占比、上下行累计流量和限额类型
- 两层交互：第一层是节点列表，第二层是单节点详情
- 详情页标签：`overview`、`node`、`history`、`ping`、`meta`
- 历史窗口：`realtime`、`4h`、`1d`、`7d`、`30d`
- History 坐标图：CPU、RAM、磁盘、网络、连接数、进程数
- 详情页图表聚焦模式：点击图表或按 `f` 可用整页查看单张图
- Ping 数据：实时 Ping 和历史 Ping 汇总
- 支持 Komari API key，能读取 IPv4、IPv6、过期时间等后台节点详情
- 支持导出当前节点状态为 JSON、CSV 或 Markdown，便于写报告、脚本处理或分享排障信息
- 支持配置文件，默认持久化到系统用户配置目录
- 支持终端窗口大小变化，窄窗口下会自动裁剪和滚动
- 支持鼠标和触控板：列表点击选择、滚轮滚动，详情页可滚动并点击切换标签/窗口
- 支持 ASCII / 无颜色兼容模式，适合 Unicode 显示异常的终端
- 支持后台检查 ktui 自身更新和 Komari 服务端新版本提示
- 支持远程一行安装脚本，自动识别系统和 CPU 架构并下载对应 Release

## 安装

Linux / macOS：

```sh
curl -fsSL https://gitea.bytevibe.dev/gary/ktui/raw/branch/main/install.sh | sh
```

Windows PowerShell：

```powershell
irm https://gitea.bytevibe.dev/gary/ktui/raw/branch/main/install.ps1 | iex
```

默认安装位置：

- Linux / macOS：`~/.local/bin/ktui`
- Windows：`%LOCALAPPDATA%\ktui\bin\ktui.exe`

安装指定版本或目录：

```sh
curl -fsSL https://gitea.bytevibe.dev/gary/ktui/raw/branch/main/install.sh | KTUI_VERSION=v0.5.0 sh
curl -fsSL https://gitea.bytevibe.dev/gary/ktui/raw/branch/main/install.sh | KTUI_INSTALL_DIR=/usr/local/bin sh
```

私有仓库或需要认证的 Release 可以传 token：

```sh
curl -fsSL https://gitea.bytevibe.dev/gary/ktui/raw/branch/main/install.sh | KTUI_UPDATE_TOKEN=your_token sh
```

## 从源码构建

需要 Go 1.26 或更新版本。

```sh
go build -o ktui ./cmd/ktui
./ktui profile add default --url https://komari.example.com --use
./ktui
```

也可以直接运行：

```sh
go run ./cmd/ktui
```

## 快速使用

查看帮助：

```sh
./ktui help
./ktui help status
./ktui help config
./ktui help keys
```

查看当前版本：

```sh
./ktui version
```

检查或安装更新：

```sh
./ktui update check
./ktui update install
```

进入 TUI 时会自动在后台检查 ktui 自身是否有新版本，也会根据当前 Komari 服务端版本检查 `komari-monitor/komari` 的 GitHub Release。发现新版本后，界面顶部会提示 `UPDATE ... available`，底部会出现 `u update`；按 `u` 或点击该提示会显示可用更新详情。ktui 自身更新请退出 TUI 后运行：

```sh
ktui update install
```

指定 Komari 地址：

```sh
./ktui --url https://komari.example.com
```

使用 API key：

```sh
./ktui --api-key your_api_key
```

只拉取一次并打印摘要，不进入 TUI：

```sh
./ktui status
```

导出当前节点状态：

```sh
./ktui export markdown -o report.md
./ktui export csv --output nodes.csv
./ktui export json
```

导出内容包含站点摘要、在线状态、CPU/RAM/磁盘百分比、实时网络、累计流量、流量上限占比、过期状态和异常原因。

## 命令结构

```text
ktui [flags]
ktui status [flags]
ktui export <markdown|csv|json> [flags]
ktui config <init|path|show|set|help>
ktui keys
ktui profile <list|current|use|add|rename|remove|help>
ktui update <check|install|help>
ktui completion <bash|zsh|fish|powershell>
ktui version
ktui help [command]
```

连接到多个 Komari 站点时使用 `ktui profile ...` 管理，并可用 `--profile name` 临时选择。TUI 专属显示参数使用 `--mode sheet|line`。`--realtime-window 1m|5m|10m` 控制 realtime 图表横轴时间范围。一次性拉取摘要使用 `ktui status`，更新检查和安装分别使用 `ktui update check`、`ktui update install`。

## Shell 补全

`ktui completion` 可以生成 Tab 补全脚本。加载后可以补全子命令、常用 flags、枚举值、配置键和已保存的 profile 名称。

当前终端临时启用：

```sh
source <(ktui completion bash)
# zsh:
source <(ktui completion zsh)
# fish:
ktui completion fish | source
```

PowerShell：

```powershell
ktui completion powershell | Out-String | Invoke-Expression
```

持久启用时，把对应命令放进 `~/.bashrc`、`~/.zshrc`、fish 配置或 PowerShell profile；也可以把 `ktui completion zsh` 输出保存为 `$fpath` 目录里的 `_ktui` 文件。

## 配置文件

初始化配置：

```sh
./ktui config init
```

首次启动未配置 URL 时会自动进入引导，并写入当前 profile。也可以提前手动添加 profile：

```sh
./ktui profile add default --url https://komari.example.com --api-key your_api_key --use
```

设置常用配置：

```sh
./ktui profile add prod --url https://komari.example.com --api-key your_api_key --use
./ktui config set mode sheet
./ktui config set realtime-window 5m
./ktui config show
```

默认配置路径由系统用户配置目录决定。查看当前实际路径：

```sh
./ktui config path
```

常见默认位置：

- Linux：`~/.config/ktui/config.json`，或 `$XDG_CONFIG_HOME/ktui/config.json`
- macOS：`~/Library/Application Support/ktui/config.json`
- Windows：`%AppData%\ktui\config.json`

也可以临时指定配置文件：

```sh
./ktui --config /path/to/config.json
KTUI_CONFIG=/path/to/config.json ./ktui
```

配置示例：

```json
{
  "profile": "default",
  "profiles": {
    "default": {
      "url": "https://komari.example.com",
      "api_key": "your_api_key"
    },
    "lab": {
      "url": "https://lab.example.com"
    }
  },
  "interval": "5s",
  "timeout": "10s",
  "realtime_window": "1m",
  "chart_y_axis": "absolute",
  "warn_cpu": 90,
  "warn_ram": 85,
  "warn_disk": 90,
  "warn_expiry_days": 7,
  "mode": "sheet",
  "ascii": false,
  "no_color": false
}
```

配置优先级：

```text
默认值 < 配置文件 < 环境变量 < 命令行参数
```

## 环境变量

```sh
KTUI_URL=https://komari.example.com ./ktui
KTUI_API_KEY=your_api_key ./ktui
KTUI_PROFILE=lab ./ktui
KTUI_MODE=line ./ktui
KTUI_REALTIME_WINDOW=5m ./ktui
KTUI_CHART_Y_AXIS=relative ./ktui
KTUI_WARN_CPU=85 KTUI_WARN_DISK=90 ./ktui
KTUI_WARN_EXPIRY_DAYS=14 ./ktui
KTUI_ASCII=1 NO_COLOR=1 ./ktui
```

## Profile 多站点

每个 profile 保存一个 Komari 站点的 URL 和可选 API key。`profile` 字段表示默认启动使用哪个 profile：

```sh
./ktui profile add prod --url https://komari.example.com --api-key your_api_key --use
./ktui profile add lab --url https://lab.example.com
./ktui profile list
./ktui profile use prod
./ktui profile rename lab staging
```

临时切换，不修改默认 profile：

```sh
./ktui --profile lab
./ktui status --profile lab
./ktui export markdown --profile lab -o lab.md
KTUI_PROFILE=prod ./ktui
```

TUI 运行中也可以在 Settings 页面选中 `profile`，用左/右方向键在已配置的 profile 之间切换。选中 `rename_profile` 后按 Enter 可以重命名当前 profile。切换后 ktui 会清空当前站点缓存并重新加载新站点数据。Settings 和 About 页面都会显示当前 profile、站点名和连接 URL。

`ktui config set url ...` 和 `ktui config set api-key ...` 会更新当前默认 profile 的连接信息。

## Realtime 图表窗口

`realtime_window` 控制详情页 `realtime` 图表横轴时间范围，可设置为 `1m`、`5m` 或 `10m`：

```sh
./ktui --realtime-window 5m
./ktui config set realtime-window 10m
```

实时样本保留数量会根据 `realtime_window` 和刷新间隔自动计算，并受内部上限保护。

## 显示模式

卡片视图，默认模式：

```sh
./ktui --mode sheet
```

逐行列表视图：

```sh
./ktui --mode line
```

`line` 模式会根据终端宽度自动增减列。宽屏下会显示 CPU、RAM、磁盘、地区、实时网络、负载、运行时间、累计流量、连接数、进程数、过期时间、系统和标签。

如果终端 Unicode 显示乱码，可以使用兼容模式：

```sh
./ktui --ascii --no-color
```

## Sheet 卡片中的 EXP

`sheet` 模式下，每个服务器卡片会显示 `EXP`：

```text
EXP 23d
EXP today
EXP expired
EXP free
EXP lifetime
EXP -
```

含义：

- `23d`：还有 23 天过期
- `today`：24 小时内过期
- `expired`：已经过期
- `free`：免费节点
- `lifetime`：Komari 返回了很远的占位过期时间
- `-`：没有拿到过期时间

过期时间、IPv4、IPv6 等字段通常来自 Komari 后台节点详情，需要配置有足够权限的 API key。

## 快捷键

- `↑` / `k`：列表页选择上一个节点；详情页向上滚动一张卡片
- `↓` / `j`：列表页选择下一个节点；详情页向下滚动一张卡片
- 鼠标/触控板滚轮：列表页切换选择；详情页滚动；设置页切换设置项
- 鼠标点击：列表页打开节点详情；底部命令可直接点击；详情页点击底部 `Back` 返回，点击标签/时间窗口切换；设置页选择设置项并可点击底部调整/返回
- `/`：编辑节点搜索，匹配名称、地区、标签、分组、IP、OS 和 UUID；`Enter` 应用，`Esc` 取消
- `c`：循环排序：默认、在线状态、CPU、RAM、累计流量、过期时间
- `v`：循环过滤：全部、离线、即将过期、高负载
- 详情页图表：点击图表或按 `f` 聚焦，`h` / `l` 或 `PgUp` / `PgDn` 切换聚焦图表，`Esc` / `b` / `q` / `Enter` 返回详情页
- `PgUp` / `PgDn`：快速滚动列表或详情页
- `Enter` / `o`：打开选中节点的详情页
- `Esc` / `b` / `q`：从详情页返回列表页
- `h` / `l`、`1`-`5`、`Tab`：切换详情页标签
- `[` / `]`：切换详情页时间窗口
- `m`：在列表页切换 `sheet` / `line` 模式
- `r`：立即刷新
- `d`：打开或重新加载选中节点的详情数据
- `a`：切换 ASCII 兼容模式
- `u`：有新版本时显示 ktui 或 Komari 服务端更新详情
- `?`：打开 about 视图
- `q` / `Ctrl-C`：在列表页退出

异常高亮阈值可以通过配置文件、`ktui config set warn-cpu 85`、`ktui config set warn-ram 85`、`ktui config set warn-disk 90`、`ktui config set warn-expiry-days 14` 或设置页调整。

## 版本信息

查看当前二进制版本：

```sh
ktui version
```

通过 GoReleaser 发布的二进制会显示 tag 版本、commit 和构建时间；本地直接 `go build` 的版本会显示为 `dev`。

## 自更新

`ktui update install` 会从 Gitea Release 下载当前系统和架构对应的压缩包，使用 `checksums.txt` 校验后替换当前二进制。只检查新版本时使用 `ktui update check`。

```sh
ktui update check
ktui update install
ktui update install --tag v0.5.0
```

默认更新源：

```text
https://gitea.bytevibe.dev/api/v1/repos/gary/ktui
```

如果仓库是私有仓库，可以通过环境变量传入 token：

```sh
KTUI_UPDATE_TOKEN=your_token ktui update install
```

Windows 无法在程序运行时直接替换当前 `.exe`，`ktui update install` 会下载新文件并提示手动替换。

## 详情页

详情页有五个标签：

- `overview`：选中服务器的实时概览
- `node`：节点系统、硬件、账单和备注信息
- `history`：CPU、RAM、磁盘、网络、连接数、进程数的坐标图
- `ping`：实时和历史 Ping 数据
- `meta`：节点元数据、Komari 版本、认证状态和 API 方法信息

`history` 和 `ping` 支持这些时间窗口：

```text
realtime / 4h / 1d / 7d / 30d
```

## 注意事项

- 如果没有配置 API key，部分后台字段可能显示为 `-` 或 `api-key required`
- 如果 Komari 实例没有开启记录功能，历史图表可能没有数据
- 如果终端太窄，部分内容会被裁剪或需要滚动查看
- 如果 Unicode 图形显示异常，使用 `--ascii --no-color`

## License

本项目使用 MIT License，详见 [LICENSE](./LICENSE)。
