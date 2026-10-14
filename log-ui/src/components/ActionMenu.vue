<template>
  <div v-if="open" ref="menu" class="amenu" role="menu" :aria-label="label" :style="{ left: x + 'px', top: y + 'px' }" data-testid="action-menu"
    @keydown="onKey">
    <template v-for="(item, i) in items" :key="item.label">
      <button type="button" role="menuitem" class="aitem" :disabled="!!item.disabled" :title="item.disabled || undefined"
        :data-testid="'action-' + item.id" @click="run(item)" :ref="(el) => { buttons[i] = el as HTMLButtonElement }">
        {{ item.label }}
        <span v-if="item.disabled" class="why">{{ item.disabled }}</span>
      </button>
    </template>
  </div>
</template>

<script lang="ts">
import { defineComponent, nextTick, onBeforeUnmount, ref, watch, type PropType } from 'vue'

export interface Action {
  id: string
  label: string
  // A reason when the action is unavailable (shown, never hidden).
  disabled?: string
  run: () => void
}

// ActionMenu is one popup list of row actions. The visible row button, the
// right-click menu and the keyboard all open this same list; Escape closes it
// and returns focus to the control that opened it.
export default defineComponent({
  name: 'ActionMenu',
  props: {
    open: { type: Boolean, default: false },
    items: { type: Array as PropType<Action[]>, required: true },
    x: { type: Number, default: 0 },
    y: { type: Number, default: 0 },
    opener: { type: Object as PropType<HTMLElement | null>, default: null },
    label: { type: String, default: 'Row actions' }
  },
  emits: ['close'],
  setup (props, { emit }) {
    const menu = ref<HTMLElement | null>(null)
    const buttons = ref<HTMLButtonElement[]>([])

    function close (restore: boolean) {
      emit('close')
      if (restore && props.opener) {
        props.opener.focus()
      }
    }
    function run (item: Action) {
      if (item.disabled) {
        return
      }
      close(false)
      item.run()
    }
    function enabled () {
      return buttons.value.filter((b) => b && !b.disabled)
    }
    function onKey (e: KeyboardEvent) {
      const list = enabled()
      const at = list.indexOf(document.activeElement as HTMLButtonElement)
      if (e.key === 'Escape') {
        e.preventDefault()
        close(true)
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        list[(at + 1) % list.length]?.focus()
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        list[(at - 1 + list.length) % list.length]?.focus()
      } else if (e.key === 'Tab') {
        close(false)
      }
    }
    function outside (e: MouseEvent) {
      if (props.open && menu.value && !menu.value.contains(e.target as Node)) {
        close(false)
      }
    }
    watch(() => props.open, async (o) => {
      if (o) {
        await nextTick()
        // Keep the menu inside the viewport.
        const el = menu.value
        if (el) {
          const r = el.getBoundingClientRect()
          if (r.right > window.innerWidth) {
            el.style.left = Math.max(4, window.innerWidth - r.width - 4) + 'px'
          }
          if (r.bottom > window.innerHeight) {
            el.style.top = Math.max(4, props.y - r.height) + 'px'
          }
        }
        enabled()[0]?.focus()
        document.addEventListener('mousedown', outside)
      } else {
        document.removeEventListener('mousedown', outside)
      }
    })
    onBeforeUnmount(() => document.removeEventListener('mousedown', outside))
    return { menu, buttons, run, onKey }
  }
})
</script>

<style scoped>
.amenu { position: fixed; z-index: 1000; min-width: 13rem; max-width: 20rem; background: var(--surface-overlay, var(--surface-card)); border: 1px solid var(--surface-border);
  border-radius: 8px; padding: 4px; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35); display: flex; flex-direction: column; }
.aitem { text-align: left; background: none; border: 0; color: var(--text-color); padding: 8px 10px; border-radius: 6px; cursor: pointer; font: inherit; font-size: 14px; }
.aitem:hover:not(:disabled), .aitem:focus-visible { background: var(--surface-hover); outline: none; }
.aitem:focus-visible { box-shadow: inset 0 0 0 2px var(--primary-color); }
.aitem:disabled { color: var(--text-color-secondary); cursor: not-allowed; }
.why { display: block; font-size: 11px; }
</style>
