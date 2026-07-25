---
id: SPEC-013
title: テストの実 DB 一本化と専用テスト DB 分離(手書きダブル廃止)
status: in-progress  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-11
updated: 2026-07-11
issues: [ISSUE-028, ISSUE-029]       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-013: テストの実 DB 一本化と専用テスト DB 分離(手書きダブル廃止)

関連: SPEC-005(Postgres 永続化 / app/migrator)/ SPEC-009(ツールチェーンのコンテナ隔離・`--network none`。本 SPEC で R3 を改訂)/ SPEC-010(CQRS 2 プール)/ SPEC-011(Postgres 一本化・infra/memory 廃止)。本 SPEC はこれらを置き換えず、**テスト戦略のみ**を更新する。

## 1. ユーザー価値(なぜ作るか)

> **この repo の開発者・CI・multi-agent ワークフロー** が **DB 永続化層に依存するテストを、手書きのモック/スタブではなく実 Postgres に対して実行できるようになり**、**「モックは通るが実 DB で壊れる」乖離を排除しつつ、開発用データを汚さずにテストできる** 価値を得る。

- **対象ユーザー**: app/api・app/auth を開発・レビューする人と subagent(tester / impl-* / review-*)、および CI
- **解決する課題**: 現状、DB を要する検証は `//go:build integration` タグで隔離され、通常のユニットテスト(service / route 層)は**手書きのテストダブル**(fake / stub / spy リポジトリ)で DB を代替している。このため (a) モックの振る舞いと実 DB の振る舞いが乖離しても `make check` は緑になり、(b) ダブルの保守コストと実態と乖離した死んだコメント(例: 既に廃止された `infra/memory` を指す記述)が蓄積し、(c) 実 DB 経路のカバレッジは integration タグを明示実行したときしか得られない
- **得られる価値**: DB に触れるテストはすべて実 Postgres に対して回るため、SQL・トランザクション・制約・sqlc 生成コードまで含めた本物の振る舞いを日常の `make check` で検証できる。テストは専用の `api_test` / `auth_test` データベースに閉じ、`make up` 等で使う開発用 `api` / `auth` データを汚さない
- **価値の検証方法**: `//go:build integration` タグが repo から消え、DB 依存テスト(infra/postgres のリポジトリ / 2 プール、route の正常系・全フロー、authorize / token フロー等)がすべて `make check` の一部として `api_test` / `auth_test` に対して緑になり、DB を代替する手書きダブル(in-memory fake)と実態と乖離したコメントが除去され(R2 例外 2 の薄い計装デコレータのみ実 DB backed で残置可)、開発用 `api` / `auth` データベースがテスト実行で変更されないことを確認できたら成功とみなす

## 2. ユーザー体験(何ができるようになるか)

### ユーザーストーリー

- 開発者として、`make check` を叩くだけで DB 層まで本物で検証したい。なぜなら「モックでは通ったが実 DB で落ちる」を CI 到達前に潰したいから。
- 開発者として、テストを流しても `make up` で見ている開発データが消えたり書き換わったりしてほしくない。なぜならテストと手元の動作確認を同じ DB で混ぜたくないから。
- tester / review agent として、実装の振る舞いを実 DB で確認したい。なぜなら手書きダブルは「実装者が想定した振る舞い」であって「DB の実際の振る舞い」ではないから。

### 利用フロー

1. 開発者 / subagent が `make check`(または個別の `make test`)を叩く
2. ラッパーが test Postgres を起動し、`app/migrator` で `api_test` / `auth_test` を作成 + マイグレーション適用してからテストを実行する
3. 各テストは実 `postgres.New*Repository` を実 test DB に配線して走り、テスト間はデータ隔離(方式は実装計画で決定)により互いに汚染しない
4. 開発用 `api` / `auth` データベースは一切触られない
5. pre-commit / CI も同じ経路を通り、ローカル = CI が同一の実 DB 検証で一致する

## 3. 要件(何を満たすべきか)

