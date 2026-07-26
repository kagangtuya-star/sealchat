import assert from 'node:assert/strict'

import { resolveTheaterReducedMotion } from '../src/views/theater/shared/theater-reduced-motion'

const matchMedia = (reduce: boolean, noPreference: boolean) => (query: string) => ({
  matches: query.includes('no-preference') ? noPreference : reduce,
})

assert.deepEqual(resolveTheaterReducedMotion(false, matchMedia(false, true)), {
  systemReducedMotion: false,
  userReducedMotion: false,
  forceMotion: true,
  effectiveReducedMotion: false,
})
assert.deepEqual(resolveTheaterReducedMotion(false, matchMedia(true, false)), {
  systemReducedMotion: true,
  userReducedMotion: false,
  forceMotion: true,
  effectiveReducedMotion: false,
})
assert.deepEqual(resolveTheaterReducedMotion(true, matchMedia(false, true)), {
  systemReducedMotion: false,
  userReducedMotion: true,
  forceMotion: true,
  effectiveReducedMotion: false,
})
assert.deepEqual(resolveTheaterReducedMotion(false, matchMedia(true, true)), {
  systemReducedMotion: false,
  userReducedMotion: false,
  forceMotion: true,
  effectiveReducedMotion: false,
})
assert.deepEqual(resolveTheaterReducedMotion(false, matchMedia(true, false), false), {
  systemReducedMotion: true,
  userReducedMotion: false,
  forceMotion: false,
  effectiveReducedMotion: true,
})

console.log('theater reduced-motion runtime tests passed')
