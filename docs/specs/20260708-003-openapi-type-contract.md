---
id: SPEC-003
title: Go⇄TypeScript 型共有基盤(OpenAPI 契約 / B2 方式)
status: approved  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-08
updated: 2026-07-10
issues: [ISSUE-009, ISSUE-011, ISSUE-012, ISSUE-013, ISSUE-023]       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-003: Go⇄TypeScript 型共有基盤(OpenAPI 契約 / B2 方式)

## 1. ユーザー価値(なぜ作るか)

> **このリポジトリの開発者(および multi-agent ワークフロー)** が **app/api(Go)と app/web(TypeScript)の request/response 型を単一の契約から自動で一致させられるようになり**、**バックエンドとフロントエンドの型ズレを手作業で同期する負担と、ズレに気づけないまま壊れるリスク** を無くす。

- **対象ユーザー**: cc-orchestrator の開発者(impl-api / impl-web を含む各 agent と、変更をレビューする人)
- **解決する課題**: 現状、API の契約が **二重定義**になっている。app/api は Go の DTO struct、app/web は `features/tasks/api/schema.ts` に手書きの Zod スキーマを持ち、片方を変えても他方は自動追従しない。フィールド名・型・必須/任意・ステータスコードのズレが**実行時**まで発見されない。
- **得られる価値**:
  - API 契約の正が 1 つ(`openapi.yaml`)になり、web の型(と Zod による実行時検証)がそこから生成される
  - Go の DTO を変えたら生成物の差分が出る → レビューで契約変更が可視化される
  - CI のドリフト検査で「Go を変えたのに再生成し忘れ」を落とせる(手動同期に逆戻りしない)
- **価値の検証方法**: 以下がすべて満たされたら成功とみなす。
  1. app/web の tasks 機能の request/response 型・Zod スキーマが `openapi.yaml` からの生成物に置き換わり、手書き `schema.ts` の DTO 定義が消える
  2. app/api の DTO(例: レスポンスのフィールド追加)を変更 → `make openapi` で `openapi.yaml` が更新 → web の `bun run generate` で生成 TS が更新される、という一連が再現できる
  3. Go の DTO と生成 TS を意図的にズラした状態で CI のドリフト検査が **fail** する
  4. app/api のランタイムバイナリ / `go.mod` の import が標準ライブラリのみのまま維持される

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- **API 実装者(impl-api)** として、handler の DTO を変えたら注釈から `openapi.yaml` を再生成するだけでよい。なぜなら web 側の型は自動追従し、手で TS を直す必要がないから。
- **web 実装者(impl-web)** として、API のレスポンス型を自分で Zod に書き写さずに済む。なぜなら `openapi.yaml` から型 + Zod + TanStack Query が生成されるから。
- **レビュアー** として、契約変更を `openapi.yaml` の差分として一目で確認できる。なぜなら生成物がコミットされるから。

### 利用フロー

**契約を変更するとき(開発フロー):**

1. impl-api が handler の DTO とアノテーションを変更する
2. `cd app/api && make openapi` で `app/api/docs/openapi.yaml` を再生成する
3. impl-web が `cd app/web && bun run generate` で型 + Zod + TanStack Query を再生成する
4. 生成された `openapi.yaml` と TS を実装差分と一緒にコミットする
5. CI がドリフト検査(再生成して `git diff --exit-code`)で、コミット漏れが無いことを保証する

**契約を消費するとき(web の実装):**

1. impl-web は生成された型 / Zod / query オプションを import して使う
2. wire(DTO)→ ドメイン型 `Task` への変換は従来どおり `toDomain()` を経由する(生成対象外)

## 3. 要件(何を満たすべきか)

### 機能要件

- [ ] R1: app/api の tasks エンドポイント(`GET /tasks` / `GET /tasks/{id}` / `POST /tasks` / `POST /tasks/{id}/start` / `POST /tasks/{id}/complete`)に swag v2 アノテーションを付与し、**OpenAPI 3.1** の `app/api/docs/openapi.yaml` を生成できる。エンドポイント集合と DTO は **Go 実装を正**とし(D2/D3 の解決 = web を Go に合わせる)、priority フィールドは先行する **SPEC-002** で Go に追加された後の契約を対象とする
- [ ] R2: 生成される `openapi.yaml` が、現行の wire 契約(フィールド・JSON タグ・必須/任意・ステータスコード・エラーレスポンス形状)を正確に表現している
- [ ] R3: app/web が `openapi.yaml` から **型 + Zod スキーマ + TanStack Query** を `@hey-api/openapi-ts` で生成し、`features/tasks/api/schema.ts`(DTO 部)と `client.ts` の役割を置き換える
- [ ] R4: 外部データ(API レスポンス)は生成 Zod で実行時検証してから型を付ける(web 規約「外部データは Zod 検証」を維持)
- [ ] R5: wire(DTO)→ ドメイン型 `Task` の変換境界(`toDomain()` と `features/tasks/domain/`)は codegen 対象外で温存し、依存方向 `components → hooks → (api | domain)` を崩さない
- [ ] R6: CI に**ドリフト検査**を追加する。Go→`openapi.yaml`→生成 TS を再生成し、`git diff` が非空なら fail する
- [ ] R7: 契約変更〜再生成の手順が Makefile(`make openapi`)と package.json script(`bun run generate`)として提供され、`.claude/rules/{api,web}.md` の「コマンド」表に反映される

