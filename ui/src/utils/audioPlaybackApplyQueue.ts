export class AudioPlaybackApplyQueue {
  private readonly queues = new Map<string, Promise<unknown>>();

  enqueue<T>(key: string, task: () => Promise<T> | T): Promise<T> {
    const normalizedKey = key || 'default';
    const previous = this.queues.get(normalizedKey) ?? Promise.resolve();
    const run = previous.catch(() => undefined).then(task);
    let settled: Promise<T>;
    settled = run.finally(() => {
      if (this.queues.get(normalizedKey) === settled) {
        this.queues.delete(normalizedKey);
      }
    });
    this.queues.set(normalizedKey, settled);
    return settled;
  }
}
