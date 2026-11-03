const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const vm = require('node:vm');
const ts = require('typescript');
const pinia = require('pinia');
const vue = require('vue');

const compile = (path) => ts.transpileModule(readFileSync(require.resolve(path), 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
}).outputText;
const source = compile('../src/stores/audioStudio.ts');
function evaluate(compiled, imports = {}, globals = {}) {
  const exports = {};
  vm.runInNewContext(compiled, {
    exports, console: { ...console, warn() {}, error() {} },
    require(name) {
      if (name in imports) return imports[name];
      if (name === 'pinia') return pinia;
      if (name === 'vue') return vue;
      throw new Error(`Unexpected import: ${name}`);
    },
    ...globals,
  });
  return exports;
}
const playbackState = evaluate(compile('../src/stores/audioPlaybackState.ts'));
const applyQueue = evaluate(compile('../src/utils/audioPlaybackApplyQueue.ts'));
const sceneHelpers = evaluate(compile('../src/stores/audioStudioSceneHelpers.ts'));
const tick = () => new Promise((resolve) => setImmediate(resolve));
function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function harness(t, options = {}) {
  let nextId = 1;
  const instances = [];
  // 模拟 HTML5 Audio 的加载与 play promise 分离；playing() 在兑现前返回 false。
  class Howl {
    constructor(config) {
      this.config = config;
      this.sounds = new Map();
      this.playCalls = [];
      this.pauseQueue = [];
      this.loaded = 'unloaded';
      this.position = 0;
      this.looping = config.loop;
      this.disposed = false;
      instances.push(this);
    }
    state() { return this.loaded; }
    load() { this.loaded = 'loading'; return this; }
    finishLoad() { this.loaded = 'loaded'; this.config.onload(); }
    play(id) {
      assert.equal(this.disposed, false, 'unloaded Howl must never be played');
      const soundId = id ?? nextId++;
      this.playCalls.push(soundId);
      if (this.position >= this.duration() && !this.looping) {
        this.sounds.set(soundId, 'ended');
        queueMicrotask(() => this.config.onend(soundId));
        return undefined;
      }
      this.sounds.set(soundId, 'pending');
      return soundId;
    }
    finishPlay(id = this.playCalls.at(-1)) {
      this.sounds.set(id, 'playing');
      this.config.onplay(id);
      const pauses = this.pauseQueue.splice(0);
      pauses.forEach((soundId) => this.pause(soundId));
    }
    failPlay() {
      const id = this.playCalls.at(-1);
      this.sounds.set(id, 'ended');
      this.config.onplayerror(id, 'autoplay blocked');
    }
    playing() { return [...this.sounds.values()].some((state) => state === 'playing'); }
    pause(id) {
      for (const soundId of id == null ? this.sounds.keys() : [id]) {
        if (this.sounds.get(soundId) === 'pending') this.pauseQueue.push(soundId);
        else {
          this.sounds.set(soundId, 'paused');
          this.config.onpause(soundId);
        }
      }
      return this;
    }
    end() {
      const id = this.playCalls.at(-1);
      if (!this.looping) { this.sounds.set(id, 'ended'); this.position = 0; }
      this.config.onend(id);
    }
    unload() { this.disposed = true; this.sounds.clear(); this.loaded = 'unloaded'; return null; }
    seek(value) { if (value !== undefined) this.position = value; return this.position; }
    duration() { return 60; }
    loop(value) { if (value !== undefined) this.looping = value; return this.looping; }
    rate() { return this; }
    volume() { return this; }
  }
  const timers = new Map();
  const events = new Map();
  let timerId = 0, revision = 1, server = null;
  const calls = { get: [], post: [] };
  const api = {
    async get(url, config) {
      calls.get.push({ url, config });
      if (options.get) return options.get(url, config);
      if (url === '/api/v1/audio/state') return { data: { state: server } };
      return { data: { id: url.split('/').at(-1), name: 'audio' } };
    },
    async post(url, body) {
      calls.post.push({ url, body: body === undefined ? undefined : JSON.parse(JSON.stringify(body)) });
      if (options.post) return options.post(url, body);
      if (url === '/api/v1/audio/state') {
        server = { ...JSON.parse(JSON.stringify(body)), revision: ++revision, updatedBy: 'self' };
        return { data: { state: server } };
      }
      return { data: { streamUrl: `https://audio.test/${url}`, expiresAt: Date.now() + 60000 } };
    },
  };
  const user = { info: { id: 'self' }, checkPerm: () => options.manage !== false };
  const module = evaluate(source, {
    howler: { Howl, Howler: { volume() {}, ctx: options.ctx } },
    nanoid: { nanoid: () => `track-${nextId++}` },
    './_config': { api, urlBase: '' },
    './user': { useUserStore: () => user },
    './utils': { useUtilsStore: () => ({}) },
    './chat': { useChatStore: () => ({}), chatEvent: { on: (name, fn) => events.set(name, fn) } },
    '@/models/audio-cache': { audioDb: { assets: { put: async () => {} } }, toCachedMeta: (asset) => asset },
    '@/utils/pinyinMatch': {},
    '@/utils/embeddedRuntime': { detectEmbeddedRuntime: () => ({ runtimeKind: 'web', thirdPartyFrameLikely: false }) },
    '@/utils/uploadTimeout': {},
    './audioPlaybackState': playbackState,
    './audioStudioAssetCollections': evaluate(compile('../src/stores/audioStudioAssetCollections.ts')),
    './audioStudioSceneHelpers': sceneHelpers,
    '@/views/theater/shared/stage-types': { normalizeStageMusicSnapshot: (value) => value },
    '@/utils/audioPlaybackApplyQueue': applyQueue,
  }, {
    window: {
      localStorage: { getItem: () => null },
      setTimeout: (fn) => { const id = ++timerId; timers.set(id, fn); return id; },
      clearTimeout: (id) => timers.delete(id),
      setInterval: () => ++timerId, clearInterval() {}, addEventListener() {},
    },
    document: { addEventListener() {}, removeEventListener() {} },
  });
  const store = module.useAudioStudioStore(pinia.createPinia());
  store.currentWorldId = 'world';
  store.currentChannelId = 'channel';
  store.assets = ['a', 'b', 'c', 'd'].map((id) => ({ id, name: id }));
  const statePosts = () => calls.post.filter((call) => call.url === '/api/v1/audio/state');
  const assertSingle = () => {
    for (const howl of instances) {
      const active = [...howl.sounds.values()].filter((state) => state === 'playing' || state === 'pending').length;
      assert.ok(active <= 1, `Howl has ${active} concurrent sound instances`);
      if (!howl.disposed) assert.ok(Object.values(store.tracks).some((track) => vue.toRaw(track.howl) === howl), 'orphan Howl');
    }
    assert.ok(instances.filter((howl) => !howl.disposed).length <= 3);
  };
  t.after(() => { assertSingle(); store.resetLocalPlaybackRuntime(); store.$dispose(); });
  async function assign(type = 'music', id = 'a', playing = true) {
    store.isPlaying = playing;
    await store.assignAssetToTrack(type, store.assets.find((asset) => asset.id === id), { force: true });
    const howl = store.tracks[type].howl;
    howl.finishLoad();
    if (playing) howl.finishPlay();
    assertSingle();
    return howl;
  }
  function snapshot(overrides = {}) {
    return {
      channelId: 'channel', scopeType: 'world', scopeId: 'world', worldPlaybackEnabled: true,
      tracks: store.buildTrackStatePayload(), isPlaying: store.isPlaying, position: 0,
      capturedAtMs: Date.now(), revision: 1, loopEnabled: false, playbackRate: 1,
      ...overrides,
    };
  }
  return { store, instances, calls, events, timers, assign, assertSingle, statePosts, snapshot,
    setServer: (value) => { server = value; }, setManage: (value) => { user.checkPerm = () => value; } };
}