### 機能要件

- [ ] R1: **`//go:build integration` タグの廃止**。DB 依存テスト(`infra/postgres` のリポジトリ / `OpenPair` 2 プール、`infra/repotest` 契約の呼び出し、route の正常系・全フロー、auth の authorize / token / refresh フロー、`testsupport`)を通常のテスト対象へ一本化し、`make test` / `make check` の一部として実 DB に対して実行する
- [ ] R2: **手書きリポジトリテストダブルの廃止と実リポジトリへの置換**。service / route 層で DB を代替している fake / stub / spy(`fakeRepository` / `stubListPageRepository` / `readerSpy` / `writerSpy` / `failingRepository` / `dbErrorRepository` / `notFoundRepository`、auth の `stubClientOnlyRepo` / `alwaysNotFoundAuthCodeRepo` 等)を、実 test DB に配線した `postgres.New*Repository` で置き換える。**in-memory fake を backing に残すことは禁止**(下記例外の計装も含め、データ経路は必ず実 test DB を通す)。**例外(理由付きで最小限のみ)**:
  - **例外 1(障害系)**: 実 DB では現実的に誘発できない障害系(汎用 DB エラー → HTTP マッピング等)は、実 DB で誘発する手段(接続 close / context cancel / 制約違反)へ寄せるか、寄せられないものだけを「意図的に残すダブル」として残す
  - **例外 2(実 DB 非観測な実装 seam の検証計装)**: 実 DB が原理的に観測できない性質 — service 層のポート振り分け(読み→Reader / 書き→Writer、SPEC-010)や narrow-port の compile-time 型証明など、`reader == writer` の単一プールでは DB から区別できないもの — を検証する spy は、**実 `postgres.New*Repository`(test DB)をラップして呼び出しを数える薄い計装デコレータ**として残してよい(in-memory fake を backing にしない)
  - 線引きと残す/消すの判断は実装計画で確定し、**異常系および SPEC-010 準拠(ルーティング・narrow-port)のカバレッジを後退させない**
- [ ] R3: **専用テスト DB の分離**。テストは `api_test` / `auth_test` データベース(識別子制約 `^[a-z_][a-z0-9_]*$` によりハイフン不可 → underscore)に対して実行し、開発用 `api` / `auth` を汚染しない。作成は `app/migrator` の既存経路(`ensureDatabase` + goose 適用)を再利用し、per-run で `DB_NAME` を test DB に向ける
- [ ] R4: **テスト間データ隔離**。実行順非依存・並列安全を保つ隔離(truncate / トランザクションロールバック / 一意 ID 等)を用意する。方式は実装計画で決定(planner 委任)。`.claude/rules/testing.md`「実行順序に依存するテストを書かない / 実時間 sleep に依存しない」を維持する
- [ ] R5: **`make check` / pre-commit / CI の統合**。通常の検査フローが test Postgres 起動 + `api_test` / `auth_test` へのマイグレーション適用を前提としてテストを実行する。ローカルは compose の Postgres、CI は既存 `*-integration` ジョブの Postgres 起動を通常 `check` ジョブへ統合する(integration ジョブの統廃合)
- [ ] R6: **SPEC-009 R3 の改訂**。「check / build / test を `--network none` で実行」のうち、**テスト実行フェーズのみ** test Postgres にのみ到達可能な private network(インターネット egress なし)へ緩和する。install フェーズ(network 有効)、fmt / lint / vet / build / generate(offline 維持)の方針は変えない。サプライチェーン防御の意図(非依存取得フェーズはインターネットに出さない)は維持する
- [ ] R7: **不要コード・コメントの削除**。廃止した build tag、置換した手書きダブル、実態と乖離したコメント(auth の `infra/repotest/*_contract.go` に残る「for infra/memory」記述、`testsupport` の「never compiled in default `make test`」等の説明、`db.md` / `testing.md` の該当記述)を実態に合わせて削除・更新する

### 非機能要件

