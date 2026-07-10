---
id: ISSUE-012
title: hey-api 生成 fetch クライアント(client.gen.ts)が未使用の SSE 実装(createSseClient / serverSentEvents.gen.ts 約242行)を常に client に組み込み、SSE エンドポイントが無い本番バンドルに混入する
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-09
specs: [SPEC-003]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-012: hey-api 生成 fetch クライアント(client.gen.ts)が未使用の SSE 実装(createSseClient / serverSentEvents.gen.ts 約242行)を常に client に組み込み、SSE エンドポイントが無い本番バンドルに混入する

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/web の利用者(タスク管理のブラウザユーザー)** の **ページ初回ロードの軽さ(ダウンロード / パース量)** が **本 API に SSE エンドポイントが無いのに、生成クライアントが SSE 実装一式を常に client に組み込んで本番バンドルへ出荷することで、わずかに損なわれる**。

- **影響を受けるユーザー**: app/web の利用者(バンドルをダウンロードするブラウザユーザー)。副次的に、バンドルサイズを気にする開発者。
- **損なわれる価値**: バンドルの無駄。SSE を一切使わないのに SSE クライアント実装(約242行)が出荷される。推定 gzip 数KB。
- **影響範囲・頻度**: 常時(生成クライアントを import する限り)。ただし現状はサンプル規模で体感影響はほぼ無い。B2(消費エンドポイントが増える)で消費が広がっても SSE 分は一定量、構造的に混入し続ける。
- **回避策**: なし(生成物の構造上プロパティ単位で tree-shake されない)。上流ツールの対応待ちか、クライアントテンプレートの差し替えが必要。

## 2. 現象(何が起きているか)

SPEC-003(OpenAPI 型共有基盤)のレビューで「今回修正せず追跡する」と判断された指摘。設定ミスではなく、`@hey-api/openapi-ts@0.98.2` のバージョン制約に由来する構造的なバンドル無駄。

### 期待する動作

本 API(tasks)に SSE エンドポイントが無いなら、生成 fetch クライアントに SSE 実装(`createSseClient` / `serverSentEvents.gen.ts`)が含まれず、本番バンドルにも載らない。

### 実際の動作

- 生成 fetch クライアント `app/web/src/features/tasks/api/generated/client/client.gen.ts` が、`createSseClient` を無条件に import(`:3` `import { createSseClient } from '../core/serverSentEvents.gen';`)し、`client` オブジェクトに組み込む(`:230` `return createSseClient({ ... })`)。
- SSE 実装本体は `app/web/src/features/tasks/api/generated/core/serverSentEvents.gen.ts`(約242行、`wc -l` = 242)。実バンドルに含まれることは `"Last-Event-ID"` 等のマーカー(`serverSentEvents.gen.ts:115`)で検出できる。
- 本 API(tasks: create / list / get / start / complete)に SSE エンドポイントは無い。にもかかわらず `client` オブジェクトのメソッドとして SSE 実装が常に組み込まれるため、**プロパティ単位では tree-shake されず** SSE 一式が出荷される。推定 gzip 数KB。

### 再現手順

第三者がコード観察 + ビルドで確認できる。

1. `cd app/web && grep -n "createSseClient\|serverSentEvents" src/features/tasks/api/generated/client/client.gen.ts` → `:3`(import)と `:230`(呼び出し)を確認。
2. `wc -l src/features/tasks/api/generated/core/serverSentEvents.gen.ts` → 約242行の SSE 実装本体を確認。
3. `bun run build` 後の本番バンドルを `"Last-Event-ID"`(`serverSentEvents.gen.ts:115`)等の文字列で grep し、SSE 実装がバンドルに含まれていることを確認。
4. 本 API に SSE エンドポイントが無い(`app/api/route/router.go` の登録が create / list / get / start / complete のみ)ことを確認 → 未使用の SSE が混入していると判定できる。

### 環境・条件

