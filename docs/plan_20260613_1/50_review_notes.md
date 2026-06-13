# Review Notes

## Review Findings

- 実装前レビュー待ち。
- 旧 JSON field は Go の通常 unmarshal で無視できるため、破壊的 migration は不要。
- `selectedGenre` は Stable Audio 3 prompt に渡されているが、旧ファイル BGM由来の概念なので削除する。今後の prompt 分類は別途 preset / mood として設計する。

## Fixed Items

- なし。

## Deferred Items

- 生成設定の preset 化。
- Irodori 話者選択 UI。
- Stable Audio 3 prompt preset / mood UI。

## Rejected Options

- 旧 provider を fallback として残す: 今回の目的が「過去要素の削除」であり、設定項目から消すだけでは backend の複雑さが残るため採用しない。
- config migration version を追加する: 今回は旧 field を読み捨てれば十分で、明示 migration の保守コストに見合わないため採用しない。

## Current Non-Goals

- Gemini TTS の再導入。
- ファイル BGM 再生の再導入。
- ローカル生成が未設定の場合に外部 provider へ fallback すること。

## Future Plans

- 生成パラメータを初心者向け preset と詳細設定に分ける。
- local smoke test を通常 CI 相当の軽量検証に組み込む。

## Next Plan Candidates

- 生成設定 preset / profile 管理。
- モデル配置チェックと初回セットアップ UI。

## Documentation Feedback

- 実装後、`docs/requirement.md`、`docs/specification.md`、`README.md` から Gemini / file BGM の現行仕様を削除する。