test('one start entry deduplicates loading, play promises, remote snapshots and unlock gestures', async (t) => {
  const h = harness(t);
  h.store.isPlaying = true;
  await h.store.assignAssetToTrack('music', h.store.assets[0], { force: true });
  const howl = h.store.tracks.music.howl;
  await h.store.playAll({ force: true });
  h.store.playTrack('music');
  assert.equal(howl.playCalls.length, 0, 'do not enqueue play before load');
  howl.finishLoad();
  const payload = h.snapshot({ updatedBy: 'peer' });
  await h.store.applyRemotePlayback(payload, { source: 'pull' });
  h.store.pendingRemotePlay = true;
  h.store.tryResumeRemotePlayback();
  h.store.tryResumeRemotePlayback();
  await h.store.playAll({ force: true });
  assert.equal(howl.playCalls.length, 1);
  h.assertSingle();
  howl.finishPlay();
  h.assertSingle();
});

test('next/previous and rapid cached changes each create only one current sound', async (t) => {
  const h = harness(t);
  const old = await h.assign();
  h.store.tracks.music.playlistAssetIds = ['a', 'b', 'c'];
  h.store.setTrackPlaylistMode('music', 'sequential');
  const next = h.store.playNextInPlaylist('music');
  const prev = h.store.playPrevInPlaylist('music');
  const latest = h.store.playNextInPlaylist('music');
  await Promise.all([next, prev, latest]);
  assert.equal(old.disposed, true);
  assert.equal(h.store.tracks.music.assetId, 'b');
  const howl = h.store.tracks.music.howl;
  howl.finishLoad();
  howl.finishPlay();
  assert.equal(howl.playCalls.length, 1);
  h.assertSingle();
});