### 非機能要件

- **std-lib 維持**: app/api のランタイムバイナリと `go.mod` の require/import に外部依存を増やさない。swag は `go run <pkg>@<pinned-version>` のビルド時 CLI として使い、`-ot yaml`(YAML のみ出力)で生成 `docs.go` をコンパイル対象にしない。Go ソースに増えるのはアノテーションコメントのみ。
- **サプライチェーン**: web の生成ツール(`@hey-api/openapi-ts` 等)は devDependencies として追加し、`bunfig.toml` の `minimumReleaseAge`(21日)ゲートを満たす版に固定する(満たせない場合のみ `minimumReleaseAgeExcludes` を検討)。
- **再現性**: 生成物(`openapi.yaml` と生成 TS)はコミットし、レビュー可能な差分にする。CI は生成物の一致のみ検査し、各ジョブに跨る stack のツールチェーンを要求しない設計を優先する。
- **既存テスト**: 移行後も web の既存テスト(component / hooks / MSW)と api の `go test` がグリーンであること。

### スコープ外(やらないこと)

- app/auth の OpenAPI 化(現状 app/web からの消費が無いため対象外。将来別 Spec)
- REST から gRPC / GraphQL への移行(A2 / A3 は不採用、下記代替案参照)
- MSW モックの OpenAPI からの自動生成(任意の発展。今回は既存 MSW を維持し、契約一致は R6 のドリフト検査で担保)
- Go 側での OpenAPI spec のランタイム配信(Swagger UI 等のホスティング)

## 4. 設計(どう実現するか)

### 方針

**B2: Go を契約の正とし、注釈から OpenAPI 3.1 を生成 → web はそれを消費して生成。** 採用理由は SPEC 検討時の比較(下表)のとおり、「REST 維持」「Go std-lib 維持」「web の Zod + React Query を活かす」を満たしつつ、契約の正を Go 実装側に置いて spec と実装のズレを構造的に起きにくくできるため。

```
app/api の handler 注釈 ──swag v2 (go run, -ot yaml)──▶ app/api/docs/openapi.yaml (コミット)
                                                              │  ../api/docs/openapi.yaml
                                                              ▼
app/web  @hey-api/openapi-ts ──▶ 型 + Zod + TanStack Query (生成物をコミット)
                                    └─ features/tasks/api の DTO 型/検証/クライアントを置換
                                    └─ toDomain() と domain/ は温存(生成対象外)
CI: Go→yaml→TS を再生成し git diff --exit-code(ドリフト検査)
```

### アーキテクチャ / データ / インターフェース

- **app/api**:
  - handler(`route/task_handler.go` 等)と一般 API 情報(`cmd/api/main.go` 上)に swag v2 アノテーションを付与
  - `make openapi` = `go run github.com/swaggo/swag/v2/cmd/swag@<pinned> init -g cmd/api/main.go -o docs --outputTypes yaml`(正確なパッケージパス・版・フラグは planner が固定)
  - 生成物: `app/api/docs/openapi.yaml`(YAML のみ。`docs.go` は生成しない/コンパイルしない)
- **app/web**:
  - `@hey-api/openapi-ts` の設定(input = `../api/docs/openapi.yaml`、plugins = typescript / zod / tanstack-query)を追加
  - `bun run generate` で生成。出力先は `features/tasks/api/` 配下の生成専用ディレクトリ(命名・配置は planner)
  - `schema.ts` の DTO Zod と `client.ts` の fetch 関数を生成物へ置換。`toDomain()` は生成 DTO 型を入力に取る薄いアダプタとして残す
  - `hooks/useTasks.ts` は生成された query オプション/関数を利用する形に調整
