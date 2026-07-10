---
id: ISSUE-019
title: SPEC-006 refresh_token グラントの deferred hardening(consumed 行の定期 GC 欠如 / 再利用検知パスのタイミングサイドチャネル)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-10
updated: 2026-07-10
specs: [SPEC-006]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-019: SPEC-006 refresh_token グラントの deferred hardening(consumed 行の定期 GC 欠如 / 再利用検知パスのタイミングサイドチャネル)

SPEC-006(app/auth の refresh_token グラント)実装レビューで確認された、**実害は現状小さいが将来強化すべき** 2 課題を 1 件にまとめる。いずれも Blocker / Major ではなく、SPEC-006 のスコープ外として **意図的に見送った** deferred hardening。現状のサンプル / デモ規模では実害なし。

- 課題 1(パフォーマンス / ストレージ肥大): `refresh_tokens` の consumed 行に対する定期 GC(bulk purge)が無く、正常なローテーションのたびに孤立 consumed 行が恒久残存する
- 課題 2(セキュリティ / 軽微): 再利用検知パスが consumed トークンのときのみ追加 DB 操作(family 失効)を行うため、「未知トークン」と「実在の consumed トークン」で応答時間差が生じ得る(タイミングオラクル)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth(認可サーバー)を Postgres モードで恒久運用する運用者** の **永続化基盤の運用安定性・コスト予測性**(課題 1)と、**盗用トークンによる攻撃に対する副次情報の非開示**(課題 2)が、**将来の強化余地として損なわれ得る**。現状のサンプル / デモ規模では実害なし。

- **影響を受けるユーザー**: app/auth を Postgres モードで長期運用し、RDS のストレージ・autovacuum を管理する運用者(課題 1)。タイミング解析を試みる攻撃者に対峙する運用者(課題 2)
- **損なわれる価値**:
  - 課題 1: `refresh_tokens` テーブルサイズがローテーショントラフィックに対して有界に保たれない(ストレージ・インデックス・autovacuum コストの単調増加)
  - 課題 2: 「未知トークン」と「実在の consumed トークン」の応答時間差という副次情報を攻撃者に与えない性質
- **影響範囲・頻度**:
  - 課題 1: 累積的・単調増加。正常なローテーション 1 回ごとに「二度と evict されない孤立 consumed 行」が 1 行ずつ恒久的に残る
  - 課題 2: 特定条件下でごく限定的(既に盗んだトークン、または非現実的な総当たりが前提)
- **回避策**:
  - 課題 1: あり(運用者が手動 / 定期に `DELETE FROM refresh_tokens WHERE expires_at <= now()` 等を実行できる。恒久的な自動 GC は未実装)
  - 課題 2: なし(設計変更を要する)。ただし悪用価値が低く、実運用上の必要性は低い

## 2. 現象(何が起きているか)

### 期待する動作

- 課題 1: expired / 古い consumed の `refresh_tokens` 行が、有限のバックログを超えて蓄積しないよう定期回収され、テーブルサイズがローテーショントラフィックに対して有界に保たれる。
- 課題 2: 再利用検知パスの応答時間が、提示トークンの存在有無・consumed 状態に依存して観測可能な差を生じない。

### 実際の動作

- 課題 1: expired 行の削除は、当該 `token_hash` が再度 `FindByTokenHash` / `Rotate` で引かれたときのみ発火する **lazy eviction のみ**(`app/auth/infra/postgres/refreshtoken_repository.go:62-88` の `DeleteExpiredRefreshToken`、`app/auth/db/queries/refresh_tokens.sql:35-43`)。ローテーション済みの旧 hash は正規フローでは二度と提示されないため、正常なローテーション 1 回ごとに孤立 consumed 行が恒久残存する。テーブル全体を対象とした定期 bulk purge は存在しない。
- 課題 2: `refreshTokenGrant` は、提示トークンが consumed(実在・ローテ済み)のときのみ `RevokeFamily`(追加の DB / ロック操作)を実行してから `ErrReused`(→ `invalid_grant`)を返す(`app/auth/service/authorization_service.go:362-367`)。未知トークンは手前の `FindByTokenHash` が `ErrNotFound` で早期リターンする(`:353-356`)。よって「未知トークン」と「実在の consumed トークン」で応答時間に差が生じ得る。