test('asset and signed URL responses in reverse order cannot install obsolete players', async (t) => {
  const lookups = new Map(), tokens = new Map();
  const h = harness(t, {
    get: (url) => { const result = deferred(); lookups.set(url, result); return result.promise; },
    post: (url) => { const result = deferred(); tokens.set(url, result); return result.promise; },
  });
  h.store.assets = [];
  h.store.isPlaying = true;
  const first = h.store.loadTrackAsset('music', 'first');
  const second = h.store.loadTrackAsset('music', 'second');
  lookups.get('/api/v1/audio/assets/second').resolve({ data: { id: 'second' } });
  await tick();
  const third = h.store.loadTrackAsset('music', 'third');
  lookups.get('/api/v1/audio/assets/third').resolve({ data: { id: 'third' } });
  await tick();
  tokens.get('/api/v1/audio/assets/third/play-token').resolve({ data: { streamUrl: 'third' } });
  await third;
  const howl = h.store.tracks.music.howl;
  howl.finishLoad(); howl.finishPlay();
  tokens.get('/api/v1/audio/assets/second/play-token').resolve({ data: { streamUrl: 'second' } });
  lookups.get('/api/v1/audio/assets/first').reject(new Error('old lookup failed'));
  await Promise.all([first, second]);
  assert.equal(h.instances.length, 1, 'obsolete signed response must not even create a Howl');
  assert.equal(h.store.tracks.music.assetId, 'third');
  assert.equal(h.store.tracks.music.status, 'playing');
  h.assertSingle();
});

test('callbacks from unloaded players cannot change the replacement or advance its playlist', async (t) => {
  const h = harness(t);
  const old = await h.assign();
  h.store.tracks.music.playlistAssetIds = ['a', 'b', 'c'];
  h.store.setTrackPlaylistMode('music', 'sequential');
  await h.store.playNextInPlaylist('music');
  const current = h.store.tracks.music.howl;
  const id = old.playCalls[0];
  for (const name of ['onload', 'onplay', 'onpause', 'onstop', 'onend', 'onloaderror', 'onplayerror']) old.config[name](id, 'late');
  assert.equal(h.store.tracks.music.howl, current);
  assert.equal(h.store.tracks.music.status, 'loading');
  assert.equal(h.store.tracks.music.assetId, 'b');
  current.finishLoad(); current.finishPlay();
  h.assertSingle();
});

for (const mode of ['single', 'sequential', 'shuffle']) {
  test(`${mode} has a single loop/advance driver, including one-item lists`, async (t) => {
    const h = harness(t);
    const howl = await h.assign();
    h.store.tracks.music.playlistAssetIds = ['a'];
    h.store.setTrackPlaylistMode('music', mode);
    h.store.setTrackLoop('music', true);
    assert.equal(howl.looping, mode === 'single');
    howl.end();
    howl.config.onend(howl.playCalls[0]); // 已排队的重复 end 回调
    await tick();
    if (mode === 'single') {
      assert.equal(h.instances.length, 1);
      assert.equal(howl.playCalls.length, 1, 'native loop must not call play again');
      assert.equal(h.store.tracks.music.status, 'playing');
      await h.store.playNextInPlaylist('music');
      await h.store.playPrevInPlaylist('music');
      assert.equal(howl.playCalls.length, 1);
    } else {
      assert.equal(h.instances.length, 2);
      assert.equal(howl.disposed, true);
      const current = h.store.tracks.music.howl;
      current.finishLoad(); current.finishPlay();
      assert.equal(current.playCalls.length, 1);
    }
    h.assertSingle();
  });
}