- 対象 stack: app/web(TypeScript / React)。
- ツール: `@hey-api/openapi-ts@0.98.2`(`package.json:28`)の fetch クライアント生成物。
- 発見文脈: SPEC-003 実装のレビューで指摘され、「今回修正せず追跡する」と判断された。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `client.gen.ts:3` が `createSseClient` を無条件 import、`:230` で `client` オブジェクトに組み込む。SSE 本体は `serverSentEvents.gen.ts`(約242行)。
- 事実: SSE 実装は `client` オブジェクトのプロパティ / メソッドとして組み込まれるため、バンドラのプロパティ単位 tree-shake が効かず、SSE エンドポイントを一切呼ばなくても実装が残る。
- 事実: `@hey-api/openapi-ts@0.98.2` には SSE 生成を無効化する config オプションが無い(バージョン制約)。したがって `openapi-ts.config.ts` の設定ミスではなく、生成ツールの構造的挙動。
- 事実: 本 API に SSE エンドポイントは無いため、混入する SSE 実装は完全に未使用。
- 仮説: B2(SPEC-003 の方式)で web が消費する Go エンドポイントが増えても、SSE 分は API の SSE 有無に関わらず一定量が構造的に混入し続ける(消費エンドポイント数に比例して増えるのは他の生成物側で、SSE は固定の無駄として残る)。

### 根本原因

`@hey-api/openapi-ts@0.98.2` の fetch クライアント生成物が SSE サポートを常時同梱する設計で、当該バージョンにそれを無効化する設定手段が無いこと。SSE が `client` オブジェクトのプロパティとして組み込まれるためプロパティ単位の tree-shake が効かず、SSE を使わないプロジェクトでも実装一式が本番バンドルへ出荷される。設定ミスではなくツールのバージョン制約が根本。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: 実害はサンプル規模で軽微(推定 gzip 数KB)。急ぎ不要。上流の対応状況を追跡し、無効化手段が入ったら反映する。
- 候補(いずれも確定ではない):
  - **(a) 上流の更新待ち**: `@hey-api/openapi-ts` の更新で SSE 除外オプション / プロパティ単位で tree-shake 可能な生成形が提供されたら、config で有効化し再生成する。
  - **(b) クライアントテンプレートの検討**: SSE を含まない別のクライアントテンプレート / プラグイン構成を検討する。
- いずれも `bunfig.toml` の `minimumReleaseAge`(21日)ゲートを満たす版で反映する。

### 実施内容

- [ ] `@hey-api/openapi-ts` の更新で SSE 除外オプション / tree-shake 可能な生成形が提供されるかを追跡する
- [ ] 提供されたら bunfig ゲートを満たす版へ更新し、config を調整して再生成(impl-web / checker)
- [ ] 代替として SSE を含まないクライアントテンプレート構成の可否を検討する

### 再発防止

- 生成クライアントを更新・差し替えるときは、本番バンドルに未使用機能(SSE 等)が混入していないかをバンドル検査(マーカー文字列 grep / サイズ計測)で確認する観点をレビューに含める。

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。SPEC-003(OpenAPI 型共有基盤)のレビューで「今回修正せず追跡する」と判断された、hey-api 生成 fetch クライアントへの未使用 SSE 実装の混入を独立 Issue として起票した。
- 事実確認: `client.gen.ts:3` が `createSseClient` を無条件 import し `:230` で `client` に組み込む。SSE 本体は `serverSentEvents.gen.ts`(`wc -l` = 242 行)、バンドル混入は `"Last-Event-ID"`(`:115`)で検出可能。本 API に SSE エンドポイントは無い(`app/api/route/router.go` は create / list / get / start / complete のみ)。プロパティ単位では tree-shake されず、推定 gzip 数KB が出荷される。
- 事実確認: `@hey-api/openapi-ts@0.98.2`(`package.json:28`)には SSE 生成を無効化する config オプションが無い(バージョン制約)。設定ミスではなくツールの構造的挙動。B2 で消費エンドポイントが増えても SSE 分は構造的に混入し続ける。
- 現状影響: サンプル規模で体感実害はほぼ無し。バンドルの無駄(推定 gzip 数KB)としてのみ存在。
- severity は **low** と判定。判定根拠: 機能不全ではなくバンドルサイズの軽微な無駄で、サンプル規模では実害がほぼ無い。ただし回避策が無く(プロパティ単位 tree-shake が効かない)、上流の対応がある構造的問題のため、忘れないよう追跡は必要。緊急度は低いため low。
- 相互リンク: frontmatter `specs` に **SPEC-003** を追加し、SPEC-003 側 `issues` にも本 Issue を追記した。判断根拠: 本件は SPEC-003 で導入した `@hey-api/openapi-ts` 生成物に固有の課題であり、生成ツール / クライアントテンプレートの選定・更新は SPEC-003 の設計判断に属する。
- 次にやること: `@hey-api/openapi-ts` の更新で SSE 除外オプション / tree-shake 可能な生成形が提供されるかを追跡し、提供され次第 bunfig ゲートを満たす版へ更新して config を調整・再生成する。
</content>
