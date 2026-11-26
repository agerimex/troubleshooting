import { expect, test, type Page } from '@playwright/test'
import { mockApi, span } from './mockApi'

// Layout checks: at desktop, tablet and phone widths no page scrolls
// sideways and no two controls (buttons, inputs, selects, links) overlap.
// Wide tables scroll inside their own container, which is allowed.

const WIDTHS = [1440, 1024, 390]
const PAGES = ['logs', 'executions', 'problems', 'releases', 'quality', 'settings']

async function layoutIssues (page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const vw = document.documentElement.clientWidth
    const issues: string[] = []
    if (document.documentElement.scrollWidth > vw + 1) {
      issues.push(`page scrolls sideways: ${document.documentElement.scrollWidth} > ${vw}`)
    }
    const visible = (e: Element) => {
      const r = e.getBoundingClientRect()
      const s = getComputedStyle(e)
      return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none'
    }
    // A table cell's content must stay inside the cell: a long name that runs
    // into the next column is drawn over it.
    for (const cell of [...document.querySelectorAll('td, th')].filter(visible)) {
      const box = cell.getBoundingClientRect()
      for (const child of [...cell.querySelectorAll('*')].filter(visible)) {
        const r = child.getBoundingClientRect()
        if (r.right > box.right + 2 || r.left < box.left - 2) {
          issues.push(`spills out of its cell: ${child.tagName} "${(child as HTMLElement).innerText?.trim().slice(0, 30) ?? ''}"`)
          break
        }
      }
    }
    const controls = [...document.querySelectorAll('button, input, select, textarea, a')].filter(visible)
    const label = (e: Element) => `${e.tagName} "${((e as HTMLElement).innerText || (e as HTMLInputElement).placeholder || e.getAttribute('aria-label') || '').trim().slice(0, 30)}"`
    for (let i = 0; i < controls.length; i++) {
      for (let j = i + 1; j < controls.length; j++) {
        const a = controls[i]
        const b = controls[j]
        if (a.contains(b) || b.contains(a)) continue
        const x = a.getBoundingClientRect()
        const y = b.getBoundingClientRect()
        const ox = Math.min(x.right, y.right) - Math.max(x.left, y.left)
        const oy = Math.min(x.bottom, y.bottom) - Math.max(x.top, y.top)
        if (ox > 2 && oy > 2) issues.push(`overlap: ${label(a)} × ${label(b)}`)
      }
    }
    return issues
  })
}

for (const width of WIDTHS) {
  test(`UI-LAYOUT-001 @${width}px: no page scrolls sideways, no cell content spills, no controls overlap`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 })
    const spans = [
      span('/api/v1/problems/services', { spanId: '00000000000000c1', service: 'LogAnalysis', spanKind: 'server', status: 'ok', statusCode: 2, duration: 27000 }),
      span('a-rather-long-operation-name/with/several/segments/to/wrap', { spanId: '00000000000000c2', service: 'recording-worker', status: 'error', statusCode: 1 })
    ]
    await mockApi(page, () => ({ spans }), () => ({ count: spans.length }))
    await page.goto('/')
    for (const p of PAGES) {
      await page.getByTestId('main-nav-' + p).click()
      await page.waitForTimeout(400)
      expect(await layoutIssues(page), `${p} at ${width}px`).toEqual([])
    }
  })
}
