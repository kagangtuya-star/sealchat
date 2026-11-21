// zhlint 懒加载：只在首次扫描时动态加载浏览器构建（其主入口会引用 Node 的 fs），整页共享一个实例。
// 规则不用 preset：只开启标点宽度与全角标点旁空格两类，引号、括号、中英文间距等会改变行文风格的规则保持关闭。
import type { ProofLint } from './logProofread';

/** 不含句号：连续英文句点、单个句点交给本地规则判断；引号不在列表中，保留角色原有的引号风格。 */
export const ZHLINT_RULES = Object.freeze({
  fullwidthPunctuation: '，：；？！',
  noSpaceBeforePauseOrStop: true,
  noSpaceAfterFullwidthPauseOrStop: true,
  skipAbbrs: ['Mr.', 'Mrs.', 'Dr.', 'Jr.', 'Sr.', 'vs.', 'etc.', 'i.e.', 'e.g.', 'a.k.a.'],
  skipPureWestern: true,
});

const silent = { log: () => {}, warn: () => {}, error: () => {} };

let pending: Promise<ProofLint> | null = null;

export const loadZhlintLint = (): Promise<ProofLint> => {
  pending ||= import('zhlint/dist/zhlint.es.js').then(({ run }): ProofLint => (text) => run(text, {
    // zhlint 会就地改写 rules 对象（迁移旧键名），每次传入新副本。
    rules: { ...ZHLINT_RULES, skipAbbrs: [...ZHLINT_RULES.skipAbbrs] },
    // 保留 zhlint 的默认解析行为；传入空 hyperParse 会错误地重复末尾字符。
    logger: silent,
  }).result).catch((error: unknown) => {
    pending = null;
    throw error;
  });
  return pending;
};
