# Mix Space Desktop

Mix Space 后台的桌面客户端，基于 [MyGo](https://mygo.run) 和 [Onevra](https://github.com/oneo-me/onevra) 构建（版本都锁定在 `go.mod` 中）。

它是 Web 后台的原生外壳：窗口、侧边栏、菜单栏、命令面板和设置窗口是原生的，内容区就是服务端在 `/proxy/qaqdmin` 提供的 Web 后台，显示在 web view 里。客户端本身不实现后台功能，所以服务端升级后，桌面端无需发版就能用上新版后台。

优先支持 macOS，Windows 和 Linux 使用同一份代码构建。

<p align="center">
  <img src="docs/screenshots/palette.png" alt="原生侧边栏与 Cmd+K 命令面板" width="720" />
</p>

## 维护边界：不修改 mx-core 核心代码

这个应用作为 mx-core 的桌面客户端单独维护，**所有改动都限定在 `apps/desktop/` 目录内**，不修改 `apps/core`（服务端）、`apps/admin`（Web 后台）和共享包。

唯一的例外是 `.github/README.md`：它让 GitHub 仓库首页显示这份介绍（GitHub 优先显示 `.github/README.md`，所以根目录的 mx-core README 保持不动）。它由 `apps/desktop/README.md` 生成，不要直接编辑：改完 `apps/desktop/README.md` 后，在 `apps/desktop` 里运行 `go run ./internal/ghreadme`（`go generate` 也会运行它）。两份不一致时 `go test ./...` 会失败。

它只使用 mx-core 已经公开的能力，对它们只读、不改：

| 依赖的上游能力 | 位置 | 用途 |
| --- | --- | --- |
| Web 后台页面 `/proxy/qaqdmin` | `apps/core` pageproxy | 内容区的 web view |
| Better Auth `/auth/ok`、设备授权、`/device/web-handoff` | `apps/core` auth | 探测服务器、Passkey 登录、把令牌换成后台 Cookie |
| 设备客户端 `mxs-cli` | `apps/core/src/modules/auth/auth.constant.ts` | 服务端只接受白名单里的客户端，桌面端借用 CLI 的，所以授权页显示 “Mix Space CLI” |
| 路由元数据、中英文标题、Lucide 图标 | `apps/admin/src/views/**/page.tsx` | 生成原生侧边栏（`go generate`） |
| 存储键 `admin:sidebar-collapsed`、`theme-mode`、`mx-admin-ui-locale`，两个 `aria-label`，`index.html` 里的 `#app`、`#initial-loader` | `apps/admin` | 预加载脚本：收起 Web 侧边栏，同步外观和语言，判断页面何时加载完成 |
| 写作页的编辑器和顶栏（见 `docs/writing-plan.md`） | `apps/admin` 写作页 | 预加载脚本：字数、专注模式样式、打字机滚动 |
| 页面顶栏的标题块：带 `MobileHeaderAffordance`（上面两个 `aria-label` 的按钮）和 `h1`/`h2` 的那个元素，以及标题旁的数字 | `apps/admin` 各页面顶栏 | `inject/page.css`：原生标题栏已显示页面名时，隐藏网页标题，计数移进玻璃面包屑 |
| 设计 token（`--color-*`）、Tailwind 灰阶 `--color-neutral-*`、编辑器的 `--rc-host-*`、`<html>` 上的 `.dark` | `apps/admin/src/styles/tokens.css`、`@haklex/rich-editor` | 配色主题（`inject/themes/*.css`） |

上游变化时，在这里适配，不去改上游：

- 后台增删页面：在 `apps/desktop` 里运行 `go generate`（需要先 `pnpm install`），检查 `nav_gen.go` 和 `icons/` 的 diff。
- 后台改了存储键、`aria-label` 或 `#app`/`#initial-loader`：更新 `inject/` 里的脚本和样式。不更新的话，Web 侧边栏的按钮会重新出现，加载转圈要等 8 秒超时才消失，但不影响使用。
- 服务端改了认证接口：更新 `auth.go` 和 `server.go`。

如果某项功能确实需要服务端配合（比如给桌面端单独注册一个设备客户端），应当另向 mx-core 提 PR，不要放在这个应用的改动里。

## 功能

### 原生窗口与侧边栏

Onevra 的侧边栏外壳：macOS 上有标题栏材质，Windows 上有 Mica。侧边栏可以拖动调宽、折叠，从后台路由自动生成，并跟随当前页面高亮。标题栏显示面包屑，点击上级可以跳回去。

标题栏的面包屑是悬浮的液态玻璃胶囊。页面就是侧边栏里的某一项时（比如“博文”“读者”“速记”“写手记”），网页顶栏里重复的标题和图标会隐藏，标题旁的计数（“草稿箱 10”的 10）移到面包屑里；右侧的筛选、“新建”等按钮和下方的摘要行保留在原位，顶栏只剩标题时整行隐藏。详情页等不在侧边栏里的页面，网页标题照常显示。原生标题栏整行是窗口拖动区，点不到下面的网页，所以网页按钮不移进标题栏那一行。

在 macOS 上，点关闭按钮或 `⌘W` 只隐藏窗口，应用留在后台继续运行（后台的 WebSocket 通知照常收到），点 Dock 图标恢复原样；`⌘Q` 才退出。Windows 和 Linux 上没有托盘图标，关闭窗口仍然退出应用。

### 命令面板 `⌘K`

可以用中文、英文或路径搜到任意后台页面，也能执行刷新、前进后退、切换外观或语言、更换服务器、退出登录等操作。

`⌘K` 不是菜单快捷键，而是视图快捷键：后台编辑器自己绑定了 `⌘K`（插入链接），页面处理过的按键会留给页面。

### 写作

写作页（写博文、写手记、写页面）上的原生增强，规划见 [docs/writing-plan.md](docs/writing-plan.md)：

- **专注模式 `⇧⌘F`**：在当前窗口内铺满（不进入系统全屏），收起侧边栏、隐藏写作页顶栏。再按一次即恢复。
- **打字机滚动**：光标所在行停在略高于中间的位置，在“显示”菜单或设置球里开关，设置会保存。
- **液态玻璃**：写作页上网页铺满窗口，标题栏的面包屑和右下角的设置球都是悬浮的玻璃（MyGo glass 插件，macOS 26 及以上是系统原生 Liquid Glass，能透出下面的正文）。
- **设置球**：点开后向左展开统计（正文字数，写作时长“本次 4m · 累计 1h 20m”，累计按文章存在本机、只在窗口显示时计时）、专注和打字机两个开关，再点收起。

### 设置 `⌘,`

<p align="center">
  <img src="docs/screenshots/settings.png" alt="设置窗口" width="360" />
  <img src="docs/screenshots/settings-dark.png" alt="设置窗口（深色）" width="360" />
</p>

独立的可搜索设置窗口，包含服务地址、界面语言、外观（浅色、深色、跟随系统，应用和后台一起切换）、配色主题，以及账户相关的操作。

### 配色主题

可以给后台换一套配色，目前有 **Atom**（One Light / One Dark，参照 [obsidian-atom](https://github.com/kognise/obsidian-atom)）。浅色和深色仍由“外观”决定，主题只决定两者各自的颜色。

服务器上跑的通常是官方镜像，桌面端不能改后台代码，所以主题是预加载脚本注入的一份 CSS（`inject/themes/<id>.css`）。它覆盖后台和编辑器读取的 CSS 变量：后台的设计 token、Tailwind 的灰阶和彩色色阶（徽标、状态色换成 Atom 的色相）、编辑器 `@haklex/rich-editor` 留给宿主页面的 `--rc-host-*`，以及代码编辑器（Monaco、CodeMirror）的背景。所有规则都限定在 `<html data-mx-theme="<id>">` 下，属性由预加载脚本按“主题范围”设置：

- **仅写作页**（默认）：只在写博文、手记、页面（包括编辑草稿）时换色，按写作页逐项适配；原生侧边栏和窗口保持原有配色。
- **全局**：后台所有页面一起换色，原生界面（侧边栏底色、命令面板、设置窗口）也换成主题配色。后台由服务器提供，部分页面可能不一致，设置里会提示。

换色前脚本会检查页面是否定义了主题要覆盖的变量。如果后台改了名，就不套用主题，弹出提示，原生界面也恢复原有配色。

新增主题：在 `inject/themes/` 下放一个 `<id>.css`，并在 `themes.go` 的 `colorThemes` 里加一项（名称和原生界面的浅色、深色配色）。设置和命令面板会自动列出它。

### 连接与登录

<p align="center">
  <img src="docs/screenshots/setup.png" alt="连接服务器" width="360" />
  <img src="docs/screenshots/login.png" alt="Passkey 设备授权" width="360" />
</p>

- **密码**：在 web view 里使用后台自己的登录表单。
- **Passkey**：在系统浏览器中完成设备授权（`auth.go`），与 mxs CLI 和 iOS 客户端的方式相同。浏览器里用 Passkey 登录并确认验证码后，应用把会话令牌存进钥匙串，再通过 `/device/web-handoff` 换成后台的 Cookie。之后启动时如果 Cookie 已过期，会自动用钥匙串里的令牌重新登录。

连接失败、登录成功、失败或过期、退出登录，都会用 Toast 提示。

## 开发

需要 Go 1.27 及以上。

```sh
cd apps/desktop
go tool mygo dev     # 运行并热重载
go test ./...        # 测试，包括各界面的无界面渲染
go tool mygo build   # 生成 build/darwin-arm64/Mix Space.app 和 .dmg
```

截图是原生界面的无界面渲染（Web 后台部分留空）。界面改动后重新生成：

```sh
README_SCREENSHOTS=docs/screenshots go test -run TestReadmeScreenshots
```

`SCREENSHOTS=<dir> go test -run TestScreens` 会把测试里渲染的各个界面保存为 PNG，方便检查。

修改 `apps/desktop/README.md` 后，运行 `go run ./internal/ghreadme` 同步仓库首页的 `.github/README.md`。

## 代码结构

| 文件 | 内容 |
| --- | --- |
| `main.go` | 应用状态、连接、web view、登录流程 |
| `views.go` | 窗口外壳、侧边栏、标题栏面包屑，以及设置、登录、错误页 |
| `palette.go` | `⌘K` 命令面板 |
| `prefs.go` | `⌘,` 设置窗口 |
| `menu.go` | 菜单栏 |
| `inject/` | 注入 Web 后台页面的全部改动：预加载脚本 `preload.js`、改后台界面的样式 `page.css`、配色主题 `themes/`。后台的 DOM 依赖都集中在这里 |
| `inject.go` / `bridge.go` | 嵌入并组装预加载脚本；页面调用的 Go 端接口 |
| `auth.go` / `server.go` | 设备授权、令牌交换、服务器探测 |
| `nav.go` / `nav_gen.go` / `icons/` | 侧边栏数据（`nav_gen.go` 和 `icons/` 是生成的） |
| `internal/gennav` | 侧边栏生成器 |
| `internal/ghreadme` | 由 `README.md` 生成 `.github/README.md` |

主题使用 MyGo 默认主题，没有用 Onevra 的配色，这样原生部分和 Web 后台的观感更接近。
