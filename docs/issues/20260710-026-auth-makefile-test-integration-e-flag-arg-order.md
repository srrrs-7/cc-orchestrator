---
id: ISSUE-026
title: app/auth/Makefile の test-integration が docker compose run の -e フラグ引数順違反で exec error になる(SPEC-009 由来・api と非対称)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: medium  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: [SPEC-009]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-026: app/auth/Makefile の test-integration が docker compose run の -e フラグ引数順違反で exec error になる(SPEC-009 由来・api と非対称)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth を開発・検証する人 / CI / subagent(tester・checker)** の **「auth の実 DB 統合テストを規約どおりのコマンドで実行できる」価値** が **`make test-integration` が起動直後に exec error で必ず失敗するため損なわれている**。

- **影響を受けるユーザー**: `cd app/auth && make test-integration` を実行する開発者・subagent(tester)、および CI の `auth-integration` ジョブ(`.github/workflows/cicd.yml`)
- **損なわれる価値**: `.claude/rules/db.md`・`.claude/rules/auth.md` が定める「auth の実 DB 統合テストの正式コマンド = `make test-integration`」という契約が壊れている。Postgres が到達可能でもテストに到達する前に失敗する
- **影響範囲・頻度**: 常時(postgres の状態に関わらず、`make test-integration` を叩けば必ず再現)。ただし go test 実体(`go test -tags=integration ./infra/postgres/...`)自体は正常で、直接叩けば PASS する(SPEC-010 の tester が同等コマンドを直接実行し全 PASS を確認済み)。あくまで `make` ラッパー経由の呼び出し経路の不具合
- **回避策**: あり。toolbox コンテナ内で `go test -tags=integration ./infra/postgres/...` を直接実行する / api 側と同じ正しい引数順で `docker compose run` を手組みする。ただし CI の `auth-integration` ジョブは `make test-integration` 固定で呼ぶため、CI 側にはこの回避策が効かない(下記「3. 原因 → 調査ログ」参照)

## 2. 現象(何が起きているか)

### 期待する動作

`cd app/auth && make test-integration` が toolbox コンテナ内で `go test -tags=integration ./infra/postgres/...` を実行し、事前に auth データベースへマイグレーション適用済みの Postgres に対して統合テストが走る(api 側の `make test-integration` と対称に動く)。

### 実際の動作

`make test-integration` が統合テストに到達する前に、コンテナ起動直後の exec で失敗する:

```
exec: "-e": executable file not found in $PATH
```

`-e`(本来は `docker compose run` の環境変数フラグ)が、コンテナ内で実行するコマンドの argv[0] として誤って exec されている。

### 再現手順

1. `cd app/auth`
2. `make test-integration` を実行する(Postgres の起動有無に関わらず再現する。exec error は DB 接続より前に起きる)
3. `exec: "-e": executable file not found in $PATH` で失敗することを確認する

補助的な確認(docker を起動せず、展開後のコマンド文字列だけを見る):

1. `cd app/auth`
2. `make -n test-integration`(dry-run。レシピを実行せず表示)を実行する
3. 展開結果が `... docker compose -f <root>/compose.yml -f <root>/compose.tools.yml run --rm --workdir /workspace/app/auth tools -e DB_HOST=postgres -e ... -e DB_SSLMODE=disable make test-integration-native` となり、**サービス名 `tools` の直後に `-e ...` が並んでいる**(= `-e ...` 以降がすべて「コンテナ内で実行するコマンド」に回る)ことを確認する

### 環境・条件

- 対象ファイル: `app/auth/Makefile`(host ブランチ、`IN_TOOLBOX` 未設定側の `test-integration` ターゲット)
- SPEC-009 Phase B(transparent toolbox wrappers)以降。`git log` 上の混入コミット候補は `999eed0`(SPEC-009 Phase A)/ `f627d39`(SPEC-009 Phase B)
- 実行形態: `docker compose run [OPTIONS] SERVICE [COMMAND...]`(OPTIONS は SERVICE より前でなければならない)

## 3. 原因(なぜ起きているか)

### 調査ログ

事実:

- `app/auth/Makefile` L85 で `DB_ONLINE` 変数が **既にサービス名 `tools` を末尾に含んだ形**で定義されている:
  `DB_ONLINE := $(TOOLBOX_ENV) $(DB_COMPOSE) run --rm $(WORKDIR) tools`
- `app/auth/Makefile` L232-237 の `test-integration` ターゲットは、その `$(DB_ONLINE)` の**後ろに** `-e DB_HOST=... ... -e DB_SSLMODE=... make test-integration-native` を付け足している。
- 展開後の実コマンドは
  `... run --rm --workdir /workspace/app/auth tools -e DB_HOST=... ... -e DB_SSLMODE=disable make test-integration-native`
  となり、`docker compose run` は SERVICE(`tools`)より後ろをすべて「コンテナ内 COMMAND」として解釈する。よって COMMAND の argv[0] が `-e` になり、`exec: "-e": executable file not found in $PATH` で落ちる(`docker compose run [OPTIONS] SERVICE [COMMAND...]` の引数順違反)。
