# HTTP Signature

サーバー間通信の認証。Inbox への POST と署名付き Actor fetch で使用。
ActivityPub 仕様自体には認証方式の規定はないが、fediverse では HTTP Signatures (draft-cavage-http-signatures) が標準。

> **凡例:** [仕様] = W3C/RFC/Draft の規定。[fediverse] = Mastodon/GoToSocial 等の実装で事実上必要な挙動。

## 署名生成

```
Signature: keyId="https://example.com/users/alice#main-key",
           algorithm="rsa-sha256",
           headers="(request-target) host date digest",
           signature="base64..."
```

**[仕様] Signature ヘッダーのパラメータ:**
- `keyId` (必須) — 署名検証に必要な鍵を特定する不透明な文字列
- `signature` (必須) — base64 エンコードされた署名値
- `algorithm` (推奨) — 署名文字列構築メカニズム。仕様では `hs2019` を推奨、`rsa-sha256` は deprecated 扱い
- `headers` (オプション) — 署名に含むヘッダーフィールドのリスト。未指定時は `(created)` がデフォルト
- `created` (推奨) — Unix タイムスタンプ形式の署名作成時刻
- `expires` (オプション) — 署名有効期限

**[仕様] 署名対象文字列の構築:**
- 各ヘッダー: `小文字のヘッダー名: 値` の形式
- 複数行は改行 `\n` で連結（最終行には改行なし）
- 先頭・末尾の空白は削除

**[仕様] `(request-target)` 疑似ヘッダー:**
- `小文字のメソッド + スペース + パス` で構成
- 例: `post /users/bob/inbox`

**[fediverse] 実際の運用:**
- `algorithm` は `rsa-sha256` が最も普及（仕様上は deprecated だが fediverse では標準）
- `headers` は `(request-target) host date` が事実上必須。POST には `digest` も含める
- `Digest` ヘッダー: リクエストボディの SHA-256 ハッシュ (`SHA-256=base64...`)。RFC 3230 準拠

## 署名検証の流れ

1. `Signature` ヘッダーをパース → `keyId` を取得
2. `keyId` の URI から Actor を fetch → `publicKey.publicKeyPem` を取得
3. `headers` で指定されたヘッダーから署名対象文字列を再構築
4. 公開鍵で `signature` を検証
5. POST の場合、`Digest` ヘッダーとボディの SHA-256 も検証

## 対応アルゴリズム

| アルゴリズム | 仕様上の位置づけ | fediverse での状況 |
|-----------|---------------|-----------------|
| hs2019 | 推奨 | GTS が送信時の algorithm フィールドに設定（実際の署名は RSA-SHA256） |
| rsa-sha256 | deprecated | Mastodon / Misskey が送信に使用。最も普及、事実上の標準 |
| RSA-SHA512 | - | GTS が検証対応 |
| Ed25519 | (仕様外) | GTS が検証対応 |
| RFC 9421 (HTTP Message Signatures) | (後継仕様) | Mastodon v4.5.0 でデフォルト検証対応 |

## 鍵ペア

- Actor 作成時に RSA-2048 鍵ペアを生成
- 秘密鍵は PEM 形式で DB に保存
- 公開鍵は Actor JSON の `publicKey.publicKeyPem` で公開
