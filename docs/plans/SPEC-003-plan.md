# SPEC-003 実装計画: Go⇄TypeScript 型共有基盤(OpenAPI 契約 / B2 方式)

- 起点: `docs/specs/20260708-003-openapi-type-contract.md`(status: approved)
- 対象 stack: `app/api`(Go・注釈のみ)/ `app/web`(TypeScript・生成消費)/ `.github`(CI ドリフト検査)。app/auth は対象外
- 成果物: swag v2 注釈 + `make openapi` + `app/api/docs/openapi.yaml`(コミット)、`@hey-api/openapi-ts` による web 生成物(コミット)、CI ドリフト検査ジョブ

---

## ⚠️ 冒頭: 着手前に必要なユーザー判断(契約整合ゲート)

調査の結果、**app/api(Go の実コード = B2 では契約の正)と app/web の現行実装は、3 点で wire 契約が食い違っている**。SPEC-003 は「Go を正として web を自動追従させる」ものなので、Go からそのまま OpenAPI を生成すると **現行 web が壊れる**。R1 / T1 が想定する「4 エンドポイント(…`PATCH /tasks/{id}/status`)」は **web 側の契約**であり、**Go の実コードには存在しない**(Go は `POST /tasks/{id}/start` と `POST /tasks/{id}/complete`)。したがって着手前に「生成する契約をどちらに寄せるか」の確定が必要。

### 契約ドリフト表(調査で確認した事実)

| # | 項目 | app/api(Go・正)の実際 | app/web の現行前提 | SPEC-003 での扱い |
|---|---|---|---|---|
| D1 | `priority` フィールド | **無し**。`route.taskResponse` = `{id,title,status,created_at,updated_at}`、`createTaskRequest` = `{title}` | **有り**。`TaskDto`/`createTaskRequest`/mock/`domain` すべて `priority`(low\|medium\|high)を持つ。UI(`CreateTaskForm`・`TaskItem`)と `sortByPriority` が依存 | **要判断(下記 選択肢 P)** |
| D2 | ステータス遷移 | `POST /tasks/{id}/start`・`POST /tasks/{id}/complete`(body 無し) | `PATCH /tasks/{id}/status` + `{status}`(`updateTaskStatus`) | **要判断(下記 選択肢 Q)**。推奨は Go に合わせる |
| D3 | エラー包み | `{"error": string}`(`route.errorResponse`) | `{"message": string}` を読む(`shared/api/http.ts`・mock) | **要判断(下記 選択肢 R)**。推奨は Go に合わせる |

補足: web は `VITE_API_BASE_URL ?? "/api"` を前置し `/api/tasks` を叩く。Go は `/tasks` を提供する。これは契約ドリフトではなく **クライアントの baseUrl 設定**で吸収する(生成クライアントの `baseUrl` を `/api` に設定、OpenAPI の `servers`/paths は Go の実パス `/tasks…`)。

### 判断が必要な選択肢と推奨

- **D1 priority — 選択肢 P(推奨: P2)**
  - P1: Go の現状(priority 無し)を正とする → 生成型に priority が無くなり、**web の priority 機能(作成フォームの優先度・`sortByPriority`・`TaskItem` 表示)を削除する回帰**が発生。SPEC-002(Task に優先度を追加)の web 先行実装を巻き戻すことになり、非機能要件「既存 web テストがグリーン」も広範に崩れる。
  - **P2(推奨): 先に SPEC-002(Go に priority を追加)を通してから SPEC-003 の web 生成(T4)を行う。** そうすれば生成契約に priority が含まれ、web は priority を維持できる(B2 の「Go が正」も満たす)。SPEC-002 は現在 `status: draft` なので、承認 → impl-api 実装 が SPEC-003 の T4 の前提になる。
  - この判断が付くまで **T3(annotation の DTO)/T4(生成)は着手不可**(注釈が「priority を含むか」で変わるため)。**admin/ユーザーの確定を待つ**。
