# ISSUE-024 実装計画: gosec を Go 3 スタックの lint / CI に恒久組み込みし、既存 gosec 由来指摘を一括ハードニングする

- 起点 Issue: `docs/issues/20260710-024-go-gosec-not-integrated-in-lint-and-ci.md`
- 併せて解消/整理する関連 Issue: ISSUE-021(healthcheck SSRF)/ ISSUE-010(api の `http.Server` タイムアウト未設定)/ ISSUE-004(`/authorize` open-redirect 不変条件)
- 作成日: 2026-07-10 / 作成: planner

---

## 0. スコープと前提(先に読むこと)

本計画は「gosec 有効化」単体ではなく、**gosec を有効化すると `make lint`(CI)が落ちる**という事実を前提に、**それが顕在化させる既存指摘の解消 / 正当な抑制まで含めた一括ハードニング**として設計する。gosec を config に入れた瞬間に 3 スタックの `make lint` が赤くなるため、「各スタック内で 先に指摘を解消 → 最後に gosec を有効化」という順序を厳密に守る(§3 手順)。

調査で確定した前提(§6 リスクに詳述):

- **golangci-lint のバージョンは CI で `1.64.8` に pin 済み**(`.github/workflows/cicd.yml` の `env.GOLANGCI_LINT_VERSION`)。gosec はこの golangci-lint に**バンドル**されているため別バイナリ導入は不要。`.golangci.yml` は **v1 系スキーマ**で書く(v2 スキーマは 1.64.8 で動かない)。
- **既存の `.golangci.yml` は 3 スタックのどこにも無く、`//nolint` / `#nosec` ディレクティブもコードベースに 1 つも無い**(grep 確認済み)。よって新規に追加する抑制はすべて本計画由来で、既存の抑制と衝突しない。
- **Issue に書かれた gosec ルール番号は実際の gosec ID と一致しない可能性が高い。** 標準 gosec では SSRF/可変 URL は **G107**、Slowloris は **G112**、そして **open-redirect に相当するルールは存在しない**(G704 / G710 は標準 gosec の ID ではない)。したがって「番号で決め打ち」せず、**各スタックで gosec のベースラインスキャンを先に走らせ、実際に出た指摘を正典として対応する**(§3 手順の Step 1)。

---

## 1. 方針

### 1.1 有効化方式: golangci-lint の gosec linter を有効化する(gosec 単体導入はしない)

- 既存の `make lint` = `golangci-lint run ./...`(各スタックの Makefile)にそのまま乗る。CI も各 Go job が `make check` → `make lint` を既に実行しているため、**config を置くだけで CI が自動的に gosec を拾う**(新コマンドの発明が不要 = impl-ci の契約に適合)。
- 退けた代替案: **gosec 単体を `make lint` / CI に追加**(`gosec ./...`)。golangci-lint と gosec の 2 経路・2 バージョンを管理することになり、抑制ディレクティブ(`//nolint` vs `#nosec`)も二重化する。既に golangci-lint 基盤がある以上、単一経路に寄せる方が単純で drift も少ないため退けた。

### 1.2 config の配置: **スタックごとに `.golangci.yml` を置く**(リポジトリルート共有にしない)

- 採用理由:
  1. **agent の担当境界に一致する。** impl-ci の charter は `.github/` 配下限定、impl-api/impl-auth/impl-db は各 `app/<stack>` 配下限定。**リポジトリルートの `.golangci.yml` を編集できる impl agent が現状の orchestration に存在しない**(root 直下ファイルは `.claude/` / CLAUDE.md 以外どの agent のスコープにも入らない)。per-stack なら各ファイルが所有 impl agent の境界内に収まる。
  2. **discovery が最も堅牢。** 各 Makefile は自スタックのディレクトリ基点で `golangci-lint run ./...` を実行するため、同ディレクトリ直下の `.golangci.yml` が最優先で確実に拾われる(親ディレクトリ探索の挙動に依存しない)。
  3. **CI 変更が実質不要。** config がスタック内に co-locate されるため、既存の `make check` レーンがそのまま gosec を実行する。
- 退けた代替案: **リポジトリルート共有 `.golangci.yml`**。single-source という点では魅力的だが、(a) 上記の所有者不在問題(どの impl agent もルート直下を編集できない)、(b) golangci-lint の親ディレクトリ config 探索に依存する、の 2 点で本計画では退ける。3 スタック分の重複は生じるが、内容は小さく(gosec + nolintlint を enable するだけ)、各ファイル先頭コメントで「他 2 スタックと同一に保つ」不変条件を明記して drift を抑える。将来 orchestration にルート直下ファイルの所有 agent を定義できれば共有化を再検討する(§6)。

