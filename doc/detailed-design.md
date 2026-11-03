# プレゼン同期アプリ 詳細設計

> ステータス: Draft（実装開始可能）
> 最終更新: 2026-11-19
> 要件定義: [requirements.md](./requirements.md)
> 基本設計: [basic-design.md](./basic-design.md)

## 1. 目的と対象

要件と基本設計を、実装時に判断が分かれない粒度へ具体化する。MVPのDB、HTTP API、WebSocket、認証、PDF保存、定期処理、エラー処理、テストを対象とする。本書にない新機能はMVPへ含めない。

## 2. 全体構成

```text
Browser (React SPA)
  ├─ HTTP API ───────┐
  └─ WebSocket ──────┤
                     v
              Go application
              ├─ Auth / Deck / Member
              ├─ Session / WebSocket Hub
              └─ PDF Storage
                │              │
                v              v
           PostgreSQL        Garage
```

- Goは単一プロセス・単一インスタンスで開始する。
- PostgreSQLを永続データの正、GarageをPDF本体の保存先とする。
- WebSocket接続と配信HubはGoプロセスのメモリで管理する。
- Redisと複数GoインスタンスはMVP対象外とする。

## 3. リポジトリ構成

```text
apps/
  web/src/                  React + TypeScript + Vite
    api/ auth/ components/ features/ pages/ router/
  server/
    cmd/server/
    internal/
      auth/ deck/ member/ session/ realtime/ storage/ httpapi/ config/
    sqlc/                   sqlc生成コード
db/
  migrations/              goose SQL
  queries/                  sqlc用SQL
  sqlc.yaml
infra/compose.yaml
doc/
```

- HTTPハンドラーから直接SQLを呼ばず、サービス層を経由する。
- トランザクション境界はサービス層に置く。
- sqlc生成物は手動編集しない。

## 4. データベース設計

### 4.1 共通方針

- 時刻は `timestamptz`、アプリ内部はUTCとする。
- UUIDv4はGo側で生成する。
- メール検索には、前後空白を除去し小文字化した値を使う。
- Viewer、接続、スライド操作、開催履歴のテーブルは作らない。
- 論理削除はSlideDeckだけに使う。

### 4.2 users

| カラム | 型 | 制約・用途 |
|---|---|---|
| id | uuid | PK |
| email | text | NOT NULL、表示用 |
| normalized_email | text | NOT NULL、UNIQUE、権限追加時の検索用 |
| display_name | text | NOT NULL |
| created_at / updated_at | timestamptz | NOT NULL |

Googleログイン時に検証済みメールアドレスと表示名を更新する。本人識別の正はメールではなく `(issuer, subject)` とする。変更後メールが別Userと衝突した場合は自動統合せずログインエラーにする。MVPでは統合UIを作らない。

### 4.3 oidc_identities

| カラム | 型 | 制約・用途 |
|---|---|---|
| id | uuid | PK |
| user_id | uuid | NOT NULL、FK users ON DELETE CASCADE |
| issuer / subject | text | NOT NULL、組み合わせUNIQUE |
| created_at / updated_at | timestamptz | NOT NULL |

`user_id` にindexを設定する。将来、1 Userへ複数プロバイダーを紐づけられる。

### 4.4 login_sessions

| カラム | 型 | 制約・用途 |
|---|---|---|
| id | uuid | PK、管理用 |
| user_id | uuid | NOT NULL、FK users ON DELETE CASCADE |
| token_hash | bytea | NOT NULL、UNIQUE |
| expires_at | timestamptz | NOT NULL |
| created_at / last_seen_at | timestamptz | NOT NULL |

- Cookieには32 byteの暗号学的乱数をBase64URL化して保存する。
- DBにはトークンそのものではなくSHA-256ハッシュを保存する。
- 初期有効期限は7日、ログアウト時はレコードを削除する。

### 4.5 slide_decks

| カラム | 型 | 制約・用途 |
|---|---|---|
| id | uuid | PK、公開URLに使用 |
| title | varchar(200) | NOT NULL |
| current_pdf_version_id | uuid | NULL可、FK pdf_versions |
| created_by_user_id | uuid | NOT NULL、FK users |
| created_at / updated_at | timestamptz | NOT NULL |
| deleted_at | timestamptz | NULL可 |

通常検索では `deleted_at IS NULL` を必須にする。現行PdfVersionが同じSlideDeckに属することはサービス層で保証する。削除後もSession URLの「削除済み」表示のためSlideDeckとSessionレコードは残す。

