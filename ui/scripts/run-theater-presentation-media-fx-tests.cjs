// Uses the existing TypeScript compiler and node:assert; no browser or test framework.
const fs = require('node:fs')
const path = require('node:path')
const Module = require('node:module')
const ts = require('typescript')
const uiRoot = path.join(__dirname, '..')
// The spec exercises the real attachment helper with a controlled API transport.
// Avoid evaluating the browser-only configuration (window / import.meta.env) in Node.
const config = path.join(uiRoot, 'src/stores/_config.ts')
require.cache[config] = { exports: { api: { get() { throw Error('unexpected API request') } }, urlBase: 'http://sealchat.test' } }
const original = Module._resolveFilename
Module._resolveFilename = function (request, parent, ...args) {
  if (request.startsWith('@/')) request = path.join(uiRoot, 'src', request.slice(2))
  return original.call(this, request, parent, ...args)
}
require.extensions['.ts'] = (module, filename) => module._compile(ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename,
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
}).outputText, filename)
require(path.join(__dirname, 'theater-presentation-media-fx.spec.ts'))
