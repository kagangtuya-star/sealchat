import { useUtilsStore } from '@/stores/utils';

const DEFAULT_UPLOAD_TIMEOUT_MS = 20_000;

export const getUploadTimeoutMs = (): number => {
  const seconds = useUtilsStore().config?.storage?.uploadTimeoutSeconds;
  return Number.isFinite(seconds) && (seconds ?? 0) > 0
    ? Math.round((seconds as number) * 1000)
    : DEFAULT_UPLOAD_TIMEOUT_MS;
};
