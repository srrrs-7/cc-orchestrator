---
id: SPEC-010
title: DB infra 層の CQRS read/write 分離(Reader/Writer + writer/reader 2 プール)
status: done  # draft | approved | in-progress | done | dropped | superseded
created: 2026-07-10
updated: 2026-07-10
issues: []       # 関連Issue ID (例: [ISSUE-003])
supersedes: null # 置き換える旧Spec ID
---

# SPEC-010: DB infra 層の CQRS read/write 分離(Reader/Writer + writer/reader 2 プール)

## 1. ユーザー価値(なぜ作るか)

> **サービスの運用者 / 開発者** が **読み取り負荷を書き込みと独立にスケールできる永続化基盤(将来の read replica への振り分け)を、アプリのコード変更なしに接続情報の注入だけで有効化できるように**なり、**読み取りスパイクで書き込み系を巻き込まない運用余地と、CQRS が構造として明示された保守しやすいコード** を得る。

- **対象ユーザー**: 本アプリ(app/api・app/auth)の運用者・開発者。エンドユーザーの体験は不変(内部アーキテクチャ変更)
- **解決する課題**: 現状、各集約の `Repository` は読み(`FindByID` / `ListPage` 等)と書き(`Save` 等)を 1 つの interface に束ね、`infra/postgres` は単一の `*sql.DB` プールで両方を捌く。このため (1) ドメイン層で読み/書きの責務境界がコード上に現れず、(2) 将来 RDS read replica を足しても、読み取りだけを別エンドポイントへ振り分ける構造上の受け皿がない
- **得られる価値**:
  - 読み系クエリを writer とは別の接続プール(将来の reader endpoint)へ流せる構造ができ、読み取り負荷のスケールアウト余地が生まれる
  - `Repository` が Reader / Writer に分割され、各 service が「読むだけ」「書く」責務を型で表明できる(CQRS の command/query 分離を infra 層で体現)
- **価値の検証方法**:
  - `DB_READER_HOST` を writer と別ホストに設定したとき、読み系クエリが reader プールへ、書き系が writer プールへ流れることを統合テストで確認できる
  - `DB_READER_HOST` 未設定時は従来どおり単一ホストで全クエリが動作し、既存の contract test / integration test が緑のまま(後方互換)であることを確認できる

## 2. ユーザー体験(何ができるようになるか)

内部アーキテクチャ変更のため、エンドユーザー向けの画面・API の挙動変化はない。「ユーザー」を運用者・開発者と読み替える。

### ユーザーストーリー

- **運用者**として、読み取り負荷が上がったとき、アプリを再デプロイせず `DB_READER_HOST` に read replica のエンドポイントを注入するだけで読み系を replica へ逃がしたい。なぜなら書き込み系(writer)を読み取りスパイクから隔離して安定させたいから
- **開発者**として、service を書くとき「この操作は読むだけか / 書くか」を型(Reader / Writer)で表明したい。なぜなら command と query の混在を防ぎ、意図しない書き込みをコンパイル時に排除したいから

### 利用フロー

1. 開発者が service を実装するとき、読み取りだけの操作には `Reader`、状態変更を伴う操作には `Writer` を依存として受け取る
2. 起動時、`cmd/<bin>` が writer 用 `DB_*` と reader 用 `DB_READER_*` を読み、`infra/postgres` が writer プールと reader プールを構築する(`DB_READER_HOST` 未設定なら reader は writer と同一接続にフォールバック)
3. 実行時、Reader 実装は reader プール、Writer 実装は writer プールへクエリを発行する
4. 運用者が本番で read replica を用意したら、`DB_READER_HOST` 等を replica エンドポイントに向けるだけで読み系が replica へ流れる(iac 側の replica プロビジョニングは本 Spec のスコープ外・別 Spec)

## 3. 要件(何を満たすべきか)

### 機能要件

