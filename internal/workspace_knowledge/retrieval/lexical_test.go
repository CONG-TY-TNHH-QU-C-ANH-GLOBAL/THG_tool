package retrieval

import (
	"reflect"
	"testing"
)

func TestNormalizeLexicalVietnameseAndUnicode(t *testing.T) {
	for _, item := range []struct{ input, want string }{
		{"DỊCH VỤ KHO MỸ", "dich vu kho my"},
		{"di\u0323ch vu\u0323 kho My\u0303", "dich vu kho my"},
		{"Đóng-gói / đổi trả", "dong goi doi tra"},
		{" Áo   POD\n1688 ", "ao pod 1688"},
		{"仓库 产品", "仓库 产品"},
		{"が", "が"}, // Non-Latin combining marks must retain meaning.
		{"T-shirt M40", "t shirt m40"},
	} {
		if got := NormalizeLexical(item.input); got != item.want {
			t.Errorf("NormalizeLexical(%q) = %q; want %q", item.input, got, item.want)
		}
	}
	if !reflect.DeepEqual(LexicalTokens("dịch vụ kho Mỹ"), LexicalTokens("dich vu kho My")) {
		t.Fatal("accented and unaccented queries produced different terms")
	}
	if !reflect.DeepEqual(LexicalTokens("a é ỹ 仓 产品"), map[string]struct{}{"产品": {}}) {
		t.Fatal("word length must use runes, not UTF-8 byte length")
	}
}

func TestLegacyTokenizeReplayUnchanged(t *testing.T) {
	want := map[string]struct{}{"ch": {}, "kho": {}}
	if got := Tokenize("dịch vụ kho Mỹ"); !reflect.DeepEqual(got, want) {
		t.Fatalf("v1 replay semantics changed: got %#v, want %#v", got, want)
	}
}
