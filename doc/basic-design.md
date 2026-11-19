# プレゼン同期アプリ 基本設計

> ステータス: Draft
> 最終更新: 2026-11-20
> 要件定義: [requirements.md](./requirements.md)
> 詳細設計: [detailed-design.md](./detailed-design.md)

## 1. 目的

本書は、要件を実装可能なデータモデル、画面、API、リアルタイムイベントへ落とし込むための基本設計を管理する。

## 2. データモデル

### 2.1 主要エンティティ

| エンティティ | 役割 |
|---|---|
| User | アプリ内部のユーザー |
| OidcIdentity | OIDCプロバイダー上のIdentityとUserの対応 |
| SlideDeck | スライド資料のタイトルや状態 |
| PdfVersion | アップロードされたPDFの各バージョン |
| SlideDeckMember | SlideDeckに対するOwner・Operator権限 |
| Session | 同期プレゼンの状態、URL、現在スライド |

### 2.2 ID方針

- 各エンティティは、それぞれ独立したUUIDv4を主キーとして持つ。
- UUIDv4はPostgreSQLではなく、Goアプリケーション側で生成する。
- SlideDeckとSessionは別々のUUIDを持つ。
- UUIDv4は十分なランダム性を持つため、SlideDeckとSessionのUUIDをそのまま公開URLに使用する。
- 連番IDや、内部IDとは別の公開IDカラムはMVPでは設けない。
- UUIDを知っていること自体をOwner・Operator権限の根拠にはせず、更新操作は必ず認証・認可する。
- PDFアップロード時は、SlideDeckとPdfVersionのUUIDを先に生成し、GarageのオブジェクトキーとDBレコードの両方に使用する。

Garageのオブジェクトキー:

```text
decks/{slideDeckId}/versions/{pdfVersionId}.pdf
```

### 2.3 User

アプリ内部で一意となるユーザー。Owner・Operator権限はUserへ紐づける。

想定フィールド:

- `id`
- `email`
- `display_name`
- `created_at`
- `updated_at`

### 2.4 OidcIdentity

OIDCプロバイダー上のIdentityを、アプリ内部のUserへ紐づける。

想定フィールド:

- `id`
- `user_id`
- `issuer`
- `subject`
- `created_at`
- `updated_at`

制約:

- `issuer` と `subject` の組み合わせを一意にする。
- 初期リリースではGoogleのみ使用する。
- 将来、1人のUserに複数のOIDC Identityを紐づけられる構造とする。

### 2.5 SlideDeck

スライド資料を表す。PDF本体とは分離し、現在使用するPDFバージョンを参照する。

想定フィールド:

- `id`
- `title`
- `current_pdf_version_id`
- `created_by_user_id`
- `created_at`
- `updated_at`
- `deleted_at`

### 2.6 PdfVersion

アップロードまたは差し替えられたPDFの1バージョンを表す。

想定フィールド:

- `id`
- `slide_deck_id`
- `version_number`
- `storage_key`
- `page_count`
- `created_by_user_id`
- `created_at`

保持ルール:

- セッション開始時に、その時点の `PdfVersion` をセッションへ固定する。
- PDF差し替え後も、開催中のセッションが参照する旧バージョンを保持する。
- 旧バージョンを参照する開催中のセッションがなくなった時点で、その旧バージョンを削除する。
- 履歴目的で旧バージョンを永久保存しない。
- `SlideDeck.current_pdf_version_id` が参照する現行バージョンは削除しない。

### 2.7 SlideDeckMember

UserがSlideDeckに対して持つ権限を表す。

想定フィールド:

- `slide_deck_id`
- `user_id`
- `role`: `owner` または `operator`
- `created_at`
- `created_by_user_id`

制約:

- `slide_deck_id` と `user_id` の組み合わせを一意にする。
- OwnerはOperatorの権限を含む。
- Viewerはレコードを作成せず、Owner・Operator以外のユーザーまたは匿名ユーザーとして扱う。
- SlideDeckには常に1人以上のOwnerが必要。

### 2.8 Session

同期プレゼンの状態を表す。同じSlideDeckに複数のSessionを作成できる。

想定フィールド:

- `id`
- `slide_deck_id`
- `pdf_version_id`: 開催中のみ設定し、終了時は `null`
- `status`: `waiting`、`active`、`ended`
- `current_slide_number`
- `created_by_user_id`
- `started_at`
- `last_operated_at`
- `created_at`
- `updated_at`

制約・挙動:

- `id` は独立したUUIDv4とし、セッション専用URLに使用する。
- Sessionは開催履歴ではなく、同じURLで繰り返し利用する同期ルームとして扱う。
- 過去の開催、操作、使用PDFバージョンを記録する履歴エンティティは作成しない。
- セッション開始・再開時に `current_slide_number` を1へ戻す。
- 開始・再開時に現行の `PdfVersion` を `pdf_version_id` へ固定する。
- SlideDeckに現行PdfVersionが存在しない場合、セッションを開始・再開できない。
- Operatorの「前へ」「次へ」で `last_operated_at` を更新する。
- `last_operated_at` から30分間操作がなければ `ended` にする。
- セッション終了時に `pdf_version_id` を `null` にし、過去に使用したPDFバージョンとの対応履歴は保持しない。
- 終了したSessionには、同じURLで再開するための `id`、`slide_deck_id`、状態などの最小情報だけを保持する。
- セッション再開時は、その時点の現行PdfVersionを改めて固定する。
- Viewerの接続情報は保存しない。

## 3. データライフサイクル

### 3.1 PDF差し替え

1. Ownerが新しいPDFをアップロードする。
2. 新しいPdfVersionを作成する。
3. SlideDeckの現行バージョンを新しいPdfVersionへ切り替える。
4. 開催中のSessionは、固定済みの旧PdfVersionを使い続ける。
5. Session終了時にPdfVersionへの参照を外す。
6. 旧PdfVersionを参照する開催中のSessionがなくなったら、旧PDFとPdfVersionレコードを削除する。

### 3.2 PDF新規登録

1. GoアプリケーションでSlideDeckのUUIDv4を生成する。
2. GoアプリケーションでPdfVersionのUUIDv4を生成する。
3. UUIDを使用してGarageのオブジェクトキーを決定する。
4. PDFをGarageへ保存する。
5. PostgreSQLへSlideDeck、PdfVersion、Owner権限を登録する。

失敗時:

- Garageへの保存に失敗した場合は、PostgreSQLへ何も登録しない。
- Garageへの保存成功後にPostgreSQLへの登録が失敗した場合は、アップロードしたPDFをGarageから削除する。
- PostgreSQLへのSlideDeck、PdfVersion、Owner権限の登録は1つのDBトランザクションで実行する。
- 補償処理としてのGarage削除にも失敗した場合はエラーを記録し、孤立オブジェクト清掃の対象とする。

## 4. URL設計

### 4.1 通常閲覧

```text
/decks/{slideDeckId}
/decks/{slideDeckId}?page={pageNumber}
```

- `slideDeckId` にはSlideDeckのUUIDv4を使用する。
- `page` を省略した場合は1枚目を表示する。
- `page` を指定した場合は、その番号のスライドを表示する。
- `page` が数値でない場合、1未満の場合、またはPDFのページ数を超える場合は1枚目を表示する。
- 不正または範囲外の `page` は、URLを `page=1` へ正規化する。
- 通常閲覧では、ユーザーごとに独立してスライドを移動する。

### 4.2 セッション

```text
/sessions/{sessionId}
```

- `sessionId` にはSessionのUUIDv4を使用する。
- セッションの参加・待機・開催中・終了後の表示には、同じURLを使用する。
- Sessionを再開した場合も同じURLを使用する。
- SlideDeckの通常閲覧URLとSessionのURLは分離する。
- Viewer、Operator、Ownerで同じセッションURLを使用する。
- 匿名ユーザーおよび対象SlideDeckの権限を持たないユーザーはViewerとして表示する。
- 対象SlideDeckのOperatorまたはOwnerとして認証されたユーザーには、セッション操作UIを表示する。
- スライド操作APIはサーバー側でもOperatorまたはOwnerの権限を検証する。
- Operator専用URLや秘密の操作用URLは発行しない。

## 5. リアルタイム同期

### 5.1 通信方式