### 再現手順

課題 1(consumed 行の恒久残存):

1. app/auth を Postgres モードで起動する(`DB_HOST` 等を設定)。
2. 認可コード交換で refresh token を発行させる(`Save` が `refresh_tokens` に 1 行 INSERT。`refreshtoken_repository.go:47-52`)。
3. `POST /token`(`grant_type=refresh_token`)で正常にローテーションする。`ConsumeRefreshToken` が旧行を `consumed = true` に **UPDATE**(削除しない。`refresh_tokens.sql:45-64`)し、新行を INSERT する。
4. `SELECT count(*) FROM refresh_tokens WHERE consumed = true` で旧行が残存していることを確認する。旧 `token_hash` は正規フローでは二度と提示されないため、`DeleteExpiredRefreshToken`(`refreshtoken_repository.go:72`)は起動せず、`expires_at` 経過後も恒久的に残る。
5. ローテーションを繰り返すたびに consumed 行が単調増加し、上限も定期リーパーも無いことを確認する。

課題 2(タイミングサイドチャネル):

1. app/auth を起動する。
2. 実在するがローテ済み(consumed)の refresh token を `POST /token`(`grant_type=refresh_token`)に提示し、応答時間を計測する(`RevokeFamily` の実行を含む経路)。
3. ランダムな未知 refresh token を提示し、応答時間を計測する(`FindByTokenHash` が即 `ErrNotFound` で返る経路)。
4. 2 と 3 の応答時間差(統計的な差)を確認する。※ 悪用には既に盗んだトークン、または 256bit 乱数の非現実的な総当たりが前提のため、再現できても実害評価は「低」。

### 環境・条件

- 対象: SPEC-006 の app/auth refresh_token グラント + `infra/postgres` 永続化。
- `infra/memory` 実装: 課題 1 はプロセス再起動でマップがリセットされ自然回収されるため恒久蓄積しない(Postgres 化で失われる劣化パス。ISSUE-015(authorization_codes)と同型)。課題 2 のタイミング差は memory / postgres 共通(`service` 層のロジックに起因)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実(課題 1): `ConsumeRefreshToken` は `UPDATE ... SET consumed = true ... RETURNING`(削除ではない。`db/queries/refresh_tokens.sql:45-64`、migration `db/migrations/000002_create_refresh_tokens.sql:22-28` の `consumed` 列コメント)。consumed-but-unexpired 行を残すのは再利用検知(`FindByTokenHash` が consumed 行も返す。`refresh_tokens.sql:19-33`)の前提として **意図的**。
- 事実(課題 1): expired 行の削除は `FindByTokenHash` 内の lazy eviction のみ(`refreshtoken_repository.go:62-88`)。`DeleteExpiredRefreshToken` は `WHERE token_hash = $1 AND expires_at <= now()` の単一 hash 対象(`refresh_tokens.sql:35-43`)。テーブル全体の bulk purge / cron / scheduled は存在しない。
- 事実(課題 1): `RevokeFamily` は再利用検知の発火時に `DELETE ... WHERE family_id = $1`(`refresh_tokens.sql:66-74`)で当該 family の全行を削除するが、これは検知が起きた family のみ。正常にローテートされ続ける family の旧 consumed 行は削除経路に乗らない。
- 事実(課題 2): `authorization_service.go:362-367` で提示トークンが `Consumed()` のときのみ `RevokeFamily`(追加 DB 操作)を実行してから `ErrReused` を返す。未知トークンは `:353-356` の `FindByTokenHash` が `ErrNotFound` で早期リターンする。処理経路の差が応答時間差の原因。
- 見積り(課題 1): 行数は概ね O(ユーザー数 × 平均リフレッシュ回数)で単調増加。上限も定期リーパーも無い。サンプル / デモ規模では実害なし。実測は未実施(未調査)。
- 評価(課題 2): refresh token は 256bit 乱数(`migrations/000002_create_refresh_tokens.sql:8-12`、SPEC-006 R8)。この差分を悪用するには既に盗んだトークンか非現実的な総当たりが前提で、実質的な悪用価値は低い。

