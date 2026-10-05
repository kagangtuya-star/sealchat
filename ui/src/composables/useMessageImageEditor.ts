import { computed, onUnmounted, ref, watch, type Ref } from 'vue';
import {
  canvasToBlob,
  exportToCanvas,
  type SaveParameters,
  useBackground,
  useCrop,
  useFreehand,
  useMove,
  useRectangle,
} from 'vue-paint';
import { compressImage } from '@/composables/useImageCompressor';
import { createDefaultMediaFxSpec, mediaFxFilterHasContent, normalizeMediaFxSpec, type MediaFxSpec } from '@/features/media-fx/media-fx';
import { bakeMediaFxFilterToCanvas } from '@/features/media-fx/media-fx-canvas';

export type MessageImageEditorTool = 'move' | 'freehand' | 'rectangle' | 'crop';

const DEFAULT_WIDTH = 1280;
const DEFAULT_HEIGHT = 720;
const DEFAULT_COLOR = '#e11d48';
const DEFAULT_THICKNESS = 6;

const loadImageDimensions = async (file: File) => {
  const objectUrl = URL.createObjectURL(file);
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const nextImage = new Image();
      nextImage.onload = () => resolve(nextImage);
      nextImage.onerror = () => reject(new Error('图片解码失败'));
      nextImage.src = objectUrl;
    });
    return {
      width: image.naturalWidth || DEFAULT_WIDTH,
      height: image.naturalHeight || DEFAULT_HEIGHT,
    };
  } finally {
    URL.revokeObjectURL(objectUrl);
  }
};

const buildExportPngName = (fileName: string) => {
  const normalized = String(fileName || 'chat-image').trim() || 'chat-image';
  const baseName = normalized.replace(/\.[^.]+$/, '') || 'chat-image';
  return `${baseName}.png`;
};

const buildTools = (file: File) => [
  useBackground({ blob: file }),
  useMove(),
  useFreehand(),
  useRectangle(),
  useCrop(),
];

interface CropSnapshot {
  file: File;
  width: number;
  height: number;
  history: any[];
  settings: {
    tool: MessageImageEditorTool;
    thickness: number;
    color: string;
    angleSnap: boolean;
  };
  lastDrawTool: MessageImageEditorTool;
}

const cloneHistory = (value: any[]) => JSON.parse(JSON.stringify(value || []));