- [x] R1: 各集約の永続化ポートを **Reader**(query 系)と **Writer**(command 系)の 2 つの interface に分割する。対象集約:
  - app/api: `task`(Reader: `FindByID` / `FindByTitle` / `ListPage`、Writer: `Save`)
  - app/auth: `authcode`(Reader: `FindByCode`、Writer: `Save` / `Consume`)/ `refreshtoken`(Reader: `FindByTokenHash`、Writer: `Save` / `Rotate` / `RevokeFamily`)。**`user` / `client` は command メソッドを持たない read-only 集約のため Reader-only(既存ポートを Reader 相当として据え置き、空 Writer を作らない)**。「Save/Delete/消費系 → Writer」規則は Writer 系メソッドが存在する集約にのみ適用する
- [x] R2: `infra/postgres` は **writer 用**と **reader 用**の 2 つの `*sql.DB` プールを持ち、Reader 実装は reader プール、Writer 実装は writer プールを使ってクエリを発行する
- [x] R3: reader 接続が未設定(`DB_READER_HOST` が空)のときは reader は **writer 接続にフォールバック**し、単一ホスト運用で全クエリが従来どおり動く(後方互換・fail-safe。既存デプロイ・ローカル・CI は env 追加なしで無変更動作する)
- [x] R4: 実行時本体(`cmd/api` / `cmd/authz`)の env 契約に reader 用 `DB_READER_HOST` / `DB_READER_PORT` / `DB_READER_NAME` / `DB_READER_USER` / `DB_READER_PASSWORD` / `DB_READER_SSLMODE` を追加する。各項目は **未設定時に対応する writer 値へフォールバック**する(例: `DB_READER_HOST` 未設定 → `DB_HOST`)。`SelectMode`(Postgres / memory 選択)と memory フォールバック条件は不変
- [x] R5: `infra/memory` は単一実装のまま Reader / Writer の両 interface を満たす(in-memory は物理分離しない。同一ストアを読み書きする)。既存の contract test を Reader / Writer 両観点で満たす
- [x] R6: service 層・route 層から見た振る舞い(戻り値・エラー契約・`errors.Is`/`errors.As` 判定)は不変。既存の contract test / integration test が緑のまま通る

### 非機能要件

- 新規 runtime 依存を増やさない(Postgres ドライバは `pgx` のみのまま)。reader プールも writer と同一の接続プール上限方針(`maxOpenConns` 等)を適用する
- 2 プール構築時、reader が writer にフォールバックするケースでは **接続を二重に開かない**(同一 `*sql.DB` を共有する)。別ホストのときのみ 2 本目のプールを開く
- 接続情報(reader 分含む)を discrete な `DB_READER_*` 環境変数で扱い、コード・tfvars に平文で書かない(既存 R6/SPEC-005 の方針を踏襲)
- ISSUE-016 の最小権限ロール(`api_app` / `auth_app`)と整合させる。reader が別資格情報を使う場合も既存のロール設計を壊さない(reader 資格情報の権限設計は iac スコープに属するため本 Spec では env 契約のみ用意)

### スコープ外(やらないこと)

- **iac(app/iac)の RDS read replica の実プロビジョニング**。本 Spec はアプリ側のコード・env 契約のみ。read replica リソース追加・reader endpoint の service への配線は別 Spec に切り出す(`DB_READER_HOST` を注入すれば有効化できる形にとどめる)
- **full CQRS**(書き込みモデルと非正規化した read model の分離・投影・結果整合性の同期機構)。今回は同一テーブルへの読み/書きを 2 プールで振り分けるのみ
- read replica のレプリケーション遅延(replica lag)に対するアプリ側の強整合対処(read-after-write を writer へ回す等)。設計で seam と既知リスクを明示するに留める(§4・§リスク)
- web / migrator の変更(migrator は writer のみで従来どおり)

## 4. 設計(どう実現するか)

### 方針

CQRS の command/query 分離を「ドメインのポート分割」と「infra の 2 プール」の 2 層で実現する。物理分離(read replica)は将来の env 注入で有効化できる seam を用意するだけで、本 Spec ではリソースを作らない。

