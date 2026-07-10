---
id: ISSUE-011
title: SPEC-003 で生成した TanStack Query プラグイン出力(react-query.gen.ts)がアプリコードから未使用で、hooks は独自に useQuery/useMutation を組む設計との乖離(dead code / 設計方針の要整理)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-09
updated: 2026-07-09
specs: [SPEC-003]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-011: SPEC-003 で生成した TanStack Query プラグイン出力(react-query.gen.ts)がアプリコードから未使用で、hooks は独自に useQuery/useMutation を組む設計との乖離(dead code / 設計方針の要整理)

## 1. ユーザー価値への影響(なぜ対応するか)

> **cc-orchestrator の開発者(impl-web / レビュアー / codegen 設計を追う各 agent)** の **「生成物の正=どれを使うのが正解か」を一目で判断できること** が **`@tanstack/react-query` プラグインが query オプション(react-query.gen.ts)を生成・コミットしているのに一切 import されず、実際の hooks は別経路(手書き)で組まれているため、設計意図が読み取れなくなること** で損なわれる。

- **影響を受けるユーザー**: cc-orchestrator の開発者。**エンドユーザー(タスク管理利用者)への実行時影響は無い**(生成 query オプションは未 import のため tree-shake され、本番バンドルに載らず実行時コストは 0)。
- **損なわれる価値**: 生成基盤(SPEC-003)の設計の一貫性・可読性。「生成 query オプションを使う」という Spec §4 の記述と、実装(手書き hooks)が食い違い、どちらが正の設計かが不明瞭になる。dead code とドリフト検査対象が無駄に増える。
- **影響範囲・頻度**: 常時(生成物がコミットされている限り)。ただし機能面の実害は無く、開発者の認知負荷・保守面に限定。
- **回避策**: 不要(機能は正しく動作している)。設計方針の確定=整理そのものが対応。

## 2. 現象(何が起きているか)

SPEC-003(OpenAPI 型共有基盤)のレビューで「今回修正せず追跡する」と判断された指摘(R3)。機能上のバグではなく、生成物の設計方針の整理課題。

### 期待する動作

生成した TanStack Query プラグイン出力(`react-query.gen.ts` の queryOptions / mutationOptions)が hooks から利用されるか(Spec §4「hooks は生成 query オプションを利用する」)、あるいは使わない前提なら生成対象から外れているか、いずれかで生成物と実装が一貫している。

### 実際の動作

- `app/web/openapi-ts.config.ts` の `plugins` に `@tanstack/react-query`(`:22`)が指定され、`app/web/src/features/tasks/api/generated/@tanstack/react-query.gen.ts`(queryOptions / mutationOptions)が生成・コミットされている。
- しかし当該生成物は **アプリコードから一切 import されていない**(`generated/` ディレクトリ外に `react-query.gen` / `generated/@tanstack` を参照する import は無いことを grep で確認)。
- 実際の hooks(`app/web/src/features/tasks/hooks/useTasks.ts`)は、生成 query オプションを使わず **独自に** `useQuery` / `useMutation` を組み立て、検証済みの `client.ts` 関数(`fetchTasks` / `fetchTaskById` / `createTask` / `startTask` / `completeTask`)を `queryFn` / `mutationFn` に渡している(`useTasks.ts:1-3`, `:10-66`)。
- 結果として `react-query.gen.ts` は dead code。未 import のため tree-shake され本番バンドルには載らず、実行時コストは 0。

### 再現手順

第三者がコード観察でそのまま確認できる。

1. `app/web/openapi-ts.config.ts:14-23` の `plugins` に `"@tanstack/react-query"` が含まれることを確認。
2. `app/web/src/features/tasks/api/generated/@tanstack/react-query.gen.ts` が生成・コミットされていることを確認(`ls`)。
3. `cd app/web && grep -rn "react-query.gen\|generated/@tanstack" src --include="*.ts" --include="*.tsx" | grep -v "/generated/"` を実行 → 出力が空(= 生成物ディレクトリ外からの import が存在しない)。
4. `app/web/src/features/tasks/hooks/useTasks.ts:1-3`, `:10-66` が、生成 query オプションではなく手書きの `useQuery` / `useMutation` + `client.ts` 関数で組まれていることを確認。

### 環境・条件

- 対象 stack: app/web(TypeScript / React)。
- ツール: `@hey-api/openapi-ts@0.98.2`(`package.json:28`)、プラグイン `@tanstack/react-query`。
- 発見文脈: SPEC-003 実装のレビューで指摘 R3 として記録され、「今回修正せず追跡する」と判断された。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `openapi-ts.config.ts:22` が `@tanstack/react-query` プラグインを有効化し、`react-query.gen.ts` が生成・コミットされている。
- 事実: 当該生成物はアプリコードから未 import(grep で確認)。tree-shake により本番バンドルに載らず実行時コストは 0。
- 事実: `useTasks.ts` は独自に `useQuery` / `useMutation` を組み、検証済み `client.ts` 関数を渡している。この手書き hooks は、`client.ts` 側で zod 検証 + `toDomain` 変換(SPEC-003 の R4 / R5)を経た値を返すため、**機能上は正しく検証境界を満たしている**。
- 事実: 一方で生成 queryOptions は素の生成 SDK 呼び出しをそのまま返す設計で、**zod 検証 + `toDomain` 変換(R4 / R5)を組み込んでいない**。そのまま hooks で使うと web 規約「外部データは zod 検証」および R4 / R5 の検証境界を満たせない。
- 事実: このため現状の手書き hooks は「正しく検証境界を満たす」ための合理的な選択だが、Spec §4「hooks は生成された query オプション/関数を利用する」という設計記述とは乖離している(生成 query オプションを使っていない)。
- 仮説: config で `@tanstack/react-query` を有効にしたまま実装では手書きに寄せた結果、生成物だけが取り残されて dead code 化した(生成プラグインの取捨を実装確定後に config へ反映していない)。

