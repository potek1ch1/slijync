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