- **domain**: `domain/<集約>/repository.go` の単一 `Repository` を `Reader`(query 系メソッド)と `Writer`(command 系メソッド)に分割する。後方互換のため、必要なら両方を埋め込んだ合成 interface(`Repository = interface { Reader; Writer }`)を残し、既存の配線を段階移行できるようにする(planner が判断)
- **service**: 各 service は必要な側だけを依存として受け取る(読むだけの query service は `Reader`、状態変更する command service は `Writer`)。振る舞いは不変
- **infra/postgres**:
  - `db.go` に writer / reader 両方の `Config` を扱う仕組みを追加。`OpenPair(ctx, writerCfg, readerCfg)` 相当で 2 プールを開く。reader cfg が writer と同値(= フォールバック)なら 1 本の `*sql.DB` を共有し二重に開かない
  - Reader 実装(例: `task_reader.go` の `TaskReader`)は reader プールの `*sqlcgen.Queries`、Writer 実装(`task_writer.go` の `TaskWriter`)は writer プールの `*sqlcgen.Queries` を保持。sqlcgen は現状のまま(生成物は read/write で共通)
  - `infra/memory` は 1 構造体が Reader / Writer 両 interface を満たす(同一ストア)
- **cmd/<bin>/env.go**: writer 用 `DB_*` に加え reader 用 `DB_READER_*` を読み、未設定項目は writer 値で補完して reader `Config` を組み立て、`OpenPair` に渡す。`SelectMode` は writer の `DB_HOST` を従来どおり見る(reader の有無は選択に影響しない)
- **rules**: `.claude/rules/db.md` を Reader/Writer 分割・2 プール・`DB_READER_*` フォールバック契約で両スタック対応に更新する

### アーキテクチャ / データ / インターフェース

```
domain/task/repository.go
  Reader interface { FindByID; FindByTitle; ListPage }   // query
  Writer interface { Save }                              // command
  Repository interface { Reader; Writer }                // 互換合成(任意)

infra/postgres/
  db.go        OpenPair(ctx, writerCfg, readerCfg) (writer, reader *sql.DB, err error)
               - readerCfg == writerCfg なら reader = writer(同一プール共有)
  task_reader.go  TaskReader{ q: sqlcgen.New(readerDB) }  // task.Reader を満たす
  task_writer.go  TaskWriter{ q: sqlcgen.New(writerDB) }  // task.Writer を満たす

cmd/api/env.go
  DB_HOST/PORT/NAME/USER/PASSWORD/SSLMODE          -> writer Config
  DB_READER_HOST/PORT/NAME/USER/PASSWORD/SSLMODE   -> reader Config
    (各項目 未設定 -> writer の対応値へフォールバック)
```

- **env フォールバック規則**: reader の各 `DB_READER_X` が空文字なら writer の `DB_X` を採用。結果として reader cfg が writer cfg と完全一致するなら `OpenPair` は単一プールを共有する(接続二重化を避ける R 非機能)
- **auth の correctness-sensitive read**: authcode の消費(発行直後の引き換え)や refresh token の rotation は「書いた直後に読む」フローで、replica lag があると異常(未検出・二重使用)を起こしうる。今回は reader がデフォルトで writer にフォールバックするため実害は出ないが、**別ホストの reader を将来足したときに壊れないよう、これら correctness-critical な lookup をどちら側(Reader/Writer)に置くかを planner が明示し、レビュー(review-spec / review-security)で確認する**。安全側の既定は「単一利用トークンの検証・消費に付随する読みは writer 側に置く(または Writer interface にまとめる)」

### 検討した代替案と不採用理由

| 案 | 不採用理由 |
|---|---|
| interface のみ分割(DB 接続は単一のまま) | infra 層で読み系を別エンドポイントへ流す受け皿ができず、「読み取りスケール」という中心価値が実現しない(ユーザーがこのレベルを不採用と選択) |
| full CQRS(read model / 投影 / 結果整合性の同期機構) | 本サンプルの規模に対し過剰。投影更新と結果整合性の複雑さ(同期遅延・再構築)を持ち込む価値がない |
| 単一プールで read-only トランザクション(`BeginTx` read-only)を使う | 同一接続内の指定に過ぎず、別ホスト(read replica)へ物理的に振り分けられない。目的(エンドポイント分離)を満たさない |
| iac に read replica も同時に追加 | スコープが肥大化し、レプリケーション遅延の運用・コスト判断を伴う。アプリ側の seam を先に用意し、replica プロビジョニングは別 Spec で独立に扱う方が変更が小さく安全(ユーザー選択) |

## 5. 実装計画