- **契約テストの単一性維持**: `infra/repotest` の `Run<集約>RepositoryContract` は単一の正のまま。テストロジックを実装ごとに二重化しない(SPEC-011 の思想を継続)
- **決定性**: 全テストは実行順非依存・並列実行安全。データ隔離により flakiness を作らない
- **サプライチェーン防御の維持**: SPEC-009 の「install 時のみ network / 非依存フェーズはインターネット非到達」を維持。テストフェーズに与えるのは DB への到達のみで、インターネット egress は与えない
- **実行時間**: 実 DB 化に伴うテスト時間増は許容する。ただし接続再利用・軽量な隔離手段でオーバーヘッドを最小化する。**副作用として、DB を代替していたモックの排除により service 層に純オフラインのユニットテスト tier が残らなくなる**(service テストも実 test DB を要する)。これは R2「in-memory fake を backing に残すことは禁止」の帰結として受容する(オフラインの即時フィードバックより、実 DB による本物の振る舞い検証を優先)。実行時間・fail-fast の将来最適化余地は plan §6 および関連 Issue に記録する
- **公開契約不変**: 本 SPEC はテスト戦略の変更に閉じる。DB スキーマ・sqlc 生成・ドメインポート・HTTP / OpenAPI 契約・実行時 env 契約・本番 runtime の挙動は変えない

### スコープ外(やらないこと)

- **DB に触れない純粋ロジックテストの実 DB 化**: 集約の状態遷移・VO 検証などリポジトリを持たない domain 層テスト、および `cmd/*/env.go` の env → `Config` 写像テスト(`t.Setenv` ベース。設計上 dial しない)や `persistence_selection_test.go` の `Config.DSN` / `Validate` テストは、そもそも DB 接続を持たないため対象外。実 DB 化しない(「全テスト実 DB」の意図は "DB を代替しているモックの排除" であり、DB を必要としない純粋テストを DB 依存にすることではない)
- DB スキーマ / マイグレーション / sqlc の内容変更(テスト用の付加的変更を除く)
- 本番 runtime の永続化選択ロジック(SPEC-011 の fail-closed)の変更
- test DB を dev DB と同居させる案(汚染回避というユーザー要件に反する)

## 4. 設計(どう実現するか)

### 方針

「DB を代替するモックを、DB に触れるテストから排除する」を主目的とする。手段は (1) `integration` タグを廃止して DB 依存テストを日常検査に載せる、(2) 手書きダブルを実 test DB backed の実リポジトリへ置換する、(3) テストを開発用 DB から隔離するため専用 `api_test` / `auth_test` を用意する、(4) それを可能にするため SPEC-009 のテストフェーズ network 制約を「DB のみ到達可 / インターネット非到達」に緩和する。純粋ロジックテスト(DB 非依存)は対象外として区別を明確にする。

### アーキテクチャ / データ / インターフェース