- **D2 ステータス遷移 — 選択肢 Q(推奨: Q1)**
  - Q1(推奨): 契約を Go の実装(start/complete)に合わせ、web を `PATCH status` から `start()`/`complete()` の 2 コールへ移行する。Go ドメインは元々 start/complete で遷移をモデル化しており、B2 では Go を変えないのが筋。web 側の変更は小さい(`TaskItem` は既に domain の `startTask`/`completeTask` で次状態を算出しているため、遷移種別は web 内で判別済み)。ISSUE-008 が「別課題」として記録した乖離もこれで解消。
  - Q2: Go に `PATCH /tasks/{id}/status` を新設して web を維持 → Go の業務 API 追加であり SPEC-003(型共有基盤)のスコープを超える。R1 が `PATCH` を名指すため spec-owner が R1 を Q に合わせて再整合するのが望ましい(§3 の編集は本計画の権限外。**経緯に注記**して spec-owner に委ねる)。
- **D3 エラー包み — 選択肢 R(推奨: R1)**
  - R1(推奨): 契約を Go の `{"error": string}` に合わせ、web の `shared/api/http.ts` とモックの読み取りキーを `error` に変更。エラー形状も OpenAPI の `components/schemas` に定義し生成型へ載せる。
  - R2: Go を `{"message": string}` に変更 → Go の presentation 変更で B2 の非侵襲方針に反する。

> 本計画は **「Q1 + R1 を採用し、D1 は P2(SPEC-002 先行)を推奨」** を作業ベースラインとして手順・変更ファイル・テスト戦略を記述する。生成の *機構*(型 + Zod + Query を生成し、`schema.ts` の DTO を置換、`toDomain` は温存)は D1〜D3 の決定に依らず同一で、決定は「注釈に載せるフィールド/エンドポイント集合」だけを左右する。決定が P1/Q2/R2 に振れた場合の差分は各セクションに明記する。

---

## 方針

### 採用アプローチ

1. **Go を契約の正とし、swag v2 の注釈から OpenAPI 3.1 を直接生成する(B2)。** ランタイム/`go.mod` は標準ライブラリのみを維持し、swag は `go run <pkg>@<pinned>` のビルド時 CLI として隔離、`-ot yaml`(YAML のみ)で `docs.go` を生成もコンパイルもしない。Go ソースに増えるのは **注釈コメントのみ**。
2. **web は `@hey-api/openapi-ts` で `型 + Zod + TanStack Query` を生成**し、`features/tasks/api/generated/` に配置してコミットする。`schema.ts` の手書き DTO Zod と `client.ts` の fetch 実装を生成物へ置換する。
3. **`toDomain()`(wire DTO → ドメイン `Task`)と `features/tasks/domain/` は codegen 対象外で温存**し、依存方向 `components → hooks → (api | domain)` を崩さない。生成 DTO 型を入力に取る薄いアダプタとして `toDomain` を残す。
4. **生成物(`openapi.yaml` と web 生成 TS)はコミット**し、CI に **ドリフト検査**(再生成して `git diff --exit-code`)を追加する。
5. **契約整合の 3 ドリフト(D1〜D3)は上のゲートで確定**してから T3/T4 を着手する(B2 では Go が正なので、注釈が契約そのものになる)。

### 退けた代替案(SPEC 4 の比較に加え、本計画レベルの判断)