詳細は **`docs/plans/SPEC-010-plan.md`**(planner 作成)が正。確定した設計判断の要点:

- **domain**: 書き込みを持つ集約(api `task`、auth `authcode` / `refreshtoken`)にのみ `Reader` / `Writer` / 合成 `Repository interface { Reader; Writer }` を additive 追加(メソッド増減なし → 既存 contract test は無改変で緑)。read-only 集約(auth `user` / `client`)は command を持たないため **Reader-only** で据え置く(空 Writer を作らない。R1 解釈として review-spec で確認)
- **infra**: `OpenPair(ctx, writerCfg, readerCfg)` で 2 プールを開く。`readerCfg == writerCfg` なら reader を開かず writer の `*sql.DB` を共有(**二重に開かない**)。api `task` のみ `TaskReader`(readerDB)/ `TaskWriter`(writerDB)へ構造体分割し、互換合成 `NewTaskRepository(db)` を残す。auth の authcode/refreshtoken は単一 `*sql.DB`(writer)のまま
- **プール振り分け(配線で決定)**: api=task の reads→reader・`Save`→writer / auth=`user`・`client` reads→reader、`authcode`・`refreshtoken` は read も write も **writer 固定**(単一利用トークンの read-after-write を replica lag から守る安全側既定。正しさの権威は writer 上の atomic `Consume` / `Rotate`)
- **env**: `cmd/<bin>/env.go` に `DB_READER_{HOST,PORT,NAME,USER,PASSWORD,SSLMODE}` を追加、各項目未設定は writer 値へフォールバック。`SelectMode` は writer `DB_HOST` 基準で不変

確定タスク(担当 agent):

- [x] T1: 実装計画の作成(planner) — 完了(`docs/plans/SPEC-010-plan.md`)
- [x] T2: domain ポート分割 — impl-api(`domain/task`)∥ impl-auth(`domain/{authcode,refreshtoken}`;`user`/`client` は doc のみ)
- [x] T3: infra/postgres 2 プール化 + Reader/Writer 実装分割(impl-db、api・auth 横断:`OpenPair` / api の `task_reader.go`・`task_writer.go`・合成 `task_repository.go`)
- [x] T4: `infra/memory` を Reader/Writer 両 interface 対応(compile-time `var _` 表明)— impl-api / impl-auth
- [x] T5: `cmd/<bin>/env.go` + `main.go` 配線(`DB_READER_*` フォールバック・`OpenPair`・プール振り分け)— impl-api / impl-auth
- [x] T6: `.claude/rules/db.md` 更新(impl-db)— env 契約・`OpenPair`・Reader/Writer seam・振り分け
- [x] T7: テスト(tester、TDD 先行)— env fallback(unit)、`OpenPair` 共有/別ホスト振り分け(integration・プール close で可視化)、既存 contract 緑維持
- [x] T8: checker → review-security / review-performance / review-spec — 完了(下記経緯)

## 6. 経緯(時系列・追記のみ)

### 2026-07-10

- 初版作成。「DB infra 層を CQRS で read/write 分離するアーキテクチャに変える」というユーザー要望を起点に、スコープを対話で確定:
  - 分離レベル = **接続プールも分離**(interface の Reader/Writer 分割 + writer/reader の 2 プール、reader 未設定時は writer にフォールバック)
  - 対象 = **app/api + app/auth 両方**
  - iac(RDS read replica の実プロビジョニング)= **今回スコープ外**(`DB_READER_HOST` 注入で有効化できる seam のみ用意し、replica 追加は別 Spec)