test('ordinary native loop never persists an end or stops the watcher', async (t) => {
  const h = harness(t);
  const howl = await h.assign();
  howl.end();
  await tick();
  assert.equal(h.store.isPlaying, true);
  assert.equal(h.statePosts().length, 0);
  assert.equal(howl.playCalls.length, 1);
});

test('natural end persists per-track false without pausing other mixed tracks or sync echoes', async (t) => {
  const h = harness(t);
  const music = await h.assign('music');
  const ambience = await h.assign('ambience', 'b');
  h.store.setTrackLoop('music', false);
  music.end();
  await tick();
  const body = h.statePosts().at(-1).body;
  assert.equal(body.persist, true);
  assert.equal(body.isPlaying, true);
  assert.equal(body.tracks.find((track) => track.type === 'music').isPlaying, false);
  assert.equal(body.tracks.find((track) => track.type === 'ambience').isPlaying, true);
  assert.equal(ambience.playing(), true);
  assert.equal(h.store.tracks.music.status, 'ready');
  await h.store.applyRemotePlayback(h.snapshot({ ...body, revision: 2, updatedBy: 'self' }));
  await tick();
  assert.equal(h.statePosts().length, 1);
  assert.equal(music.playCalls.length, 1);
  h.store.setTrackLoop('ambience', false);
  ambience.end();
  await tick();
  assert.equal(h.statePosts().at(-1).body.isPlaying, false);
  assert.equal(h.store.isPlaying, false);
});

test('offline end is committed before reconnect/visibility pulls and cannot replay an old snapshot', async (t) => {
  let offline = true, server = null, getCount = 0;
  const h = harness(t, {
    async post(url, body) {
      if (url !== '/api/v1/audio/state') return { data: { streamUrl: 'audio' } };
      if (offline) throw new Error('offline');
      server = { ...JSON.parse(JSON.stringify(body)), revision: 2, updatedBy: 'self' };
      return { data: { state: server } };
    },
    async get() { getCount++; return { data: { state: server } }; },
  });
  const howl = await h.assign();
  h.store.setTrackLoop('music', false);
  const stale = h.snapshot();
  h.store.markAppliedState('world:world', stale.revision, stale.capturedAtMs);
  howl.end();
  await tick();
  assert.ok(h.store.pendingCommitPayload);
  await h.store.applyRemotePlayback(stale, { source: 'pull', reason: 'visible' });
  assert.equal(howl.playCalls.length, 1);
  await h.store.fetchPlaybackState('channel', { force: true, reason: 'connected' });
  assert.equal(getCount, 0, 'failed end commit must not be overwritten by an old GET');
  offline = false;
  await h.store.fetchPlaybackState('channel', { force: true, reason: 'connected' });
  assert.equal(server.isPlaying, false);
  assert.equal(getCount, 1);
  assert.equal(howl.playCalls.length, 1);
  assert.equal(h.store.pendingCommitPayload, null);
  h.store.resetLocalPlaybackRuntime();
  await h.store.fetchPlaybackState('channel', { force: true, reason: 'channel-reenter' });
  const restored = h.store.tracks.music.howl;
  restored.finishLoad();
  assert.equal(restored.playCalls.length, 0);
});

test('pause during load and during play promise cannot leak a delayed start or create a second sound', async (t) => {
  const h = harness(t);
  h.store.isPlaying = true;
  await h.store.assignAssetToTrack('music', h.store.assets[0], { force: true });
  const howl = h.store.tracks.music.howl;
  h.store.pauseAll({ force: true });
  howl.finishLoad();
  assert.equal(howl.playCalls.length, 0);
  h.store.playTrack('music');
  h.store.pauseTrack('music', { force: true });
  h.store.playTrack('music');
  assert.equal(howl.playCalls.length, 1);
  howl.finishPlay();
  if (h.store.tracks.music.playPending) howl.finishPlay();
  assert.equal(new Set(howl.playCalls).size, 1, 'resume must use the same sound ID');
  h.assertSingle();
});

