export type InterjectSwitchRule = 'invert' | 'preserve' | 'forceOoc' | 'forceIc';

export interface InterjectAvailabilityArgs {
  isEditing: boolean;
  isConnected: boolean;
  spectatorInputDisabled: boolean;
  draftText: string;
  hasMeaningfulDraft?: boolean;
  hasUploadingInlineImages: boolean;
  hasFailedInlineImages: boolean;
}

export const resolveInterjectTargetMode = (
  currentMode: 'ic' | 'ooc',
  rule: InterjectSwitchRule,
): 'ic' | 'ooc' => {
  switch (rule) {
    case 'preserve':
      return currentMode;
    case 'forceOoc':
      return 'ooc';
    case 'forceIc':
      return 'ic';
    case 'invert':
    default:
      return currentMode === 'ic' ? 'ooc' : 'ic';
  }
};

export const shouldAllowInterject = (args: InterjectAvailabilityArgs): boolean => {
  if (args.isEditing) return false;
  if (!args.isConnected) return false;
  if (args.spectatorInputDisabled) return false;
  if (args.hasUploadingInlineImages) return false;
  if (args.hasFailedInlineImages) return false;
  if (args.hasMeaningfulDraft) return true;
  return args.draftText.trim().length > 0;
};