### 4.6 pdf_versions

| カラム | 型 | 制約・用途 |
|---|---|---|
| id | uuid | PK |
| slide_deck_id | uuid | NOT NULL、FK slide_decks |
| version_number | integer | NOT NULL、1以上 |
| storage_key | text | NOT NULL、UNIQUE |
| page_count | integer | NOT NULL、1以上 |
| file_size_bytes | bigint | NOT NULL、1以上 |
| sha256 | bytea | NOT NULL |
| created_by_user_id | uuid | NOT NULL、FK users |
| created_at | timestamptz | NOT NULL |

- `(slide_deck_id, version_number)` をUNIQUEにする。
- キーは `decks/{slideDeckId}/versions/{pdfVersionId}.pdf` とする。
- 現行版またはactive Sessionが参照する版は削除しない。

### 4.7 slide_deck_members

| カラム | 型 | 制約・用途 |
|---|---|---|
| slide_deck_id / user_id | uuid | 複合PK、各FK |
| role | text | `owner` または `operator` |
| created_by_user_id | uuid | NOT NULL、FK users |
| created_at | timestamptz | NOT NULL |

OwnerはOperator権限を包含する。最後のOwner削除禁止と自己削除禁止はトランザクション内で検証する。user_idにindexを設定する。

### 4.8 sessions

| カラム | 型 | 制約・用途 |
|---|---|---|
| id | uuid | PK、公開URLに使用 |
| slide_deck_id | uuid | NOT NULL、FK slide_decks |
| pdf_version_id | uuid | NULL可、FK pdf_versions |
| status | text | `waiting` / `active` / `ended` |
| current_slide_number | integer | NOT NULL、初期値1 |
| revision | bigint | NOT NULL、初期値0 |
| created_by_user_id | uuid | NOT NULL、FK users |
| started_at / last_operated_at | timestamptz | NULL可 |
| created_at / updated_at | timestamptz | NOT NULL |

- active時だけpdf_version_id、started_at、last_operated_atを必須とする。
- `(slide_deck_id, created_at DESC)` と、active行のlast_operated_atにindexを設定する。
- 状態変更のたびにrevisionを増やし、クライアントで古い配信を無視する。

### 4.9 排他制御

Owner削除時はメンバー行を `FOR UPDATE` し、実行者がOwner、対象が本人以外、Ownerが2人以上であることを同一トランザクションで検証する。

セッション操作時はSession行を `FOR UPDATE` し、active、権限、ページ範囲を検証してからページ、最終操作時刻、revisionを更新する。コミット後にWebSocket配信する。同時操作はDBで直列化され、最後にコミットされた有効操作が最終状態になる。

## 5. HTTP API

### 5.1 共通

- ベースパスは `/api`、JSONキーはsnake_case、日時はRFC 3339 UTCとする。
- UUID不正 `400`、未認証 `401`、権限不足 `403`、不存在 `404`、削除済み公開資料 `410` とする。

```json
{"error":{"code":"forbidden","message":"この操作を実行する権限がありません"}}
```

内部エラー、SQL、Garageキー、OIDC tokenはレスポンスへ含めない。

### 5.2 認証

| Method | Path | 内容 |
|---|---|---|
| GET | `/api/auth/google/start` | Google認証開始 |
| GET | `/api/auth/google/callback` | Callbackとログイン作成 |
| POST | `/api/auth/logout` | ログアウト |
| GET | `/api/me` | User取得。未ログインは `user: null` |

`return_to` は同一サイトの相対パスだけ許可する。

### 5.3 SlideDeck

| Method | Path | 権限 | 内容 |
|---|---|---|---|
| GET | `/api/decks` | ログイン | 関係する資料一覧 |
| POST | `/api/decks` | ログイン | タイトルとPDFで作成 |
| GET | `/api/decks/{deck_id}/management` | Operator以上 | 管理用詳細 |
| PATCH | `/api/decks/{deck_id}` | Owner | タイトル変更 |
| PUT | `/api/decks/{deck_id}/pdf` | Owner | PDF差し替え |
| DELETE | `/api/decks/{deck_id}` | Owner | 資料削除 |
| GET | `/api/public/decks/{deck_id}` | 公開 | 通常閲覧メタデータ |
| GET | `/api/public/decks/{deck_id}/pdf` | 公開 | 現行PDF |

作成・差し替えはmultipartを使う。PDF取得はRange、ETag、If-None-Matchへ対応し、Goからストリーム配信する。

