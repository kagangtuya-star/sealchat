type TiptapCoreModule = typeof import('@tiptap/core');

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    ruby: {
      setRuby: (rubyText: string) => ReturnType;
      unsetRuby: () => ReturnType;
    };
  }
}

export interface RubyOptions {
  HTMLAttributes: Record<string, unknown>;
}

const buildRubyStyleVariableString = (attributes: Record<string, any>) => {
  const variables: string[] = [];
  const pushVar = (name: string, value: unknown) => {
    const normalized = String(value || '').trim();
    if (!normalized) {
      return;
    }
    variables.push(`${name}: ${normalized}`);
  };
  pushVar('--ruby-font-family', attributes.rubyFontFamily);
  pushVar('--ruby-font-size', attributes.rubyFontSize);
  pushVar('--ruby-color', attributes.rubyColor);
  pushVar('--ruby-font-weight', attributes.rubyFontWeight);
  pushVar('--ruby-font-style', attributes.rubyFontStyle);
  const existingStyle = String(attributes.style || '').trim();
  return [existingStyle, variables.join('; ')].filter(Boolean).join('; ');
};

export const createRubyExtension = ({
  Mark,
  mergeAttributes,
}: Pick<TiptapCoreModule, 'Mark' | 'mergeAttributes'>) => Mark.create<RubyOptions>({
  name: 'ruby',

  addOptions() {
    return {
      HTMLAttributes: {},
    };
  },

  addAttributes() {
    return {
      rubyText: {
        default: null,
        parseHTML: (element: HTMLElement) => {
          const rubyText = element.querySelector('rt')?.textContent || '';
          return rubyText.trim() || null;
        },
        renderHTML: (attributes: Record<string, any>) => {
          const rubyText = String(attributes.rubyText || '').trim();
          if (!rubyText) {
            return {};
          }
          return {
            'data-ruby-text': rubyText,
          };
        },
      },
      rubyFontFamily: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute('data-ruby-font-family') || null,
        renderHTML: (attributes: Record<string, any>) => {
          const value = String(attributes.rubyFontFamily || '').trim();
          return value ? { 'data-ruby-font-family': value } : {};
        },
      },
      rubyFontSize: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute('data-ruby-font-size') || null,
        renderHTML: (attributes: Record<string, any>) => {
          const value = String(attributes.rubyFontSize || '').trim();
          return value ? { 'data-ruby-font-size': value } : {};
        },
      },
      rubyColor: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute('data-ruby-color') || null,
        renderHTML: (attributes: Record<string, any>) => {
          const value = String(attributes.rubyColor || '').trim();
          return value ? { 'data-ruby-color': value } : {};
        },
      },
      rubyFontWeight: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute('data-ruby-font-weight') || null,
        renderHTML: (attributes: Record<string, any>) => {
          const value = String(attributes.rubyFontWeight || '').trim();
          return value ? { 'data-ruby-font-weight': value } : {};
        },
      },
      rubyFontStyle: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute('data-ruby-font-style') || null,
        renderHTML: (attributes: Record<string, any>) => {
          const value = String(attributes.rubyFontStyle || '').trim();
          return value ? { 'data-ruby-font-style': value } : {};
        },
      },
    };
  },

  parseHTML() {
    return [
      {
        tag: 'ruby',
      },
      {
        tag: 'span[data-ruby-text]',
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'span',
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, {
        class: 'tiptap-ruby',
        style: buildRubyStyleVariableString(HTMLAttributes),
      }),
      0,
    ];
  },

  addCommands() {
    return {
      setRuby:
        (rubyText: string) =>
        ({ commands }) => {
          const normalized = String(rubyText || '').trim();
          if (!normalized) {
            return commands.unsetMark(this.name);
          }
          return commands.setMark(this.name, { rubyText: normalized });
        },
      unsetRuby:
        () =>
        ({ commands }) =>
          commands.unsetMark(this.name),
    };
  },
});
