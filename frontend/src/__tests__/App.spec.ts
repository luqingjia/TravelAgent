import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'

import App from '../App.vue'

describe('App', () => {
  it('mounts router outlet', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/', component: { template: '<div>outlet-ok</div>' } }],
    })
    router.push('/')
    await router.isReady()

    const wrapper = mount(App, {
      global: { plugins: [router] },
    })
    expect(wrapper.text()).toContain('outlet-ok')
  })
})
