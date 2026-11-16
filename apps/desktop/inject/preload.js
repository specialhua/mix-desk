/* eslint-disable no-undef -- placeholders, which inject.go replaces */
// The preload script (inject.go): a template, whose placeholders (upper-case
// names between double underscores) inject.go replaces with JSON before the
// page gets it.
;(() => {
  // The web admin's page only: not an error page or the API's pages.
  if (
    location.origin !== __ORIGIN__ ||
    location.pathname.replace(/\/$/, '') !== __PATH__
  )
    return
  try {
    localStorage.setItem('admin:sidebar-collapsed', 'true')
    localStorage.removeItem('theme-mode')
    localStorage.setItem('mx-admin-ui-locale', __LANG__)
  } catch {
    /* the page works without it */
  }
  const sidebarButtons = [
    'button[aria-label="展开侧边栏"]',
    'button[aria-label="Expand sidebar"]',
    'button[aria-label="打开导航"]',
    'button[aria-label="Open navigation"]',
  ].join(',')
  // The script runs as the document is created: WebView2 (Windows) runs it
  // before there is an <html> element to add to, which WKWebView has.
  const addStyle = (css) => {
    const style = document.createElement('style')
    style.textContent = css
    const put = () =>
      (document.head || document.documentElement).appendChild(style)
    if (document.documentElement) return put()
    const wait = new MutationObserver(() => {
      if (document.documentElement) {
        wait.disconnect()
        put()
      }
    })
    wait.observe(document, { childList: true })
  }
  // The app's changes to the admin's UI (page.css).
  addStyle(__CSS__)
  const call = (name, arg) => {
    try {
      window.mygo?.call(name, arg)
    } catch {
      /* the page works without it */
    }
  }

  // The pages the native header names (navPaths, contentTitle in
  // views.go): their own title is hidden, the rest of their header kept.
  const NAV = new Set(__NAV__)
  const applyTitled = () => {
    const root = document.documentElement
    if (!root) return
    const path = (location.hash.replace(/^#/, '') || '/').replace(/[?#].*$/, '')
    root.classList.toggle('mx-desktop-titled', NAV.has(path))
    scheduleCount()
  }

  // The count beside the title of a page the native header names (the
  // title block of page.css): a number of its own in the block, as "10"
  // beside 草稿箱. The app shows it in the glass breadcrumb.
  const TITLE_BLOCK = ':has(> :is(' + sidebarButtons + ')):has(h1, h2)'
  const COUNT = /^\d[\d,.]*$/
  let lastCount = null,
    countTimer = 0
  const reportCount = () => {
    countTimer = 0
    const route = location.hash.replace(/^#/, '') || '/'
    let count = ''
    const block =
      document.documentElement.classList.contains('mx-desktop-titled') &&
      document.querySelector(TITLE_BLOCK)
    if (block) {
      for (const el of block.querySelectorAll('span, small, sup')) {
        const text = el.textContent.trim()
        if (!el.firstElementChild && COUNT.test(text)) {
          count = text
          break
        }
      }
    }
    const key = route + '\n' + count
    if (key === lastCount) return
    lastCount = key
    call('Bridge.HeaderCount', { route, count })
  }
  const scheduleCount = () => {
    if (!countTimer) countTimer = setTimeout(reportCount, 100)
  }

  let last = null
  const report = () => {
    applyTheme()
    applyTitled()
    const route = location.hash.replace(/^#/, '') || '/'
    if (route === last) return
    last = route
    call('Bridge.Route', route)
  }
  for (const name of ['pushState', 'replaceState']) {
    const original = history[name]
    history[name] = function (...args) {
      const result = original.apply(this, args)
      queueMicrotask(report)
      return result
    }
  }
  addEventListener('hashchange', report)
  addEventListener('popstate', report)
  addEventListener('DOMContentLoaded', report)

  // As isPublicRoute in views.go: the pages shown without a session.
  const isPublic = (route) => /^\/(?:login|setup|collab)(?=$|[/?])/.test(route)
  let ready = false,
    observer = null
  const markReady = () => {
    if (ready) return
    ready = true
    if (observer) observer.disconnect()
    // Once the frame with it has painted.
    requestAnimationFrame(() =>
      requestAnimationFrame(() => call('Bridge.Ready', true)),
    )
  }
  const checkReady = () => {
    const app = document.getElementById('app')
    if (
      !app ||
      !app.firstElementChild ||
      document.getElementById('initial-loader')
    )
      return
    const route = location.hash.replace(/^#/, '') || '/'
    if (isPublic(route) || document.querySelector(sidebarButtons)) markReady()
  }
  addEventListener('DOMContentLoaded', () => {
    call('Bridge.Ready', false)
    observer = new MutationObserver(checkReady)
    observer.observe(document.documentElement, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['aria-label'],
    })
    checkReady()
    setTimeout(markReady, __READY_TIMEOUT__)
  })

  // The write pages (writing.go).
  const isWrite = () =>
    /^\/(?:posts|notes|pages)\/edit(?=$|\?)/.test(
      location.hash.replace(/^#/, ''),
    )
  // The root editor, not one nested in a block such as a callout.
  const EDITOR =
    '[data-lexical-editor="true"]:not([data-lexical-editor] [data-lexical-editor]), .cm-content'

  // Color themes (themes.go): the CSS of each, every rule under
  // html[data-mx-theme], and the one chosen, which the app changes through
  // __mxDesktopTheme.
  const THEMES = __THEMES__
  let theme = __THEME__
  addStyle(Object.values(THEMES).join('\n'))
  // Whether the page defines the variables the themes override: checked
  // once a theme is to apply, with none applied, and again after a change.
  let themeChecked = false,
    themeOK = false
  const THEME_VARS = [
    '--color-fg',
    '--color-surface-page',
    '--color-neutral-500',
  ]
  const applyTheme = () => {
    const root = document.documentElement
    // The admin's styles are in from DOMContentLoaded.
    if (!root || document.readyState === 'loading') return
    const on =
      !!(theme && THEMES[theme.id]) && (theme.scope === 'global' || isWrite())
    if (on && !themeChecked) {
      themeChecked = true
      root.removeAttribute('data-mx-theme')
      const style = getComputedStyle(root)
      themeOK = THEME_VARS.every(
        (name) => style.getPropertyValue(name).trim() !== '',
      )
      if (!themeOK) call('Bridge.ThemeUnsupported')
    }
    if (on && themeOK) {
      if (root.getAttribute('data-mx-theme') !== theme.id)
        root.setAttribute('data-mx-theme', theme.id)
    } else if (root.hasAttribute('data-mx-theme')) {
      root.removeAttribute('data-mx-theme')
    }
  }
  window.__mxDesktopTheme = (t) => {
    theme = t
    themeChecked = false
    applyTheme()
  }

  const CJK =
    /[\u3040-\u30FF\u3400-\u4DBF\u4E00-\u9FFF\uF900-\uFAFF\uAC00-\uD7AF]/g
  const countWords = (text) => {
    const cjk = (text.match(CJK) || []).length
    const words = (
      text
        .replaceAll(CJK, ' ')
        .match(/[\p{L}\p{N}]+(?:['’.-][\p{L}\p{N}]+)*/gu) || []
    ).length
    return cjk + words
  }
  let lastStats = '',
    statsTimer = 0
  const reportStats = () => {
    statsTimer = 0
    if (!isWrite()) {
      lastStats = ''
      return
    }
    const editor = document.querySelector(EDITOR)
    const stats = { words: editor ? countWords(editor.innerText || '') : 0 }
    const key = JSON.stringify(stats)
    if (key === lastStats) return
    lastStats = key
    call('Bridge.WriteStats', stats)
  }
  const scheduleStats = () => {
    if (!isWrite()) {
      lastStats = ''
      return
    }
    if (!statsTimer) statsTimer = setTimeout(reportStats, 400)
  }

  const typed = (e) => {
    if (
      typewriter &&
      isWrite() &&
      e.target instanceof Element &&
      e.target.closest(EDITOR)
    )
      requestAnimationFrame(centerCaret)
  }

  // Typewriter scrolling: the caret's line a little above the middle of
  // the editor's scroll area, leaving room below it for the editor's menus
  // (the slash menu flips above the caret, under the toolbar, without it).
  const TYPEWRITER_LINE = 0.4
  let typewriter = false
  // The scroll area around the editor; never the document, which the
  // admin's layout keeps still.
  const scroller = (el) => {
    for (
      let n = el.parentElement;
      n && n !== document.body;
      n = n.parentElement
    ) {
      if (
        /auto|scroll/.test(getComputedStyle(n).overflowY) &&
        n.scrollHeight > n.clientHeight
      )
        return n
    }
    return null
  }
  const caretRect = (range) => {
    const rects = range.getClientRects()
    // A DOMRectList, which has no .at().
    // eslint-disable-next-line unicorn/prefer-at
    if (rects.length) return rects[rects.length - 1]
    const r = range.getBoundingClientRect()
    if (r.height) return r
    const node =
      range.startContainer.nodeType === 1
        ? range.startContainer
        : range.startContainer.parentElement
    return node ? node.getBoundingClientRect() : null
  }
  const centerCaret = () => {
    if (!typewriter || !isWrite()) return
    const sel = getSelection()
    if (!sel || !sel.rangeCount || !sel.isCollapsed) return
    const range = sel.getRangeAt(0)
    const node =
      range.startContainer.nodeType === 1
        ? range.startContainer
        : range.startContainer.parentElement
    const editor = node && node.closest('[data-lexical-editor="true"]')
    if (!editor) return
    const rect = caretRect(range)
    const box = scroller(editor)
    if (!rect || !box) return
    const delta =
      rect.top +
      rect.height / 2 -
      (box.getBoundingClientRect().top + box.clientHeight * TYPEWRITER_LINE)
    if (Math.abs(delta) > 2) box.scrollBy({ top: delta })
  }

  window.__mxDesktopModes = (focus, tw, header) => {
    if (header)
      document.documentElement.style.setProperty(
        '--mx-desktop-header',
        header + 'px',
      )
    const c = document.documentElement.classList
    c.toggle('mx-desktop-focus', !!focus)
    c.toggle('mx-desktop-typewriter', !!tw)
    typewriter = !!tw
    if (typewriter) requestAnimationFrame(centerCaret)
  }

  // The AI assistant of the write page (ideas_draft.go): open it and fill
  // in the prompt, without sending it. Waits for the draft whose text
  // includes `expect` to show in the editor; resolves to "ok", "disabled"
  // (not a Lexical draft) or "timeout".
  const AI_BUTTON = [
    'button[aria-label="AI 助手"]',
    'button[aria-label="AI assistant"]',
    'button[aria-label="隐藏 AI 助手"]',
    'button[aria-label="Hide AI assistant"]',
  ].join(',')
  const AI_INPUT = [
    'textarea[placeholder="输入修改要求"]',
    'textarea[placeholder="Describe what to change"]',
  ].join(',')
  window.__mxDesktopAiPrompt = (text, timeout, expect) =>
    new Promise((resolve) => {
      const deadline = Date.now() + (timeout || 20000)
      let clicked = false
      const step = () => {
        if (Date.now() > deadline) return resolve('timeout')
        const editor = isWrite() && document.querySelector(EDITOR)
        const loaded =
          editor &&
          editor.matches('[data-lexical-editor="true"]') &&
          (!expect || (editor.textContent || '').includes(expect))
        const button = loaded && document.querySelector(AI_BUTTON)
        if (!button) return setTimeout(step, 200)
        if (button.disabled) return resolve('disabled')
        // Click once: a second click would close the panel.
        if (button.getAttribute('aria-pressed') !== 'true' && !clicked) {
          clicked = true
          button.click()
        }
        const input = document.querySelector(AI_INPUT)
        if (!input) return setTimeout(step, 200)
        const set = Object.getOwnPropertyDescriptor(
          HTMLTextAreaElement.prototype,
          'value',
        ).set
        set.call(input, text)
        // React's onChange of a textarea listens to input events. No Enter,
        // no click on Send: the user sends it.
        input.dispatchEvent(new Event('input', { bubbles: true }))
        input.focus()
        resolve('ok')
      }
      step()
    })

  addEventListener('DOMContentLoaded', () => {
    try {
      window.mygo?.call('Bridge.Modes').then(
        (m) => {
          if (!m) return
          window.__mxDesktopModes(m.focus, m.typewriter, m.header)
          if (
            m.theme &&
            (m.theme.id !== theme.id || m.theme.scope !== theme.scope)
          )
            window.__mxDesktopTheme(m.theme)
        },
        () => {},
      )
    } catch {
      /* the page works without it */
    }
    // The admin's layout fills the window and scrolls inside: a write page
    // scrolled as a whole (the editor bringing the caret into view through
    // every ancestor) shows a strip of nothing at the bottom. Keep it still.
    addEventListener(
      'scroll',
      (e) => {
        const doc = document.scrollingElement
        if (e.target === document && isWrite() && doc && doc.scrollTop)
          doc.scrollTop = 0
      },
      true,
    )
    new MutationObserver(() => {
      scheduleStats()
      scheduleCount()
    }).observe(document.documentElement, {
      childList: true,
      subtree: true,
      characterData: true,
    })
    addEventListener('input', typed, true)
    addEventListener(
      'keyup',
      (e) => {
        if (typewriter && /^(?:Arrow|Page|Home|End|Enter)/.test(e.key))
          requestAnimationFrame(centerCaret)
      },
      true,
    )
    document.addEventListener('selectionchange', () => {
      if (typewriter) requestAnimationFrame(centerCaret)
    })
    scheduleStats()
    applyTheme()
    applyTitled()
  })
})()
