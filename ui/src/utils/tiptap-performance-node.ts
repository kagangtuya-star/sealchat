type TiptapCoreModule = typeof import('@tiptap/core');

export type PerformanceCommandType = 'delay' | 'pause';

export interface PerformanceCommandAttrs {
  command: PerformanceCommandType;
  value?: number | string | null;
}

export const formatPerformanceCommandLabel = (attrs: PerformanceCommandAttrs) => {
  const command = attrs.command === 'pause' ? 'pause' : 'delay';
  const value = attrs.value == null || attrs.value === '' ? '' : `-${attrs.value}`;
  return command === 'pause' ? '[暂停并高亮]' : `[停顿${value}]`;
};

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    performanceCommand: {
      insertPerformanceCommand: (attrs: PerformanceCommandAttrs) => ReturnType;
    };
  }
}

export const createPerformanceCommandExtension = ({
  Node,
  mergeAttributes,
}: Pick<TiptapCoreModule, 'Node' | 'mergeAttributes'>) => Node.create({
  name: 'performanceCommand',

  inline: true,
  group: 'inline',
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      command: {
        default: 'delay',
        parseHTML: (element: HTMLElement) => element.getAttribute('data-performance-command') || 'delay',
        renderHTML: (attributes: PerformanceCommandAttrs) => ({
          'data-performance-command': String(attributes.command || 'delay'),
        }),
      },
      value: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute('data-performance-value'),
        renderHTML: (attributes: PerformanceCommandAttrs) => {
          if (attributes.value == null || attributes.value === '') {
            return {};
          }
          return {
            'data-performance-value': String(attributes.value),
          };
        },
      },
    };
  },

  parseHTML() {
    return [{ tag: 'span[data-performance-command]' }];
  },

  renderHTML({ node, HTMLAttributes }) {
    const command = node.attrs.command === 'pause' ? 'pause' : 'delay';
    return [
      'span',
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, {
        class: `tiptap-performance-command tiptap-performance-command--${command}`,
        contenteditable: 'false',
      }),
      formatPerformanceCommandLabel({
        command,
        value: node.attrs.value,
      }),
    ];
  },

  addCommands() {
    return {
      insertPerformanceCommand:
        (attrs: PerformanceCommandAttrs) =>
        ({ commands }) =>
          commands.insertContent({
            type: this.name,
            attrs: {
              command: attrs.command,
              value: attrs.value ?? null,
            },
          }),
    };
  },
});