- auth の単一利用トークン(authcode 消費・refresh token rotation)の read-after-write が replica lag で壊れうる点を既知リスクとして明記。既定フォールバックにより現時点で実害はないが、別ホスト reader 導入時に備え、correctness-critical read の Reader/Writer 配置を planner が明示しレビューで確認することとした。
- ユーザーがスコープと初版内容を承認。status を `approved` に更新し、planner に実装計画(`docs/plans/SPEC-010-plan.md`)の作成を委譲する。
- planner が `docs/plans/SPEC-010-plan.md` を作成。確定した設計判断: (1) 書き込みを持つ集約のみ Reader/Writer/合成 Repository を additive 追加(既存 contract test 無改変で緑)、read-only の `user`/`client` は Reader-only 据え置き、(2) `OpenPair` で 2 プール・reader==writer 時は単一プール共有、(3) api task のみ infra 構造体を `TaskReader`/`TaskWriter` に分割・auth は AuthorizationService が既に集約別リポジトリを保持するため配線で振り分け(service 署名不変)、(4) auth の `authcode`/`refreshtoken` は read も writer 固定(replica lag 対策の安全側既定)。planner が挙げたユーザー判断点(`user`/`client` の Reader-only 解釈、将来 replica 時の api read-modify-write)は admin 裁量で採用・申し送りとし、いずれも review-spec で確認する。status を `in-progress` に更新し、TDD パイプライン(tester → impl-api ∥ impl-auth ∥ impl-db → tester → checker → review-*)に着手する。

- **実装完了・全フェーズ通過(status → done)**。
  - **実装**: tester がフェーズ1で赤テストを先行作成(env fallback / Reader/Writer compile-time / service 振り分け / `OpenPair` 共有・別ホスト振り分けの integration)し、固定シグネチャ(`OpenPair(...) (writer, reader *sql.DB, closeFn func() error, err error)` / `NewTaskReader` / `NewTaskWriter` / `NewTaskService(reader, writer, dupChk)` / `Env.DBReader` + `writerConfig()` / `readerConfig()`)を確定。impl-api ∥ impl-auth ∥ impl-db が並列で実装(差分はスコープどおり・越境編集なし)。
  - **検証(価値の検証方法の充足)**: tester が実 Postgres で `-tags=integration` を実行し、(a) 等価 cfg → `writer==reader`(単一プール共有=二重に開かない)、(b) 別ホスト cfg → 別プール、(c) プール close 可視化で読み=reader・書き=writer の振り分け、(d) auth の authcode `FindByCode`/`Consume` が reader プール close の影響を受けない(writer 固定)、(e) 既存 contract/integration が無改変で緑、をすべて PASS で確認。checker が両 stack `make check`(fmt-check + lint + vet + build + test)green・sqlc drift ゼロを確認。→ 「`DB_READER_HOST` を別ホストにすると読み系が reader・書き系が writer に流れ、未設定時は既存テスト緑」という価値の検証条件を満たしたため done とする。
  - **レビュー(Blocker/Major 0)**: security(0/0/0/Info3)・performance(0/0/Minor1/Info2)・spec(R1〜R6 充足・user/client の Reader-only に合意)。差し戻しなし。
  - **将来の read replica 実配線 Spec への申し送り**(いずれも本 Spec のスコープ外・別 Spec で再検討): (i) reader を「同一インスタンス・別資格情報(reader 専用低権限ロール; ISSUE-016 の文脈)」で使う構成では `Config` 全項目一致でないため 2 本目のプールを開き、共有 RDS の接続予算が倍化する(`database/sql` 仕様上不可避)。iac 側で総接続数 ≤ `max_connections` を検証する仕組み、または当該構成での `maxOpenConns` 明示縮小を検討(perf Minor-1)。(ii) 別ホスト reader 時、`OpenPair` は writer→reader を逐次 `Open`(各 ping 5s)するため起動レイテンシが最大 2 倍。並列 `Open` を検討(perf Info-1)。(iii) 将来 client 失効・user 更新の runtime 書き込み機能を追加する際は、`user`/`client` の read を reader(replica)に固定したままだと失効直後の stale read で認可上のリスクが生じうるため、その時点で reader 固定を再評価(security Info)。(iv) api の read-modify-write(`FindByID`→`Save`)を replica 導入時に writer へ寄せるか(R-1)。
  - **付随発見**: tester が SPEC-010 と無関係の既存バグ(`app/auth/Makefile` の `test-integration` が `docker compose run` の `-e` 引数順違反で exec error。SPEC-009 由来・api と非対称)を発見。**ISSUE-026**(severity: medium)として起票済み。CI の `auth-integration` ジョブも同経路で失敗している可能性があり、修正(impl-auth)は本 Spec と切り離して別途対応。
  - コミットは未実施(ユーザー指示があれば行う)。
