#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

echo "[1/2] 运行导出实体相关核心测试..."
go test ./service -run 'TestBuildBBCodeTextLineNormalizesNestedEntitiesForPlainText|TestBuildBBCodeTextLineFromQuickFormat|TestEnhancePlainContentForHTMLExportNormalizesNestedEntities|TestStripRichTextDecodesNestedEntities' -count=1

echo "[2/2] 运行协议层嵌套实体归一化测试..."
go test ./protocol -run 'TestNormalizeNestedEntitiesMultiRound' -count=1

echo "测试通过：导出实体转义修复相关用例全部通过。"