### 1.3 抑制の機構: `//nolint:gosec // <理由>`(gosec ネイティブの `#nosec` ではなく golangci-lint ネイティブ)

- golangci-lint 経由で gosec を回す場合、**golangci-lint の nolint プロセッサが確実に扱うのは `//nolint:<linter>`** であり、gosec ネイティブの `#nosec` の解釈は golangci-lint のバージョン挙動に依存する。1.64.8 で確実に効く `//nolint:gosec` に統一する。
- **理由コメントを機械強制**するため、各 config で `nolintlint` linter を有効化し `require-explanation: true` / `require-specific: true` を設定する。これにより「抑制には必ず理由を書く」(ISSUE-021 / ISSUE-024 の再発防止方針)がコンパイル外のゲートで強制される。既存 `//nolint` がゼロなので副作用なく導入できる。
- 注記: ISSUE-021 本文は "`#nosec G704`" と書いているが、本計画では実際の機構に合わせ **`//nolint:gosec`**、ルール ID も **ベースラインスキャンで確認した実際の ID(SSRF は G107 の見込み)** を採用する。ISSUE-021 側の記述はこの計画に沿って issue-creator が後で更新する(§5)。

### 1.4 各既存 Issue への対応スタンス

| Issue | gosec が出す想定 | 本計画での対応 |
|---|---|---|
| ISSUE-021 healthcheck SSRF | **G107**(可変 URL)を api/auth の healthcheck 2 ファイルで検出する見込み | 偽陽性(URL は運用者制御・外部注入経路なし)として **`//nolint:gosec` + 根拠コメントで抑制**。挙動は不変(テスト不要)。 |
| ISSUE-010 api タイムアウト | **G112**(Slowloris)を `app/api/cmd/api/main.go` の `http.Server` で検出する見込み | **実修正**。auth と対称に 4 種タイムアウト(`ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout`)を付与。tester が検証。 |
| ISSUE-004 open-redirect | **標準 gosec に該当ルールが無いため、恐らく検出されない** | ベースラインスキャンで確認。**出なければコード変更なし**(型/回帰テストによる不変条件ハードニングは ISSUE-004 のスタンス通り本計画スコープ外=引き続き deferred)。万一 gosec が当該 `http.Redirect` を検出したら、`redirect_uri` は `ValidateRedirectURI` 済みである旨の根拠付き `//nolint:gosec` を付す。 |

- **ISSUE-010 の body-size 上限(`http.MaxBytesReader`)部分は gosec が検出しないため本計画スコープ外**とし、ISSUE-010 は「タイムアウト部分は解消 / body-size 部分は open のまま」として扱う(§6・§5)。

---

## 2. 変更ファイル

### app/api(impl-api)
- 追加: `app/api/.golangci.yml` — gosec + nolintlint を有効化(v1 スキーマ)
- 変更: `app/api/cmd/api/main.go` — `http.Server` に 4 種タイムアウトを付与(auth と対称)。テスト容易化のため `newServer(addr string, h http.Handler) *http.Server` ヘルパを抽出(タイムアウトをここで設定)
- 変更: `app/api/cmd/healthcheck/main.go` — `client.Get(url)` 行(34)に `//nolint:gosec` + 根拠コメント
- 追加: `app/api/cmd/api/main_test.go`(tester) — `newServer` が 4 種タイムアウトを非ゼロで設定していることを検証
- (ベースラインで他の gosec 指摘が出た場合)当該ファイルに修正 or 根拠付き抑制を追加

### app/auth(impl-auth)
- 追加: `app/auth/.golangci.yml` — gosec + nolintlint を有効化(v1 スキーマ、api と同一内容)
- 変更: `app/auth/cmd/healthcheck/main.go` — `client.Get(url)` 行(34)に `//nolint:gosec` + 根拠コメント(api と同一の文言)
- 変更(条件付き): `app/auth/route/response.go` — ベースラインで `http.Redirect`(144 行)に gosec 指摘が出た場合のみ、根拠付き `//nolint:gosec`。出なければ変更なし
- (ベースラインで他の gosec 指摘が出た場合)当該ファイルに修正 or 根拠付き抑制