### 根本原因

- 課題 1: SPEC-006 は単回使用ローテーション + 再利用検知(consumed 行を残して replay を検知する)のセマンティクスを SQL で正しく表現することに焦点を当て、「有効でなくなった consumed / expired 行の回収」は既存の lazy eviction 契約(`infra/memory` と同格)をそのまま踏襲した。memory では再起動で暗黙回収されていたが、永続化により回収されない行が恒久残存するようになった。**テーブル全体を定期クリーンアップする実行主体(スケジュールタスク等)が存在しない** ことが根本原因(ISSUE-015 = authorization_codes と同型の欠落)。
- 課題 2: 再利用検知(`RevokeFamily`)を検知時のみ同期実行する素直な実装が、応答時間を提示トークンの状態(存在有無・consumed)に依存させている。応答時間を定時間化 / 非同期化していないことが原因。

## 4. 対応(どう解決するか)

### 対応方針

- **今回のスコープ(SPEC-006 = refresh_token グラント実装)では対応しない。** Blocker / Major ではなく、レビューで意図的にスコープ外として見送った deferred hardening。
- 課題 1: `expires_at` を対象にしたテーブル全体の定期 bulk purge(例: `DELETE FROM refresh_tokens WHERE expires_at <= now()`)、または `consumed = true AND created_at < now() - retention` の保持期間ベースの定期削除ジョブ(cron / pg_cron / ECS scheduled task / アプリ内の終了条件付きティッカー)。ISSUE-015(authorization_codes の無制限増加)と同じ実行主体で両テーブルをまとめて回収する設計が合理的。SPEC-006 plan(`docs/plans/SPEC-006-plan.md` §リスク)が言及する「期限切れ consumed トークンの再利用検知漏れ(30 日窓)」も、consumed 行の保持期間設計と併せて検討するとよい。
- 課題 2: `RevokeFamily` の有無に関わらず応答時間を均す(最小処理時間の設定)か、family 失効を非同期化する。
- 参照: `app/auth/infra/postgres/refreshtoken_repository.go:62-88`(lazy eviction のみの `FindByTokenHash`)、`app/auth/db/queries/refresh_tokens.sql:35-74`、`app/auth/service/authorization_service.go:362-367`、`docs/plans/SPEC-006-plan.md` §リスク、関連: ISSUE-015。

### 実施内容(将来対応時のチェックリスト)

- [ ] (課題 1) `expires_at` 全件、または保持期間ベースの bulk purge クエリを `db/queries` に追加し sqlc 再生成(impl-db)
- [ ] (課題 1) 周期実行の実行主体を決めて実装する(スケジュール ECS タスク / pg_cron / アプリ内ティッカー。ISSUE-015 との共通化を検討。iac を伴う場合は impl-iac)
- [ ] (課題 1) `refresh_tokens(expires_at)` にインデックスを付与するか、bulk purge のコストを評価する
- [ ] (課題 1) 30 日窓の再利用検知漏れ(SPEC-006 plan §リスク)と consumed 行の保持期間を併せて設計する
- [ ] (課題 2) 再利用検知パスを定時間化するか family 失効を非同期化する(impl-auth)
- [ ] (課題 2) タイミング差が観測可能でないことをテストで確認する

### 再発防止

- 永続化する短命 / 単回使用エンティティ(TTL・consumed を持つ行)は、lazy eviction だけでなく「正規フローで二度と提示されない行を回収する定期 purge」をセットで設計する、を `.claude/rules/db.md` の永続化設計チェックに加えることを検討する(ISSUE-015 と共通の学び)。
- 認証基盤の分岐処理は、成功 / 失敗・存在 / 非存在で観測可能な副作用(応答時間・追加 I/O)の差を持たせない、という設計チェックを review-security の観点に加えることを検討する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-10

