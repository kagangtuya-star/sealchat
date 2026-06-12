export const FLOATING_BADGE_DRAG_THRESHOLD = 6;

export type FloatingBadgeGestureState = {
  pointerId: number;
  startX: number;
  startY: number;
  windowX: number;
  windowY: number;
  dragActivated: boolean;
};

export const createFloatingBadgeGesture = (input: {
  pointerId: number;
  startX: number;
  startY: number;
  windowX: number;
  windowY: number;
}): FloatingBadgeGestureState => ({
  pointerId: input.pointerId,
  startX: input.startX,
  startY: input.startY,
  windowX: input.windowX,
  windowY: input.windowY,
  dragActivated: false,
});

export const updateFloatingBadgeGesture = (
  state: FloatingBadgeGestureState,
  clientX: number,
  clientY: number,
): { dragActivated: boolean; position: { x: number; y: number } } => {
  const deltaX = clientX - state.startX;
  const deltaY = clientY - state.startY;
  if (!state.dragActivated) {
    const distance = Math.hypot(deltaX, deltaY);
    if (distance >= FLOATING_BADGE_DRAG_THRESHOLD) {
      state.dragActivated = true;
    }
  }
  return {
    dragActivated: state.dragActivated,
    position: {
      x: state.windowX + deltaX,
      y: state.windowY + deltaY,
    },
  };
};

export const finishFloatingBadgeGesture = (state: FloatingBadgeGestureState): { action: 'toggle' | 'none' } => ({
  action: state.dragActivated ? 'none' : 'toggle',
});
