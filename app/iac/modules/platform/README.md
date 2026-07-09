# platform module

api / auth など複数の `modules/service` インスタンスが共有する土台(ALB + HTTP リスナー +
ECS クラスタ/capacity providers)を作成する。ターゲットグループ・リスナールール・ECS
サービス/タスク定義は持たない(SPEC-004)。

## なぜ ECS サービスを持つモジュールから ALB/クラスタを切り離したか(循環回避)

旧 `modules/app` は ALB・ECS クラスタ・ECS サービスを 1 モジュールにまとめていた。api と auth の
2 サービスを持たせるにあたり、単純に `modules/app` を `for_each` で 2 回展開する案は **循環依存**
を生むため退けた:

- auth の `ISSUER` 環境変数は CloudFront ドメイン(`module.cdn` の出力)を必要とする
- `module.cdn` は ALB DNS 名(旧 `modules/app` の出力)を必要とする
- ALB と ECS サービスが同一モジュールにあると、`module.app` が `module.cdn` の出力を必要とし
  かつ `module.cdn` が `module.app` の出力を必要とする **相互参照**になり、Terraform が
  依存関係を解決できない

ALB/リスナー/クラスタだけを本モジュール(`platform`)に切り出すことで、依存を
`platform → cdn → service` の一方向 DAG に開ける:

```
network → platform(ALB+listener+cluster) → cdn(S3+CloudFront, 依存は ALB DNS のみ)
                    │                          │
                    └──────────────┬───────────┘
                                   ▼
                        service_api / service_auth
```

CloudFront は **ALB DNS 名だけ**に依存し、ターゲットグループ/ECS サービスの登録には依存しない
(リスナーの default action は `fixed-response` 403 で、特定のターゲットグループを参照しない)。
これにより単一 `apply` で auth の issuer に実 CloudFront ドメインを注入できる(詳細は
`docs/plans/SPEC-004-plan.md` の「循環回避の DAG」)。

## コスト上の選択理由

### ALB 1 本を api / auth で共用する

auth 用に別 ALB を新設すると固定費(約$16〜20/月)が単純に倍増する。ALB は 1 本のまま共用し、
サービスごとのターゲットグループ + リスナールール(`modules/service`)で振り分けることで、
auth 追加による固定費の増分をほぼゼロに抑える(R6、Spec §4 代替案表)。

### ECS クラスタを共有する

ECS クラスタ自体は課金対象ではなく(タスクにのみ課金)、api / auth のタスクを同じクラスタに
まとめて配置管理を簡潔にする。`capacity_provider_strategy`(FARGATE / FARGATE_SPOT)は
`modules/service` 側のサービス単位で選択できるため、クラスタを分ける必要はない。

### CloudFront ⇔ ALB 間が HTTP である点のトレードオフ

本サンプルはカスタムドメイン / ACM 証明書を使わない(Spec スコープ外)ため、ALB のリスナーは
HTTP(80)のみで HTTPS 化していない。CloudFront → ALB 間の通信は AWS のバックボーンネットワーク
内を通るが、TLS 終端は CloudFront までであり、ALB までの区間は暗号化されない(カスタムヘッダの
値もこの区間では平文で流れる)。ACM 証明書とカスタムドメインを用意できる環境では、ALB に
HTTPS リスナーを追加し `modules/cdn` の `custom_origin_config` を `https-only` に変更することを
推奨する。

## default action が `fixed-response` 403 である理由

リスナーの default action は特定のターゲットグループを参照せず `fixed-response` 403 を返す。
これにより、`modules/service` 側のリスナールール(ヘッダ条件)にマッチしないリクエストは
一切どのサービスにも forward されない(default-deny)。特定サービスへの forward は必ず
`modules/service` が付与するヘッダ条件つきリスナールール経由に限られる。
