import assert from 'node:assert/strict';
import {
  buildHybridCaretAnchorHtml,
  findImageMarkerAtPosition,
  normalizeCursorAfterTextInsertion,
} from '../src/views/chat/components/inputs/chatInputHybridMarkers';

const markerToken = '[[图片:marker_1]]';
const sample = `A${markerToken}B`;
const markerStart = 1;
const markerEnd = markerStart + markerToken.length;

const backwardDeleteProbe = markerEnd;
assert.equal(
  findImageMarkerAtPosition(sample, backwardDeleteProbe),
  null,
  '图片尾边界不应被当作图片内部，避免 Backspace 越界删图',
);

assert.deepEqual(
  findImageMarkerAtPosition(sample, markerStart),
  {
    markerId: 'marker_1',
    start: markerStart,
    end: markerEnd,
  },
  '图片起始边界应仍可命中，用于 Delete 删除图片',
);

assert.equal(
  buildHybridCaretAnchorHtml().includes('hybrid-input__caret-anchor'),
  true,
  '图片后应生成不可见的光标锚点，供组件在原子节点后稳定落点',
);

assert.equal(
  normalizeCursorAfterTextInsertion(
    `${markerToken}a`,
    markerToken.length,
    {
      inputType: 'insertText',
      data: 'a',
      selectionStart: markerToken.length,
      selectionEnd: markerToken.length,
      previousValue: markerToken,
    },
  ),
  markerToken.length + 1,
  '图片后第一次插入文本时，应将测得的旧边界光标修正到新文本之后',
);

console.log('chat-input-hybrid marker regressions passed');
