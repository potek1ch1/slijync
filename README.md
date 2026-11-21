# slijync

PDFスライドを通常閲覧またはリアルタイム同期セッションで共有するアプリケーション。

## 必要なもの

- Docker
- Docker Compose

ローカルでGoやReactを直接動かす場合は、Go 1.25以上とNode.js 22以上も使用する。

## 起動

```bash
cp .env.example .env
docker compose up --build
```

起動後:

- Web: http://localhost:5173
- API health check: http://localhost:8080/api/health
- Garage S3 API: http://localhost:3900
- PostgreSQL: localhost:5432

`migrate`サービスはPostgreSQLの起動後に未適用のgoose migrationだけを実行し、正常終了する。`server`はmigrationの正常終了後に起動する。

## migrationの追加

適用済みmigrationは変更せず、新しい連番ファイルを追加する。

```text
db/migrations/000002_create_initial_schema.sql
db/migrations/000003_add_example_column.sql
```

追加後は次を実行する。

```bash
make migrate
```

開発環境では`db/migrations`をmigrationコンテナへread-only mountしているため、イメージを再ビルドしなくても新しいSQLが読み込まれる。gooseは`goose_db_version`を確認し、未適用ファイルだけを順番に実行する。

すでに適用したmigrationを開発中に作り直す必要がある場合は、通常はDownして書き換えるのではなく、新しいmigrationで修正する。DB volumeを削除する操作は全ローカルデータを失うため、明示的に初期化するときだけ行う。

## よく使うコマンド

```bash
make up
make down
make logs
make migrate
make sqlc
make test
```

## ディレクトリ

```text
apps/web       React + TypeScript + Vite
apps/server    Go + chi
db/migrations goose migration
db/queries    sqlc queries
infra         Docker ComposeとGarage設定
doc           要件・設計
```

## 設定

開発用の初期値は`.env.example`にある。本番ではCookie secret、DB password、Garage credentialsを必ず変更する。
