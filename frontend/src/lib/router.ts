import { writable } from 'svelte/store'

function normalizePath(pathname: string): string {
  if (!pathname || pathname === '/') return '/'
  return pathname.replace(/\/+$/, '') || '/'
}

export const routePath = writable<string>(normalizePath(window.location.pathname))

function navigate(to: string, replaceState: boolean) {
  const url = new URL(to, window.location.origin)
  if (url.origin !== window.location.origin) {
    window.location.assign(url)
    return
  }
  const nextURL = `${url.pathname}${url.search}${url.hash}`
  if (replaceState) history.replaceState(history.state, '', nextURL)
  else history.pushState(history.state, '', nextURL)
  routePath.set(normalizePath(url.pathname))
}

export function push(to: string) {
  navigate(to, false)
}

export function replace(to: string) {
  navigate(to, true)
}

export function startRouter() {
  const handlePopState = () => {
    routePath.set(normalizePath(window.location.pathname))
  }
  const handleClick = (event: MouseEvent) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return

    const target = event.target
    const anchor = target instanceof Element ? target.closest<HTMLAnchorElement>('a[href]') : null
    if (!anchor || anchor.target || anchor.download || anchor.hasAttribute('data-native-navigation')) return

    const url = new URL(anchor.href, window.location.href)
    if (url.origin !== window.location.origin || url.pathname.startsWith('/api/')) return

    event.preventDefault()
    push(`${url.pathname}${url.search}${url.hash}`)
  }

  window.addEventListener('popstate', handlePopState)
  document.addEventListener('click', handleClick)
  return () => {
    window.removeEventListener('popstate', handlePopState)
    document.removeEventListener('click', handleClick)
  }
}
