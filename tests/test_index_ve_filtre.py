"""Retrieval'in iki yapisal sozu:

1. FILTRE — `role` yanlis cagin bilgisini GORUNMEZ kilar. Yeni surumde
   calisirken legacy notu okunamamali (ve tersi).
2. INDEX — korpusun haritasi her oturuma yuklenir. Tek uretici (`ctx index`),
   cok tuketici (SessionStart hook · AGENTS.md blogu · CI). Bayatlik yapisal
   kapatilir: blok isaretleri arasi tazelenir, `--kontrol` bayatsa 1 doner.
"""


def veri(role="current"):
    return {"project": "test", "role": role, "memory_path": "./test-memory"}


# ------------------------------------------------------------- FILTRE

def test_filtre_tanimi_iki_rolu_kapsar(ctx_modul):
    assert ctx_modul.FILTRE["current"] == {"product", "current"}
    assert ctx_modul.FILTRE["legacy"] == {"product", "legacy"}


def test_product_her_iki_rolde_de_gorunur(ctx_modul):
    """Teknoloji-bagimsiz urun gercegi cagdan bagimsizdir — iki filtrede de var."""
    assert "product" in ctx_modul.FILTRE["current"]
    assert "product" in ctx_modul.FILTRE["legacy"]


def test_legacy_current_rolunde_gorunmez(ctx_modul):
    assert "legacy" not in ctx_modul.FILTRE["current"]
    assert "current" not in ctx_modul.FILTRE["legacy"]


def test_historical_varsayilan_filtrelerin_DISINDA(ctx_modul):
    """`historical` bugunu tarif etmez: istenmeden yuklenmez (--all ile gelir)."""
    assert "historical" not in ctx_modul.FILTRE["current"]
    assert "historical" not in ctx_modul.FILTRE["legacy"]


def test_state_klasorleri_taranmaz(ctx_modul, hafiza, yaz):
    """tasks/ ve sessions/ DURUM tutar, durable bilgi degil — gezinmeye girmez."""
    yaz(hafiza, "domain/gercek.md")
    (hafiza / "tasks" / "OPEN.md").write_text("---\ntitle: sahte\n---\n", encoding="utf-8")
    yollar = [g.as_posix() for _p, g in ctx_modul.notlari_gez(hafiza)]
    assert "domain/gercek.md" in yollar
    assert not any(y.startswith(("tasks/", "sessions/")) for y in yollar)


# -------------------------------------------------------------- INDEX

def test_index_govdesi_yalniz_role_uygun_notlari_yazar(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/urun.md", lifecycle="product", description="urun gercegi")
    yaz(hafiza, "architecture/yeni.md", lifecycle="current", description="bugunku mimari")
    yaz(hafiza, "legacy/eski.md", lifecycle="legacy", description="eski agac bilgisi")

    govde = ctx_modul.index_govde(veri("current"), hafiza)
    assert "urun.md" in govde and "yeni.md" in govde
    assert "eski.md" not in govde, "legacy notu current haritasina sizdi"

    govde_legacy = ctx_modul.index_govde(veri("legacy"), hafiza)
    assert "eski.md" in govde_legacy and "urun.md" in govde_legacy
    assert "yeni.md" not in govde_legacy


def test_index_satiri_notun_KENDISI_degil_aciklamasidir(ctx_modul, hafiza, yaz):
    """Harita = yol + description. Govde haritaya girerse "korpusu topluca
    yukleme" yasagi delinir."""
    yaz(hafiza, "domain/a.md", description="tek cumlelik ozet",
        govde="BU GOVDE HARITAYA GIRMEMELI cunku harita bir indekstir.")
    govde = ctx_modul.index_govde(veri(), hafiza)
    assert "tek cumlelik ozet" in govde
    assert "HARITAYA GIRMEMELI" not in govde


def test_index_not_sayisini_bildirir(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/a.md")
    yaz(hafiza, "domain/b.md")
    assert "2 not" in ctx_modul.index_govde(veri(), hafiza)


def test_index_bos_korpusta_cokmez(ctx_modul, hafiza):
    assert isinstance(ctx_modul.index_govde(veri(), hafiza), str)


# ------------------------------------------------- blok degistirme (tuketici)

def test_blok_isaretleri_arasi_degisir_gerisi_KORUNUR(ctx_modul):
    """AGENTS.md kullanicinin kendi talimatlarini tasir; index yazimi yalniz
    isaretlerin arasina dokunmali."""
    metin = (f"# AGENTS\n\nUstteki kural.\n\n{ctx_modul.INDEX_BAS}\nESKI HARITA\n"
             f"{ctx_modul.INDEX_SON}\n\nAlttaki kural.\n")
    yeni = ctx_modul._blok_degistir(metin, "YENI HARITA")
    assert "YENI HARITA" in yeni and "ESKI HARITA" not in yeni
    assert "Ustteki kural." in yeni and "Alttaki kural." in yeni


def test_isaret_yoksa_None_doner_dosya_bozulmaz(ctx_modul):
    """Isaretsiz dosyaya korlemesine yazmak kullanicinin AGENTS.md'sini ezerdi."""
    assert ctx_modul._blok_degistir("# AGENTS\n\nisaret yok\n", "HARITA") is None
