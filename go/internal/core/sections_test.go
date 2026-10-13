package core

import (
	"strings"
	"testing"
)

const layered = "Giris.\n\n## Geçerli\n\nAynı seviye serbest.\n\n### Ayrinti\n\nmin engel.\n\n" +
	"## Tarihçe\n\n- 2026-10-04: aynı seviye ENGEL (artis_yok)\n\n### eski alt baslik\n\neski govde\n\n" +
	"## Kod\n\n`app/x.py`\n"

func TestCurrentBodyDropsHistory(t *testing.T) {
	got := CurrentBody(layered)
	for _, gone := range []string{"artis_yok", "eski govde", "## Tarihçe"} {
		if strings.Contains(got, gone) {
			t.Errorf("tarihce kalmamali (%q):\n%s", gone, got)
		}
	}
	for _, kept := range []string{"Aynı seviye serbest", "min engel", "## Kod", "app/x.py", "Giris"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q kalmali:\n%s", kept, got)
		}
	}
	// Başlık katlanarak eşleşir: büyük harf, aksansız ve İngilizce.
	for _, h := range []string{"## TARİHÇE", "## Tarihce (eski kararlar)", "## History"} {
		if strings.Contains(CurrentBody("a\n"+h+"\nx-gizli\n## Sonra\ny\n"), "x-gizli") {
			t.Errorf("%q tarihce sayilmali", h)
		}
	}
	// Seviye 3 "Tarihçe" bir alt başlıktır, bölüm değil.
	if !strings.Contains(CurrentBody("## A\n### Tarihçe\nkalir\n"), "kalir") {
		t.Error("### Tarihçe bolum sayilmamali")
	}
	if body := "duz not\n"; CurrentBody(body) != body {
		t.Error("bolumsuz not aynen donmeli")
	}
}

func TestHasCurrentSectionAndHeadings(t *testing.T) {
	if !HasCurrentSection(layered) || HasCurrentSection("## Kod\nx\n") {
		t.Error("Geçerli bolumu tanima hatali")
	}
	if !HasCurrentSection("## Current\nx\n") || !HasCurrentSection("## GECERLI DURUM\nx\n") {
		t.Error("katlanmis/ingilizce Geçerli tanınmalı")
	}
	h := strings.Join(Headings(layered), "|")
	if h != "Geçerli|Ayrinti|Kod" {
		t.Errorf("basliklar (tarihce haric) = %q", h)
	}
}
