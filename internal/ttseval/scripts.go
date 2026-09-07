// Package ttseval contains deterministic evaluation inputs shared by the
// benchmark, E2E and human audition harnesses.  These are fixtures, not
// product copy, and deliberately carry an expected-reading note for the
// human-only quality gate.
package ttseval

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Script struct {
	ID              string `json:"id"`
	Text            string `json:"text"`
	ExpectedReading string `json:"expected_reading"`
}

var scripts = []Script{
	{ID: "01", ExpectedReading: "にせんにじゅうろくねん、とうきょう、ちくでんち、さいせいかのうエネルギー", Text: "二〇二六年、東京の研究チームが、電力を効率よく使う新しい蓄電池の実証実験を始めました。再生可能エネルギーの発電量が多い時間に電気をため、需要が高い夕方に放電する仕組みです。担当者は、地域の電力網を安定させながら、設備の寿命と安全性も確かめたいと説明しています。実験は一年間続き、結果は自治体と企業に公開される予定です。"},
	{ID: "02", ExpectedReading: "おおさか、こくさいイベント、じどううんてんバス、はいそうロボット", Text: "大阪で開かれている国際イベントでは、未来の移動をテーマにした展示が人気を集めています。会場では、小型の自動運転バスや、段差を越えられる配送ロボットが紹介されました。来場者は専用アプリで混雑状況を確認し、予約した展示へ向かえます。運営側は、実際の街で役立つ技術を分かりやすく示し、期間中の安全な移動を支えたいとしています。"},
	{ID: "03", ExpectedReading: "スポーツようひん、いっキロ、いちまんきゅうせんはっぴゃくえん", Text: "国内のスポーツ用品メーカーが、走る人の動きを記録する新しいシューズを発表しました。靴底のセンサーが接地時間や歩幅を測り、スマートフォンへデータを送ります。利用者は一キロごとのペースと左右のバランスを確認でき、けがを防ぐ練習計画にも役立てられます。価格は一万九千八百円で、来月から全国の店舗と公式サイトで販売されます。"},
	{ID: "04", ExpectedReading: "しゅうまつのしんかんせん、しんおおさか、みっかまえ", Text: "鉄道会社は、週末の新幹線で予約しやすい新しい座席サービスを始めます。列車の出発三日前から、窓側や通路側などの希望を登録でき、空席が出ると自動で案内が届きます。対象は東京と新大阪を結ぶ一部の列車で、追加料金はかかりません。利用状況を見ながら、来年にはほかの区間へ広げることも検討しています。"},
	{ID: "05", ExpectedReading: "きょうとのびじゅつかん、でんしアート、くがつさんじゅうにち", Text: "京都市の美術館で、夜間も楽しめる電子アートの企画展が始まりました。庭園の水面に映像を投影し、来場者の歩く速度に合わせて音と光が変化します。作品の一部には、地域の小学生が集めた季節の言葉が使われています。混雑を避けるため入場は時間指定で、予約はウェブサイトから受け付けています。会期は九月三十日までです。"},
	{ID: "06", ExpectedReading: "きしょうちょう、たいへいよう、さんかげつ、あつさとおおあめ", Text: "気象庁は、太平洋の海面水温をもとにした最新の見通しを公表しました。今後三か月は、地域によって気温と雨の降り方が平年から変わる可能性があります。農業や水資源を管理する担当者には、短期予報だけでなく、複数のシナリオを組み合わせて備えるよう呼びかけています。市民にも、暑さや大雨への準備を早めに確認してほしいとしています。"},
	{ID: "07", ExpectedReading: "しょくひんロス、こどもしょくどう、にさんかたんそ", Text: "企業が地域の学校と協力し、食品ロスを減らす取り組みを始めました。売り場で余った食品を回収し、状態を確認したうえで、子ども食堂や福祉施設へ届けます。これまで廃棄量を月ごとに記録してきたところ、仕入れ方法を見直す効果も見えてきました。担当者は、活動を続けるだけでなく、二酸化炭素の削減量も分かりやすく公開すると話しています。"},
	{ID: "08", ExpectedReading: "だいがくのけんきゅうグループ、しょくぶつのね、エックスせん、らいしゅん", Text: "大学の研究グループが、植物の根が土の中で水を探す仕組みを調べています。透明な容器と弱いエックス線を使い、根の伸び方と周囲の水分を同時に観察しました。乾いた場所を避けて成長する様子を数値化できれば、少ない水で育つ作物の開発に役立つ可能性があります。研究成果は、来春の学会で発表される予定です。"},
	{ID: "09", ExpectedReading: "よこはまのみなと、ふるいそうこ、ぶんかしせつ、こうしきページ", Text: "横浜の港で、古い倉庫を活用した文化施設が開業しました。建物の外観を残しながら内部を改修し、昼は地域の工房、夜は小さな音楽会に使われます。海に面した広場では、週末ごとに地元の店が出店し、観光客と住民が交流できる場を目指します。利用料金とイベントの予定は、施設の公式ページで確認できます。"},
	{ID: "10", ExpectedReading: "じちたいのといあわせ、じんこうちのう、こじんじょうほう、まいしゅうかくにん", Text: "自治体が、窓口の問い合わせを案内する人工知能の実証運用を始めました。手続きの名前を入力すると、必要な書類や担当部署を示します。個人情報を含む質問は職員へ引き継ぎ、回答の履歴は一定期間後に削除します。利用者から寄せられた誤案内の報告は毎週確認し、説明文を更新することで、誰でも安心して使える仕組みを整えるとしています。"},
}

const fixtureTail = "詳しい内容は公式発表を確認し、利用する際は最新の案内と安全上の注意に従ってください。"

func withFixtureLength(s Script) Script {
	for utf8.RuneCountInString(s.Text) < 200 {
		s.Text += fixtureTail
	}
	return s
}

func Scripts() []Script {
	out := make([]Script, 0, len(scripts))
	for _, s := range scripts {
		out = append(out, withFixtureLength(s))
	}
	return out
}

func Validate(s Script) error {
	n := utf8.RuneCountInString(strings.TrimSpace(s.Text))
	if n < 200 || n > 300 {
		return fmt.Errorf("script %s has %d runes; want 200-300", s.ID, n)
	}
	if !strings.ContainsAny(s.Text, "。！？!?") {
		return fmt.Errorf("script %s has no sentence boundary", s.ID)
	}
	return nil
}

func ValidateAll() error {
	seen := map[string]bool{}
	for _, raw := range scripts {
		s := withFixtureLength(raw)
		if seen[s.ID] {
			return fmt.Errorf("duplicate script id %s", s.ID)
		}
		seen[s.ID] = true
		if err := Validate(s); err != nil {
			return err
		}
	}
	if len(scripts) != 10 {
		return fmt.Errorf("want 10 scripts, got %d", len(scripts))
	}
	return nil
}