### app/migrator(impl-db)
- 追加: `app/migrator/.golangci.yml` — gosec + nolintlint を有効化(v1 スキーマ、api/auth と同一内容)
- 変更(高確度): `app/migrator/database.go` — `"CREATE DATABASE "+quoteIdentifier(name)`(127 行)は gosec の **G202**(SQL 文字列連結)を誘発する見込み。`validateIdentifier` allowlist + `quoteIdentifier` の二重防御を根拠に `//nolint:gosec` を付す(既存の詳細コメント 43-51 行を参照する形で)
- (ベースラインで他の gosec 指摘が出た場合)当該ファイルに修正 or 根拠付き抑制

### .github/(impl-ci)
- 原則 **変更不要**(既存の `api` / `auth` / `migrator` job が `make check` → `make lint` を実行済みで、config を置けば gosec が自動的に走る)。
- impl-ci の実務は「担保と検証」:
  - `.golangci.yml` のスキーマが pin 済み golangci-lint `1.64.8`(v1)と整合することを確認する。バージョン整合のためにあえて bump が必要なら `env.GOLANGCI_LINT_VERSION` を更新する(既定は bump しない)
  - `cicd.yml` の説明コメント(冒頭のコマンド契約説明)に「lint に gosec を含む」旨を追記する程度のドキュメント修正は可(`.github/` 内)

---

## 3. 手順(実行 agent と順序)

> 原則: **スタックごとに「指摘解消 → gosec 有効化」を完結**させる。3 スタックは config が独立しているため **並列実行可**(§3.A の 3 レーンは並列)。スタック内では必ず「先に修正/抑制 → 最後に `.golangci.yml` 追加」の順を守る。

### Step A(並列・3 レーン): 各 Go スタックのベースライン取得 → 指摘解消 → gosec 有効化

各レーンの共通サブ手順:
1. **ベースライン取得**: 自スタックのディレクトリで `golangci-lint run --no-config --enable-only=gosec ./...` を実行し、**実際に出た gosec 指摘(ルール ID・ファイル:行)を列挙**する。以降はこの列挙を正典とする(Issue の番号で決め打ちしない)。
2. 列挙された各指摘を「実修正」または「根拠付き `//nolint:gosec` 抑制」で解消する(下記レーン別)。
3. 自スタックに `.golangci.yml`(gosec + nolintlint 有効、v1 スキーマ)を追加する。
4. `make lint`(= config 適用済みの `golangci-lint run ./...`)を実行し、**gosec を含めて green** になることを自己確認する。

- **A-1 impl-api(app/api)**:
  - 実修正: `cmd/api/main.go` の `http.Server` に 4 種タイムアウトを付与(auth の 44-47 行の定数・コメントに倣う)。`newServer` ヘルパを抽出して tester が検証できる形にする(ISSUE-010 G112)。
  - 抑制: `cmd/healthcheck/main.go:34` の `client.Get(url)` に `//nolint:gosec // G107: URL is operator-controlled (os.Args / HEALTHCHECK_URL / const default), used only by the container HEALTHCHECK self-probe; no external-injection path exists (ISSUE-021)`(ID はベースラインの実際値に合わせる)。
  - ベースラインで出たその他の指摘も解消。
- **A-2 impl-auth(app/auth)**:
  - 抑制: `cmd/healthcheck/main.go:34` に api と同一文言の `//nolint:gosec`(2 ファイルの非対称を作らない)。
  - `route/response.go` の `http.Redirect` は **ベースラインに出た場合のみ**、`redirect_uri` が `ValidateRedirectURI` 済み + `isUnverifiedAuthorizeError` 不変条件で保護されている旨の根拠付き `//nolint:gosec` を付す(ISSUE-004)。出なければコード変更なし。
  - ベースラインで出たその他の指摘も解消。
- **A-3 impl-db(app/migrator)**:
  - 抑制(高確度): `database.go:127` の `CREATE DATABASE` 連結に、`validateIdentifier`(allowlist)+ `quoteIdentifier`(二重引用)の二重防御を根拠にした `//nolint:gosec`(G202 想定)。
  - ベースラインで出たその他の指摘も解消。

### Step B(Step A 完了後): tester

- **impl-api の実修正に対する検証**を追加(ISSUE-010 タイムアウト)。詳細は §4。
- 3 スタックで `make test`(必要に応じ `make test-race`)が green であることを確認。

### Step C(Step A/B 完了後): checker

