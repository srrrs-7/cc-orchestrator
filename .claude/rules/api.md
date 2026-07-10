---
paths:
  - "app/api/**"
---

# app/api — Go 規約

## コマンド

実行はすべて `app/api` ディレクトリで行う。checker / tester はこれを実行する。各ターゲットの実体は `app/api/Makefile` が単一の情報源。

| 目的 | コマンド |
|---|---|
| format(チェック) | `make fmt-check` |
| format(自動修正) | `make fmt` |
| lint | `make lint` |
| type check 相当 | `make vet` && `make build` |
| test | `make test`(race 検査は `make test-race`) |
| 上記すべて | `make check` |
| OpenAPI 契約生成 | `make openapi`(swag v2 注釈から `docs/openapi.yaml` を生成。SPEC-003。生成であり検査ではないため `make check` には含めない) |

## レイアウト

- `cmd/<binary>/main.go` — エントリポイント(main は配線のみ、ロジックを書かない)
- `internal/` — アプリケーションコード本体。外部公開するパッケージ以外はすべて internal に置く
- パッケージは層ではなくドメインで切る(`internal/user`, `internal/order` など)

## コーディング

- `context.Context` は第一引数で受け渡す。struct フィールドに保持しない
- エラーは `fmt.Errorf("...: %w", err)` でラップして文脈を付与する。`panic` をエラーハンドリングに使わない
- 呼び出し側で分岐したいエラーは sentinel error(`var ErrNotFound = errors.New(...)`)またはカスタム型を定義し、`errors.Is` / `errors.As` で判定する
- goroutine を起動するコードは終了条件(context cancel / WaitGroup / errgroup)を必ず持つ
- interface は使う側のパッケージで最小限に定義する(提供側で大きな interface を切らない)
- DB アクセス・外部 API 呼び出しは interface 越しにし、テストで差し替え可能にする
