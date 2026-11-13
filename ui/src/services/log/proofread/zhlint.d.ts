// zhlint 的浏览器构建（dist/zhlint.es.js）没有随包提供声明；这里只声明校对用到的公开 API。
declare module 'zhlint/dist/zhlint.es.js' {
  export interface ZhlintValidation {
    name: string;
    message: string;
    index: number;
    length: number;
    target: string;
  }
  export interface ZhlintResult {
    origin: string;
    result: string;
    validations: ZhlintValidation[];
    disabled?: boolean;
  }
  export interface ZhlintOptions {
    logger?: Pick<Console, 'log' | 'warn' | 'error'>;
    rules?: Record<string, unknown>;
    hyperParse?: unknown[];
    ignoredCases?: unknown[];
  }
  export function run(str: string, options?: ZhlintOptions): ZhlintResult;
}
