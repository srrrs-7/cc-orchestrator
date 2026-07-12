---
paths:
  - "app/api/**"
---

# app/api — Go 規約

`app/api` はエリック・エヴァンス DDD レイヤ化アーキテクチャの Go サンプル(タスク管理)。依存の向きは一方向 `route → service → domain` で、`domain` はどの層にも依存しない。標準ライブラリ主体で、永続化層 `infra/postgres` のみ `pgx` に依存する(SPEC-005 / SPEC-011。DB 規約の正は `.claude/rules/db.md`)。SPEC-015 で OIDC リソースサーバー化(auth の JWKS で Bearer JWT を検証。`AUTH_*` env 未設定なら検証無効)。アーキテクチャの詳細は `app/api/README.md` が正。

## コマンド

実行はすべて `app/api` ディレクトリで行う。checker / tester はこれを実行する。各ターゲットの実体は `app/api/Makefile` が単一の情報源。SPEC-009 により全コマンドは toolchain コンテナ内で実行される(ホストで go を直接実行しない)。`make test` は実 test DB `api_test` を要する(SPEC-013。正規経路はルート `make migrate-test` で用意 → `REQUIRE_DB=1`。意味論の正は `.claude/rules/testing.md`)。

| 目的 | コマンド |
|---|---|
| format(チェック) | `make fmt-check` |
| format(自動修正) | `make fmt` |
| lint | `make lint` |
| type check 相当 | `make vet` && `make build` |
| test | `make test`(race 検査は `make test-race`) |
| 上記すべて | `make check` |
| OpenAPI 契約生成 | `make openapi`(swag v2 注釈から `docs/openapi.yaml` を生成。SPEC-003。生成であり検査ではないため `make check` には含めない) |
| 依存解決(go.mod 編集・merge 競合解消の後) | `make tidy`(`go mod tidy` を network 有効の toolchain コンテナ経由で実行。生成系と同様 `make check` には含めない) |

## レイアウト

レイヤ別トップディレクトリ構成を採る(`internal/` は用いない。`app/auth` と同型)。

- `cmd/api/main.go` — エントリポイント = コンポジションルート(配線のみ・ロジックを持たない)。ほかに `cmd/healthcheck`
- `domain/<aggregate>/` — ドメイン層(他層に非依存)。集約ごとにパッケージを切る(現状 `task`)。集約ルートはフィールド非公開で状態遷移は振る舞いメソッド経由のみ。永続化ポート(`repository.go` の `Reader` / `Writer` / 合成 `Repository`。SPEC-010)もここで宣言する
- `service/` — アプリケーション層(ユースケース。ドメインを協調させる薄い層)
- `infra/` — インフラ層(ドメインが宣言したポートの実装)。`postgres`(Repository 実装。`pgx`)・`jwt`(Bearer JWT 検証。auth の JWKS を参照、SPEC-015)・`repotest`(共有ふるまい契約テスト)
- `route/` — プレゼンテーション層(HTTP ハンドラ・ルーティング・エラー変換。`auth_middleware.go` が Bearer 保護、SPEC-015)
- `docs/openapi.yaml` — OpenAPI 契約の正(swag v2 注釈から `make openapi` で生成し、web が型を生成する。SPEC-003)

## コーディング

- `context.Context` は第一引数で受け渡す。struct フィールドに保持しない
- エラーは `fmt.Errorf("...: %w", err)` でラップして文脈を付与する。`panic` をエラーハンドリングに使わない
- 呼び出し側で分岐したいエラーは sentinel error(`var ErrNotFound = errors.New(...)`)またはカスタム型を定義し、`errors.Is` / `errors.As` で判定する
- goroutine を起動するコードは終了条件(context cancel / WaitGroup / errgroup)を必ず持つ
- interface は使う側のパッケージで最小限に定義する(提供側で大きな interface を切らない)
- DB アクセス・外部 API 呼び出しは interface 越しにし、テストで差し替え可能にする
