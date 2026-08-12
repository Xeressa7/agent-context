"""Arama katmani: Turkce-guvenli katlama, alan agirliklari, VEYA mantigi.

Bu testlerin cogu 2026-08-12'de ELLE dogrulanmis davranislari kilitler; o turda
olculmus iki ariza vardi ve ikisi de burada regresyon olarak duruyor:
  1. VE elemesi — "tenant izolasyonu" sorgusu dogru notu getirmiyordu
  2. ciplak .lower() ile Turkce metin (I/i) yanlis katlaniyordu
"""
from pathlib import Path


def puanla(ctx, goreli, fm, govde, sorgu):
    """_not_puanla'yi sorgu metniyle cagirmak icin ince sarmalayici."""
    kelimeler = [ctx.indir(k) for k in sorgu.split() if k.strip()]
    ifade = ctx.indir(" ".join(sorgu.split()))
    metin = "---\n---\n" + govde
    return ctx._not_puanla(Path(goreli), fm, metin, kelimeler, ifade)


# --------------------------------------------------------------- indir()

def test_indir_combining_dot_uretmez(ctx_modul):
    """Python'da "İ".lower() TEK karakter degil: `i` + U+0307 (combining dot).
    O hale gelen metin "istanbul" aramasiyla ESLESMEZ. `indir` once Turkce-guvenli
    tablo ile katlar, boylece nokta ayri bir karakter olarak sizmaz."""
    assert "̇" in "İSTANBUL".lower()          # ciplak .lower() bozuyor
    assert "̇" not in ctx_modul.indir("İSTANBUL")
    assert ctx_modul.indir("İSTANBUL") == "istanbul"
    assert ctx_modul.indir("İZOLASYON") == "izolasyon"


def test_indir_aksani_duzlestirir_arama_ascii_ile_de_tutar(ctx_modul):
    """Ikinci asama ASCII katlamasi: kullanici Turkce karakter yazmadan da
    (klavyesi olmayan makinede, komut satirinda) notu bulabilsin."""
    assert ctx_modul.indir("ŞABLON") == "sablon"
    assert ctx_modul.indir("ŞĞÜÖÇ") == "sguoc"
    # I -> ı -> i zinciri: iki asamadan sonra ASCII 'i'de bulusur
    assert ctx_modul.indir("IZGARA") == "izgara"


def test_indir_turkce_ve_ascii_yazim_ayni_anahtara_duser(ctx_modul):
    """Aramanin asil sozu: "şablon" ile "sablon" ayni sonucu vermeli."""
    assert ctx_modul.indir("şablon") == ctx_modul.indir("sablon")
    assert ctx_modul.indir("HAKEDİŞ") == ctx_modul.indir("hakedis")


# --------------------------------------------------- alan agirlik sirasi

def test_yol_govdeden_agir_basar(ctx_modul):
    fm = {"title": "T", "description": "D"}
    yol_isabeti = puanla(ctx_modul, "domain/billing.md", fm, "alakasiz metin", "billing")
    govde_isabeti = puanla(ctx_modul, "domain/baska.md", fm, "icinde billing gecen govde", "billing")
    assert yol_isabeti[0] > govde_isabeti[0]


def test_kod_parcasi_govdeden_agir_basar(ctx_modul):
    """Backtick icindeki dosya/fonksiyon adi ayri cikarilir (agirlik 60) —
    `distinct_lab_names` diye aratinca notu bulabilmek icin."""
    fm = {"title": "T", "description": "D"}
    kodlu = puanla(ctx_modul, "domain/a.md", fm, "kod: `distinct_lab_names` burada", "distinct_lab_names")
    duz = puanla(ctx_modul, "domain/b.md", fm, "distinct_lab_names duz metinde", "distinct_lab_names")
    assert kodlu[0] > duz[0]


def test_baslik_aciklamadan_agir_basar(ctx_modul):
    baslikta = puanla(ctx_modul, "domain/a.md", {"title": "kontenjan", "description": "x"}, "govde", "kontenjan")
    aciklamada = puanla(ctx_modul, "domain/b.md", {"title": "x", "description": "kontenjan"}, "govde", "kontenjan")
    assert baslikta[0] > aciklamada[0]


# ------------------------------------------------------- VEYA mantigi

