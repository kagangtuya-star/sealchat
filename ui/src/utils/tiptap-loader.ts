import { createSpoilerExtension } from './tiptap-spoiler';
import { createRubyExtension } from './tiptap-ruby';

type TiptapCoreModule = typeof import('@tiptap/core');
type TiptapVueModule = typeof import('@tiptap/vue-3');

export interface TipTapBundle {
  Editor: TiptapCoreModule['Editor'];
  Node: TiptapCoreModule['Node'];
  Extension: TiptapCoreModule['Extension'];
  mergeAttributes: TiptapCoreModule['mergeAttributes'];
  EditorContent: TiptapVueModule['EditorContent'];
  BubbleMenu: TiptapVueModule['BubbleMenu'];
  StarterKit: any;
  TextStyle: any;
  Color: any;
  Image: any;
  Highlight: any;
  TextAlign: any;
  Spoiler: ReturnType<typeof createSpoilerExtension>;
  Ruby: ReturnType<typeof createRubyExtension>;
}

let tiptapBundlePromise: Promise<TipTapBundle> | null = null;

export const loadTipTapBundle = (): Promise<TipTapBundle> => {
  if (!tiptapBundlePromise) {
    tiptapBundlePromise = Promise.all([
      import('@tiptap/core'),
      import('@tiptap/vue-3'),
      import('@tiptap/starter-kit'),
      import('@tiptap/extension-text-style').then((module) => ({ default: module.TextStyle })),
      import('@tiptap/extension-color').then((module) => ({ default: module.Color })),
      import('@tiptap/extension-image'),
      import('@tiptap/extension-highlight'),
      import('@tiptap/extension-text-align'),
    ]).then(([tiptapCore, tiptapVue, starterKit, textStyle, color, image, highlight, textAlign]) => ({
      Editor: tiptapCore.Editor,
      Node: tiptapCore.Node,
      Extension: tiptapCore.Extension,
      mergeAttributes: tiptapCore.mergeAttributes,
      EditorContent: tiptapVue.EditorContent,
      BubbleMenu: tiptapVue.BubbleMenu,
      StarterKit: starterKit.default,
      TextStyle: textStyle.default,
      Color: color.default,
      Image: image.default,
      Highlight: highlight.default,
      TextAlign: textAlign.default,
      Spoiler: createSpoilerExtension(tiptapCore),
      Ruby: createRubyExtension(tiptapCore),
    }));
  }

  return tiptapBundlePromise;
};
