# 维护边界与上游依赖

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
| 草稿接口 `POST /drafts`（同源请求，带后台 Cookie；Lexical 格式），写作页 `#/posts/edit?draftId=`、`#/notes/edit?draftId=` | `apps/core` draft、`apps/admin` 写作页 | 想法一瞬：把主题或勾选的闪念推送为草稿并打开；`GET /drafts/:id` 检查推送过的草稿是否还在 |
| 写作页 AI 助手按钮 `aria-label`「AI 助手」/「AI assistant」（打开后 `aria-pressed="true"`），输入框 `placeholder`「输入修改要求」/「Describe what to change」 | `apps/admin` 写作页 | 预加载脚本：打开 AI 助手并预填提示词（不发送） |
| 设计 token（`--color-*`）、Tailwind 灰阶 `--color-neutral-*`、编辑器的 `--rc-host-*`、`<html>` 上的 `.dark` | `apps/admin/src/styles/tokens.css`、`@haklex/rich-editor` | 配色主题（`inject/themes/*.css`） |

上游变化时，在这里适配，不去改上游：

- 后台增删页面：在 `apps/desktop` 里运行 `go generate`（需要先 `pnpm install`），检查 `nav_gen.go` 和 `icons/` 的 diff。
- 后台改了存储键、`aria-label` 或 `#app`/`#initial-loader`：更新 `inject/` 里的脚本和样式。不更新的话，Web 侧边栏的按钮会重新出现，加载转圈要等 8 秒超时才消失，但不影响使用。
- 服务端改了认证接口：更新 `auth.go` 和 `server.go`。

如果某项功能确实需要服务端配合（比如给桌面端单独注册一个设备客户端），应当另向 mx-core 提 PR，不要放在这个应用的改动里。
