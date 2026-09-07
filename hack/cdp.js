// Queries the live ComicGo WebView2 page over the Chrome DevTools Protocol.
// Launch the exe with WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS containing
// --remote-debugging-port=9222 first, then run: node hack/cdp.js
const CDP = 'http://127.0.0.1:9222'

async function main() {
  let list
  for (let i = 0; i < 20; i++) {
    try {
      list = await (await fetch(CDP + '/json')).json()
      if (list.some((t) => t.type === 'page')) break
    } catch {}
    await new Promise((r) => setTimeout(r, 500))
  }
  const page = list && list.find((t) => t.type === 'page')
  if (!page) {
    console.error('no CDP page target')
    return
  }
  const ws = new WebSocket(page.webSocketDebuggerUrl)
  await new Promise((res, rej) => {
    ws.onopen = res
    ws.onerror = rej
  })
  let id = 0
  const pending = new Map()
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data)
    if (m.id && pending.has(m.id)) {
      pending.get(m.id)(m)
      pending.delete(m.id)
    }
  }
  const send = (method, params = {}) =>
    new Promise((resolve) => {
      const mid = ++id
      pending.set(mid, resolve)
      ws.send(JSON.stringify({ id: mid, method, params }))
    })

  await send('Runtime.enable')
  const expr = `(async () => {
    const out = { hasGo: false, count: null, countErr: null, body: '', title: '' };
    try { out.hasGo = !!window.go && !!window.go.main && !!window.go.main.App; } catch(e){ out.goErr=String(e); }
    try { out.count = await window.go.main.App.CountComics(''); } catch(e){ out.countErr = String(e && e.message || e); }
    try { out.body = document.body ? document.body.innerText.slice(0, 900) : 'NO BODY'; } catch(e){ out.bodyErr=String(e); }
    out.title = document.title;
    return JSON.stringify(out);
  })()`
  const r = await send('Runtime.evaluate', {
    expression: expr,
    awaitPromise: true,
    returnByValue: true,
  })
  console.log('RESULT', r.result?.result?.value ?? JSON.stringify(r))
  ws.close()
}

main().catch((e) => {
  console.error('cdp error', e)
  process.exit(1)
})