| 案 | 退けた理由 |
|---|---|
| swag v1(Swagger 2.0)+ swagger2openapi 変換を第一候補にする | v2 が 3.1 を直接出力でき変換ツールが 1 段減る。v1 はフォールバックとして「リスク/未確定事項」に保持 |
| hey-api の生成 SDK/クライアントを一切使わず「型 + Zod」だけ生成し、`client.ts` を手書き維持 | R3「client.ts の役割を置換」を満たしにくく、query 側の追従も手作業に戻る。生成 Query(tanstack-query plugin)+ 生成クライアントを採用し、`ApiError` 正規化と Zod 検証を境界に挟む形にする |
| 生成 TS を lint/format 対象に含める | 生成物は `any`/default export/命名が biome 規約(`noExplicitAny`・`noDefaultExport`)や整形と衝突しうる。生成ディレクトリは **biome の対象から除外**しつつ **tsgo typecheck と build には含める**(型の健全性は担保)。コミットするので差分レビューは可能 |
| Go 側の生成 yaml 妥当性を `go test` で検証 | app/api は std-lib のみで **YAML パーサを持たない**(外部依存を足せない)。yaml 妥当性は「hey-api が入力に取れること」+ CI のドリフト検査(+任意で spectral/redocly lint を CI に追加)で担保し、`go test` は既存の振る舞いテスト + 追加する httptest 契約テストに限定する |
| swag に priority/PATCH を勝手に足して両者を一致させる | Go の業務 API 変更は SPEC-003(型共有基盤)のスコープ外。D1 は SPEC-002、D2 は spec-owner/ISSUE-008 の領域として切り分ける(上記ゲート参照) |

---

## 変更ファイル(stack ごと)

> 凡例: [新] 新規 / [変] 変更 / [生] 生成物(コミット対象)。担当 agent を併記。**(D1依存)** は priority 決定で内容が変わる箇所。

### app/api(impl-api / T3、tester は契約テスト)

- [変] `cmd/api/main.go` — swag v2 の一般 API 情報注釈(`// @title` `// @version` `// @description` `// @BasePath /` 等)を `main`/`run` 付近のコメントとして付与。**ロジック変更なし**。
- [変] `route/task_handler.go` — 各ハンドラ(create/list/get/start/complete)にオペレーション注釈(`@Summary` `@Tags` `@Accept` `@Produce` `@Param` `@Success` `@Failure` `@Router`)を付与。**成功/レスポンス型は `route.taskResponse`(全フィールド string)を参照**(`service.TaskDTO` は `time.Time` なので参照しない)。**(D1依存)** priority を載せるかは決定次第。
- [変] `route/response.go` — `errorResponse`(`{error}`)を swag のスキーマ(`@Failure … {object} errorResponse`)として参照できるようにする。必要なら swag 参照解決のため型を export、または `--parseInternal` 等で解決(下記リスク参照)。
- [変] `Makefile` — `openapi` ターゲット追加。`check` には含めない(生成は検査ではない。ドリフトは CI が担保)。ヘルプ表に 1 行追加。
- [生] `app/api/docs/openapi.yaml` — 生成 OpenAPI 3.1(YAML のみ)。`docs.go`/`swagger.json` は生成しない。
- [新] `route/task_handler_test.go`(tester / T2)— `net/http/httptest` による **wire 契約テスト**(全 5 エンドポイントの JSON 形状・フィールド・ステータスコード・`errorResponse` 形状)。R2 の実行可能な基準にする。table-driven。
- 検証: `make openapi` 後に `git status` で **`go.mod`/`go.sum` が変化しないこと**、`docs.go` が生成されないこと、`go build ./...` が通ることを確認(std-lib 維持)。

### app/web(impl-web / T4、tester はテスト移行)