- **テスト DB 命名**: `api_test` / `auth_test`(underscore)。理由は `app/migrator` の `domain/migration.DatabaseName` が識別子を `^[a-z_][a-z0-9_]*$`(+ 63byte)で検証しハイフンを許さないため。ユーザー表記の「api-test / auth-test」の意図(= api / auth 用のテスト DB)を、識別子として正当な underscore 形で実現する
- **test DB の作成・マイグレーション**: `app/migrator` の既存 `ensureDatabase` + goose 適用経路を再利用する。`-target api|auth` の既定 `DB_NAME`(= `api` / `auth`)に対し、`DB_NAME=api_test` / `auth_test` を注入して test DB を作成 + 適用する(migrator 本体のロジック変更は最小。必要なら test 用の薄い導線を planner が設計)
- **`testsupport`(api / auth)**: `//go:build integration` を外し、既定接続先を test DB(`DB_NAME` 既定を `api_test` / `auth_test`)へ変更する。`OpenTestDB` の `DB_HOST` 未設定時 `t.Skip` は安全弁として残すか、check が常に DB を用意する前提に合わせて扱いを planner が決める。`Truncate*` 等の隔離ヘルパをここへ集約する
- **service / route テスト**: 実 `postgres.New*Repository`(api の task は `NewTaskRepository` / 必要に応じ `NewTaskReader` / `NewTaskWriter`、auth は各集約の実装)を test DB に配線する。異常系(DB エラー時の HTTP 5xx マッピング等)は、実 DB で誘発できるもの(接続 close・context cancel・制約違反)へ寄せ、寄せられないものだけを理由付きの最小ダブルとして残す(R2)
- **`make check` / Makefile**: 各 stack の `check` が test DB 起動 + マイグレーション適用を前提にテストを実行するよう導線を追加する。ルート `Makefile` / 各 stack `Makefile` / pre-commit hook / CI の変更点は planner が網羅列挙する
- **CI(`cicd.yml`)**: 現状の `api-integration` / `auth-integration` ジョブ(Postgres service + migrator でスキーマ適用)を通常 `api` / `auth` `check` ジョブへ統合するか、`check` ジョブ自身が Postgres service を持つ形に再編する(方式は impl-ci が planner の計画に沿って実装)
- **SPEC-009 network(R6)**: テスト実行時のみ、compose の `postgres`(test DB)へ到達できる private network 上で実行し、インターネットへは出さない。install の network 有効・非テストフェーズの offline は不変。SPEC-009 側は本 SPEC の決定を経緯 + §4 に反映する(`.claude` / spec 更新は admin)
- **データ隔離(R4)**: 方式は planner 委任(トランザクションロールバック / テスト毎 truncate / 一意 ID のいずれか、または併用)。既存 `testsupport.Truncate*` を延長する案が最有力だが、コミット/複数コネクション依存の検証(2 プール・rotation の atomic 操作等)との相性を planner が評価して決める

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| 現状維持(unit = offline 手書きダブル / integration = 実 DB の 2 層) | ユーザー要件(全 DB テストを実 DB 化・モック乖離の排除)に反する。ダブルの保守コストと死んだコメントの蓄積という現状課題も解消しない |
| test DB を作らず開発用 `api` / `auth` に相乗り | テスト実行で開発データが汚染される。ユーザーが明示的に避けたい点 |
| test DB 名を `api-test` / `auth-test`(ハイフン) | `app/migrator` の識別子検証 `^[a-z_][a-z0-9_]*$` に反し作成不能。仮に quoting で通しても全経路でのクォート必須化を招く。underscore で意図を満たす |
| DB 依存テストを in-memory fake に一本化 | 目的と正反対。SPEC-011 で `infra/memory` は既に廃止済み |
| テストフェーズも `--network none` を維持(SPEC-009 不変) | 実 DB(別コンテナ)へ到達できず本 SPEC が成立しない。DB のみ到達可 / インターネット非到達で供給網防御の意図は保てる |

## 5. 実装計画

詳細は `docs/plans/SPEC-013-plan.md`(planner が作成)。着手前に planner が以下を設計する:

