English | [简体中文](browser-extension.zh-CN.md)

# neuDrive Browser Extension Guide

The neuDrive browser extension adds a lightweight sidecar to supported AI chat pages. It is best when you want to stay inside an existing web chat UI and pull your neuDrive context into that page, or import conversations from the page back into neuDrive.

Note: the current extension UI text is Chinese, so this guide includes the actual button labels where it helps.

## Supported Browsers

- Chrome
- Edge
- Other Chromium-based browsers that support Manifest V3

## Supported Sites

- Claude Web: `https://claude.ai`
- ChatGPT Web: `https://chat.openai.com`, `https://chatgpt.com`
- Gemini Web: `https://gemini.google.com`
- Kimi Web: `https://kimi.moonshot.cn`

Current conversation import is available on:

- Claude Web
- ChatGPT Web

Batch conversation import is currently available on:

- Claude Web only

## What The Extension Can Do

- Sign in to hosted neuDrive through the official browser flow
- Or connect to a custom hub with `Hub URL + scoped token`
- Show a floating `neuDrive` button inside supported chat pages
- Inject these context blocks into the current input box:
  - preferences
  - project context
  - skills
- Import the current conversation into neuDrive
- On Claude Web, review a live list of sidebar conversations and batch-import the selected ones
- Open the dashboard and token-management pages from the popup
- Configure auto-inject and per-platform toggles in the popup

Important: context injection writes text into the page's input box. That content is not sent to Claude / ChatGPT / Gemini / Kimi until you send the message yourself.

## Install

This repo contains the unpacked extension source in `extension/`.

For local development:

1. Open `chrome://extensions` in Chrome or Edge.
2. Turn on `Developer mode`.
3. Click `Load unpacked`.
4. Select the repo's `extension/` directory.

If you already have a copied build directory such as `dist/neudrive-extension`, you can load that folder instead.

After the extension is loaded, a `neuDrive` item should appear in your extensions toolbar.

## Connect To neuDrive

You have two connection paths.

### Option A: Official neuDrive Login

Recommended for most users.

1. Click the extension icon to open the popup.
2. Click `登录官方 neuDrive`.
3. Finish the browser sign-in flow on `https://www.neudrive.ai`.
4. Return to the chat page or popup; the extension should show `Connected`.

### Option B: Manual Hub URL + Token

Use this when you are connecting to a self-hosted hub or a non-default environment.

1. Click the extension icon.
2. Enter `Hub URL`.
3. Enter a scoped token.
4. Click `Connect`.

If you want conversation import to work, your token must include write access to the tree, such as `write:tree`. Read access must also cover whichever data you want to inject.

## Use In A Chat Page

After the extension is connected:

1. Open Claude, ChatGPT, Gemini, or Kimi.
2. Click the floating `neuDrive` button.
3. Use the in-page panel actions.

### Import Current Conversation

Use this when you want to archive the current chat into neuDrive.

- Claude Web: prefers Claude's internal conversation API, then falls back to page capture if needed
- ChatGPT Web: currently imports from the page DOM

Imported conversations are normalized into the canonical neuDrive conversation format and written under `/conversations/...`.

### Inject Preferences / Project / Skills

Use these when you want to bring stored neuDrive context into the current chat.

- `Inject preferences`
- `Inject project context`
- `Inject skills`

The extension inserts text into the page input. Review it, edit it if needed, then send it yourself.

## Batch Import On Claude Web

This flow is meant for “import many recent / visible sidebar conversations”, not guaranteed full-account export.

1. Open a Claude Web conversation page and make sure the left sidebar is visible.
2. Open the neuDrive in-page panel.
3. Click `批量导入对话`.
4. Review the dedicated batch-import page.
5. The list updates as Claude's sidebar changes.
6. By default everything is selected.
7. Optionally uncheck individual conversations.
8. Optionally click `尽量加载更多历史`.
9. Click `确认导入`.
10. Keep the page open until the progress bar completes.

During import, the extension shows:

- current progress
- a progress bar
- warnings not to close or refresh the page
- a final result summary

Typical outcomes:

- `rate limited`: Claude or neuDrive asked the extension to slow down; the extension retries automatically
- `forbidden / inaccessible`: the current Claude session could not read that conversation

## Popup Features

The popup is for connection state and global settings.

It lets you:

- see whether the extension is connected
- confirm which hub it is using
- open the dashboard
- open token management
- enable or disable auto-inject
- enable or disable supported platforms individually
- disconnect

## Limitations And Recommendations

- The extension depends on page structure and, on Claude, some undocumented internal APIs. Web UI changes can break parts of the flow.
- Batch import on Claude is a best-effort workflow based on the sidebar and current session access.
- Some Claude conversations may return `403` even when they appear in the sidebar.
- Large batches can still hit `429` rate limits; the extension retries, but not forever.
- ChatGPT batch import is not implemented yet.
- For complete or durable Claude history migration, prefer the dashboard's official Claude export ZIP importer.

## Related Docs

- [Setup Guide](./setup.md)
- [Chinese Browser Extension Guide](./browser-extension.zh-CN.md)