def test_eslesmeyen_kelime_notu_ELEMEZ(ctx_modul):
    """Olculmus ariza (2026-08-12): VE elemesi yuzunden cok kelimeli sorgu
    dogru notu dusuruyordu. Bir kelime tutmuyorsa not aday olarak KALIR."""
    fm = {"title": "tenant izolasyonu", "description": "d"}
    sonuc = puanla(ctx_modul, "arch/a.md", fm, "govde", "tenant izolasyonu bulunmayankelime")
    assert sonuc is not None
    puan, _gerekce, eslesen = sonuc
    assert eslesen == 2  # ucten ikisi tuttu
    assert puan > 0


def test_hicbir_kelime_tutmazsa_aday_degil(ctx_modul):
    sonuc = puanla(ctx_modul, "domain/a.md", {"title": "T", "description": "D"}, "govde", "hicalakasizsorgu")
    assert sonuc is None


def test_kapsama_carpani_tam_eslesmeyi_one_alir(ctx_modul):
    """Iki notun ham puani ayniyken, sorgunun TAMAMINI karsilayan onde olmali."""
    tam = puanla(ctx_modul, "domain/a.md", {"title": "tenant izolasyonu", "description": "d"},
                 "govde", "tenant izolasyonu")
    yarim = puanla(ctx_modul, "domain/b.md", {"title": "tenant baska", "description": "d"},
                   "govde", "tenant izolasyonu")
    assert tam[0] > yarim[0]


def test_tam_ifade_bonusu(ctx_modul):
    """Cok kelimeli sorgunun tamami bir alanda birebir geciyorsa ek puan alir."""
    bitisik = puanla(ctx_modul, "domain/a.md", {"title": "tenant izolasyonu", "description": "d"},
                     "govde", "tenant izolasyonu")
    dagilmis = puanla(ctx_modul, "domain/b.md", {"title": "tenant ve baska izolasyonu", "description": "d"},
                      "govde", "tenant izolasyonu")
    assert bitisik[0] > dagilmis[0]
    assert "ifade" in bitisik[1]


# ------------------------------------------------------------ gerekce

def test_gerekce_en_agir_alani_once_yazar(ctx_modul):
    sonuc = puanla(ctx_modul, "domain/billing.md", {"title": "billing", "description": "billing"},
                   "billing", "billing")
    assert sonuc[1][0] in ("yol", "ifade")


# ------------------------------------------------- govde / kod ayirma

def test_govde_ayir_frontmatteri_atlar(ctx_modul):
    metin = "---\ntitle: gizli\n---\n\nasil govde\n"
    govde = ctx_modul._govde_ayir(metin)
    assert "asil govde" in govde
    assert "gizli" not in govde


def test_govde_ayir_frontmattersiz_metni_bozmaz(ctx_modul):
    assert ctx_modul._govde_ayir("duz metin") == "duz metin"


def test_kod_parcalari_yalniz_kod_gorunumlu_olani_alir(ctx_modul):
    """Her backtick kod degildir; dogal dil parcasi kod sayilirsa agirlik 60
    yanlis nota gider."""
    kod = ctx_modul._kod_parcalari("`app/services/billing.py` ve `bir cumle daha uzun`")
    assert "billing.py" in kod


# ------------------------------------------------- dilden bagimsizlik

def test_normalizasyon_diger_dillerde_de_calisir(ctx_modul):
    """Arama normalizasyonu Unicode kurallariyla yapilir, dile ozel tabloyla
    degil — bu yuzden tek bir dile bagli kalmaz."""
    assert ctx_modul.indir("CRÈCHE") == "creche"          # fr
    assert ctx_modul.indir("GRÜNDGENS") == "grundgens"    # de
    assert ctx_modul.indir("MAÑANA") == "manana"          # es
    assert ctx_modul.indir("ŁÓDŹ") == "lodz"              # pl
    assert ctx_modul.indir("ØSTERGAARD") == "ostergaard"  # da
    assert ctx_modul.indir("STRASSE") == ctx_modul.indir("STRAßE")  # de


def test_aksanli_ve_ascii_yazim_ayni_anahtara_duser(ctx_modul):
    for aksanli, ascii_ in [("şablon", "sablon"), ("crèche", "creche"),
                            ("mañana", "manana"), ("Łódź", "lodz")]:
        assert ctx_modul.indir(aksanli) == ctx_modul.indir(ascii_)