- [ ] T1: planner が方針を計画化 — データ隔離方式の決定、R2 の異常系ダブルの線引き(実 DB 誘発へ寄せる/最小限残す)、影響ファイルの stack 別網羅、`app/migrator` の test DB 導線、`make check` / pre-commit / CI の変更点、SPEC-009 R6(テストフェーズ network)の compose 表現、テスト戦略、リスク
- [ ] T2: tester が現状テストの棚卸しと、実 DB 化後に**等価以上のカバレッジ**(特に異常系)を保つ移行後テスト設計を用意
- [ ] T3: impl-db が `app/migrator` の test DB 対応、`testsupport`(api / auth)の build tag 解除・test DB 既定化・隔離ヘルパ、`infra/postgres` の `integration` タグ除去、`infra/repotest` の死んだ「infra/memory」コメント整理
- [ ] T4: impl-api / impl-auth が各 stack の service / route テストを実 test DB backed へ移行、手書きダブル削除、残す異常系の実 DB 誘発化
- [ ] T5: impl-ci がルート `Makefile` / 各 `Makefile` の `check` を test DB 起動 + マイグレーション前提に、pre-commit hook、`cicd.yml` の `check` ジョブへの Postgres 統合(integration ジョブ統廃合)、SPEC-009 R6 の compose network 表現を実装
- [ ] T6: admin が `.claude/rules/testing.md` / `db.md` の記述、SPEC-009(§4 + 経緯に R3→R6 改訂)、CLAUDE.md「永続化」「コマンド早見表」注記を更新(`.claude` / CLAUDE.md / spec 整備は admin 権限)
- [ ] T7: checker / tester が全テスト実 DB 緑・実行順非依存・並列安全・CI 緑を検証
- [ ] T8: review-security(テストフェーズ network 縮小の妥当性・egress・DB 資格情報の扱い)/ review-performance(テスト時間・接続 / 隔離コスト)/ review-spec(要件充足・**異常系カバレッジ非後退**の確認)。今回対応しない指摘は issue-creator が起票

## 6. 経緯(時系列・追記のみ)

### 2026-07-11

- 初版作成(status: approved)。ユーザー要望:「全てのテストを実 DB アクセスにする。開発時のデータを汚染しないよう api-test / auth-test 用のテスト DB を作成する。関連して不要なコード・コメントを削除する」。
- admin が現状を調査(Explore)。DB 接続層は現在 **層ごとに使い分けた併用**構成: 実 DB アクセスは `//go:build integration` で隔離(通常 `make check` から除外)、service / route の DB 依存部は**手書きテストダブル**(モックライブラリ不使用)で代替、`infra/repotest` の契約テストは実 DB 実装専用(SPEC-011 で in-memory 廃止済み)。純粋ロジック・env → Config 写像は DB 非依存の通常テスト。
- **ユーザー決定**: (1) **全面一本化** — `integration` タグを廃止し DB 依存テストを全て test DB へ、`make check` / pre-commit / CI 通常ジョブも test DB 起動を前提化、SPEC-009 の「通常チェックはオフライン」保証(R3)は本件で改訂。純粋 domain ロジックテスト(repo 非依存)は対象外に維持。(2) テスト間データ隔離の方式は planner に委任。
- **設計上の確定事項**: test DB 名は `app/migrator` の識別子検証(`^[a-z_][a-z0-9_]*$`)によりハイフン不可のため **underscore**(`api_test` / `auth_test`)を採用。SPEC-009 R3 はテスト実行フェーズのみ「test Postgres へのみ到達可 / インターネット非到達」へ緩和し、供給網防御の意図は保持(R6)。異常系カバレッジ(DB エラー → HTTP マッピング等)を後退させないことを R2 の必須条件として明記。
- 本作業に先立ち、レビュー品質を上げるため review-security / review-performance / review-spec の 3 agent を「既定は不合格 / 承認には能動的検査の根拠を要する」批判的スタンスへ強化済み(admin による `.claude/` 整備)。
- パイプライン: planner(T1)→ tester(T2)→ impl-db / impl-api / impl-auth / impl-ci(T3–T5、scope 独立部は並列)→ admin(T6: rules / SPEC-009 / CLAUDE.md 更新)→ checker / tester(T7)→ review-*(T8)。実装は planner の計画確定後に着手する。

### 2026-07-11(追記: R2 例外条項の拡張 — reader/writer spy の裁定)