test('channel reset rejects pending and queued applies; world mode keeps its current player across channels', async (t) => {
  const token = deferred();
  const h = harness(t, { post: () => token.promise });
  const payload = h.snapshot({ tracks: [{ ...h.store.buildTrackStatePayload()[0], assetId: 'a', isPlaying: true }], isPlaying: true });
  const applying = h.store.applyRemotePlayback(payload);
  await tick();
  const queued = h.store.applyRemotePlayback({ ...payload, revision: 2 });
  h.store.resetLocalPlaybackRuntime();
  token.resolve({ data: { streamUrl: 'old-world' } });
  await Promise.all([applying, queued]);
  assert.equal(h.instances.length, 0);
  assert.equal(h.store.tracks.music.assetId, null);
  assert.equal(h.store.isPlaying, false);

  const kept = await h.assign();
  h.store.fetchPlaybackState = async () => {};
  await h.store.setActiveChannel('other');
  assert.equal(h.store.tracks.music.howl, kept);
  assert.equal(kept.disposed, false);
  h.store.worldPlaybackEnabled = false;
  await h.store.setActiveChannel('third');
  assert.equal(kept.disposed, true);
});

test('apply queue serializes a shared runtime and recovers after rejection', async () => {
  const queue = new applyQueue.AudioPlaybackApplyQueue();
  const gate = deferred(), calls = [];
  const first = queue.enqueue('runtime', async () => { calls.push('first'); await gate.promise; throw new Error('failed'); });
  const failure = assert.rejects(first, /failed/);
  const next = queue.enqueue('runtime', () => { calls.push('next'); });
  await tick();
  assert.deepEqual(calls, ['first']);
  gate.resolve();
  await Promise.all([failure, next]);
  assert.deepEqual(calls, ['first', 'next']);
});

test('unlock retries only blocked tracks and retains one sound ID per track', async (t) => {
  const h = harness(t, { manage: false });
  const payload = h.snapshot({ isPlaying: true, tracks: h.store.buildTrackStatePayload().map((track, index) => ({
    ...track, assetId: ['a', 'b', 'c'][index], isPlaying: true, loopEnabled: false,
  })) });
  await h.store.applyRemotePlayback(payload, { source: 'pull' });
  const [music, ambience, sfx] = ['music', 'ambience', 'sfx'].map((type) => h.store.tracks[type].howl);
  for (const howl of [music, ambience, sfx]) howl.finishLoad();
  music.failPlay(); ambience.failPlay(); sfx.finishPlay();
  assert.equal(h.store.pendingRemotePlay, true, 'one successful track must not clear another blocked track');
  h.store.tryResumeRemotePlayback();
  h.store.tryResumeRemotePlayback();
  assert.equal(music.playCalls.length, 2);
  assert.equal(new Set(music.playCalls).size, 1);
  assert.equal(sfx.playCalls.length, 1);
  music.finishPlay(); ambience.finishPlay();
  music.end();
  h.store.pendingRemotePlay = true;
  h.store.tryResumeRemotePlayback();
  assert.equal(music.playCalls.length, 2, 'gestures must never revive a naturally ended track');
  assert.equal(h.statePosts().length, 0, 'listeners must not synchronize an end back to the server');
  h.assertSingle();
});

for (const conflict of [false, true]) {
  test(`late ${conflict ? 'conflict' : 'ack'} cannot clear a newer natural-end commit`, async (t) => {
    const firstReply = deferred();
    let count = 0;
    const h = harness(t, { async post(url, body) {
      if (url !== '/api/v1/audio/state') return { data: { streamUrl: 'audio' } };
      count++;
      if (count === 1) return firstReply.promise;
      return { data: { state: { ...body, revision: 3 } } };
    } });
    const howl = await h.assign();
    h.store.markAppliedState('world:world', 1);
    h.store.setTrackLoop('music', false);
    const old = h.store.commitPlaybackSync({ syncReason: 'play-all' });
    await tick();
    howl.end();
    const endCommit = h.store.pendingCommitPayload;
    assert.equal(endCommit.isPlaying, false);
    const state = h.snapshot({ isPlaying: true, revision: 2 });
    if (conflict) firstReply.reject({ response: { status: 409, data: { state } } });
    else firstReply.resolve({ data: { state } });
    await old;
    await tick();
    assert.equal(h.statePosts().length, 2);
    assert.equal(h.statePosts()[1].body.isPlaying, false);
    assert.equal(h.statePosts()[1].body.persist, true);
    assert.equal(h.statePosts()[1].body.baseRevision, 2);
    assert.equal(h.store.pendingCommitPayload, null);
    assert.equal(howl.playCalls.length, 1);
  });
}