- 3 スタックそれぞれで `make check`(= fmt-check + lint + vet + build + test)を実行し、**gosec を含む `make lint` が全 3 スタックで green** であることを確認する(ISSUE-024 の主目的の受け入れ判定)。
- あわせて `golangci-lint run --no-config --enable-only=gosec ./...` を各スタックで再走させ、**未対応の gosec 指摘が 0 件**であること(= 抑制/修正の網羅)を確認する。

### Step D(並列可・Step A 完了後): impl-ci

- `.golangci.yml`(v1 スキーマ)が CI の pin 済み golangci-lint `1.64.8` と整合することを確認する。
- 既存 CI(`api` / `auth` / `migrator` job)が config 追加により gosec を実行することを、job 定義上で確認する(コマンド追加は不要なはず)。必要なら `cicd.yml` の説明コメントに「lint は gosec を含む」旨を追記する。
- CI 変更が本当に不要と確認できたら、その旨を報告する(no-op も成果)。

### Step E: レビュー(並列)

- review-security(抑制の妥当性 = 偽陽性判断が正しいか、根拠コメントが十分か)、review-spec(ISSUE-024 の受け入れ条件充足)。review はコードを変更しない。Blocker/Major は該当 impl agent に差し戻し(→ Step B/C 再実行)。

### Step F: 記録更新(issue-creator)

- §5 の相互参照方針に従い、ISSUE-024 / 021 / 010 / 004 の経緯・status を skill 手順で更新する。

---

## 4. テスト戦略

- **方式**: gosec 有効化・抑制は挙動不変のため新規テスト不要(検証は checker の `make lint` green + ベースライン 0 件で担保)。**実修正が入る api タイムアウト(ISSUE-010)のみ後付けでユニットテストを追加**する(TDD で先行させてもよいが、既存 auth と同型の小さな修正のため後付けを許容)。
- **api タイムアウト検証(tester、`app/api/cmd/api/main_test.go`、`package main`)**:
  - `newServer(addr, handler)` が返す `*http.Server` の `ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout` がいずれも**非ゼロ**(かつ意図した定数値)であることを表形式でアサートする。ゼロ値=無制限の退行を検出できる。
  - 実サーバ起動や sleep 依存は使わない(テスト規約:実時間依存を書かない)。
- **回帰検出(ISSUE-024 の主眼)**: gosec が `make lint` / CI の恒久ゲートに入ることで、healthcheck の SSRF・api タイムアウトの再退行・migrator の SQL 連結などが再導入されれば CI が fail する。これが「再現性・回帰検出」の担保そのもの。
- **観点カバレッジ**: 正常系(4 タイムアウトが設定済み)/ 境界(ゼロ値でない)を最低限カバー。異常系(タイムアウト超過の実挙動)は実時間依存になるため単体では扱わず、設定の存在検証で代替する。

### 要件 ↔ 手順/テスト 対応表

| 要件(Issue) | 手順 | テスト/検証 |
|---|---|---|
| ISSUE-024: gosec が 3 スタックの lint に組み込まれる | Step A(各 `.golangci.yml` 追加) | Step C `make lint` green ×3 |
| ISSUE-024: gosec が CI で回る | Step D(既存 `make check` レーンが自動で拾う) | Step C/D で CI コマンド経路を確認 |
| ISSUE-024: 既存指摘が再現でき回帰検出できる | Step A-1 のベースライン取得で再現 → 抑制/修正後も gosec ゲート常設 | ベースライン 0 件(Step C)+ 恒久 CI ゲート |
| ISSUE-024: 抑制には理由コメント必須 | nolintlint(`require-explanation`)を各 config で有効化 | nolintlint が理由なし抑制を fail |
| ISSUE-021: healthcheck SSRF 抑制 | Step A-1 / A-2 の `//nolint:gosec` | Step C green(挙動不変) |
| ISSUE-010: api タイムアウト付与 | Step A-1 の 4 タイムアウト実装 | §4 の `main_test.go` |
| ISSUE-004: open-redirect | Step A-2(検出時のみ抑制、既定は不変) | Step A-2 ベースライン結果を記録 |

---

## 5. 起点 Issue への相互参照方針(実更新は issue-creator が実施)

> planner はファイルを直接編集しない。以下は issue-creator が skill 手順(経緯への追記のみ・過去エントリ不編集・frontmatter の status/updated 更新)で行う内容の指定。