- planner 計画のレビューで、ユーザーが「R2 が明示列挙した `readerSpy`/`writerSpy` を計画が『残す』としているのは Spec 逸脱では」と指摘。planner のコード確認により、原案の spy は背後で**共有 `fakeRepository`(in-memory fake)を backing に温存**しており、R2 の核心(DB 代替の排除)に**反する**ことが判明。
- 一方、この spy には実 DB が原理的に観測できない固有カバレッジ(service 層のポート振り分け=SPEC-010 R2、および narrow-port の compile-time 型証明。`reader == writer` の単一プールでは DB から区別不能)があり、単純廃止は SPEC-010 準拠の回帰ガードを失う。
- **ユーザー決定(admin 提示の (a) を採用)**: spy を「**実 `postgres.New*Repository`(test DB)をラップして呼び出しを数える薄い計装デコレータ**」へ作り替え、in-memory fake を排除しつつルーティング/narrow-port のカバレッジを保全する。これに合わせ **R2 の例外条項を「障害系」限定から拡張**し、(1) 障害系、(2) 実 DB 非観測な実装 seam の検証計装、の 2 類型を認める形へ改訂。併せて「in-memory fake を backing に残すことは禁止」を明文化(§3 R2 / §1 価値の検証方法)。退けた案: (b) reader/writer を別 test DB に向けデータ差で観測(production の replica 同一データ前提に反する虚構・型証明も得られず非推奨)、(c) ルーティングテスト廃止(SPEC-010 準拠の明示的後退)。
- **planner への差し戻し事項**: 計画 §4 の当該行を「fakeRepository 温存」から「実 postgres Reader/Writer をラップする計数デコレータ」へ改訂。加えて Q1 の裁定として、truncate 方式は妥当(tx ロールバックは 2 プール跨ぎ不可・`r.db.BeginTx` の自前 tx・コミット済み reuse 検出の再現不能により atomic 操作を検証不能)と確認しつつ、**将来 `t.Parallel()` を導入すると truncate + 共有 DB は破綻し、その際は「並列単位ごとの別 DB(`api_test_<n>` / `CREATE DATABASE ... TEMPLATE`)」へ移行が要る**旨を計画 §6(リスク/前提)に追記させる。

### 2026-07-11(追記: T8 パフォーマンスレビューの将来最適化を ISSUE-028 として起票)

- T8 の review-performance が「今回は対応しない・将来の最適化余地」として挙げた 3 点 —(1)`go test -p 1 ./...` の DB 非依存パッケージまでの一律直列化(実測: api 約 2.3–2.7 倍・auth 約 2 倍 / +400–650ms)、(2)`testsupport.OpenTestDB` の per-test 接続 open/close(約 1.7ms × 概算 110–150 回 ≈ +200–300ms、リーク/枯渇リスクは無し)、(3)CI / pre-commit の DB provisioning が offline チェックより前に無条件実行され fail-fast が後退(+ `migrate-test` の両 target 常時実行)— を **ISSUE-028** として起票し相互リンク(frontmatter `issues: [ISSUE-028]`)。
- いずれも本 Spec の非機能要件「実行時間増は許容」の範囲内で `docs/plans/SPEC-013-plan.md` §6 が既知トレードオフとして記載済み。**不具合ではなく将来の最適化候補**(重要度 low)としての記録であり、本 Spec の完了判定には影響しない。

### 2026-07-11(追記: T8 セキュリティ再検証の一貫性課題を ISSUE-029 として起票)

- T8 の review-security 再検証で、本 Spec の Major 修正(pre-commit hook を offline / db-test の 2 フェーズに分離し `go test` フェーズを `tools-db`=internet 非到達へ移し、「コード実行フェーズが egress」の Major を解消)後に残る **供給網防御の一貫性課題**を検出。Minor = hook 経路の offline フェーズ(api / auth の fmt-check/lint/vet/build)が `tools`(network 有効)で実行される一方 CI・直接 `make check` は `tools-offline`(`--network none`)で実行する非対称性(SPEC-009 R3 からの部分的後退。fmt/lint/vet/build はコード非実行のため悪用可能性は低い)。Info = `app/migrator` の hook 経由 test(SPEC-013 以前からの既存挙動・DB 非依存)が network 有効な `tools` で走る点。
- これらを **ISSUE-029** として起票し相互リンク(frontmatter `issues` に追記)。severity low・今回は対応せず、SPEC-009 R3/R6 の一貫性強化の将来課題として記録。本 Spec の主目的(test フェーズの egress 遮断)は達成済みで完了判定には影響しない。