- **.github(impl-ci)**: ドリフト検査ジョブ(Go と Bun をセットアップ → `make openapi` → `bun run generate` → `git diff --exit-code`)
- **契約の版整合**: swag v2 が OpenAPI 3.1 を出力し、`@hey-api/openapi-ts` が 3.1 を入力に取れることを前提とする(planner が版で検証。もし不整合なら swag v1 + 変換へフォールバックする判断を planner が提示)

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| A1 OpenAPI spec-first(手書き openapi.yaml) | spec と Go 実装の一致を別途担保する必要。Go 実装を正にできる B2 の方がズレに強い |
| A2 Protocol Buffers + Connect | REST を捨てる大改修 + `connect-go` 依存で std-lib 方針に反する |
| A3 GraphQL(gqlgen) | REST を捨てる + `gqlgen` 依存。タスク管理規模にオーバースペック |
| B1 Go struct → TS 型直接生成(tygo 等) | 型のみで実行時検証が無く、web 規約「外部データは Zod 検証」と噛み合わない |
| C1 tRPC / C2 Zod 共有 | バックが TS 前提。現状 Go では不成立 |
| swag v1(Swagger 2.0) | 2.0→3.0 変換の一手間とツールが増える。v2 で 3.1 を直接出力できるため第一候補は v2(不整合時のフォールバックとしては保持) |

## 5. 実装計画

詳細計画は planner が `docs/plans/SPEC-003-plan.md` に作成した(方針・変更ファイル・手順・テスト戦略・リスクは同ファイルが正)。概要タスク:

> **着手前ゲート(planner が発見・要確認)**: Go の実コード(B2 では契約の正)と app/web の現行実装は 3 点で wire 契約が食い違う — (D1) `priority` は Go に無く web に有る、(D2) 遷移は Go が `POST …/start`・`…/complete` / web が `PATCH …/status`、(D3) エラー包みが Go `{error}` / web `{message}`。R1 が挙げる `PATCH /tasks/{id}/status` は **web 側の契約**で Go には存在しない。生成契約をどちらへ寄せるかを確定してから T3/T4 に着手する(詳細・推奨は plan 冒頭「契約整合ゲート」)。

- [ ] T1: (planner) 現行 wire 契約の精密な棚卸し(`route/response.go` / `task_handler.go` のフィールド・JSON タグ・ステータス・エラー形状)と、swag v2 版・`@hey-api/openapi-ts` 版・出力配置・ドリフト検査方式の確定
- [ ] T2: (tester) 契約・生成物に対する検証テストの先行設計(Go: 生成 yaml の妥当性 / web: 生成 Zod と `toDomain` の整合、既存テストの移行方針)
- [ ] T3: (impl-api) swag v2 アノテーション付与 + `make openapi` 追加 + `openapi.yaml` 生成・コミット。ランタイム std-lib 維持を確認
- [ ] T4: (impl-web) `@hey-api/openapi-ts` 導入 + `bun run generate` 追加 + `schema.ts`/`client.ts` を生成物へ置換 + `toDomain`/hooks 調整(bunfig ゲート順守)
- [ ] T5: (impl-ci) CI にドリフト検査ジョブを追加
- [ ] T6: (tester) テスト実行・不足補完 → (checker) format / lint / typecheck → (review-*) セキュリティ / パフォーマンス / 仕様準拠レビュー
- [ ] T7: 指摘対応(Blocker/Major は impl へ差し戻し)、`.claude/rules/{api,web}.md` のコマンド表更新、本 Spec のステータス・経緯更新

> 注: T3(api)完了で `openapi.yaml` が確定してから T4(web)を着手する依存がある(web 生成は yaml を入力に取るため)。T2 の一部は T3 と並行可。

## 6. 経緯(時系列・追記のみ)

### 2026-07-08

- 初版作成。「フロント⇄バックの request/response 型を型推論させる方法」の比較検討から、方式 **B2(Go 注釈 → OpenAPI 生成 → web で消費)** を採用する方針が決定した。
- ユーザーとの確認で以下を決定:
  - 対象範囲: **app/api ⇄ app/web(tasks 機能)のみ**(app/auth は除外)
  - OpenAPI 版: **swag v2 → OpenAPI 3.1 を直接出力**(swag v1 + 変換はフォールバックとして保持)
  - web 生成ツール: **@hey-api/openapi-ts**(型 + Zod + TanStack Query)