- 起票。SPEC-006(app/auth の refresh_token グラント)の実装レビューで「今回は対応せず将来強化」と判断された deferred hardening 2 件を 1 Issue にまとめて記録。Blocker / Major ではなく、SPEC-006 のスコープ外として意図的に見送ったもの。
- 事実確認(課題 1): consumed 行は再利用検知のため即削除せず `consumed = true` に UPDATE する(`db/queries/refresh_tokens.sql:45-64`、`db/migrations/000002_create_refresh_tokens.sql:22-28`)。expired 削除は `FindByTokenHash` 内 lazy eviction のみ(`infra/postgres/refreshtoken_repository.go:62-88`、単一 hash 対象の `DeleteExpiredRefreshToken` = `refresh_tokens.sql:35-43`)。ローテ済み旧 hash は正規フローで二度と提示されないため、正常ローテーション 1 回ごとに孤立 consumed 行が恒久残存。テーブル全体の bulk purge / cron / scheduled は存在しない。行数は概ね O(ユーザー数 × 平均リフレッシュ回数)で単調増加(上限・定期リーパーなし。実測は未実施 = 未調査)。
- 事実確認(課題 2): `service/authorization_service.go:362-367` は提示トークンが consumed のときのみ `RevokeFamily`(追加 DB 操作)を実行してから `ErrReused`(→ `invalid_grant`)を返す。未知トークンは `:353-356` の `FindByTokenHash` が `ErrNotFound` で早期リターンするため、両者で応答時間差が生じ得る。refresh token は 256bit 乱数(`migrations/000002_create_refresh_tokens.sql:8-12`、R8)ゆえ悪用には盗んだトークン or 非現実的総当たりが前提で、悪用価値は低い。
- severity は **low** と判定。判定根拠: 両課題ともレビューで Minor と評価され、現状のサンプル / デモ規模では実害なし・機能は失われず、課題 1 は手動 / 定期 `DELETE ... WHERE expires_at <= now()` の回避策があり、課題 2 は悪用価値が低く回避策不要。ただし課題 1 は ISSUE-015(authorization_codes の無制限増加。review-performance が Major、severity は medium)と同型で、恒久運用・大規模化では medium への再評価が妥当。今回は「意図的な deferred・現状の実害小」を重視して low とした。
- 相互リンク: 本 Issue frontmatter の `specs` に SPEC-006 を設定。SPEC-006 側 frontmatter の `issues` への ISSUE-019 追記は、Spec 編集担当(admin / spec skill)に依頼が必要(issue-creator は `docs/issues` のみ編集する)。
- 次にやること: 将来 planner が課題 1 の purge 実行主体(ISSUE-015 との共通化を検討)と課題 2 の定時間化 / 非同期化を確定し、impl-db / impl-auth(必要なら impl-iac)が実装、tester / checker / review を通す。

### 2026-07-10(追記: 課題 3 = refresh_token グラントでユーザー未検出時に invalid_grant でなく汎用 500 を返す)