- [変] `package.json` — devDep に `@hey-api/openapi-ts@<pinned>` を追加。生成クライアントが要求する runtime 依存(例: `@hey-api/client-fetch@<pinned>`)を追加。`"generate": "openapi-ts"` script を追加。いずれも **21 日ゲートを満たす版に固定**(満たせない場合のみ `minimumReleaseAgeExcludes` を検討)。
- [変] `bunfig.toml` — 原則変更なし(ピン版がゲートを通る前提)。どうしても新しい版が必要なときのみ hey-api パッケージを `minimumReleaseAgeExcludes` に追記。
- [新] `openapi-ts.config.ts` — `input: "../api/docs/openapi.yaml"`、`output: "src/features/tasks/api/generated"`、plugins: `@hey-api/typescript` / `zod` / `@tanstack/react-query`、client は fetch。named export・zod v4 互換の出力になるよう設定(下記リスク参照)。
- [変] `biome.json` — `files.includes` に `"!src/features/tasks/api/generated/**"` を追加し、生成物を lint/format 対象外にする(typecheck/build には残す)。
- [生] `src/features/tasks/api/generated/**` — 生成された 型 / Zod / query オプション / クライアント(コミット)。
- [変] `src/features/tasks/api/schema.ts` — 手書き DTO Zod(`taskSchema`/`taskListSchema`/`TaskDto`)を削除し、**生成 Zod/型の re-export + `toDomain`/`toDto`/`createTaskRequestSchema`(ドメイン向けの入力検証)** に整理。`toDomain` は生成 DTO 型を入力に取る形へ。**(D1依存)**。
- [変] `src/features/tasks/api/client.ts` — 生成クライアント/SDK 呼び出しに置換。外部データは **生成 Zod で検証**してから `toDomain`。**(D2依存)** `updateTaskStatus` を `startTask`/`completeTask`(= `POST …/start` `…/complete`)に分割、または「次状態→対応エンドポイント」への薄いディスパッチにする。
- [変] `src/features/tasks/hooks/useTasks.ts` — 生成 query/mutation オプションを利用。server/client state 分離・`ApiError` 型は維持。**(D2依存)** `useUpdateTaskStatus` を start/complete に対応。
- [変] `src/features/tasks/components/TaskItem.tsx` — **(D2依存)** 遷移呼び出しを start/complete に合わせる(domain の `startTask`/`completeTask` から遷移種別は既に判別可能)。
- [変] `src/features/tasks/components/CreateTaskForm.tsx` — **(D1依存)** priority 決定に応じてフォーム項目を維持/削除。
- [変] `src/mocks/handlers.ts` — 生成契約に整合(**(D2)** start/complete エンドポイント、**(D3)** `error` キー、**(D1)** priority 有無)。
- [変] `src/shared/api/http.ts` — **(D3依存)** エラー本文の読み取りキーを `error` に変更(生成クライアント採用でこの層を薄くする場合はエラー正規化を interceptor 側へ寄せる)。
- [変] tester 管轄: `src/features/tasks/api/schema.test.ts`(生成 Zod + `toDomain` に対する検証へ改稿)、`components/*.test.tsx`・`hooks/useTasks.test.tsx`・`api/client.test.ts`(D1〜D3 の決定に合わせて更新)。
- [変] `src/features/tasks/domain/task.ts` — 原則不変。**(D1依存)** priority を wire から外す決定なら、domain の priority を「フロント専用の派生」として残すか削るかを決定に合わせる(推奨 P2 なら不変)。

### .github(impl-ci / T5)

- [変] `.github/workflows/cicd.yml` — **契約ドリフト検査ジョブ**を追加。Go + Bun をセットアップ → `app/api` で `make openapi` → `app/web` で `bun install --frozen-lockfile && bun run generate` → `git diff --exit-code -- app/api/docs/openapi.yaml app/web/src/features/tasks/api/generated`。`app/api/**`・`app/web/**`・当ファイル変更時に発火。**唯一 Go+Bun 双方を要する跨り stack ジョブ**であることを job コメントに明記(NFR「跨り stack を要求しない設計を優先」の明示的な例外)。

### .claude/rules(admin / T7、メタ作業)

- [変] `.claude/rules/api.md` のコマンド表に `make openapi`(契約生成)を追記(R7)。
- [変] `.claude/rules/web.md` のコマンド表に `bun run generate`(契約消費・生成)を追記(R7)。

### docs(planner: 本計画、admin/planner: 起点反映)

- [新] 本ファイル `docs/plans/SPEC-003-plan.md`。
- [変] `docs/specs/20260708-003-openapi-type-contract.md` の §5 と経緯を最小限整合(planner が本タスク内で実施。§3 の R1 endpoint 記述は権限外のため経緯で spec-owner に申し送り)。

---

## 手順(agent 割り当てと依存/並列)

**厳守する依存**: T3(app/api で `openapi.yaml` 確定)→ T4(app/web で生成)。web 生成は yaml を入力に取るため。さらに **契約整合ゲート(D1〜D3)→ T3** の依存がある(注釈が契約そのもの)。