test('natural end during another remote track load is persisted once after apply', async (t) => {
  const token = deferred();
  const h = harness(t, { post: async (url, body) => {
    if (url.endsWith('/b/play-token')) return token.promise;
    if (url === '/api/v1/audio/state') return { data: { state: { ...body, revision: 3 } } };
    return { data: { streamUrl: 'a' } };
  } });
  const music = await h.assign();
  h.store.setTrackLoop('music', false);
  const payload = h.snapshot({ revision: 2 });
  payload.tracks[1].assetId = 'b'; payload.tracks[1].isPlaying = true;
  const applying = h.store.applyRemotePlayback(payload, { source: 'pull' });
  await tick();
  assert.equal(h.store.isApplyingRemoteState, true);
  music.end();
  assert.equal(h.statePosts().length, 0);
  token.resolve({ data: { streamUrl: 'b' } });
  await applying;
  await tick();
  assert.equal(h.statePosts().length, 1);
  assert.equal(h.statePosts()[0].body.tracks[0].isPlaying, false);
  assert.equal(h.statePosts()[0].body.tracks[1].isPlaying, true);
  await h.store.applyRemotePlayback(payload, { source: 'pull', reason: 'visible' });
  assert.equal(music.playCalls.length, 1);
});

test('a newer deliberate replay is allowed but lower-revision idle pulls are rejected', async (t) => {
  const h = harness(t, { manage: false });
  const howl = await h.assign();
  h.store.setTrackLoop('music', false);
  const playing = h.snapshot({ revision: 4 });
  h.store.markAppliedState('world:world', 4);
  howl.end();
  await h.store.applyRemotePlayback({ ...playing, revision: 3 }, { source: 'pull', reason: 'connected' });
  await h.store.applyRemotePlayback(playing, { source: 'pull', reason: 'visible' });
  assert.equal(howl.playCalls.length, 1);
  await h.store.applyRemotePlayback({ ...playing, revision: 5, capturedAtMs: Date.now() + 1 }, { source: 'pull' });
  assert.equal(howl.playCalls.length, 2);
  h.assertSingle();
});

test('S3 direct streams use the same guarded loader and channel/authority cleanup', async (t) => {
  const h = harness(t);
  h.store.audioLibrary.mode = 's3';
  const asset = { id: 'aud:s3:v1:asset', name: 'S3' };
  h.store.isPlaying = true;
  await h.store.assignAssetToTrack('music', asset, { force: true, initialSeek: 12 });
  const howl = h.store.tracks.music.howl;
  const request = h.calls.post.find((call) => call.url === '/api/v1/audio/library/play-token');
  assert.equal(request.body.worldId, 'world');
  assert.equal(request.body.ref, asset.id);
  howl.finishLoad(); howl.finishPlay();
  assert.equal(howl.seek(), 12);
  h.store.setPlaybackAuthority(false);
  for (const name of ['onload', 'onplay', 'onend']) howl.config[name](howl.playCalls[0]);
  assert.equal(howl.disposed, true);
  assert.equal(h.store.isPlaying, false);
  assert.equal(h.store.tracks.music.howl, null);
});

test('playlist navigation preserves paused tracks while another mixed track is playing', async (t) => {
  const h = harness(t);
  const music = await h.assign('music', 'a', false);
  const ambience = await h.assign('ambience', 'c');
  h.store.tracks.music.playlistAssetIds = ['a', 'b'];
  h.store.setTrackPlaylistMode('music', 'sequential');
  await h.store.playNextInPlaylist('music');
  const current = h.store.tracks.music.howl;
  current.finishLoad();
  assert.equal(current.playCalls.length, 0);
  assert.equal(music.disposed, true);
  assert.equal(ambience.playing(), true);
});

for (const mode of ['sequential', 'shuffle']) {
  test(`${mode} automatically advances a multi-item list once`, async (t) => {
    const h = harness(t);
    const old = await h.assign();
    h.store.tracks.music.playlistAssetIds = ['a', 'b', 'c'];
    h.store.setTrackPlaylistMode('music', mode);
    assert.equal(old.looping, false);
    old.end();
    await tick();
    const track = h.store.tracks.music;
    assert.equal(track.assetId, track.playlistAssetIds[track.playlistIndex]);
    if (mode === 'sequential') assert.equal(track.assetId, 'b');
    assert.equal(h.instances.length, 2);
    assert.equal(old.disposed, true);
    track.howl.finishLoad(); track.howl.finishPlay();
    h.assertSingle();
  });
}