- リアルタイム同期にはWebSocketを使用する。
- Viewer、Operator、Ownerは同じセッション用WebSocketエンドポイントへ接続する。
- 匿名Viewerの接続を許可する。
- 認証済みユーザーが接続した場合、サーバーは対象SlideDeckに対するOwner・Operator権限を判定する。
- Viewerは状態イベントの受信のみ可能とする。
- Owner・Operatorは状態イベントの受信に加えて、操作イベントを送信できる。
- WebSocket接続時に最新のセッション状態を送信する。
- 切断後に再接続した場合も、最新のセッション状態を送信する。

再接続:

- Viewer、Operator、Ownerのすべてで自動再接続を行う。
- 再接続間隔は、1秒、2秒、5秒、10秒と延長し、その後は最大30秒間隔とする。
- 再接続に成功するか、ユーザーが画面を離れるまで再試行する。
- 再接続成功後、サーバーから最新の `session_state` を受信する。
- クライアントは受信した状態を正として、現在表示しているスライドとセッション状態を更新する。
- Operatorが切断中に行ったローカル操作は、再接続後に再送しない。

### 5.2 スライド操作イベント

OperatorまたはOwnerが「前へ」「次へ」を操作した場合、クライアントは相対操作ではなく、移動先のスライド番号を明示して送信する。

```json
{
  "type": "set_slide",
  "slide": 3
}
```

サーバーの処理:

1. 送信者が対象SlideDeckのOperatorまたはOwnerであることを検証する。
2. Sessionが `active` であることを検証する。
3. `slide` が1以上かつ開催中のPdfVersionのページ数以下であることを検証する。
4. 検証に成功した場合、Sessionの `current_slide_number` を更新する。
5. `last_operated_at` を更新し、30分の無操作タイマーをリセットする。
6. 更新後の現在スライドを、接続中の全クライアントへ配信する。

### 5.3 競合

- サーバーへ最後に届いた有効な `set_slide` を採用する。
- `NEXT` や `PREVIOUS` のような相対操作イベントは使用しない。
- Viewerから送信された `set_slide` は拒否する。
- 無効なスライド番号は拒否し、Sessionの状態を変更しない。

### 5.4 同期状態

サーバーからクライアントへ、少なくとも以下の状態を配信する。

```json
{
  "type": "session_state",
  "status": "active",
  "slide": 3
}
```

- 初回接続時、途中参加時、再接続時に最新の `session_state` を送信する。
- スライド変更時に全接続クライアントへ最新の `session_state` を送信する。
- Sessionの開始、終了、再開時にも最新状態を送信する。

## 6. 認証セッション

### 6.1 ログイン

1. ユーザーがGoogleログインを開始する。
2. サーバーがOIDCの認可フローを実行する。
3. ID Tokenを検証し、`issuer` と `subject` からOidcIdentityを特定する。
4. 対応するUserを取得または新規作成する。
5. サーバー側にログインセッションを作成する。
6. ブラウザへセッションIDをCookieとして設定する。

### 6.2 セッションCookie

- CookieにはOIDCのID Tokenやアクセストークンを直接保存しない。
- Cookieには推測困難なサーバー側セッションIDだけを保存する。
- `HttpOnly` を有効にする。
- `Secure` を有効にする。
- `SameSite=Lax` を使用する。
- JavaScriptからセッションCookieを読み取れないようにする。
- 認証トークンを `localStorage` や `sessionStorage` に保存しない。

### 6.3 WebSocket認証

- WebSocket接続時に、HTTPリクエストと同じセッションCookieを使用する。
- 有効なログインセッションがある場合はUserを特定し、対象SlideDeckのOwner・Operator権限を判定する。
- Cookieがない、またはログインセッションが無効な場合は匿名Viewerとして接続する。
- 接続確立後も、操作イベントの受信時にOwner・Operator権限をサーバー側で検証する。

## 7. アプリケーション構成

### 7.1 構成方針

- MVPでは、Web画面、HTTP API、認証、PDF管理、セッション管理、WebSocketを同じアプリケーションとして実装・デプロイする。
- 初期段階ではリアルタイム同期専用サービスを分離しない。
- ログインセッションとWebSocket認証を同じアプリケーション内で共有する。

### 7.2 内部モジュール

同じアプリケーション内でも、責務ごとにコードを分離する。

