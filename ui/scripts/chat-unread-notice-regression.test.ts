import assert from 'node:assert/strict';
import { nextUnreadCountMapForMessageNotice } from '../src/stores/chatUnreadNotice';

const publicTree = [
  {
    id: 'root-channel',
    children: [
      {
        id: 'nested-channel',
        children: [],
      },
    ],
  },
];

const privateChannels = [
  {
    id: 'friend:1',
    children: [],
  },
];

assert.deepEqual(
  nextUnreadCountMapForMessageNotice({
    channelId: 'nested-channel',
    currentChannelId: 'root-channel',
    unreadCountMap: {},
    channelTree: publicTree,
    channelTreePrivate: privateChannels,
  }),
  { 'nested-channel': 1 },
  '子频道收到 notice 时应立即累加未读数',
);

assert.deepEqual(
  nextUnreadCountMapForMessageNotice({
    channelId: 'root-channel',
    currentChannelId: 'root-channel',
    unreadCountMap: { 'root-channel': 3 },
    channelTree: publicTree,
    channelTreePrivate: privateChannels,
  }),
  { 'root-channel': 3 },
  '当前所在频道收到 notice 时不应增加未读数',
);

assert.deepEqual(
  nextUnreadCountMapForMessageNotice({
    channelId: 'unknown-channel',
    currentChannelId: 'root-channel',
    unreadCountMap: { 'nested-channel': 1 },
    channelTree: publicTree,
    channelTreePrivate: privateChannels,
  }),
  { 'nested-channel': 1 },
  '不在当前可见频道树内的频道不应修改未读数',
);

console.log('chat unread notice regressions passed');
