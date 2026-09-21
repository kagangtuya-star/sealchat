/** Transient portrait residency. Only the playing queue item may enter here. */
export interface DialogueActorReference {
  worldId: string
  sourceChannelId: string
  identityId: string
  sharedIdentityId?: string | null
}

export type DialogueInactiveStyle = 'dim' | 'dim-grayscale' | 'none'

export interface DialogueResidencyRules {
  maxPortraits: number
  allowList: readonly string[]
  denyList: readonly string[]
}

export interface DialogueResident<T> {
  actorKey: string
  actor: DialogueActorReference
  portrait: T
}

// Keys are always interpreted inside a world-scoped controller, never globally.
export const dialogueActorKey = (actor: DialogueActorReference): string => (
  actor.sharedIdentityId?.trim()
    ? `shared:${actor.sharedIdentityId.trim()}`
    : `identity:${actor.identityId.trim()}`
)

export const dialoguePortraitAllowed = (key: string, rules: DialogueResidencyRules): boolean => (
  !rules.denyList.includes(key)
  && (rules.allowList.length === 0 || rules.allowList.includes(key))
)

export class DialogueResidency<T> {
  private residents: DialogueResident<T>[] = []
  private currentKey: string | null = null

  constructor(private readonly worldId: string) {}

  snapshot(): { residents: DialogueResident<T>[]; currentKey: string | null } {
    return { residents: this.residents.map(item => ({ ...item, actor: { ...item.actor } })), currentKey: this.currentKey }
  }

  /** Call from runtime.queue.current, never from message-created or waiting. */
  play(actor: DialogueActorReference | null, portrait: T | null, rules: DialogueResidencyRules): void {
    this.currentKey = null
    if (actor?.worldId === this.worldId && actor.identityId.trim()) {
      const actorKey = dialogueActorKey(actor)
      if (portrait === null) {
        this.residents = this.residents.filter(item => item.actorKey !== actorKey)
      } else if (dialoguePortraitAllowed(actorKey, rules)) {
        this.currentKey = actorKey
        const resident = { actorKey, actor: { ...actor }, portrait }
        const index = this.residents.findIndex(item => item.actorKey === actorKey)
        if (index < 0) this.residents.push(resident)
        else this.residents[index] = resident
      }
    }
    this.configure(rules)
  }

  configure(rules: DialogueResidencyRules): void {
    const limit = Number.isFinite(rules.maxPortraits)
      ? Math.max(1, Math.min(12, Math.trunc(rules.maxPortraits))) : 4
    this.residents = this.residents.filter(item => dialoguePortraitAllowed(item.actorKey, rules))
    if (!this.residents.some(item => item.actorKey === this.currentKey)) this.currentKey = null
    while (this.residents.length > limit) {
      const index = this.residents.findIndex(item => item.actorKey !== this.currentKey)
      if (index < 0) break
      this.residents.splice(index, 1)
    }
  }

  end(): void { this.currentKey = null }

  clear(): void {
    this.residents = []
    this.currentKey = null
  }
}