### P0. 契約整合ゲート(admin / ユーザー)
- 本計画冒頭の D1(選択肢 P)・D2(Q)・D3(R)を確定する。**D1 が P2(SPEC-002 先行)なら、SPEC-002 の承認 → impl-api 実装を先行させる**(別パイプライン)。
- 確定するまで T3/T4 は着手しない。

### P1. 先行作成(ゲート確定後、並列可)
- **tester(T2)**: `route/task_handler_test.go` を httptest で追加。全 5 エンドポイントの JSON 形状・フィールド・成功/エラーのステータスコード・`errorResponse` 形状を pin(= OpenAPI が満たすべき実行可能な契約)。既存 Go テストは振る舞い不変なので流用。
- **impl-web(準備)**: `openapi-ts.config.ts`・`package.json` の `generate` script・`biome.json` の生成ディレクトリ除外を用意(生成実行は T4 で yaml 確定後)。依存版の 21 日ゲート適合を確認。
- **impl-ci(下書き)**: ドリフト検査ジョブを cicd.yml に追加(検証はパイプライン成立後の P4)。
- これら P1 は互いに独立で **1 メッセージで並列起動可**。

### P2. app/api 注釈と生成(impl-api / T3)— *P1 と一部並列可だが yaml 確定は T4 の前提*
1. `cmd/api/main.go` に一般 API 情報注釈、`route/task_handler.go`/`route/response.go` にオペレーション/スキーマ注釈を付与(**(D1〜D3) の確定に従う**)。
2. `Makefile` に `openapi` ターゲット追加:
   `go run github.com/swaggo/swag/v2/cmd/swag@<pinned> init -g cmd/api/main.go -o docs -ot yaml`(**正確な v2 タグ・フラグ名は impl-api が検証し pin**。`--parseInternal`/`--parseDependency` の要否も検証)。
3. `make openapi` で `app/api/docs/openapi.yaml` を生成・コミット。
4. **std-lib 維持を検証**: `go.mod`/`go.sum` 無変化、`docs.go` 非生成、`go build ./...` 成功。
5. tester(P1)の httptest がグリーンであることを確認(注釈と実装の一致確認)。

### P3. app/web 生成と置換(impl-web / T4)— *T3 完了(yaml 確定)後に着手*
1. `bun run generate` で `features/tasks/api/generated/` を生成・コミット。
2. `schema.ts` の DTO Zod を生成物へ置換、`toDomain`/`toDto` を生成 DTO 型入力に調整。
3. `client.ts`/`useTasks.ts` を生成 SDK/query に移行(**(D2)** start/complete)。外部データは生成 Zod で検証してから型付け(R4)。`ApiError` 正規化を境界に維持。
4. `mocks/handlers.ts`・`shared/api/http.ts`・`TaskItem.tsx`/`CreateTaskForm.tsx` を決定に合わせて整合(**(D1〜D3)**)。
5. 依存方向 `components → hooks → (api | domain)` と named export・`any` 禁止を維持。

### P4. CI ドリフト検査確定(impl-ci / T5)— *P2/P3 成立後に検証*
- P1 で下書きしたジョブが、実際に `make openapi` + `bun run generate` 後 `git diff --exit-code` でグリーンになること、意図的にズラすと fail することを確認(R6 の受け入れ)。

### P5. テスト実行・チェック・レビュー(T6)
- **tester**: `app/api` は `make test`(既存 + httptest 契約テスト)、`app/web` は `bun run test`(生成 Zod/`toDomain`・hooks・components・MSW)を実行し不足を補完。落ちたら理由を報告(skip しない)。
- **checker**(tester グリーン後): `app/api` `make check`、`app/web` `bun run format:check` / `lint` / `typecheck` / `build`。生成物除外設定が効いていること、typecheck/build に生成物が含まれることを確認。
- **review-security / review-performance / review-spec**(checker 通過後に並列): サプライチェーン(新規依存・`go run` の隔離)、生成物のサイズ/実行時検証コスト、R1〜R7 と本計画の対応をレビュー。

