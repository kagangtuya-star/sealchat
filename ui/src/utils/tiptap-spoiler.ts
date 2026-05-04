type TiptapCoreModule = typeof import('@tiptap/core');

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    spoiler: {
      toggleSpoiler: () => ReturnType;
    };
  }
}

export interface SpoilerOptions {
  HTMLAttributes: Record<string, unknown>;
}

export const createSpoilerExtension = ({
  Mark,
  mergeAttributes,
}: Pick<TiptapCoreModule, 'Mark' | 'mergeAttributes'>) => Mark.create<SpoilerOptions>({
  name: 'spoiler',

  addOptions() {
    return {
      HTMLAttributes: {},
    };
  },

  parseHTML() {
    return [
      {
        tag: 'span[data-spoiler]',
      },
      {
        tag: 'span.tiptap-spoiler',
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'span',
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, {
        class: 'tiptap-spoiler',
        'data-spoiler': 'true',
      }),
      0,
    ];
  },

  addCommands() {
    return {
      toggleSpoiler:
        () =>
        ({ commands }) =>
          commands.toggleMark(this.name),
    };
  },
});
