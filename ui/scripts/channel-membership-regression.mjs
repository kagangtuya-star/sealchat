import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';

const source = readFileSync(new URL('../src/views/components/ChannelSettings/TabMember.vue', import.meta.url), 'utf8')
  .match(/<script[^>]*>([\s\S]*?)<\/script>/)[1];
const ast = ts.createSourceFile('TabMember.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const names = ['loadManagedMembers', 'dataLoadMember', 'removeUserRole', 'selectedMembersSet',
  'buildRoleSuffixMap', 'buildRoleMemberMap', 'handleSyncMembers'];
const declarations = names.map(name => {
  const statement = ast.statements.find(item => ts.isVariableStatement(item)
    && item.declarationList.declarations.some(entry => entry.name.getText(ast) === name));
  assert.ok(statement, `missing declaration: ${name}`);
  return statement.getText(ast);
});
const compiled = ts.transpileModule(declarations.join('\n') + `\n({${names.join(',')}});`, {
  compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
}).outputText;
const member = (userId, grantSource, channelId = 'target') => ({ userId, grantSource, roleId: `ch-${channelId}-member` });

function harness(rows = [member('world', 'world'), member('manual', 'manual'), member('both', 'manual+world')]) {
  const notices = [];
  const calls = [];
  const state = {
    rows, notices, calls,
    props: { channel: { id: 'target' } },
    currentUserId: { value: 'actor' },
    canRemoveMember: () => true,
    dialog: {}, dialogAskConfirm: async () => true,
    refreshRoleCandidatesIfNeeded: async () => {},
    console: { error() {} },
    defaultRoleSuffixes: ['-member'],
    syncSourceChannelId: { value: 'source' }, syncMode: { value: 'replace' },
    syncSubmitting: { value: false }, syncDialogVisible: { value: true },
    message: Object.fromEntries(['success', 'error', 'warning', 'info'].map(type => [type, text => notices.push({ type, text })])),
  };
  state.chat = {
    channelMemberList: async (id, { page, pageSize }) => {
      const items = rows.filter(row => row.roleId.startsWith(`ch-${id}-`));
      return { data: { items: items.slice((page - 1) * pageSize, page * pageSize), total: items.length } };
    },
    channelRoleList: async id => ({ data: { items: [{ id: `ch-${id}-member` }] } }),
    userRoleLink: async (roleId, ids) => {
      calls.push({ operation: 'link', roleId, ids: Array.from(ids) });
      for (const id of ids) {
        const existing = rows.find(row => row.roleId === roleId && row.userId === id);
        if (existing?.grantSource === 'world') existing.grantSource = 'manual+world';
        else if (!existing) rows.push({ userId: id, roleId, grantSource: 'manual' });
      }
    },
    userRoleUnlink: async (roleId, ids) => {
      calls.push({ operation: 'unlink', roleId, ids: Array.from(ids) });
      let removedCount = 0;
      for (const id of ids) {
        const index = rows.findIndex(row => row.roleId === roleId && row.userId === id);
        if (index < 0 || rows[index].grantSource === 'world') continue;
        if (rows[index].grantSource === 'manual+world') rows[index].grantSource = 'world';
        else rows.splice(index, 1);
        removedCount++;
      }
      return { removedCount };
    },
  };
  state.doMemberReload = async () => state.api.dataLoadMember();
  state.api = vm.runInNewContext(compiled, state);
  return state;
}

test('management filters only world-only member grants after pagination', async () => {
  const state = harness([...Array.from({ length: 101 }, (_, i) => member(`world${i}`, 'world')),
    member('manual', 'manual'), member('both', 'manual+world'),
    { userId: 'admin', grantSource: 'world', roleId: 'ch-target-admin' }]);
  const list = await state.api.dataLoadMember();
  assert.equal(Array.from(list.items, item => item.userId).join(','), 'manual,both,admin');
  assert.equal(list.total, 3);
});

for (const userId of ['manual', 'both']) {
  test(`${userId} is removed from the explicit list and only world permissions remain for both`, async () => {
    const state = harness();
    await state.api.removeUserRole(userId, 'ch-target-member');
    assert.equal((await state.api.dataLoadMember()).items.some(item => item.userId === userId), false);
    assert.equal(state.rows.find(item => item.userId === userId)?.grantSource, userId === 'both' ? 'world' : undefined);
    assert.equal(state.notices.at(-1).text, '成员已成功移除');
  });
}

test('zero removals and unchanged post-removal state never show success', async () => {
  for (const count of [0, 1]) {
    const state = harness();
    state.chat.userRoleUnlink = async () => ({ removedCount: count });
    await state.api.removeUserRole('manual', 'ch-target-member');
    assert.equal(state.notices.some(item => item.type === 'success'), false);
    assert.equal(state.notices.at(-1).type, 'warning');
  }
});

test('adding and removing explicit members through the selector preserves world grants', async () => {
  const state = harness();
  assert.equal(await state.api.selectedMembersSet({ id: 'ch-target-member' }, ['world'], ['manual', 'both']), true);
  assert.equal(state.rows.find(item => item.userId === 'world').grantSource, 'manual+world');
  assert.equal(state.rows.find(item => item.userId === 'both').grantSource, 'world');
  assert.equal((await state.api.dataLoadMember()).items.map(item => item.userId).join(','), 'world');
  assert.equal(state.notices.at(-1).text, '成员已成功更新');
});

test('selector does not claim success for a no-op, failed addition, or zero removals', async () => {
  const unchanged = harness();
  assert.equal(await unchanged.api.selectedMembersSet({ id: 'ch-target-member' }, ['manual'], ['manual']), true);
  assert.equal(unchanged.calls.length, 0);
  assert.equal(unchanged.notices.at(-1).type, 'info');
  for (const operation of ['add', 'remove']) {
    const state = harness();
    state.chat.userRoleLink = async () => {};
    state.chat.userRoleUnlink = async () => ({ removedCount: 0 });
    const next = operation === 'add' ? ['manual', 'missing'] : [];
    assert.equal(await state.api.selectedMembersSet({ id: 'ch-target-member' }, next, ['manual']), false);
    assert.equal(state.notices.some(item => item.type === 'success'), false);
  }
});

for (const mode of ['append', 'replace']) {
  test(`sync ${mode} uses explicit memberships on both channels`, async () => {
    const state = harness([member('world', 'world', 'source'), member('participant', 'manual+world', 'source'),
      member('world', 'world'), member('oldParticipant', 'manual+world')]);
    state.syncMode.value = mode;
    await state.api.handleSyncMembers();
    assert.deepEqual(state.calls, [
      { operation: 'link', roleId: 'ch-target-member', ids: ['participant'] },
      ...(mode === 'replace' ? [{ operation: 'unlink', roleId: 'ch-target-member', ids: ['oldParticipant'] }] : []),
    ]);
    assert.equal(state.rows.find(item => item.userId === 'world' && item.roleId === 'ch-target-member').grantSource, 'world');
    assert.equal(state.notices.at(-1).text, '同步完成');
  });
}