### 5.4 メンバー

| Method | Path | 権限 | 内容 |
|---|---|---|---|
| GET | `/api/decks/{deck_id}/members` | Owner | 一覧 |
| POST | `/api/decks/{deck_id}/members` | Owner | メールとroleで追加 |
| PATCH | `/api/decks/{deck_id}/members/{user_id}` | Owner | role変更 |
| DELETE | `/api/decks/{deck_id}/members/{user_id}` | Owner | 削除 |

未登録メールは `422 user_not_registered`、重複は `409 member_already_exists`、自己削除は `409 cannot_remove_self`、最後のOwner削除・降格は `409 last_owner_required` とする。

### 5.5 Session

| Method | Path | 権限 | 内容 |
|---|---|---|---|
| GET | `/api/decks/{deck_id}/sessions` | Operator以上 | 一覧 |
| POST | `/api/decks/{deck_id}/sessions` | Operator以上 | waitingで作成 |
| GET | `/api/public/sessions/{session_id}` | 公開 | 参加画面用状態 |
| POST | `/api/sessions/{session_id}/start` | Operator以上 | 開始・再開 |
| POST | `/api/sessions/{session_id}/end` | Operator以上 | 終了 |
| GET | `/api/public/sessions/{session_id}/pdf` | 公開 | active時の固定PDF |

開始時はDeckとSessionをロックし、現行PDFを固定、ページを1、statusをactive、開始・最終操作時刻を現在時刻にしてrevisionを増やす。activeを再開始した場合は `409 session_already_active` とする。

終了時はstatusをended、PDF参照をNULL、revisionを加算する。waitingの終了も許可する。コミット後に状態配信と不要な旧PDF清掃を行う。

## 6. WebSocket

接続先は `GET /api/public/sessions/{session_id}/ws` とする。

- CookieなしはViewer、認証済みは対象Deckの権限を判定する。
- 接続時と操作時に認可し、許可したOrigin以外を拒否する。
- 最大メッセージは16 KiB、Ping/Pongで死活確認する。
- 接続直後に最新session_stateを送る。

ClientからServer:

```json
{"type":"set_slide","request_id":"0dc090cb-83c9-4e48-ae22-1da89ad95d81","slide":3}
```

ServerからClient:

```json
{
  "type":"session_state",
  "session_id":"8f6a9af7-f79f-4c75-aa39-b0b802413d90",
  "status":"active",
  "slide":3,
  "page_count":20,
  "revision":12,
  "can_operate":true,
  "deck":{"id":"9dc5f973-4bc8-4389-a5ef-9166a4810ea8","title":"発表資料","deleted":false}
}
```

waiting/endedではslideとpage_countをnullにする。削除済みはdeck.deleted=trueとしPDFを配信しない。クライアントは現在値以下のrevisionを無視する。

操作拒否時は `operation_error` とrequest_id、code、messageを返す。DBコミット前には配信しない。配信失敗でDB更新は戻さず、再接続時の最新状態で回復する。送信が滞留する接続は切断する。

## 7. 認証・認可

- Google OIDC Authorization Code Flowを使い、state、nonce、PKCEを検証する。
- iss、aud、exp、署名、nonce、`email_verified=true` を検証する。
- Google access/refresh tokenは保存しない。
- Cookie名は `slijync_session`、HttpOnly、Secure、SameSite=Lax、Path=/ とする。
- 状態変更APIとWebSocket UpgradeでOriginを検証し、本番HTTPSを必須とする。

| 操作 | Viewer | Operator | Owner |
|---|---:|---:|---:|
| 通常閲覧・参加 | ○ | ○ | ○ |
| Session作成・開始・終了・ページ操作 | × | ○ | ○ |
| タイトル・PDF変更 | × | × | ○ |
| 権限管理・Deck削除 | × | × | ○ |

UI制御に加え、すべての更新APIとWebSocket操作でサーバー側認可を行う。

## 8. PDF・Garage

### 8.1 検証

- 拡張子やContent-TypeだけでなくPDFヘッダーと構造を確認する。
- 暗号化されておらず、1ページ以上解析できるPDFだけ許可する。
- 初期上限は50 MiB、500ページとし環境変数で変更可能にする。

### 8.2 作成・差し替え

1. 一時領域へ受信し、サイズ上限とSHA-256を計算する。
2. PDF形式とページ数を検証する。
3. UUIDとstorage keyを生成してGarageへアップロードする。
4. DBトランザクションでメタデータ登録と現行版切り替えを行う。
5. DB失敗時はGarageから補償削除する。
6. 成功後、参照されない旧版を清掃する。