for (const failureStage of ['asset', 'play-token']) {
  test(`automatic playlist advance ${failureStage} failure persists a stopped track without interrupting mixing`, async (t) => {
    let revision = 1;
    const h = harness(t, {
      async get(url) {
        if (failureStage === 'asset' && url === '/api/v1/audio/assets/b') throw new Error('asset unavailable');
        return { data: { id: url.split('/').at(-1) } };
      },
      async post(url, body) {
        if (failureStage === 'play-token' && url.endsWith('/b/play-token')) throw new Error('token unavailable');
        if (url === '/api/v1/audio/state') {
          return { data: { state: { ...JSON.parse(JSON.stringify(body)), revision: ++revision, updatedBy: 'self' } } };
        }
        return { data: { streamUrl: `https://audio.test/${url}`, expiresAt: Date.now() + 60000 } };
      },
    });
    const music = await h.assign('music', 'a');
    const ambience = await h.assign('ambience', 'c');
    h.store.tracks.music.playlistAssetIds = ['a', 'b'];
    h.store.setTrackPlaylistMode('music', 'sequential');
    if (failureStage === 'asset') h.store.assets = h.store.assets.filter((asset) => asset.id !== 'b');
    const stale = h.snapshot({ updatedBy: 'peer' });

    music.end();
    await tick();
    await tick();
    const track = h.store.tracks.music;
    assert.equal(track.assetId, 'b');
    assert.equal(track.status, 'error');
    assert.equal(track.playRequested, false);
    assert.equal(h.statePosts().length, 1, 'terminal auto-advance failure must persist the end');
    const persisted = h.statePosts()[0].body;
    assert.equal(persisted.persist, true);
    assert.equal(persisted.isPlaying, true, 'other mixed tracks must continue playing');
    assert.equal(persisted.tracks.find((item) => item.type === 'music').isPlaying, false);
    assert.equal(persisted.tracks.find((item) => item.type === 'ambience').isPlaying, true);
    assert.equal(ambience.playing(), true);

    await h.store.applyRemotePlayback(stale, { source: 'pull', reason: 'visible' });
    assert.equal(h.store.tracks.music.assetId, 'b', 'stale snapshot must not replay the previous track');
    assert.equal(h.instances.filter((instance) => !instance.disposed).length, 1);
    h.assertSingle();
  });
}

test('delayed Howler loaderror after automatic advance corrects the already-persisted playing state', async (t) => {
  const h = harness(t);
  const music = await h.assign();
  h.store.tracks.music.playlistAssetIds = ['a', 'b'];
  h.store.setTrackPlaylistMode('music', 'sequential');
  music.end();
  await tick();
  const next = h.store.tracks.music.howl;
  assert.ok(next);
  assert.equal(h.statePosts().length, 1);
  const stale = h.snapshot({ ...h.statePosts()[0].body, revision: 2, updatedBy: 'peer' });
  assert.equal(stale.tracks.find((item) => item.type === 'music').isPlaying, true);

  next.config.onloaderror(null, 'stream failed');
  await tick();
  assert.equal(next.disposed, true);
  assert.equal(h.store.tracks.music.status, 'error');
  assert.equal(h.statePosts().length, 2, 'late loaderror must publish a stopped track');
  const stopped = h.statePosts()[1].body;
  assert.equal(stopped.persist, true);
  assert.equal(stopped.isPlaying, false);
  assert.equal(stopped.tracks.find((item) => item.type === 'music').isPlaying, false);
  await h.store.applyRemotePlayback(stale, { source: 'pull', reason: 'visible' });
  assert.equal(h.instances.length, 2);
  assert.equal(h.store.tracks.music.howl, null);
  h.assertSingle();
});

test('obsolete automatic-advance failure cannot stop a newer selected track', async (t) => {
  const oldToken = deferred();
  const h = harness(t, { async post(url, body) {
    if (url.endsWith('/b/play-token')) return oldToken.promise;
    if (url === '/api/v1/audio/state') return { data: { state: { ...body, revision: 2 } } };
    return { data: { streamUrl: url, expiresAt: Date.now() + 60000 } };
  } });
  const music = await h.assign();
  h.store.tracks.music.playlistAssetIds = ['a', 'b', 'c'];
  h.store.setTrackPlaylistMode('music', 'sequential');
  music.end();
  await tick();
  await h.store.playNextInPlaylist('music');
  const current = h.store.tracks.music.howl;
  current.finishLoad();
  current.finishPlay();
  oldToken.reject(new Error('obsolete token failed'));
  await tick();
  await tick();
  assert.equal(h.store.tracks.music.assetId, 'c');
  assert.equal(h.store.tracks.music.status, 'playing');
  assert.equal(h.statePosts().length, 0, 'stale loader failure must not emit an end commit');
  h.assertSingle();
});

