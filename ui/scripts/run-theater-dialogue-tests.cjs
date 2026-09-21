// Uses the existing TypeScript compiler and node:assert; no test framework needed.
const path = require('node:path')
const scripts = [
  'theater-dialogue-residency-layout.spec.ts',
  'theater-dialogue-controller-runtime.spec.ts',
  'theater-dialogue-queue-runtime.spec.ts',
  'theater-dialogue-playback-runtime.spec.ts',
  'theater-dialogue-sync.spec.cjs',
  'theater-dialogue-drag.spec.cjs',
  'theater-dialogue-panel.spec.cjs',
]
const selected = process.argv[2]
if (!selected) {
  const { execFileSync } = require('node:child_process')
  for (const file of scripts) execFileSync(process.execPath, [__filename, file], { stdio: 'inherit' })
} else {
  if (!scripts.includes(selected)) throw new Error('Unknown dialogue test')
  if (selected.endsWith('.ts')) {
    const fs = require('node:fs')
    const Module = require('node:module')
    const ts = require('typescript')
    const original = Module._resolveFilename
    Module._resolveFilename = function (request, parent, ...args) {
      if (request.startsWith('@/')) request = path.join(__dirname, '../src', request.slice(2))
      return original.call(this, request, parent, ...args)
    }
    require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
      fileName: filename, compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
    }).outputText, filename)
  }
  require(path.join(__dirname, selected))
}