- 対比: `app/api/Makefile` は同じ役割の変数 `DB_ONLINE`(L88)に **`tools` を含めず**、`test-integration` ターゲット(L254-259)側で `-e DB_* ... tools make test-integration-native` の順、すなわち **`-e` 群 → SERVICE(`tools`)→ COMMAND(`make test-integration-native`)** の正しい順に組み立てている。api 側は正常。api/auth で同名ターゲットの組み立て方が非対称になっている(api の該当コメント L82-87 は「`-e` は SERVICE より前でなければならない。SERVICE 名/コマンドは付けず、test-integration 側が `-e ...` の後に `tools` 自身を付ける」と明記しており、auth はこの意図に反している)。
- CI: `.github/workflows/cicd.yml` の `auth-integration` ジョブ(L480-506)の最終ステップ(L504-506)は `working-directory: app/auth` で `make test-integration` を実行する。**壊れているコマンドをそのまま呼ぶ経路**であり、このジョブが実行される条件(`needs.changes.outputs.auth == 'true' || needs.changes.outputs.migrator == 'true'`、L483)を満たす PR ではジョブが到達次第 fail するはず。

仮説:

- 仮説: 上記 CI の `auth-integration` ジョブは、条件を満たして実際に走った時点でこの exec error により failed になっている(要確認: 実際の CI 実行ログでの確認は本 Issue の範囲外・別途)。同ジョブは `app/api` の `api-integration`(正常)と対称構造のため、api 側 green・auth 側だけ red という兆候が観測できるはず。

### 根本原因

`app/auth/Makefile` の `DB_ONLINE` 変数が SERVICE 名(`tools`)まで内包している一方、`test-integration` ターゲットがその後ろに `-e` フラグ群を追記しているため、`docker compose run` のオプション(`-e`)が SERVICE より後ろに来て「コンテナ内で実行するコマンド」に紛れ込む。SPEC-009 で api/auth の Makefile を toolbox ラッパー化した際、この 1 ターゲットだけ api の組み立て順(`-e ... tools COMMAND`)に揃えられず非対称に取り込まれたのが原因。

## 4. 対応(どう解決するか)

### 対応方針

`app/auth/Makefile` の `test-integration` 経路を `app/api/Makefile` と同一の引数順(`-e ...` 群 → `tools` → `make test-integration-native`)に揃える。具体的な取り方はいずれか(api と同型になるものを採る):

- `DB_ONLINE` 定義から末尾の `tools` を外し(api の L88 と同じ形にする)、`test-integration` ターゲット側で `-e ...` 群の後に `tools make test-integration-native` を置く(= api の L254-259 と一致させる)。

**担当は impl-auth**(`app/auth/Makefile` の中身は auth stack 所有)。CI(`.github/workflows/cicd.yml` の `auth-integration`)が実際に fail していないかの確認は別途(impl-ci / admin の判断)。**本 Issue は起票のみで、Makefile・コードの修正は行っていない。**

### 実施内容

- [ ] `app/auth/Makefile` の `test-integration`(および必要なら `DB_ONLINE`)を api と同一の引数順に修正(impl-auth)
- [ ] 修正後 `cd app/auth && make test-integration` が exec error を出さずに統合テストへ到達することを確認(tester)
- [ ] CI `auth-integration` ジョブが実際に fail していたかの確認と、修正後の green 化確認(別途)

### 再発防止

- api/auth の対応する Makefile ターゲットは 1:1 対称であるべき(SPEC-009 の設計意図)なので、`test-integration` の組み立て方の差分をレビュー観点として明記する。
- CI の `auth-integration` ジョブは本来この不具合を検出できる位置にある(`make test-integration` を実行する)。ジョブが確実に走る条件・可視化を確認し、同種のラッパー引数順バグを CI で拾える状態を保つ。

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。SPEC-010(DB CQRS read/write 分離)の作業中、tester がフェーズ 3(実 DB 統合テスト実行)で `cd app/auth && make test-integration` を叩いた際に `exec: "-e": executable file not found in $PATH` を観測。SPEC-010 の変更差分とは無関係の既存バグと判断し切り出した。
- 調査: `app/auth/Makefile`(L85 の `DB_ONLINE` が `tools` を内包、L232-237 の `test-integration` がその後ろに `-e ...` を追記)と `app/api/Makefile`(L88 の `DB_ONLINE` は `tools` 非内包、L254-259 で `-e ... tools COMMAND` の正しい順)を突き合わせ、`docker compose run [OPTIONS] SERVICE [COMMAND...]` の引数順違反で `-e` が in-container コマンドの argv[0] に回ることを根本原因として特定(事実)。api 側は正常で、api/auth が非対称。
- 影響確認: `.github/workflows/cicd.yml` の `auth-integration` ジョブ(L504-506)が同じ `make test-integration` を呼ぶ経路であることを確認。CI も同因で fail している可能性が高い(仮説、実 CI ログでの確認は別途)。go test 実体は正常で、tester は同等コマンドを直接実行し全 PASS を確認済みのため SPEC-010 の要件検証には支障なし。
- 由来は SPEC-009(containerized toolchain。Makefile の toolbox ラッパー化)と推定し、frontmatter で SPEC-009 と相互リンク。修正は未実施(本 Issue は起票のみ)。次アクション: impl-auth が Makefile を api と同一引数順に修正 → tester 再実行 → CI 確認。