export const useMessageImageEditor = (fileRef: Ref<File | null>) => {
  const editorKey = ref(0);
  const imageWidth = ref(DEFAULT_WIDTH);
  const imageHeight = ref(DEFAULT_HEIGHT);
  const isPreparing = ref(false);
  const isSaving = ref(false);
  const errorMessage = ref('');
  const history = ref<any[]>([]);
  const tools = ref<any[]>([]);
  const settings = ref<any>({
    tool: 'freehand',
    thickness: DEFAULT_THICKNESS,
    color: DEFAULT_COLOR,
    angleSnap: false,
  });
  const lastDrawTool = ref<MessageImageEditorTool>('freehand');
  const workingFile = ref<File | null>(null);
  const cropSnapshots = ref<CropSnapshot[]>([]);
  // Bake-only Media FX: previewed with CSS while editing, baked once on export.
  const mediaFx = ref<MediaFxSpec>(createDefaultMediaFxSpec());
  let loadTaskId = 0;

  const resetEditorState = (tool: MessageImageEditorTool = 'freehand') => {
    history.value = [];
    tools.value.splice(0, tools.value.length);
    cropSnapshots.value = [];
    settings.value = {
      tool,
      thickness: DEFAULT_THICKNESS,
      color: DEFAULT_COLOR,
      angleSnap: false,
    };
    errorMessage.value = '';
    lastDrawTool.value = tool === 'crop' ? 'freehand' : tool;
    mediaFx.value = createDefaultMediaFxSpec();
    editorKey.value += 1;
  };

  const applyEditorFile = async (
    file: File,
    options?: {
      dimensions?: { width: number; height: number };
      tool?: MessageImageEditorTool;
      preserveStyle?: boolean;
    },
  ) => {
    const tool = options?.tool ?? 'freehand';
    const preserveStyle = options?.preserveStyle !== false;
    const previousColor = String(settings.value?.color || DEFAULT_COLOR);
    const previousThickness = Number(settings.value?.thickness || DEFAULT_THICKNESS);
    const dimensions = options?.dimensions ?? await loadImageDimensions(file);

    workingFile.value = file;
    imageWidth.value = dimensions.width || DEFAULT_WIDTH;
    imageHeight.value = dimensions.height || DEFAULT_HEIGHT;
    tools.value.splice(0, tools.value.length, ...buildTools(file));
    history.value = [];
    settings.value = {
      tool,
      thickness: preserveStyle ? Math.max(1, Math.min(24, Math.round(previousThickness))) : DEFAULT_THICKNESS,
      color: preserveStyle ? previousColor : DEFAULT_COLOR,
      angleSnap: false,
    };
    errorMessage.value = '';
    if (tool !== 'crop') {
      lastDrawTool.value = tool;
    }
    editorKey.value += 1;
  };

  watch(
    fileRef,
    async (file) => {
      loadTaskId += 1;
      const currentTaskId = loadTaskId;

      workingFile.value = file;
      resetEditorState();

      if (!file) {
        imageWidth.value = DEFAULT_WIDTH;
        imageHeight.value = DEFAULT_HEIGHT;
        isPreparing.value = false;
        return;
      }

      isPreparing.value = true;
      try {
        await applyEditorFile(file, {
          tool: 'freehand',
          preserveStyle: false,
        });
      } catch (error: any) {
        if (currentTaskId !== loadTaskId) {
          return;
        }
        errorMessage.value = error?.message || '图片加载失败，请重新选择文件';
      } finally {
        if (currentTaskId === loadTaskId) {
          isPreparing.value = false;
        }
      }
    },
    { immediate: true },
  );

  const activeTool = computed<MessageImageEditorTool>(() => {
    const tool = String(settings.value?.tool || 'freehand');
    if (tool === 'move' || tool === 'rectangle' || tool === 'crop') {
      return tool;
    }
    return 'freehand';
  });

  const canEdit = computed(() => !!workingFile.value && !isPreparing.value && tools.value.length > 0 && !errorMessage.value);
  const canRestoreBeforeCrop = computed(() => cropSnapshots.value.length > 0);
  const hasPendingCrop = computed(() => history.value.some((shape) => (
    shape?.type === 'crop'
    && Math.abs(Number(shape?.width) || 0) > 0
    && Math.abs(Number(shape?.height) || 0) > 0
  )));

  const setTool = (tool: MessageImageEditorTool) => {
    if (!settings.value) {
      return;
    }
    if (tool !== 'crop') {
      lastDrawTool.value = tool;
    }
    settings.value = {
      ...settings.value,
      tool,
    };
  };

  const commitCrop = async (svg: SVGElement | null, nextTool: Exclude<MessageImageEditorTool, 'crop'>) => {
    const currentFile = workingFile.value;
    if (!currentFile || !svg || !hasPendingCrop.value) {
      setTool(nextTool);
      return false;
    }

    isPreparing.value = true;
    try {
      cropSnapshots.value.push({
        file: currentFile,
        width: imageWidth.value,
        height: imageHeight.value,
        history: cloneHistory(history.value),
        settings: {
          tool: activeTool.value,
          thickness: Number(settings.value?.thickness || DEFAULT_THICKNESS),
          color: String(settings.value?.color || DEFAULT_COLOR),
          angleSnap: Boolean(settings.value?.angleSnap),
        },
        lastDrawTool: lastDrawTool.value,
      });
      const canvas = document.createElement('canvas');
      await exportToCanvas({
        svg,
        canvas,
        tools: tools.value,
        history: history.value,
      } as any);
      const blob = await canvasToBlob(canvas);
      const croppedFile = new File([blob], buildExportPngName(currentFile.name), {
        type: blob.type || 'image/png',
        lastModified: Date.now(),
      });
      await applyEditorFile(croppedFile, {
        dimensions: {
          width: canvas.width || DEFAULT_WIDTH,
          height: canvas.height || DEFAULT_HEIGHT,
        },
        tool: nextTool,
        preserveStyle: true,
      });
      return true;
    } finally {
      isPreparing.value = false;
    }
  };

  const restoreBeforeCrop = () => {
    const snapshot = cropSnapshots.value.pop();
    if (!snapshot) {
      return false;
    }
    workingFile.value = snapshot.file;
    imageWidth.value = snapshot.width || DEFAULT_WIDTH;
    imageHeight.value = snapshot.height || DEFAULT_HEIGHT;
    tools.value.splice(0, tools.value.length, ...buildTools(snapshot.file));
    history.value = cloneHistory(snapshot.history);
    settings.value = {
      tool: snapshot.settings.tool,
      thickness: Math.max(1, Math.min(24, Math.round(snapshot.settings.thickness || DEFAULT_THICKNESS))),
      color: snapshot.settings.color || DEFAULT_COLOR,
      angleSnap: Boolean(snapshot.settings.angleSnap),
    };
    lastDrawTool.value = snapshot.lastDrawTool || 'freehand';
    errorMessage.value = '';
    return true;
  };

  const selectTool = async (tool: MessageImageEditorTool, svg?: SVGElement | null) => {
    if (!settings.value || tool === activeTool.value) {
      return;
    }
    if (activeTool.value === 'crop' && tool !== 'crop') {
      await commitCrop(svg || null, tool);
      return;
    }
    setTool(tool);
  };

  const restoreLastDrawTool = async (svg?: SVGElement | null) => {
    await selectTool(lastDrawTool.value, svg);
  };

  const setColor = (color: string) => {
    if (!settings.value) {
      return;
    }
    settings.value = {
      ...settings.value,
      color,
    };
  };

  const setThickness = (value: number) => {
    if (!settings.value) {
      return;
    }
    settings.value = {
      ...settings.value,
      thickness: Math.max(1, Math.min(24, Math.round(value))),
    };
  };

  const setMediaFx = (value: MediaFxSpec) => {
    mediaFx.value = normalizeMediaFxSpec(value);
  };

  const exportEditedFile = async (params: SaveParameters) => {
    const originalFile = workingFile.value;
    if (!originalFile) {
      throw new Error('图片文件不存在');
    }

    isSaving.value = true;
    try {
      const canvas = document.createElement('canvas');
      await exportToCanvas({
        ...params,
        canvas,
      } as any);
      const filter = mediaFx.value.filter;
      const outputCanvas = mediaFxFilterHasContent(filter)
        ? bakeMediaFxFilterToCanvas(canvas, { width: canvas.width, height: canvas.height }, filter)
        : canvas;
      const blob = await canvasToBlob(outputCanvas);
      const pngFile = new File([blob], buildExportPngName(originalFile.name), {
        type: 'image/png',
        lastModified: Date.now(),
      });
      return await compressImage(pngFile, {
        maxWidth: imageWidth.value,
        maxHeight: imageHeight.value,
      });
    } finally {
      isSaving.value = false;
    }
  };

  onUnmounted(() => {
    loadTaskId += 1;
  });

  return {
    activeTool,
    canEdit,
    canRestoreBeforeCrop,
    editorKey,
    errorMessage,
    exportEditedFile,
    hasPendingCrop,
    history,
    imageHeight,
    imageWidth,
    isPreparing,
    isSaving,
    mediaFx,
    restoreLastDrawTool,
    restoreBeforeCrop,
    selectTool,
    setColor,
    setMediaFx,
    setThickness,
    setTool,
    settings,
    tools,
  };
};