- std-lib 方針の維持方法として、swag をビルド時 CLI(`go run <pkg>@<pinned>`, `-ot yaml`)に隔離し `docs.go` を非コンパイルとする設計、生成物のコミット + CI ドリフト検査、web domain 層(`toDomain`)の温存を設計に明記した。
- 版の厳密な固定・出力配置・ドリフト検査の実装方式は planner に委譲する。status は draft。ユーザー承認(approved)後に着手する。
- ユーザー承認を得て status を `approved` に更新。planner に実装計画(`docs/plans/SPEC-003-plan.md`)の作成を委譲する。
- planner が実装計画 `docs/plans/SPEC-003-plan.md` を作成。調査で Go 実コードと web の wire 契約に 3 ドリフト(D1 priority / D2 遷移エンドポイント / D3 エラー包み)を確認し、生成契約の整合方向を着手前ユーザー判断ゲートとして提示(推奨: D1 は SPEC-002 先行、D2/D3 は Go に合わせる)。R1 の `PATCH /tasks/{id}/status` は実コードに無いため、§3 の endpoint 記述は spec-owner による再整合が必要(申し送り)。
- ISSUE-009 を相互リンク(frontmatter `issues`)。ISSUE-009 は本 Spec の実装計画が特定した drift **D2**(状態遷移エンドポイント乖離: web `PATCH /tasks/:id/status` ↔ api `POST /tasks/{id}/start|complete`)を独立 Issue として追跡・可視化するもの。D2 の解消方向確定(選択肢 Q / 推奨 Q1)は本 Spec の T3/T4 着手前ゲートであり、ISSUE-009 の対応方針は本 Spec のゲート判断と同期させる。
- ゲートをユーザー判断で解決: **D1 = SPEC-002(Go に priority 追加)を先行実装してから本 Spec の生成に着手**(web の priority 機能を落とさない)/ **D2・D3 = web を Go に合わせる**(web が `POST …/start`・`…/complete` を呼び、エラーは `{error}` を読む。backend は変更しない)。これに伴い §3 R1 の endpoint 記述を Go 実体(`POST …/start`・`…/complete`)へ訂正した。着手順は **SPEC-002 → 本 Spec**。planner の plan は「D2/D3 = Go 準拠・D1 = SPEC-002 先行」をベースラインとして確定する(再委譲時に反映)。

### 2026-07-09

- 本 Spec 実装のレビューで「今回修正せず追跡する」と判断された指摘 3 件を issue-creator が起票し、frontmatter `issues` に相互リンクした:
  - **ISSUE-011**(low): 生成 TanStack Query プラグイン出力(`react-query.gen.ts`)が未使用で、hooks は独自に `useQuery`/`useMutation` を組む設計との乖離(R3)。生成 queryOptions が zod 検証 + `toDomain`(R4/R5)を組み込まないため手書き hooks が検証境界を満たす一方、§4「hooks は生成 query オプションを利用する」記述と乖離。方針整理(生成 queryOptions のラップ採用 or `@tanstack/react-query` プラグイン除去 + §4 再整合)は spec-owner / 次サイクルで判断。
  - **ISSUE-012**(low): hey-api 生成 fetch クライアントが未使用の SSE 実装(`serverSentEvents.gen.ts` 約242行)を常に client に組み込み本番バンドルへ混入(推定 gzip 数KB)。`@hey-api/openapi-ts@0.98.2` に SSE 無効化オプションが無いバージョン制約。上流更新 / 別テンプレート検討。
  - **ISSUE-013**(low): 推移依存 `js-yaml@4.1.1` の moderate 脆弱性 GHSA-h67p-54hq-rp68(ビルド時のみ・入力はリポジトリ管理下の `openapi.yaml` で信頼済み)。上流更新待ち or `overrides`/`resolutions` 固定(bunfig 21日ゲート要確認)。
- **ISSUE-009 を `resolved` に更新。** 本 Spec の **D2 解消(web を Go の `POST …/start`・`…/complete` に合わせる)** で状態遷移契約の cross-stack 乖離が解消したことを web 実コード・テストで検証し、R6 ドリフト検査(`.github/workflows/contract-drift.yml`)で再発を機械検出できる状態になったため。

### 2026-07-10

- **プロジェクト全体レビューでの契約再生成の一時的破壊と解消(ISSUE-023)。** SPEC-007(app/web の TypeScript 7 ネイティブ tsc 移行)により、本契約の web 型生成器 `@hey-api/openapi-ts@0.98.2` が TS7 の `ts.SyntaxKind` API と非互換になり `bun run generate` が失敗し、R6 ドリフト検査(`.github/workflows/contract-drift.yml`)が Go DTO 変更時に fail する状態になっていた(コミット済み生成物・typecheck・build は正常だが再生成が不能)。`@hey-api/openapi-ts` を TS7 対応の next プレリリース `0.0.0-next-20260708192938` へピン留めして解消(checker が `bun run generate` 成功・冪等 + typecheck/lint/build/test 全 pass を独立検証)。生成物は契約 `app/api/docs/openapi.yaml` 由来の型・エンドポイント内容に差分なし。stable 復帰の follow-up は ISSUE-023 / SPEC-007 経緯で追跡。frontmatter `issues` に ISSUE-023 を追加。