- 認証・User管理
- スライド資料・PDF管理
- Owner・Operator権限管理
- セッション管理
- WebSocket・リアルタイム同期

将来、同時接続数や負荷の増加により必要になった場合、WebSocket・リアルタイム同期モジュールを別サービスへ分離できる構造を目指す。ただし、MVP時点では分離を前提とした過度な抽象化は行わない。

### 7.3 技術スタック

| 領域 | 採用技術 |
|---|---|
| フロントエンド | React + TypeScript |
| フロントエンドビルド | Vite |
| バックエンド | Go |
| HTTPルーター | chi |
| データベース | PostgreSQL |
| PostgreSQLドライバー | pgx |
| DBアクセスコード生成 | sqlc |
| DBマイグレーション | goose |
| 一時データ・Pub/Sub | Valkey |
| リアルタイム通信 | WebSocket（coder/websocket） |
| PDF保存 | Garage（S3互換オブジェクトストレージ） |
| S3クライアント | AWS SDK for Go v2 |

- フロントエンドとバックエンドは同じリポジトリで管理する。
- 開発時はReactとGoを別プロセスで実行する。
- 本番では、Reactのビルド成果物をGoアプリケーションから配信できる構成とする。
- SQLは明示的に記述し、sqlcで型安全なGoコードを生成する。
- ORMは使用しない。
- トランザクションとsqlcの生成コードからのDBアクセスにはpgxを使用する。
- スキーマ変更はgooseのマイグレーションファイルで管理する。
- HTTPルーティングとミドルウェア構成にはchiを使用する。
- WebSocket実装には `github.com/coder/websocket` を使用する。
- PDF保存にはGarageを使用し、GoからAWS SDK for Go v2でS3互換APIへアクセスする。
- Garage固有のAPIへ依存せず、エンドポイント、アクセスキー、シークレットキー、バケット、リージョンを設定で差し替えられるようにする。
- Garageの認証情報はバックエンドだけで保持し、ブラウザへ公開しない。

想定ディレクトリ:

```text
/apps/web       React + TypeScript
/apps/server    Go
/doc            要件・設計
```

### 7.4 PostgreSQLとValkey

MVP:

- User、OIDC Identity、SlideDeck、PdfVersion、権限、SessionをPostgreSQLへ保存する。
- ログインセッションはValkeyへ保存し、TTLで有効期限を管理する。
- セッションの状態、現在スライド、最終操作時刻をPostgreSQLへ保存する。
- セッション状態の更新通知をValkey Pub/Subへpublishする。
- 各Goアプリケーションは通知をsubscribeし、自身にWebSocket接続しているクライアントへ配信する。
- WebSocket接続そのものはGoプロセスのメモリ上で管理する。
- Valkeyは通知と一時データに使用し、スライドセッションの永続データの正にはしない。
- Pub/Subの通知を受信できなかった場合は、WebSocket再接続時にPostgreSQLから最新状態を取得して復旧する。

将来:

- Goアプリケーションを複数インスタンスに増やしても、Valkey Pub/SubでWebSocketイベントを共有する。
- 必要になった場合は、現在状態の読み取りキャッシュをValkeyへ追加する。
- PostgreSQLはUser、SlideDeck、権限、PDFメタデータなど永続データの正とし続ける。
- Valkey Streamsなどによる操作履歴の永続化はMVPに含めない。

## 8. 画面設計

### 8.1 ログイン後ホーム

ログイン後は、自分がOwnerまたはOperatorとして登録されているスライド資料の一覧を表示する。

表示内容:

- スライド資料のタイトル
- 自分の権限（OwnerまたはOperator）
- 最終更新日時
- スライド資料を開く操作

Owner向け操作:

- PDFをアップロードして新しいスライド資料を作成する。

方針:

- Viewer向けの公開資料一覧は作成しない。
- Viewerは通常閲覧URLまたはセッションURLから対象画面へ直接アクセスする。
- OwnerとOperatorが同じUserの場合、一覧上ではOwnerとして表示する。
- 一覧には削除済みのスライド資料を表示しない。

### 8.2 スライド資料詳細

OwnerまたはOperatorが、ログイン後ホームからスライド資料を選択したときに表示する。

共通表示:

- タイトル
- PDFのプレビュー
- ページ数
- 最終更新日時
- 自分の権限
- 通常閲覧を開く操作
- 新しいセッションを作成・開始する操作
- 既存セッションの一覧
- 既存セッションを開く・再開する操作

Ownerに表示する管理操作:

- タイトル変更
- PDF差し替え
- Owner管理
- Operator管理
- スライド資料削除

Operatorに表示する操作:

- 通常閲覧
- セッション作成・開始
- 既存セッションの確認・再開

権限制御:

- Operatorにはタイトル変更、PDF差し替え、権限管理、資料削除のUIを表示しない。
- 管理APIでもOwner権限を検証し、UIの非表示だけに依存しない。
- Viewerおよび権限を持たないログインユーザーは、この管理用詳細画面へアクセスできない。

## 9. 詳細設計への引き継ぎ

以下は [detailed-design.md](./detailed-design.md) で確定済みとする。

- SlideDeck削除時のDBレコードとPDFファイルの削除順序
- メールアドレス変更時のUser更新方法
- DB制約、API契約、WebSocketメッセージ、排他制御、定期処理

## 10. 設計決定ログ

| 日付 | 決定内容 |
|---|---|
| 2026-11-17 | 開催中のセッションが参照する旧PDFを保持し、参照する開催中セッションがなくなった時点で削除する |
| 2026-11-17 | セッション終了時にPDFバージョン参照を外し、過去PDFとの対応履歴を保持しない |
| 2026-11-17 | セッション開催履歴は保存せず、同じURLを再利用するための同期ルーム情報だけを保持する |
| 2026-11-17 | 通常閲覧は `/decks/{slideDeckId}`、セッションは `/sessions/{sessionId}` に分離する |
| 2026-11-19 | SlideDeckとSessionはそれぞれ独立したUUIDv4を持ち、そのUUIDを公開URLに使用する |
| 2026-11-17 | Viewer、Operator、Ownerは同じセッションURLを使用し、認証と権限で操作可否を切り替える |
| 2026-11-17 | 通常閲覧のページ番号が不正または範囲外の場合は1枚目を表示し、URLを `page=1` へ正規化する |
| 2026-11-17 | スライド操作は移動先を指定する `set_slide` とし、最後に届いた有効な操作を採用する |
| 2026-11-17 | リアルタイム同期にはWebSocketを使用し、全権限で同じ接続先を使用する |
| 2026-11-17 | WebSocket切断時は最大30秒間隔まで延長しながら自動再接続し、最新状態へ同期する |
| 2026-11-17 | OIDC認証後はサーバー側ログインセッションを作成し、安全なCookieで保持する |
| 2026-11-17 | WebSocket認証にはHTTPと同じセッションCookieを使用する |
| 2026-11-17 | MVPではWeb/APIとWebSocketを同じアプリケーションとして実装・デプロイする |
| 2026-11-17 | フロントエンドはReact + TypeScript、バックエンドはGoを使用する |
| 2026-11-17 | データベースにはPostgreSQLを使用し、PDFはS3互換ストレージへ保存する |
| 2026-11-20 | ログインセッションをValkeyへ保存し、WebSocket更新通知にValkey Pub/Subを使用する |
| 2026-11-17 | DBアクセスにはpgx + sqlcを使用し、ORMは使用しない |
| 2026-11-17 | DBマイグレーションにはgooseを使用する |
| 2026-11-17 | HTTPルーターにはchiを使用する |
| 2026-11-17 | WebSocket実装にはcoder/websocketを使用する |
| 2026-11-17 | PDF保存にはGarageを使用し、AWS SDK for Go v2からS3互換APIへアクセスする |
| 2026-11-19 | UUIDv4はGo側で生成し、DB主キーとGarageのオブジェクトキーに使用する |
| 2026-11-19 | Garage保存後にDB登録が失敗した場合は、保存済みPDFをGarageから削除する |
| 2026-11-17 | ログイン後ホームにはOwner・Operatorとして関係するスライド資料を一覧表示する |
| 2026-11-17 | Viewer向け公開一覧は作らず、Viewerは共有URLから直接アクセスする |
| 2026-11-17 | スライド資料詳細ではOwnerに管理操作、Operatorに閲覧・セッション運営操作だけを提供する |
