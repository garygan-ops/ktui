# ktui

`ktui` 是一个用于查看 [Komari](https://github.com/komari-monitor/komari) 探针监控状态的终端 TUI 工具。它可以让你不用打开浏览器，直接在终端里查看服务器在线状态、资源占用、流量、Ping、节点详情和历史趋势。

`ktui` 不内置默认 Komari 地址。首次使用前需要通过配置文件、环境变量或命令行参数设置你的 Komari 实例地址。

## 灵感来源

这个项目的点子来源于 NodeSeek 上这位大佬的帖子：

https://www.nodeseek.com/post-710243-1

感谢原帖提供的思路。当前实现是基于 Komari 公开 API 和个人使用需求写成的 Go 版本 TUI。

## 功能

- `sheet` 卡片视图：按卡片展示每台服务器的 CPU、RAM、磁盘、网络、流量、运行时间和过期信息
- `line` 列表视图：更紧凑地逐行展示服务器概览，宽屏下会展开负载、流量、连接数、进程数、过期时间、系统和标签等列
- 两层交互：第一层是节点列表，第二层是单节点详情
- 详情页标签：`overview`、`node`、`history`、`ping`、`meta`
- 历史窗口：`realtime`、`4h`、`1d`、`7d`、`30d`
- History 坐标图：CPU、RAM、磁盘、网络、连接数、进程数
- Ping 数据：实时 Ping 和历史 Ping 汇总
- 支持 Komari API key，能读取 IPv4、IPv6、过期时间等后台节点详情
- 支持配置文件，默认持久化到系统用户配置目录
- 支持终端窗口大小变化，窄窗口下会自动裁剪和滚动
- 支持 ASCII / 无颜色兼容模式，适合 Unicode 显示异常的终端

## 构建

需要 Go 1.26 或更新版本。

```sh
go build -o ktui ./cmd/ktui
./ktui config set url https://komari.example.com
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
./ktui help config
./ktui help keys
```

查看当前版本：

```sh
./ktui version
```

检查或安装更新：

```sh
./ktui update --check
./ktui update
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
./ktui --once
```

## 配置文件

初始化配置：

```sh
./ktui config init
```

首次使用建议先设置 URL：

```sh
./ktui config set url https://komari.example.com
```

设置常用配置：

```sh
./ktui config set url https://komari.example.com
./ktui config set api-key your_api_key
./ktui config set mode sheet
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
  "url": "",
  "api_key": "",
  "interval": "5s",
  "timeout": "10s",
  "realtime_points": 0,
  "chart_y_axis": "absolute",
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
KTUI_MODE=line ./ktui
KTUI_REALTIME_POINTS=150 ./ktui
KTUI_CHART_Y_AXIS=relative ./ktui
KTUI_ASCII=1 NO_COLOR=1 ./ktui
```

## 显示模式

卡片视图，默认模式：

```sh
./ktui --sheet
```

逐行列表视图：

```sh
./ktui --line
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
- `PgUp` / `PgDn`：快速滚动列表或详情页
- `Enter` / `o`：打开选中节点的详情页
- `Esc` / `b` / `q`：从详情页返回列表页
- `h` / `l`、`1`-`5`、`Tab`：切换详情页标签
- `[` / `]`：切换详情页时间窗口
- `m`：在列表页切换 `sheet` / `line` 模式
- `r`：立即刷新
- `d`：打开或重新加载选中节点的详情数据
- `a`：切换 ASCII 兼容模式
- `q` / `Ctrl-C`：在列表页退出

## 版本信息

查看当前二进制版本：

```sh
ktui version
```

通过 GoReleaser 发布的二进制会显示 tag 版本、commit 和构建时间；本地直接 `go build` 的版本会显示为 `dev`。

## 自更新

`ktui update` 会从 Gitea Release 下载当前系统和架构对应的压缩包，使用 `checksums.txt` 校验后替换当前二进制。

```sh
ktui update --check
ktui update
ktui update --tag v0.1.0
```

默认更新源：

```text
https://gitea.bytevibe.dev/api/v1/repos/gary/ktui
```

如果仓库是私有仓库，可以通过环境变量传入 token：

```sh
KTUI_UPDATE_TOKEN=your_token ktui update
```

Windows 无法在程序运行时直接替换当前 `.exe`，`ktui update` 会下载新文件并提示手动替换。

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
