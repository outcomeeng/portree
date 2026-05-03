# portree - Git Worktree Server Manager

[![CI](https://github.com/fairy-pitta/portree/actions/workflows/ci.yaml/badge.svg)](https://github.com/fairy-pitta/portree/actions/workflows/ci.yaml)
[![codecov](https://codecov.io/gh/fairy-pitta/portree/branch/main/graph/badge.svg)](https://codecov.io/gh/fairy-pitta/portree)
[![Go Report Card](https://goreportcard.com/badge/github.com/fairy-pitta/portree)](https://goreportcard.com/report/github.com/fairy-pitta/portree)
[![Go Reference](https://pkg.go.dev/badge/github.com/fairy-pitta/portree.svg)](https://pkg.go.dev/github.com/fairy-pitta/portree)
![Go Version](https://img.shields.io/github/go-mod/go-version/fairy-pitta/portree)

**portree** は [git worktree](https://git-scm.com/docs/git-worktree) ごとに複数の dev server を自動管理する CLI ツールです。ポートの自動割り当て、環境変数の自動注入、`*.localhost` サブドメインルーティングによるリバースプロキシを提供します。

> English version: [README.md](./README.md)

---

## v0.2.x からのアップグレード — 破壊的変更

> [!WARNING]
> **v0.3.0 以降、同一リポジトリのすべての worktree は 1 つの portree インスタンスを共有します。** 状態ファイル (`.portree/state.json`) と設定ファイル (`.portree.toml`) は、呼び出し元の worktree ではなく **メイン worktree のルート** (`git rev-parse --git-common-dir` で解決) を起点に読み書きされます。これは複数開発者・複数エージェントのワークフローを正しく機能させるためのアーキテクチャ変更ですが、**同一リポジトリの異なる worktree 内で独立した portree インスタンスを並行運用すること** はできなくなりました。
>
> 以前のバージョンで worktree ごとの分離に依存していた場合(例: 開発者 A が feature ブランチで自身の portree を、開発者 B が `main` で別の portree を動かすような構成)、その運用は不可能になります。共有状態が新しいモデルです。
>
> **移行手順:**
>
> - リンク worktree 配下に残っている `.portree/` ディレクトリは削除してください — アップグレード後は無視されます。
> - feature ブランチにのみ存在しメイン worktree には存在しない `.portree.toml` は黙って無視されます。設定ファイルはメイン worktree でチェックアウトされているブランチ(または `main` に直接コミット)に配置してください。
> - `portree up` を再実行してください。状態は新しい正準位置に書き込まれます。
>
> 詳細は [ADR-15](./spx/15-multi-worktree-state.adr.md) を参照してください。

---

## デモ

![portree workflow demo](./demo/demo-workflow.gif)

---

## 特徴

- **マルチサービス** — フロントエンド、バックエンド、任意の数のサービスを worktree ごとに定義
- **ポート自動割り当て** — ハッシュベース (FNV32) のポート割り当て。worktree 間のポート衝突なし
- **共有マルチ worktree 状態** — メイン worktree ルートに 1 つの正準状態ファイル。すべての worktree(および開発者・エージェント)が同じビューを共有
- **冪等な `portree up`** — 別の worktree から再度 `up` を実行しても、既に動いているサービスは維持される
- **プロキシの自動ライフサイクル** — `up --ensure-proxy` でプロキシをバックグラウンド起動。`down --release-proxy` で他に必要な worktree がない場合のみ停止
- **サブドメインリバースプロキシ** — `branch-name.localhost:<port>` で任意の worktree にアクセス (`/etc/hosts` の編集不要)
- **HTTPS プロキシ** — 自動生成証明書またはカスタム証明書による HTTPS 対応。Secure Cookie や Service Worker が必要なローカル開発に
- **到達性インジケーター** — `portree ls` が各プロキシ URL を probe し、上流が応答しているかを表示
- **環境変数の自動注入** — `$PORT`、`$PT_BRANCH`、`$PT_BACKEND_URL` 等を自動設定
- **TUI ダッシュボード** — ターミナル上のインタラクティブ UI でサービスの起動・停止・監視
- **プロセスライフサイクル管理** — グレースフルシャットダウン (SIGTERM → SIGKILL)、ログファイル、古い PID の自動クリーンアップ
- **孤立ポートのクリーンアップ** — `portree reset` で worktree の割り当てポートを保持しているプロセスを強制終了
- **worktree ごとのオーバーライド** — ブランチ別にコマンド、ポート、環境変数をカスタマイズ
- **AI エージェント対応** — `portree ls --json` で `url`、`direct_url`、`reachable` を含む JSON 出力。エンドポイントの自動発見に対応

---

## クイックスタート

### 1. インストール

![Install demo](./demo/demo-install.gif)

```bash
# Homebrew
brew install fairy-pitta/tap/portree

# Go install
go install github.com/fairy-pitta/portree@latest

# またはソースからビルド
git clone https://github.com/fairy-pitta/portree.git
cd portree
make build
```

### 2. 初期化

![Init demo](./demo/demo-init.gif)

```bash
cd your-project
portree init
# リポジトリルートに .portree.toml を作成
```

### 3. 設定

`.portree.toml` をプロジェクトに合わせて編集:

```toml
[services.frontend]
command = "pnpm run dev"
dir = "frontend"
port_range = { min = 3100, max = 3199 }
proxy_port = 3000

[services.backend]
command = "source .venv/bin/activate && python manage.py runserver 0.0.0.0:$PORT"
dir = "backend"
port_range = { min = 8100, max = 8199 }
proxy_port = 8000

[env]
NODE_ENV = "development"
```

### 4. サービスとプロキシを起動

```bash
# 推奨: 現在の worktree のサービスを起動し、共有プロキシも(必要なら)起動する
portree up --ensure-proxy
# HTTPS で起動する場合は --https を追加:
portree up --ensure-proxy --https

# その他の起動方法
portree up                 # 現在の worktree のサービスのみ(プロキシは別途起動済みである必要)
portree up --all           # 全 worktree のサービスを一括起動
```

プロキシは worktree 間で共有されます。`--ensure-proxy` は冪等で、既に起動していれば何もしません。プロキシを「他に使用中の worktree がなくなった時だけ自動で停止」させたい場合は `portree down --release-proxy` を使ってください。

プロキシを手動管理したい場合は、`portree proxy start` でフォアグラウンド起動、`portree proxy stop` で停止できます。

### 5. ブラウザで開く

```bash
portree open                    # http://main.localhost:3000 を開く
portree open --service backend  # http://main.localhost:8000 を開く
```

---

## コマンド一覧

| コマンド                            | 説明                                                                       |
| ----------------------------------- | -------------------------------------------------------------------------- |
| `portree init`                      | `.portree.toml` 設定ファイルを作成                                         |
| `portree up`                        | 現在の worktree のサービスを起動 (冪等 — 既に動いているサービスはそのまま) |
| `portree up --all`                  | すべての worktree のサービスを起動                                         |
| `portree up --service <name>`       | 指定したサービスのみ起動                                                   |
| `portree up --ensure-proxy`         | 共有プロキシも(まだ起動していなければ)バックグラウンド起動                 |
| `portree up --ensure-proxy --https` | …HTTPS で起動(自動生成証明書)                                              |
| `portree down`                      | 現在の worktree のサービスを停止                                           |
| `portree down --all`                | すべての worktree のサービスを停止                                         |
| `portree down --service <name>`     | 指定したサービスのみ停止                                                   |
| `portree down --prune`              | 孤立したエントリと stale エントリを除去(動作中のプロセスには触れない)      |
| `portree down --release-proxy`      | 他の worktree がまだサービスを動かしていない場合のみ共有プロキシを停止     |
| `portree ls`                        | worktree、サービス、ポート、状態、PID、プロキシ URL(到達性付き)を一覧表示  |
| `portree ls --json`                 | 同じ内容を JSON で出力(`url`、`direct_url`、`reachable` を含む)            |
| `portree reset`                     | 現在の worktree の割り当てポートを保持しているプロセスを強制終了           |
| `portree reset --all`               | 全 worktree について同じ処理を実行                                         |
| `portree reset --proxy-port`        | プロキシポートを保持している portree 以外のリスナーも除去                  |
| `portree dash`                      | インタラクティブ TUI ダッシュボードを起動                                  |
| `portree proxy start`               | リバースプロキシを起動 (フォアグラウンド)                                  |
| `portree proxy start --https`       | …HTTPS で起動(自動生成証明書)                                              |
| `portree proxy stop`                | リバースプロキシを停止                                                     |
| `portree trust`                     | CA 証明書をシステム信頼ストアにインストール                                |
| `portree open`                      | 現在の worktree をブラウザで開く                                           |
| `portree doctor`                    | 設定、ポート、状態の診断チェックを実行                                     |
| `portree version`                   | バージョン情報を表示                                                       |

---

## 設定リファレンス

`.portree.toml` は git リポジトリのルートに配置します。

### `[services.<name>]`

1 つ以上のサービスを定義します。各 worktree で定義された全サービスが起動されます。

| フィールド   | 型           | 必須   | 説明                                               |
| ------------ | ------------ | ------ | -------------------------------------------------- |
| `command`    | string       | はい   | サービスを起動するシェルコマンド                   |
| `dir`        | string       | いいえ | worktree ルートからの相対パス (デフォルト: ルート) |
| `port_range` | `{min, max}` | はい   | このサービスのポート割り当て範囲                   |
| `proxy_port` | int          | はい   | リバースプロキシがリッスンするポート               |

```toml
[services.frontend]
command = "pnpm run dev"
dir = "frontend"
port_range = { min = 3100, max = 3199 }
proxy_port = 3000
```

### `[env]`

全サービスに注入されるグローバル環境変数。

```toml
[env]
NODE_ENV = "development"
DATABASE_URL = "postgres://localhost/mydb"
```

### `[worktrees."<branch>"]`

worktree ごとのオーバーライド。コマンド、固定ポート、追加環境変数をカスタマイズできます。

```toml
[worktrees.main]
services.frontend.port = 3100 # main ブランチのポートを固定

[worktrees."feature/auth"]
services.backend.command = "python manage.py runserver --settings=myapp.auth 0.0.0.0:$PORT"
services.backend.env = { DEBUG = "1" }
```

---

## 環境変数

portree は以下の環境変数を全サービスプロセスに自動注入します:

| 変数                | 例                                                  | 説明                                     |
| ------------------- | --------------------------------------------------- | ---------------------------------------- |
| `PORT`              | `3117`                                              | このサービスの割り当てポート             |
| `PT_BRANCH`         | `feature/auth`                                      | 現在のブランチ名                         |
| `PT_BRANCH_SLUG`    | `feature-auth`                                      | ブランチ名の URL-safe スラッグ           |
| `PT_SERVICE`        | `frontend`                                          | 現在のサービス名                         |
| `PT_<SERVICE>_PORT` | `PT_FRONTEND_PORT=3117`                             | 同一 worktree の各サービスのポート       |
| `PT_<SERVICE>_URL`  | `PT_BACKEND_URL=http://feature-auth.localhost:8000` | 同一 worktree の各サービスのプロキシ URL |

これにより、サービス間の通信設定を自動解決できます:

```js
// next.config.js
module.exports = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${process.env.PT_BACKEND_URL}/api/:path*`,
      },
    ];
  },
};
```

---

## 仕組み

```
┌─────────────────────────────────────────────────────────────┐
│  git リポジトリ                                              │
│                                                             │
│  main worktree          feature/auth worktree               │
│  ┌───────────────┐      ┌───────────────┐                   │
│  │ frontend :3100│      │ frontend :3117│                   │
│  │ backend  :8100│      │ backend  :8104│                   │
│  └───────────────┘      └───────────────┘                   │
│         │                      │                            │
└─────────┼──────────────────────┼────────────────────────────┘
          │                      │
    ┌─────▼──────────────────────▼─────┐
    │     portree リバースプロキシ      │
    │                                  │
    │  :3000  ←  *.localhost:3000      │
    │  :8000  ←  *.localhost:8000      │
    └──────────────────────────────────┘
          │                      │
          ▼                      ▼
  main.localhost:3000    feature-auth.localhost:3000
  main.localhost:8000    feature-auth.localhost:8000
```

1. **ポート割り当て** — `FNV32(branch:service) % range` でポートを決定。再起動しても安定。
2. **プロセス管理** — サービスはプロセスグループ付きの子プロセスとして実行。ログは `.portree/logs/` に出力。
3. **リバースプロキシ** — `proxy_port` ごとに HTTP リスナーを起動。`Host` ヘッダーのサブドメインでルーティング。
4. **`*.localhost`** — [RFC 6761](https://tools.ietf.org/html/rfc6761) により、モダンブラウザは `*.localhost` を `127.0.0.1` に自動解決。DNS 設定不要。

---

## TUI ダッシュボード

![TUI Dashboard demo](./demo/demo-tui.gif)

`portree dash` で起動:

```
╭─ portree dashboard ──────────────────────────────────────────╮
│                                                               │
│  WORKTREE        SERVICE    PORT   STATUS      PID            │
│  ──────────────────────────────────────────────────────────── │
│ ▸ main           frontend   3100   ● running   12345          │
│   main           backend    8100   ● running   12346          │
│   feature/auth   frontend   3117   ○ stopped   —              │
│   feature/auth   backend    8104   ○ stopped   —              │
│                                                               │
│  Proxy: ● running (:3000, :8000)                              │
│                                                               │
│  [s] start  [x] stop  [r] restart  [o] open in browser       │
│  [a] start all  [X] stop all  [p] toggle proxy                │
│  [l] view logs  [q] quit                                      │
╰───────────────────────────────────────────────────────────────╯
```

**キーバインド:**

| キー    | 操作                     |
| ------- | ------------------------ |
| `j`/`k` | カーソル移動 (下/上)     |
| `s`     | 選択中のサービスを起動   |
| `x`     | 選択中のサービスを停止   |
| `r`     | 選択中のサービスを再起動 |
| `o`     | ブラウザで開く           |
| `a`     | 全サービス起動           |
| `X`     | 全サービス停止           |
| `p`     | プロキシの切り替え       |
| `l`     | ログファイルパスを表示   |
| `q`     | 終了                     |

---

## 使用例

```bash
# フロントエンド + バックエンドのモノレポで作業中
cd my-project

# portree を初期化
portree init
# .portree.toml を編集してサービスを定義...

# フィーチャーブランチの worktree を作成
git worktree add ../my-project-feature-auth feature/auth

# 現在のブランチのサービスを起動
portree up
# Starting frontend (port 3100) for main ...
# Starting backend (port 8100) for main ...
# ✓ 2 services started for main

# 全 worktree のサービスを一括起動
portree up --all
# ✓ 4 services started

# 状態確認
portree ls
# WORKTREE        SERVICE    PORT   STATUS    PID
# main            frontend   3100   running   12345
# main            backend    8100   running   12346
# feature/auth    frontend   3117   running   12347
# feature/auth    backend    8104   running   12348

# JSON 出力 (AI エージェントやスクリプトに最適)
portree ls --json
# [{"worktree":"main","service":"frontend","port":3100,"status":"running",
#   "pid":12345,"url":"http://main.localhost:3000","direct_url":"http://localhost:3100"}, ...]

# プロキシ起動
portree proxy start
# アクセス:
#   http://main.localhost:3000          → frontend (main)
#   http://main.localhost:8000          → backend (main)
#   http://feature-auth.localhost:3000  → frontend (feature/auth)
#   http://feature-auth.localhost:8000  → backend (feature/auth)

# HTTPS が必要な場合
portree proxy start --https
# 自動生成証明書で HTTPS プロキシを起動
# https://main.localhost:3000 でアクセス

# CA 証明書をシステムに信頼させる (ブラウザ警告を解消)
portree trust

# ブラウザで開く
portree open
# Opening http://main.localhost:3000 ...

# TUI を使う
portree dash

# 終了時
portree down --all
# ✓ 4 services stopped
```

---

## シェル補完

portree は bash、zsh、fish、PowerShell のシェル補完をサポートしています。

**bash:**

```bash
source <(portree completion bash)
# 永続化する場合:
portree completion bash > /etc/bash_completion.d/portree
```

**zsh:**

```bash
portree completion zsh > "${fpath[1]}/_portree"
# 新しいシェルを開くと有効になります。
```

**fish:**

```bash
portree completion fish | source
# 永続化する場合:
portree completion fish > ~/.config/fish/completions/portree.fish
```

**PowerShell:**

```powershell
portree completion powershell | Out-String | Invoke-Expression
# 永続化する場合:
portree completion powershell > portree.ps1
# PowerShell プロファイルに ". portree.ps1" を追加してください。
```

---

## トラブルシューティング

### サービスが起動しない

- `.portree/logs/<branch-slug>.<service>.log` のログファイルでエラー出力を確認してください。
- `.portree.toml` の `command` を手動で実行して正しく動作するか確認してください。
- `dir` で指定したディレクトリが worktree ルートからの相対パスとして存在するか確認してください。

### ポート競合

- `portree doctor` を実行してポート競合を検出してください。
- ポートが使用中の場合、portree は linear probing で範囲内の次の空きポートを探します。
- 範囲全体が使い切られた場合は、`.portree.toml` の `port_range` を広げてください。
- 前回クラッシュした `next dev` などの古い dev サーバーが worktree の割り当てポートを保持している場合、`portree reset` でそのプロセスを強制終了できます。プロキシポートに残っているリスナーも除去するには `portree reset --proxy-port` を使ってください。

### 古いプロセス (stale process)

- `portree doctor` を実行して state ファイル内の古い PID を検出してください。出力には `portree down --prune` という解決コマンドが提示されます。
- `portree down --prune` は stale エントリ(状態は `running` だが PID が既に終了)と孤立した worktree エントリを除去します。動作中のプロセスにはシグナルを送らないため、他の worktree への影響はありません。`--all` と組み合わせれば、同じ呼び出しで全 worktree のサービスも停止できます。
- `portree reset` はより強力な手段で、現在の worktree の割り当てポートを保持しているプロセスを状態に関わらず強制終了します。

### プロキシが正しくルーティングしない

- `portree proxy start` でプロキシが起動しているか確認してください。
- ブラウザが `*.localhost` を解決できるか確認してください。モダンブラウザは RFC 6761 に従い自動解決します。
- 対象サービスが起動しているか `portree ls` で確認してください。
- プロキシは `Host` ヘッダーのサブドメインでルーティングするため、`http://<branch-slug>.localhost:<proxy_port>` でアクセスしてください。

### HTTPS 関連

- `portree proxy start --https` で自動生成された証明書は `.portree/certs/` に保存されます。
- ブラウザの証明書警告を解消するには `portree trust` で CA 証明書をシステムにインストールしてください。
- カスタム証明書を使う場合は `portree proxy start --cert <path> --key <path>` を指定してください。

---

## プラットフォームサポート

| プラットフォーム | ステータス | 備考                                                                           |
| ---------------- | ---------- | ------------------------------------------------------------------------------ |
| **macOS**        | 完全対応   | 主要開発プラットフォーム                                                       |
| **Linux**        | 完全対応   | Ubuntu, Debian, Fedora でテスト済み                                            |
| **Windows**      | 実験的     | 基本機能は動作。ファイルロックは代替実装を使用。問題があれば報告をお願いします |

---

## FAQ

### `*.localhost` は全てのブラウザで動きますか？

Chrome、Firefox、Edge、Safari などのモダンブラウザは [RFC 6761](https://tools.ietf.org/html/rfc6761) に従い `*.localhost` を `127.0.0.1` に解決します。`/etc/hosts` の編集や DNS 設定は不要です。

### 2 つの worktree が同じポートにハッシュされた場合は？

portree は linear probing を使用します。ハッシュで決まったポートが使用中の場合、範囲内の次の空きポートを探します。

### プロキシなしで使えますか？

はい。`portree up` でサービスを起動すれば、`localhost:<port>` で直接アクセスできます。プロキシはオプションです。

### ログはどこに保存されますか？

サービスのログは main worktree のルート配下の `.portree/logs/<branch-slug>.<service>.log` に書き出されます。

### 状態はどこに保存されますか？

ランタイム状態(PID、ポート割り当て、プロキシ状態)はリポジトリにつき 1 つの `.portree/state.json` に保存されます。場所は **メイン worktree のルート** (`git rev-parse --git-common-dir` で解決) で、すべての worktree がこのファイルを読み書きします。同時アクセスは `flock` ベースのロックで保護されています。リンク worktree 配下の `.portree/` ディレクトリは参照されません — 詳しくは [破壊的変更の説明](#v02x-からのアップグレード--破壊的変更) を参照してください。

### 同じリポジトリの worktree 内で別々の portree インスタンスを並行運用できますか？

できません(v0.3.0 以降)。各 git リポジトリには 1 つの portree 状態しかなく、すべての worktree がそれを共有します。これは複数 worktree でのワークフローを正しく動かすための意図的な変更です — 詳細は [ADR-15](./spx/15-multi-worktree-state.adr.md) を参照してください。完全に独立した portree インスタンスが必要な場合は、リポジトリ自体を別々にクローンしてください。

### ブランチごとに異なるコマンドを実行できますか？

はい。`.portree.toml` の `[worktrees."branch-name"]` でオーバーライドできます:

```toml
[worktrees."feature/auth"]
services.backend.command = "python manage.py runserver --settings=auth 0.0.0.0:$PORT"
services.backend.env = { DEBUG = "1" }
```

---

## プロジェクト構造

```
portree/
├── main.go                      # エントリーポイント
├── cmd/                         # CLI コマンド (cobra)
│   ├── root.go                  # ルートコマンド + リポジトリ/設定検出
│   ├── init.go                  # portree init
│   ├── up.go                    # portree up
│   ├── down.go                  # portree down
│   ├── ls.go                    # portree ls
│   ├── dash.go                  # portree dash
│   ├── proxy.go                 # portree proxy start|stop
│   ├── trust.go                 # portree trust
│   ├── open.go                  # portree open
│   └── version.go               # portree version
├── internal/
│   ├── cert/cert.go             # CA + サーバー証明書の自動生成
│   ├── config/config.go         # .portree.toml の読み込みとバリデーション
│   ├── git/
│   │   ├── repo.go              # リポジトリルート / common dir 検出
│   │   └── worktree.go          # worktree 一覧とブランチスラッグ
│   ├── state/store.go           # flock 付き JSON 状態永続化
│   ├── port/
│   │   ├── allocator.go         # FNV32 ハッシュベースのポート割り当て
│   │   └── registry.go          # ポート割り当て管理
│   ├── process/
│   │   ├── runner.go            # 単一プロセスのライフサイクル
│   │   └── manager.go           # マルチサービスオーケストレーション
│   ├── proxy/
│   │   ├── resolver.go          # スラッグ + ポート → バックエンド解決
│   │   └── server.go            # HTTP/HTTPS リバースプロキシ
│   ├── browser/open.go          # OS 対応のブラウザ起動
│   └── tui/                     # Bubble Tea TUI ダッシュボード
│       ├── app.go               # トップレベルモデル
│       ├── dashboard.go         # テーブルレンダリング
│       ├── keys.go              # キーバインド
│       ├── messages.go          # カスタムメッセージ
│       └── styles.go            # Lip Gloss スタイル
├── Makefile
├── .goreleaser.yaml
└── .github/workflows/
    ├── ci.yaml
    └── release.yaml
```

---

## コントリビューション

1. リポジトリをフォーク
2. フィーチャーブランチを作成 (`git checkout -b feature/amazing`)
3. 変更をコミット (`git commit -m 'feat: add amazing feature'`)
4. ブランチをプッシュ (`git push origin feature/amazing`)
5. Pull Request を作成

```bash
# 開発
make build      # バイナリをビルド
make test       # レースディテクタ付きでテスト実行
make lint       # golangci-lint を実行
make all        # fmt + vet + lint + test + build
```

---

## ライセンス

MIT License。詳細は [LICENSE](./LICENSE) を参照してください。