- **ISSUE-024**: 「4. 対応」に本計画(`docs/plans/ISSUE-024-plan.md`)への参照とタスク要約(方式=golangci-lint の gosec 有効化 / 配置=per-stack / 抑制=`//nolint:gosec`+nolintlint 強制)を追記。実装完了時に status を `resolved` に更新。経緯に「gosec 実 ID は G107/G112 で、open-redirect は標準 gosec に無い」というベースライン結果を記録。
- **ISSUE-021**: 経緯に「gosec 恒久組み込み(ISSUE-024)の一環で、api/auth healthcheck の SSRF(実 ID: ベースライン確認値)を偽陽性と判断し `//nolint:gosec`+根拠で抑制した」を追記し status を `resolved`(または wontfix 相当の抑制完了)に。本文の `#nosec G704` 表記は `//nolint:gosec` + 実 ID に読み替える旨を明記。
- **ISSUE-010**: 経緯に「ISSUE-024 の一環で `http.Server` の 4 タイムアウトを付与し G112 を解消(auth と対称化)。**body-size 上限(`http.MaxBytesReader`)部分は gosec 非検出のため本対応スコープ外で未対応**」を追記。status は body-size が残るため `open` を維持(タイムアウト部分のみ解消と明記)。
- **ISSUE-004**: 経緯に「ISSUE-024 のベースラインで当該 `http.Redirect` に gosec 指摘が(出た/出なかった)ことを確認。標準 gosec に open-redirect ルールが無いため gosec ゲートでは強制されない。型/回帰テストによる不変条件ハードニングは当初スタンス通り deferred のまま」を追記。status は `open` を維持。
- frontmatter の相互リンク: ISSUE-024 の本文で 021/010/004 を参照(既に相互参照済み)。

---

## 6. リスク / 未確定事項

1. **【要検証・最重要】gosec が実際に出す指摘の集合が不確定。** Issue の番号(G704/G710)は標準 gosec ID と一致せず、SSRF は G107、Slowloris は G112、open-redirect は**該当ルール無し**の見込み。planner は gosec を実行しておらず断定しない。**Step A-1/2/3 のベースラインスキャンが唯一の正典**で、そこで初めて確定する。三大既知指摘以外にも(例: G115 整数変換、G302/G306 ファイル権限、G404 弱乱数 など)想定外の gosec 指摘が 3 スタックのどこかで出る可能性がある。各 impl agent は**ベースラインで出た全件**を「実修正 or 根拠付き抑制」で潰すこと(既知 3 件だけを見て終わらせない)。この blast radius の不確定さが本計画最大のリスク。
2. **config 配置を per-stack にした帰結として 3 ファイルが重複する。** 内容 drift を防ぐため、3 つの `.golangci.yml` は同一内容に保ち、先頭コメントで相互参照する。将来ルート共有に寄せるには、orchestration にリポジトリルート直下ファイルの所有 agent を定義する必要がある(現状は不在)。**ユーザー判断が要るのはこの点**: 「per-stack 重複を許容する(本計画の既定)」か「ルート共有 config のために所有 agent の割り当てを変える」か。
3. **golangci-lint のバージョン依存。** config は CI pin の `1.64.8`(v1 スキーマ)前提。ローカル開発者が v2 系 golangci-lint を使うと v1 スキーマ config が動かない可能性がある(既存の環境前提の問題で本計画が新設するものではないが、`make lint` を叩く開発者は 1.64.x を使う必要がある)。impl-ci が CI 側の pin 整合を担保する。
4. **`#nosec` vs `//nolint:gosec` の選択**は §1.3 の通り `//nolint:gosec` に確定。ISSUE-021 本文の `#nosec G704` 表記とは食い違うが、golangci-lint 1.64.8 で確実に効く方を優先した(nolintlint による理由強制もこちらにのみ効く)。
5. **ISSUE-010 の body-size 上限は本計画スコープ外。** gosec が検出しないため CI green には不要。ISSUE-010 はタイムアウト部分のみ解消し、body-size は open のまま残す(将来別対応)。この分割をユーザーが是とするか、body-size も本計画に含めるかは判断余地あり(既定は分割=スコープ外)。
6. **nolintlint の `allow-unused: false` による誤検知**: 抑制コメントの行と gosec 指摘行がズレると「unused nolint」で fail しうる。各 impl agent は抑制追加後に `make lint` を再走して行対応を確認すること(Step A サブ手順 4)。
