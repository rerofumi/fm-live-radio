# Review Checklist

## Security

- [ ] Gemini API key field が config / UI / docs から消えている。
- [ ] LLM API key の扱いは既存と同等で、ログ出力が増えていない。

## Frontend

- [ ] Settings に旧 provider 選択肢と旧 provider 設定が表示されない。
- [ ] 「アプリ機能」と「生成設定」が分かれている。
- [ ] 「生成設定」は初期状態で閉じている。
- [ ] Genre select が main console から消えている。
- [ ] モバイル幅で modal の field が破綻しない。

## Backend

- [ ] AppConfig から旧 field が削除されている。
- [ ] Player は Stable Audio 3 BGM のみを選ぶ。
- [ ] Talk は IrodoriTTS のみを使う。
- [ ] `ScanGenres` API が削除され、参照が残っていない。

## DB / Storage

- [ ] 旧 config JSON を読み込んでもエラーにならない。
- [ ] 保存後の config JSON に旧 field が含まれない。

## QA / Test

- [ ] `mise x -- go test ./...` が成功する。
- [ ] `mise x -- npm --prefix frontend run build` が成功する。
- [ ] 必要なら `mise x -- wails generate module` が成功する。

## DevOps / Environment

- [ ] コマンドは `mise` 経由で実行されている。
- [ ] VCS 状態確認は `.jj` に従って `jj` を使っている。

## Pre-Implementation Research

- [ ] 外部 API / SDK の新規前提がない。
- [ ] Wails binding 更新方法が確認されている。

## Traceability

- [ ] `10_claim.md` の目的が `30_requirement.md` と `40_specification.md` に反映されている。
- [ ] 実装後に current docs / README が更新されている。