### P6. 指摘対応と記録(T7)
- Blocker/Major は該当 impl agent(impl-api/impl-web/impl-ci)へ差し戻し、P5(テスト→チェック→レビュー)を再実行。今回対応しない指摘は issue-creator が Issue 化。
- **admin**: `.claude/rules/{api,web}.md` のコマンド表に `make openapi` / `bun run generate` を追記(R7)。
- **admin/spec skill**: SPEC-003 の status を `in-progress`→完了時 `done`、経緯を更新。R1 の endpoint 記述(PATCH status)と実コードの乖離は spec-owner が §3 を再整合するよう経緯に申し送り。

### 依存/並列サマリ
```
P0 ゲート(D1〜D3) ─┬─(P2 が SPEC-002 先行に依存する場合あり: D1=P2)
                    ▼
P1 [tester httptest | impl-web 準備 | impl-ci 下書き] 並列
                    ▼
P2 impl-api: 注釈 + make openapi + openapi.yaml 確定 ──(yaml)──▶ P3 impl-web: generate + 置換
                    │                                              │
                    └───────────────► P4 impl-ci: ドリフト検査 検証 ◄┘
                                          ▼
P5 tester → checker → review-{security,performance,spec}(並列)
                                          ▼
P6 指摘対応(impl 差し戻し)→ rules 更新(admin)→ Spec 更新
```

---

## テスト戦略(`.claude/rules/testing.md` 準拠)