### 2026-07-11(実装・レビュー完了 / status: in-progress へ)

- **パイプライン完走**: T1 planner(計画 + 隔離方式=truncate+`-p 1` の裁定)→ T2 tester(受け入れ基準)→ T3 impl-db(testsupport のタグ削除・`*_test` 既定化・`RequireDBHost` で fail-closed 一元化・infra/postgres タグ削除・repotest 死んだコメント整理)/ T4a impl-api(service/route の実 DB 移行・`fakeRepository` 完全削除・`readerSpy`/`writerSpy` を実 postgres Reader/Writer をラップする計数デコレータ化・`failingRepository` 最小 1 個)/ T4b impl-auth(ダブル削除・helpers 統合・境界テスト不変)/ T5 impl-ci(`migrate-test`・`tools-db`+`dbnet`(`internal: true`)・CI の integration ジョブ統合・pre-commit 2 フェーズ化)→ T6 admin(`.claude/rules/{testing,db}.md`・SPEC-009 R3→R6・CLAUDE.md 更新)→ T7 checker/tester → T8 review-*。
- **実 DB 走行で本 Spec の価値を実証**: 実 Postgres に対する走行で `TestTaskService_Get` が「in-process の `time.Now()`(monotonic + ナノ秒)と Postgres `timestamptz`(マイクロ秒)の厳密 struct 等価」で決定的に FAIL —旧 `fakeRepository` が同一 struct を返すため隠れていた「モックは通るが実 DB で壊れる」アサーション欠陥を検出。impl-api がテスト側のみ(フィールド別比較 + 時刻は 1μs 許容差)で修正(domain/schema は不変)。同種パターンの走査で他に該当なし。
- **検証結果(fallback docker run + toolchain イメージ)**: auth 全パッケージ PASS(`-race` の token_concurrency / refresh_token 並行 atomic 含む)、api も修正後 全パッケージ PASS(`-race` 含む)。**決定性**(2 回連続 + shuffle・計 5 回で結果同一・flaky なし)、**開発用 `api`/`auth` DB 非汚染**(マーカー行で実行前後不変)、**`internal: true` の egress 遮断**(peer 到達可・外部 TCP/DNS/curl の 3 系統遮断を plain docker で実測)、**`make lint`(gosec 込み)= 0 issues** をいずれも確認。
- **T8 レビュー結果**: review-spec = R1–R7 すべて満たす・カバレッジ非後退・スコープ非侵犯。review-security = Blocker 0 / **Major 1**(pre-commit hook の DB テストが internet 到達可能な `tools` で走る)を検出 → impl-ci が offline / db-test の 2 フェーズ別コンテナ実行へ分離して解消(`go test`=コード実行フェーズを `tools-db`=internet 非到達へ)、review-security 再検証で **解消確認**。Minor(REQUIRE_DB forwarding の api/auth 不一致 → bare `-e REQUIRE_DB` に統一 / postgres の `ip_forward=0` 明示追加)は修正済み。review-performance = Blocker/Major 0(本番ホットパス不変を確認)。今回対応しない指摘は ISSUE-028(perf)/ ISSUE-029(hook offline network 一貫性)へ。
- **残検証(当環境で未クローズ・push / compose 環境で確定)**: (1) 実 `docker compose` 経路での `make check` と `tools-db`→`postgres` サービス名 DNS 解決(当環境に `docker compose` v2 プラグインが無く、テストは同等の `docker run` + toolchain イメージで代替検証した。egress 遮断の核心特性は実測済み)。(2) CI(`cicd.yml`)の統合済み `api`/`auth` `check` ジョブの実 green。
- **status**: 実装・全レビュー・ローカル実 DB green まで完了。上記残検証(compose 実経路 + CI green)を push 時に確認して満たせば **done** に更新する。それまで **in-progress**。