test('a reset starts a fresh apply queue without waiting for an obsolete network request', async (t) => {
  const oldToken = deferred(), newToken = deferred();
  const h = harness(t, { post: (url) => url.endsWith('/a/play-token') ? oldToken.promise : newToken.promise });
  const tracks = h.store.buildTrackStatePayload();
  const old = h.store.applyRemotePlayback(h.snapshot({ tracks: [{ ...tracks[0], assetId: 'a', isPlaying: true }], isPlaying: true }));
  await tick();
  h.store.resetLocalPlaybackRuntime();
  const current = h.store.applyRemotePlayback(h.snapshot({ tracks: [{ ...tracks[0], assetId: 'b', isPlaying: true }], isPlaying: true, revision: 2 }));
  await tick();
  assert.equal(h.store.tracks.music.assetId, 'b', 'new context must not wait for old signed URL');
  oldToken.resolve({ data: { streamUrl: 'old' } });
  await old;
  assert.equal(h.store.isApplyingRemoteState, true, 'old finally must not release the new apply guard');
  newToken.resolve({ data: { streamUrl: 'new' } });
  await current;
  assert.equal(h.instances.length, 1);
  assert.equal(h.store.tracks.music.assetId, 'b');
  h.assertSingle();
});

test('world playback that ends between channels commits its end when a channel becomes available', async (t) => {
  const h = harness(t);
  const howl = await h.assign();
  h.store.setTrackLoop('music', false);
  h.store.currentChannelId = null;
  howl.end();
  assert.equal(h.store.tracks.music.endSyncPending, true);
  await h.store.setActiveChannel('next-channel');
  await tick();
  assert.equal(h.statePosts().length, 1);
  assert.equal(h.statePosts()[0].body.channelId, 'next-channel');
  assert.equal(h.statePosts()[0].body.scopeType, 'world');
  assert.equal(h.statePosts()[0].body.isPlaying, false);
  assert.equal(howl.playCalls.length, 1);
});

test('an obsolete theater music snapshot cannot start tracks after context reset', async (t) => {
  const asset = deferred();
  const h = harness(t, { get: () => asset.promise });
  const snapshot = { version: 1, tracks: [{
    type: 'music', asset: { assetId: 'uncached' }, playlist: [], volume: 1,
    fadeIn: 0, fadeOut: 0, playbackRate: 1, loopEnabled: false,
  }] };
  const applying = h.store.applyStageMusicSnapshot(snapshot);
  h.store.resetLocalPlaybackRuntime();
  asset.resolve({ data: { id: 'uncached' } });
  await applying;
  assert.equal(h.instances.length, 0);
  assert.equal(h.store.tracks.music.assetId, null);
  assert.equal(h.store.isPlaying, false);
});

test('a delayed native-loop end callback cannot drive a newly selected playlist mode', async (t) => {
  const h = harness(t);
  const howl = await h.assign();
  h.store.tracks.music.playlistAssetIds = ['a', 'b'];
  h.store.setTrackPlaylistMode('music', 'single');
  // Howler 原生循环已重新开始，上一轮 end 回调仍在事件队列中。
  h.store.setTrackPlaylistMode('music', 'sequential');
  howl.config.onend(howl.playCalls[0]);
  await tick();
  assert.equal(h.instances.length, 1);
  assert.equal(h.store.tracks.music.assetId, 'a');
  howl.end();
  await tick();
  assert.equal(h.store.tracks.music.assetId, 'b');
  assert.equal(h.instances.length, 2);
});

test('a restored non-looping track already at its duration persists its end', async (t) => {
  const h = harness(t);
  h.store.isPlaying = true;
  h.store.tracks.music.loopEnabled = false;
  await h.store.assignAssetToTrack('music', h.store.assets[0], { force: true, initialSeek: 120 });
  const howl = h.store.tracks.music.howl;
  howl.finishLoad();
  await tick();
  assert.equal(howl.playCalls.length, 1);
  assert.equal(h.store.tracks.music.playRequested, false);
  assert.equal(h.store.tracks.music.status, 'ready');
  assert.equal(h.statePosts().length, 1);
  assert.equal(h.statePosts()[0].body.isPlaying, false);
});
