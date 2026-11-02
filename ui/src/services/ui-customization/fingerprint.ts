export const runtimeFingerprint = (pkg: { jsHash?: string; runtimeFingerprint?: string; channelCapabilities?: string[] }) =>
  pkg.runtimeFingerprint || (pkg.channelCapabilities?.length
    ? `${pkg.jsHash || ''}:${JSON.stringify([...new Set(pkg.channelCapabilities)].sort())}`
    : pkg.jsHash || '')