同じファイルの再アップロードも別PdfVersionとする。

### 8.3 Deck削除

1. DeckをロックしてOwner認可する。
2. deleted_atを設定し、active Sessionを終了してPDF参照を外す。
3. コミット後に削除済み状態を配信する。
4. Garageオブジェクトを削除し、成功したPdfVersionレコードを削除する。

Garage削除失敗時はDeckを削除済みのままにし、PdfVersionを再清掃対象として残す。公開APIからPDFは取得できない。

## 9. 定期処理

- 1分ごとに、activeかつ `last_operated_at <= now() - 30 minutes` のSessionを行ロック後に再判定して終了する。
- ページ変更だけが30分タイマーをリセットする。接続、Ping、状態取得ではリセットしない。
- コミット後に状態配信と旧PDF清掃を行う。
- 期限切れlogin sessionと、現行・active参照のどちらでもないPdfVersionを清掃する。
- 補償削除に失敗した孤立オブジェクトは構造化ログへ記録する。

重複起動しても安全になるよう、条件付き更新と行ロックを使う。

## 10. migrationとsqlc

- `db/migrations/{timestamp}_{name}.sql` にgooseのUp/Downを書く。
- Composeのmigrateサービスが一時起動し、未適用migrationだけを実行して終了する。
- serverはmigrate成功後に起動する。server自身はmigrationを暗黙実行しない。
- gooseはDB内のversion管理テーブルで適用済みを判断する。
- sqlcはmigrationのUpスキーマとdb/queriesのSQLからGoコードを生成する。

## 11. 設定

```text
APP_BASE_URL                  HTTP_ADDR
DATABASE_URL                 GOOGLE_OIDC_CLIENT_ID
GOOGLE_OIDC_CLIENT_SECRET    SESSION_COOKIE_SECRET
LOGIN_SESSION_TTL            S3_ENDPOINT
S3_REGION                    S3_BUCKET
S3_ACCESS_KEY_ID             S3_SECRET_ACCESS_KEY
PDF_MAX_BYTES                PDF_MAX_PAGES
SESSION_IDLE_TIMEOUT
```

必須値がなければ起動を失敗させ、secretをログへ出さない。

## 12. ログ・監視

- JSON構造化ログを標準出力へ出す。
- HTTP request ID、WebSocket request_id、必要なUser/Deck/Session IDを含める。
- Cookie、OIDC token、PDF内容、Garage secretは記録しない。
- HTTPエラー数、WebSocket接続数、active Session数、Garage/DB失敗数を監視対象にする。
- 操作履歴はDB保存せず、MVPの障害調査はログで行う。

## 13. テスト方針

- 単体: 認可、メール正規化、状態遷移、ページ範囲、PDF検証、Cookie token照合。
- DB統合: 最後のOwner・自己削除拒否、同時ページ操作、Session開始終了時のPDF参照、差し替え中の旧版固定、自動終了と清掃。
- API/WebSocket: 匿名Viewer、権限別API、途中参加・再接続、同時操作後のrevision、各Session状態、Range PDF取得。
- Frontend: 通常閲覧の独立性、状態別画面、権限別UI、自動再接続、page query正規化。

## 14. 実装順序

1. リポジトリ、Compose、PostgreSQL、Garage、migration、sqlc
2. Google OIDC、User、login session
3. Deck作成、PDF保存、通常閲覧
4. Owner/Operator管理
5. Session作成・開始・終了
6. WebSocket同期と競合制御
7. 自動終了、PDF差し替え・清掃、Deck削除
8. UI仕上げ、統合テスト、運用設定

## 15. 変更可能な初期設定

| 項目 | 初期値 |
|---|---:|
| ログイン有効期限 | 7日 |
| PDF最大サイズ | 50 MiB |
| PDF最大ページ数 | 500 |
| 無操作終了 | 30分 |
| 自動終了チェック | 1分 |
| WebSocket最大メッセージ | 16 KiB |

これらは運用設定であり、実装を止める未決事項ではない。デプロイ先、ドメイン、CI/CD、バックアップ保持期間はインフラ設計時に決定する。

## 16. 保留事項

MVPの機能・データ・API設計に、実装開始を止める保留事項はない。

将来検討として、Google以外のOIDC、Redis、複数アプリケーションインスタンス、アカウント統合、開催履歴、Viewer管理、非公開資料を残す。

