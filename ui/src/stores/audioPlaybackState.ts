type PlaybackStatus = 'idle' | 'loading' | 'ready' | 'playing' | 'paused' | 'error';

type PlayableHowl = {
  playing?: () => boolean;
} | null | undefined;

type PlayableTrack = {
  status?: PlaybackStatus;
  howl?: PlayableHowl;
} | null | undefined;

function safeHowlPlaying(howl?: PlayableHowl) {
  if (!howl || typeof howl.playing !== 'function') {
    return false;
  }
  try {
    return Boolean(howl.playing());
  } catch {
    return false;
  }
}

export function isTrackPlaybackActive(track?: PlayableTrack) {
  if (!track) {
    return false;
  }
  return safeHowlPlaying(track.howl) || track.status === 'playing';
}

export function hasAnyActivePlayback(tracks: Array<PlayableTrack>) {
  return tracks.some((track) => isTrackPlaybackActive(track));
}

export function normalizeTrackStatus(track?: PlayableTrack): PlaybackStatus {
  if (!track) {
    return 'idle';
  }
  if (safeHowlPlaying(track.howl)) {
    return 'playing';
  }
  if (track.status === 'playing') {
    return 'playing';
  }
  return track.status || 'idle';
}
