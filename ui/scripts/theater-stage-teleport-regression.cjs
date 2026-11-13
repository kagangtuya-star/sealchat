// node scripts/theater-stage-teleport-regression.cjs [--production]
// Compile the real Teleport bindings and loading overlay with Vue's real renderer.
// A small in-memory host models document.querySelector without browser dependencies.
if (process.argv.includes('--production')) process.env.NODE_ENV = 'production'

const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const { test } = require('node:test')
const { parse } = require('@vue/compiler-sfc')
const { parse: parseTemplate } = require('@vue/compiler-dom')
const Vue = require('vue')
const { computed, createRenderer, defineComponent, markRaw, nextTick, onBeforeUnmount, onMounted, ref } = Vue

const readTemplate = filename => parse(fs.readFileSync(path.resolve(__dirname, filename), 'utf8')).descriptor.template.content
const elements = node => [node, ...(node.children || []).flatMap(elements)]
const stageTemplate = readTemplate('../src/views/theater/stage/StageApp.vue')
const teleports = elements(parseTemplate(stageTemplate)).filter(node => node.tag === 'Teleport')
assert.equal(teleports.length, 1, 'identify the stage toolbar Teleport')
const teleportOpening = `<Teleport ${teleports[0].props.map(prop => prop.loc.source).join(' ')}>`
const hostTemplate = readTemplate('../src/views/theater/host/TheaterView.vue')
const loadingOverlay = elements(parseTemplate(hostTemplate)).find(node => node.props?.some(prop => (
  prop.name === 'class' && prop.value?.content === 'theater-sync-loading'
)))
assert.ok(loadingOverlay, 'identify the real sync loading overlay')
const renderTemplate = template => Vue.compile(template)

const createNode = type => markRaw({ type, parent: null, children: [], props: {} })
const findByClass = (node, className) => node.props.class === className
  ? node
  : node.children.map(child => findByClass(child, className)).find(Boolean) || null
const detach = node => {
  if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1)
  node.parent = null
}

function mountStage({ minimal, layout }) {
  const root = createNode('root')
  const errors = []
  const warnings = []
  const lifecycle = { mounts: 0, unmounts: 0 }
  const renderer = createRenderer({
    createElement: createNode,
    createText: text => Object.assign(createNode('#text'), { text }),
    createComment: text => Object.assign(createNode('#comment'), { text }),
    insert(node, parent, anchor = null) {
      assert.ok(parent, 'Teleport must have an existing insertion target')
      detach(node)
      const index = anchor ? parent.children.indexOf(anchor) : -1
      parent.children.splice(index < 0 ? parent.children.length : index, 0, node)
      node.parent = parent
    },
    remove: detach,
    setText(node, text) { node.text = text },
    setElementText(node, text) { node.text = text; node.children = [] },
    parentNode: node => node.parent,
    nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] || null,
    // Search only the attached tree: a component's root is not visible during its own mount.
    querySelector: selector => findByClass(root, selector.slice(1)),
    patchProp(node, key, previous, value) { node.props[key] = value },
  })
  const state = {
    theaterPermissions: ref([]),
    theaterSyncReady: ref(false),
    pipMinimal: ref(minimal),
    theaterToolbarLayout: ref(layout),
  }
  const ToolbarProbe = defineComponent({
    props: ['permissions', 'syncReady'],
    setup() {
      onMounted(() => { lifecycle.mounts++ })
      onBeforeUnmount(() => { lifecycle.unmounts++ })
    },
    render: renderTemplate('<button class="toolbar-probe">{{ permissions.length }}:{{ syncReady }}</button>'),
  })
  const StageFixture = defineComponent({
    props: ['pipMinimal', 'permissions', 'syncReady', 'toolbarLayout'],
    components: { ToolbarProbe },
    setup(props) {
      return { props, stageAppRef: ref(null), theaterToolbarLayout: computed(() => props.toolbarLayout) }
    },
    // Keep the production bindings; replace unrelated toolbar controls with a stateful child.
    render: renderTemplate(`<section ref="stageAppRef" class="theater-stage-app">
      <header class="theater-stage-toolbar">
        ${teleportOpening}
          <div class="theater-panel-switches-wrap">
            <ToolbarProbe :permissions="permissions" :sync-ready="syncReady" />
          </div>
        </Teleport>
      </header>
    </section>`),
  })
  const HostFixture = defineComponent({
    components: { StageFixture },
    setup: () => state,
    render: renderTemplate(`<main>
      <StageFixture :pip-minimal="pipMinimal" :permissions="theaterPermissions"
        :sync-ready="theaterSyncReady" :toolbar-layout="theaterToolbarLayout" />
      ${loadingOverlay.loc.source}
    </main>`),
  })
  const app = renderer.createApp(HostFixture)
  app.config.errorHandler = error => errors.push(error.message)
  app.config.warnHandler = warning => warnings.push(warning)
  app.mount(root)
  return { app, state, lifecycle, errors, warnings, find: className => findByClass(root, className) }
}

for (const minimal of [false, true]) {
  for (const layout of ['horizontal', 'vertical']) {
    test(`toolbar Teleport: initial minimal=${minimal}, layout=${layout}`, async t => {
      const fixture = mountStage({ minimal, layout })
      t.after(() => {
        fixture.app.unmount()
        assert.equal(fixture.lifecycle.unmounts, 1, 'the child is cleaned up exactly once')
        assert.deepEqual(fixture.errors, [])
        assert.deepEqual(fixture.warnings, [])
      })
      const { state, lifecycle, errors, warnings, find } = fixture
      const assertHealthy = () => {
        assert.deepEqual(errors, [], 'rendering must not access an unmounted component')
        assert.deepEqual(warnings, [], 'Teleport must not resolve a missing enabled target')
        assert.equal(lifecycle.mounts, 1, 'moving the toolbar preserves its component instance')
        assert.equal(lifecycle.unmounts, 0)
      }
      const assertPlacement = () => {
        const expected = state.pipMinimal.value || state.theaterToolbarLayout.value === 'vertical'
          ? 'theater-stage-app'
          : 'theater-stage-toolbar'
        assert.equal(find('theater-panel-switches-wrap').parent, find(expected))
      }

      await nextTick()
      assertHealthy()
      assertPlacement()
      assert.ok(find('theater-sync-loading'), 'show the overlay before sync finishes')
      const probe = find('toolbar-probe')

      // Permission publication precedes sync readiness, just as in TheaterSyncClient.reload().
      state.theaterPermissions.value = ['stage.view']
      await nextTick()
      assertHealthy()
      assert.equal(probe.text, '1:false')
      state.theaterSyncReady.value = true
      await nextTick()
      assertHealthy()
      assert.equal(probe.text, '1:true')
      assert.equal(find('theater-sync-loading'), null, 'a completed sync removes the loading overlay')

      // Reconciliation republishes equal permissions and can later revoke them.
      for (const permissions of [['stage.view'], ['stage.view'], []]) {
        state.theaterPermissions.value = permissions
        await nextTick()
        assertHealthy()
        assert.equal(probe.text, `${permissions.length}:true`)
      }

      // Enter/leave minimal PIP after mount, then move between toolbar layouts.
      for (const pipMinimal of [!minimal, minimal]) {
        state.pipMinimal.value = pipMinimal
        await nextTick()
        assertHealthy()
        assertPlacement()
      }
      state.theaterToolbarLayout.value = layout === 'horizontal' ? 'vertical' : 'horizontal'
      await nextTick()
      assertHealthy()
      assertPlacement()
      assert.equal(find('toolbar-probe'), probe, 'all moves keep the same mounted child')
    })
  }
}
