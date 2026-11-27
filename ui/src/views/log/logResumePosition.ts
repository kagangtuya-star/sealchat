// 上次编辑位置：每位用户、每个工程记一条视口锚点，只存在本浏览器。
// 只是便利功能：存储不可用或内容损坏时当作没有记录。
import type { FrameAnchor } from '@/services/log/pagination/logFrameHost';

const KEY = 'sealchat.log.resumePosition';
const MAX_ENTRIES = 50;

export interface LogResumePosition extends Partial<Pick<FrameAnchor, 'fragmentIndex' | 'offsetWithinViewport'>> {
  sectionId: string;
  nodeId: string;
  at: number;
}

type Store = Record<string, LogResumePosition>;

const entryKey = (userId: string, projectId: string) => `${userId}:${projectId}`;

const valid = (v: unknown): v is LogResumePosition => {
  const p = v as LogResumePosition;
  return !!p && typeof p.sectionId === 'string' && !!p.sectionId && p.sectionId.length <= 100
    && typeof p.nodeId === 'string' && !!p.nodeId && p.nodeId.length <= 120 && Number.isFinite(p.at)
    && (p.fragmentIndex === undefined || (Number.isSafeInteger(p.fragmentIndex) && p.fragmentIndex >= 0))
    && (p.offsetWithinViewport === undefined || Number.isFinite(p.offsetWithinViewport));
};

const readStore = (): Store => {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) || '{}');
    return raw && typeof raw === 'object' && !Array.isArray(raw) ? raw as Store : {};
  } catch {
    return {};
  }
};

export const readResumePosition = (userId: string, projectId: string): LogResumePosition | null => {
  if (!userId || !projectId) return null;
  const entry = readStore()[entryKey(userId, projectId)];
  return valid(entry) ? entry : null;
};

export const writeResumePosition = (userId: string, projectId: string, position: Omit<LogResumePosition, 'at'>) => {
  if (!userId || !projectId) return;
  const next: LogResumePosition = { ...position, at: Date.now() };
  if (!valid(next)) return;
  const store = readStore();
  store[entryKey(userId, projectId)] = next;
  // 只保留最近使用的若干工程。
  const kept = Object.entries(store).filter(([, v]) => valid(v)).sort((a, b) => b[1].at - a[1].at).slice(0, MAX_ENTRIES);
  try {
    localStorage.setItem(KEY, JSON.stringify(Object.fromEntries(kept)));
  } catch {
    // 存储不可用时忽略
  }
};
