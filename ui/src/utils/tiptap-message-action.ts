type TiptapCoreModule = typeof import('@tiptap/core');

export const MESSAGE_ACTION_NODE_TYPE = 'messageAction';
export const MESSAGE_ACTION_DATA_ATTR = 'data-rich-message-action';

export interface MessageActionAttrs {
  label: string;
  message: string;
  button: boolean;
}

export const normalizeMessageActionAttrs = (
  input: Partial<MessageActionAttrs> | null | undefined,
): MessageActionAttrs | null => {
  if (!input || typeof input !== 'object') return null;
  const label = String(input.label || '').trim();
  const message = String(input.message || '').trim();
  if (!label || !message) return null;
  return { label, message, button: input.button !== false && String(input.button) !== 'false' };
};

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    messageAction: {
      insertMessageAction: (attrs: MessageActionAttrs) => ReturnType;
    };
  }
}

export const createMessageActionExtension = ({
  Node,
  mergeAttributes,
}: Pick<TiptapCoreModule, 'Node' | 'mergeAttributes'>) => Node.create({
  name: MESSAGE_ACTION_NODE_TYPE,
  inline: true,
  group: 'inline',
  marks: 'bold italic underline strike code textStyle highlight spoiler ruby performance',
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      label: {
        default: '',
        parseHTML: (element: HTMLElement) => element.getAttribute('data-label') || element.textContent || '',
      },
      message: {
        default: '',
        parseHTML: (element: HTMLElement) => element.getAttribute('data-message') || '',
      },
      button: {
        default: true,
        parseHTML: (element: HTMLElement) => element.getAttribute('data-button') !== 'false',
      },
    };
  },

  parseHTML() {
    return [{ tag: `button[${MESSAGE_ACTION_DATA_ATTR}]` }];
  },

  renderHTML({ node, HTMLAttributes }) {
    const attrs = normalizeMessageActionAttrs(node.attrs);
    const label = attrs?.label || '';
    return [
      'button',
      mergeAttributes(HTMLAttributes, {
        type: 'button',
        class: `tiptap-message-action ${attrs?.button === false ? 'tiptap-message-action--text' : 'tiptap-message-action--button'}`,
        contenteditable: 'false',
        [MESSAGE_ACTION_DATA_ATTR]: 'true',
        'data-label': label,
        'data-message': attrs?.message || '',
        'data-button': attrs?.button === false ? 'false' : 'true',
      }),
      label,
    ];
  },

  renderText({ node }) {
    return normalizeMessageActionAttrs(node.attrs)?.label || '';
  },

  addCommands() {
    return {
      insertMessageAction:
        (attrs: MessageActionAttrs) =>
        ({ commands }) => commands.insertContent({ type: this.name, attrs }),
    };
  },
});