- プロジェクト全体レビューで、refresh_token グラントの deferred hardening として **課題 3** を追加記録する。**refresh_token グラントでトークンに紐づくユーザーが見つからない場合、OAuth の `invalid_grant`(400)ではなく汎用の 500(`server_error`)を返す。**
- 事実確認: `refreshTokenGrant`(`app/auth/service/authorization_service.go:329`)は、トークン検証後にオーナーを解決する `owner, err := s.users.FindByID(ctx, uid)`(`:387`)のエラーを `return TokenResponse{}, fmt.Errorf("service: refresh token: %w", err)`(`:388-390`)でそのまま伝播する。この `err`(`user.ErrNotFound` を含む)を受ける `route/response.go` の `tokenErrorCode`(`:152`)には **`user.ErrNotFound` の分岐が無い**(分岐は `service.ErrUnsupportedGrantType` / `client.*` / `authcode.*` / `refreshtoken.*` のみ。`:154-174`)。よって `default: return 0, "", ""`(`:175-176`)に落ち、`writeTokenError`(`:184`)が汎用の `server_error`(HTTP 500)を返す(`:190`)。
- 影響評価: **情報漏洩は無い**(500 は内部情報を出さず、`slog` にのみ記録される)。可用性の細部の課題で、正しくは「トークンは有効だがユーザーが存在しない」= `invalid_grant`(400、クライアントに再認証を促す)が適切。現状は該当経路が実質発生しない — ユーザー削除機能が無く、有効な refresh token に紐づくユーザーが消えるケースが生じないため、**現状は到達不能に近いパス**で実害なし。
- 位置づけ: 課題 1 / 2 と同じく **今回のスコープ(SPEC-006)では対応しない deferred 項目**。とくに **将来ユーザー削除機能を実装した時点** で顕在化する(削除済みユーザーの refresh token が残っていると 500 を返す)ため、そのタイミングで `tokenErrorCode` に `user.ErrNotFound → invalid_grant` のマッピングを追加することを検討する。authorizationCodeGrant 側にも同型の `s.users.FindByID`(`:261`)があり、同じマッピング欠如が該当しうるため併せて確認する。
- 対応候補(将来): `route/response.go` の `tokenErrorCode` に `case errors.Is(err, user.ErrNotFound): return http.StatusBadRequest, "invalid_grant", "..."` を追加する(担当: impl-auth。route 層のエラー変換)。テストで refresh_token / authorization_code 両グラントの user 未検出時に 400 `invalid_grant` を返すことを確認する(tester)。ISSUE-018(route エラーカテゴリ型)の対応と交差しうるため、着手時に整合を確認する。
- severity は **low** を維持(情報漏洩なし・現状到達困難・機能は失われない可用性の細部。課題 1 / 2 と同じ deferred 性質)。frontmatter は status=open 維持・updated=2026-07-10。
- 次にやること: 将来ユーザー削除機能を実装する際に、planner が本課題(user 未検出 → invalid_grant マッピング)を計画に含め、impl-auth が `tokenErrorCode` を拡張、tester が両グラントの user 未検出経路を検証する。

### 2026-07-10(修正ラウンド: 課題 3 = ユーザー未検出時の 500 → invalid_grant を解消)

- 今回の修正ラウンドで、上記 2026-07-10 追記の **課題 3(refresh_token / authorization_code グラントでトークンに紐づくユーザーが未検出のとき、`invalid_grant`(400)ではなく汎用 500(`server_error`)を返す)を解消**した。当初は「将来ユーザー削除機能を実装した時点で対応」と deferred していたが、route 層のエラー変換に閉じた低リスクな改善のため本ラウンドで前倒し対応した。
- 実施内容(impl-auth): `app/auth/route/response.go` の `tokenErrorCode` に `user.ErrNotFound → invalid_grant` の分岐を追加。refresh_token / authorization_code は同一のエラー変換経路(`tokenErrorCode`)を通るため、**両グラント共通経路で一括解消**した(2026-07-10 追記で指摘した authorizationCodeGrant 側の同型欠如も同時に解消)。RFC 6749 §5.2 準拠で HTTP 400 + `invalid_grant` を返す。description は情報漏洩防止のため「トークンは有効だがユーザーが存在しない」等の内部状態を露出しない汎用文言とした。
- 検証(tester): 回帰テスト 2 件を追加(`app/auth/route/token_user_not_found_test.go` — refresh_token グラント / authorization_code グラントそれぞれで、ユーザー未検出時に HTTP 400 + `invalid_grant` を返すことを確認)。全 pass。
- 検証(checker): app/auth の `make check`(fmt-check + lint + vet + build + test)green を確認。
- **status は open を維持。** 理由: 本 Issue が元々挙げていた他の deferred 項目が未対応で残るため。残タスクは以下:
  - 課題 1(consumed refresh_token 行の定期 GC 欠如。lazy eviction のみで正常ローテのたびに孤立 consumed 行が恒久残存。ISSUE-015(authorization_codes)と同型)
  - 課題 2(再利用検知パスのタイミングサイドチャネル。consumed 時のみ `RevokeFamily` を同期実行するため応答時間差が生じ得る)
  - severity は **low** を維持(解消したのは到達困難・情報漏洩なしの可用性の細部で、残る課題 1 / 2 の性質も不変)。frontmatter は status=open 維持・updated=2026-07-10。
- 次にやること: 残る課題 1(purge 実行主体を ISSUE-015 と共通化して設計・実装。impl-db / 必要なら impl-iac)と課題 2(定時間化 / 非同期化。impl-auth)を将来 planner が計画化する。これらが解消した時点で本 Issue をクローズ可能。