- **TDD の適用**: Go 側は **先行(T2)**。tester が httptest で「現行 wire 契約(= 生成 OpenAPI が満たすべき形)」を pin してから impl-api が注釈を付ける。web 側は生成物の形に依存するため **T4 と並走で改稿**(純粋関数 `toDomain`/`sortByPriority` 等のドメインテストのみ先行可)。
- **app/api(tester）**:
  - 追加 httptest(`route/task_handler_test.go`, table-driven, fake repository 越し): 各エンドポイントの **正常系**(status/JSON 形状/フィールド)、**異常系**(400=不正body/空title/長title、404=未存在、409=重複/遷移不正、500)、**境界**(title 長さ境界・遷移境界 todo→doing→done)。これが **R2** の実行可能な検証。
  - 既存 `service`/`domain` テストは振る舞い不変のため `make test` で回帰確認のみ。
  - **生成 yaml の妥当性**は std-lib 制約上 `go test` で YAML パースできないため、(a) hey-api が入力に取れること、(b) CI ドリフト検査、(c) 任意で CI に OpenAPI lint(spectral/redocly)を足す、で担保。
- **app/web(tester）**:
  - `schema.test.ts` 改稿: **生成 Zod** に対する parse(正常/必須欠落/enum 不正/境界)と **`toDomain` マッピング**(生成 DTO → ドメイン `Task`)の整合。**R4/R5** を検証。
  - hooks(`useTasks.test.tsx`): MSW 越しに query/mutation の loading/success/error。**(D2)** start/complete の mutation を検証。
  - components(RTL, role/label クエリ): `CreateTaskForm`・`TaskItem`・`TaskList`。実装詳細に依存しない。**(D1/D2)** 決定に合わせて更新。
  - MSW(`handlers.ts`): 生成契約に一致(**(D1〜D3)**)。契約一致の最終担保は R6 のドリフト検査。
- **要件対応表**:

| 要件 | 主な検証 |
|---|---|
| R1(3.1 生成) | `make openapi` 成功 + `openapi.yaml` に 全エンドポイント/スキーマが出る(httptest と突き合わせ)/ CI |
| R2(現行契約を正確に表現) | tester の httptest(契約 pin)↔ 生成 yaml、hey-api 生成が通ること |
| R3(型+Zod+Query で置換) | `schema.ts`/`client.ts` の DTO/fetch が生成物へ置換(コード + typecheck/build) |
| R4(外部データを Zod 検証) | `schema.test.ts`(生成 Zod parse)+ client の検証境界テスト |
| R5(`toDomain`/domain 温存・依存方向) | `toDomain` テスト + domain 純関数テスト + 依存方向は checker/review-spec |
| R6(CI ドリフト検査) | impl-ci ジョブ: 一致で pass / 故意ズラしで fail(P4) |
| R7(コマンド提供 + rules 反映) | `make openapi`/`bun run generate` 実行 + `.claude/rules/{api,web}.md` 更新 |

---

## リスク / 未確定事項

- **【要ユーザー判断・最優先】契約ドリフト D1〜D3 の解消方向(冒頭ゲート)**。特に **D1 priority** は SPEC-002(draft)の領域で、P1 を選ぶと web の priority 機能を回帰削除することになる。**推奨 P2(SPEC-002 先行)**。D2/D3 は Go に合わせる推奨(Q1/R1)だが、R1 が `PATCH /tasks/{id}/status` を名指す点は §3 の spec 修正(spec-owner 権限)が必要。**確定するまで T3/T4 着手不可**。
- **swag v2 の版・パッケージ・フラグが未確定**: `github.com/swaggo/swag/v2/cmd/swag` は本計画時点で RC/beta 級。impl-api が「入手可能な最新 v2 タグ」を検証して pin し、`init` のフラグ名(`-ot`/`--outputTypes`、3.1 出力の要否 `--v3.1` 等)、`--parseInternal`/`--parseDependency` の要否を実機で確定する。**`go run pkg@ver` が `go.mod`/`go.sum` を汚さないこと**の実測確認が std-lib 維持の肝。ダメなら **フォールバック: swag v1(Swagger 2.0)+ `swagger2openapi` で 3.1 へ変換**(ツールが 1 段増える)。
- **swag が package-local 未export DTO(`taskResponse`/`createTaskRequest`/`errorResponse`)を解決できるか**: 同一 `route` パッケージ内注釈での参照可否、または export 化/フラグ対応が要検証(impl-api)。`created_at`/`updated_at` は **`route.taskResponse` の string 型を参照**(`service.TaskDTO` の `time.Time` を参照すると生成型が `date-time` になり web と型がズレる)。
- **swag v2(3.1)↔ `@hey-api/openapi-ts` の版整合(R5 の中核リスク)**: hey-api が 3.1 を入力に取れること、`zod` plugin 出力が **web の zod v4** と噛み合うこと(ISSUE-007 で zod v4 + rolldown-vite/vitest の相性問題が既知)を検証。取れない場合のフォールバック: (a) swag に 3.0 出力オプションがあれば 3.0、(b) v1→swagger2openapi で 3.0.x、いずれかへ切替。
- **hey-api 依存の 21 日ゲート**: `@hey-api/openapi-ts` は pre-1.0 で更新頻度が高い。21 日以上前の版に pin できる想定だが、生成クライアント runtime(`@hey-api/client-fetch` 等)も同ゲート対象。満たせない場合のみ `minimumReleaseAgeExcludes` 追記を検討(サプライチェーン方針とのトレードオフを review-security が確認)。
- **生成物と biome/tsgo の相性**: 生成 TS が `any`/default export/長大整形を含みうる。生成ディレクトリを biome から除外(typecheck/build には残す)する方針。除外設定が checker の `format:check`/`lint` を汚さないことを確認。
- **CI ドリフト検査が跨り stack**: このジョブだけ Go+Bun 双方を要し、NFR「跨り stack を要求しない設計を優先」の明示例外になる。専用 job として隔離しコメントで理由を明記(impl-ci)。
- **エラー本文と `ApiError` の整合(D3依存)**: 生成クライアント採用時、エラー正規化(`ApiError`)を interceptor へ寄せるか既存 `http.ts` を残すかで実装が変わる。impl-web が最小差分の統合方式を決める(受け入れ基準: 既存 hooks/エラー表示テストがグリーン)。
- **base-url `/api` の扱い**: 生成クライアントの `baseUrl` を `VITE_API_BASE_URL ?? "/api"` に合わせる。OpenAPI の `servers`/paths は Go の `/tasks…`。MSW の `/api/tasks…` と齟齬しないこと。
