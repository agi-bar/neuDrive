[English](browser-extension.md) | 简体中文

# neuDrive 浏览器插件指南

neuDrive 浏览器插件会在支持的 AI 聊天网页里挂一个轻量 sidecar。它适合这样几类场景：你想留在现有网页聊天界面里，把 neuDrive 的上下文带进来；或者你想把网页里的对话反向导入 neuDrive。

## 支持的浏览器

- Chrome
- Edge
- 其他支持 Manifest V3 的 Chromium 浏览器

## 支持的网站

- Claude Web：`https://claude.ai`
- ChatGPT Web：`https://chat.openai.com`、`https://chatgpt.com`
- Gemini Web：`https://gemini.google.com`
- Kimi Web：`https://kimi.moonshot.cn`

当前支持“导入当前对话”的平台：

- Claude Web
- ChatGPT Web

当前支持“批量导入对话”的平台：

- 仅 Claude Web

## 插件能做什么

- 通过 neuDrive 官方浏览器流程登录 hosted neuDrive
- 或者使用 `Hub URL + scoped token` 连接自定义 hub
- 在支持的聊天页面里显示一个悬浮 `neuDrive` 按钮
- 把这些上下文注入当前输入框：
  - preferences
  - project context
  - skills
- 把当前对话导入 neuDrive
- 在 Claude Web 中先查看侧栏对话列表，再批量导入选中的会话
- 从 popup 打开 dashboard 和 token 管理页面
- 在 popup 里配置自动注入和平台开关

一个重要边界：所谓“注入”，本质上是把文本写进当前网页的输入框。只有你自己点击发送之后，这些内容才会发给 Claude / ChatGPT / Gemini / Kimi。

## 安装

这个仓库里已经包含了解压后的扩展源码目录：`extension/`。

本地开发安装方式：

1. 在 Chrome 或 Edge 中打开 `chrome://extensions`
2. 打开 `Developer mode`
3. 点击 `Load unpacked`
4. 选择仓库里的 `extension/` 目录

如果你已经有一个复制好的构建目录，比如 `dist/neudrive-extension`，也可以直接加载那个目录。

加载成功后，浏览器工具栏里应该能看到 `neuDrive`。

## 如何连接 neuDrive

插件有两种连接方式。

### 方式 A：登录官方 neuDrive

大多数用户推荐走这条。

1. 点击浏览器工具栏里的扩展图标，打开 popup
2. 点击 `登录官方 neuDrive`
3. 在 `https://www.neudrive.ai` 完成浏览器登录流程
4. 返回聊天页面或 popup，插件状态应显示为“已连接”

### 方式 B：手动填写 Hub URL + Token

适合自托管 hub 或非默认环境。

1. 点击扩展图标
2. 填写 `Hub URL`
3. 填写 scoped token
4. 点击 `连接`

如果你希望“导入对话”可以正常写入 neuDrive，这个 token 至少需要包含类似 `write:tree` 的树写权限。注入功能需要对应数据的读权限。

## 在聊天页面里怎么用

扩展连接成功后：

1. 打开 Claude、ChatGPT、Gemini 或 Kimi
2. 点击页面里的悬浮 `neuDrive` 按钮
3. 使用页面内的操作面板

### 导入当前对话

这个操作会把你当前打开的聊天归档进 neuDrive。

- Claude Web：优先走 Claude 的内部会话 API；失败时再退回页面抓取
- ChatGPT Web：当前主要走页面 DOM 抓取

导入后的对话会被整理成 neuDrive 的统一 conversation 格式，并写入 `/conversations/...`

### 注入偏好 / 项目 / 技能

当你想把 neuDrive 里已经存好的上下文带进当前聊天时，用这几个按钮：

- `注入偏好`
- `注入项目上下文`
- `注入技能`

插件会把文本写入当前聊天输入框。你可以先检查、再编辑，最后由你自己决定是否发送。

## Claude Web 的批量导入

这条链路的定位是“把当前侧栏里可见 / 可访问的一批对话导入进来”，不是一个对整账号历史百分百完整的导出替代方案。

操作步骤：

1. 打开 Claude Web 的某个对话页面，并确保左侧栏可见
2. 打开页面里的 neuDrive 面板
3. 点击 `批量导入对话`
4. 进入批量导入专属页面
5. 页面会实时显示当前可抓取的对话列表
6. 默认是全选
7. 你可以取消某些对话
8. 也可以先点 `尽量加载更多历史`
9. 点击 `确认导入`
10. 保持当前页面打开，直到进度条完成

导入过程中，插件会显示：

- 当前进度
- 进度条
- “不要关闭 / 刷新页面”的提醒
- 最终结果摘要

你可能看到的典型结果：

- `限流`：Claude 或 neuDrive 要求减速；插件会自动退避重试
- `不可访问`：当前 Claude 登录态拿不到那个会话

## Popup 里能做什么

popup 主要负责连接状态和全局设置。

你可以在里面：

- 查看当前是否已连接
- 确认当前连接的是哪个 hub
- 打开 dashboard
- 打开 token 管理
- 开启 / 关闭自动注入
- 按平台单独开关
- 断开连接

## 限制与建议

- 插件依赖网页结构；在 Claude 侧还依赖部分未公开的内部 API。网页改版后，局部功能可能失效。
- Claude 的批量导入本质上是基于“当前侧栏 + 当前登录态”的 best-effort 方案。
- 有些 Claude 会话即使出现在侧栏里，也可能返回 `403`。
- 大批量导入时仍可能遇到 `429` 限流；插件会自动重试，但不会无限重试。
- ChatGPT 的批量导入当前还没有实现。
- 如果你的目标是完整、稳定地迁移 Claude 历史对话，优先使用 dashboard 里的 Claude 官方导出 ZIP 导入器。

## 相关文档

- [接入说明](./setup.zh-CN.md)
- [English Browser Extension Guide](./browser-extension.md)
