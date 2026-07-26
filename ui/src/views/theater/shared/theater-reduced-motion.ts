export interface TheaterReducedMotionState {
  systemReducedMotion: boolean
  userReducedMotion: boolean
  forceMotion: boolean
  effectiveReducedMotion: boolean
}

type MatchMedia = (query: string) => Pick<MediaQueryList, 'matches'>

export const resolveTheaterReducedMotion = (
  userReducedMotion = false,
  matchMedia: MatchMedia = window.matchMedia.bind(window),
  forceMotion = true,
): TheaterReducedMotionState => {
  const reduceMatches = matchMedia('(prefers-reduced-motion: reduce)').matches === true
  const noPreferenceMatches = matchMedia('(prefers-reduced-motion: no-preference)').matches === true
  // Treat the system preference as explicit only when the two exclusive queries agree.
  // This avoids false positives from incomplete matchMedia mocks and WebView polyfills.
  const systemReducedMotion = reduceMatches && !noPreferenceMatches
  const normalizedUserReducedMotion = userReducedMotion === true
  return {
    systemReducedMotion,
    userReducedMotion: normalizedUserReducedMotion,
    forceMotion,
    effectiveReducedMotion: forceMotion ? false : systemReducedMotion || normalizedUserReducedMotion,
  }
}