### 根本原因

**機能上のバグではない。** 生成 query オプションが R4 / R5(zod 検証 + `toDomain` 変換)を組み込まない素の SDK 呼び出しを返す設計であるため、検証境界を満たすには手書き hooks でラップする必要があり、実装はそちらを採った。にもかかわらず config は `@tanstack/react-query` プラグインを有効なままにしているため、使われない生成物(dead code)がコミットされ、Spec §4 の設計記述と乖離している。「生成 query オプションを使うのか / 使わないのか」という設計方針が確定・整理されていないことが根本。

## 4. 対応(どう解決するか)

### 対応方針

- **前提**: 実行時実害は無い(tree-shake でコスト 0)。即時対応必須ではなく、SPEC-003 の設計方針整理として spec-owner / 次サイクルで判断する。
- 以下はいずれも**候補**であり確定方針ではない:
  - **(a) 生成 queryOptions を土台に使う**: 生成 queryOptions を `select` / `queryFn` ラップで包み、zod 検証 + `toDomain` 変換(R4 / R5)を足して hooks から利用する。Spec §4 の設計記述に実装を寄せる方向。
  - **(b) 使わない前提を明確化する**: config の `plugins` から `@tanstack/react-query` を外し、生成される dead code とドリフト検査対象を減らす。あわせて Spec §4 の「生成 query オプションを利用する」記述を「hooks は検証済み client.ts 関数を利用する」へ spec-owner が再整合する。
- いずれも「生成 query オプションを検証境界(R4 / R5)とどう両立させるか」の設計判断が前提。

### 実施内容

- [ ] spec-owner / 次サイクルで方針(a / b)を確定する
- [ ] (a) の場合: 生成 queryOptions を検証 + `toDomain` でラップして hooks を移行(impl-web / tester)
- [ ] (b) の場合: config から `@tanstack/react-query` を除去し再生成、Spec §4 の記述を再整合(impl-web / spec-owner)
- [ ] 生成物と実装・Spec 記述の一貫性を確認し、ドリフト検査対象を確定する

### 再発防止

- codegen プラグインの取捨は「生成物を実際に import して使うか」を基準に実装確定後 config へ反映し、使わない生成物をコミットしない。
- Spec の設計記述(§4)と実装(hooks の組み方)の一致をレビュー観点にし、生成物の「正の使い道」を 1 つに定める。

## 5. 経緯(時系列・追記のみ)

### 2026-07-09

- 起票。SPEC-003(OpenAPI 型共有基盤)のレビューで指摘 R3 として記録され、「今回修正せず追跡する」と判断された、生成 TanStack Query プラグイン出力の未使用 / 設計方針乖離を独立 Issue として起票した。
- 事実確認: `openapi-ts.config.ts:22` が `@tanstack/react-query` プラグインを有効化し `react-query.gen.ts` を生成・コミット。しかし当該生成物はアプリコードから未 import(`grep -rn "react-query.gen\|generated/@tanstack" src | grep -v "/generated/"` が空)。`useTasks.ts:1-66` は生成 query オプションを使わず、独自 `useQuery` / `useMutation` + 検証済み `client.ts` 関数で組んでいる。
- 事実確認: 手書き hooks は `client.ts` 経由で zod 検証 + `toDomain`(R4 / R5)を満たし機能上は正しい。一方 Spec §4「hooks は生成 query オプションを利用する」とは乖離。生成 queryOptions は R4 / R5 を組み込まない素の SDK 呼び出しを返す設計のため、そのまま使うと検証境界を満たせない。
- 現状影響: エンドユーザーへの実行時影響ゼロ(未 import で tree-shake、実行時コスト 0)。影響は開発者の認知負荷・保守面(dead code / ドリフト検査対象 / 設計記述との乖離)に限定。
- severity は **low** と判定。判定根拠: 機能上のバグではなく、実行時コストも 0(tree-shake)。損なわれるのは生成基盤の設計の一貫性・可読性という開発者体験のみで、回避策の要否も無い。緊急度は低く、SPEC-003 の設計方針整理(次サイクル / spec-owner 判断)として追跡すれば足りるため low。
- 相互リンク: frontmatter `specs` に **SPEC-003** を追加し、SPEC-003 側 `issues` にも本 Issue を追記した。判断根拠: 本件は SPEC-003 の設計記述(§4)と生成 config の乖離そのものであり、方針(a: 生成 queryOptions を検証ラップで採用 / b: プラグイン除去 + §4 再整合)の確定は SPEC-003 側の設計判断に属する。
- 次にやること: spec-owner / 次サイクルで方針(a / b)を確定し、確定後に impl-web / tester(必要なら spec-owner の §4 再整合)で生成物・実装・Spec 記述を一貫させる。
</content>
</invoke>
