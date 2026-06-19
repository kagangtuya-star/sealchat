export interface ChannelCreateSessionState {
  token: string
  isObserver: boolean
  observerMode: boolean
  observerWorldId: string
}

export const canCreateChannelSession = (state: ChannelCreateSessionState) => {
  return !!String(state.token || '').trim()
    && !state.isObserver
    && !state.observerMode
    && !String(state.observerWorldId || '').trim()
}
