"""Retrieval'in iki yapisal sozu:

1. FILTRE — `role` yanlis cagin bilgisini GORUNMEZ kilar. Yeni surumde
   calisirken legacy notu okunamamali (ve tersi).
2. INDEX — korpusun haritasi her oturuma yuklenir. Tek uretici (`ctx index`),
   cok tuketici (SessionStart hook · AGENTS.md blogu · CI). Bayatlik yapisal
   kapatilir: blok isaretleri arasi tazelenir, `--kontrol` bayatsa 1 doner.
"""


def veri(role="current"):
    return {"project": "test", "role": role, "memory_path": "./test-memory"}


# ------------------------------------------------- KAPSAM (status + scope)
#
# Iki bagimsiz eksen: `status` (active|archived) ve `scope` (etiket listesi).
# scope BOSSA not evrenseldir. Eski tek-eksenli `lifecycle` alani okunmaya
# devam eder ve ayni davranisi uretir — bu testler ikisini de kilitler.

def test_scopesuz_not_her_kapsamda_gorunur(ctx_modul):
    """Evrensel bilgi (dis sistem davranisi, mevzuat) caga bagli degildir."""
    fm = {"status": "active"}
    assert ctx_modul.gorunur(fm, ["v2"])
    assert ctx_modul.gorunur(fm, ["v1"])
    assert ctx_modul.gorunur(fm, [])


def test_scope_kesismezse_gorunmez(ctx_modul):
    fm = {"scope": "v1"}
    assert not ctx_modul.gorunur(fm, ["v2"])
    assert ctx_modul.gorunur(fm, ["v1"])


def test_coklu_scope_kesisme_yeterlidir(ctx_modul):
    fm = {"scope": "server, worker"}
    assert ctx_modul.gorunur(fm, ["worker", "cli"])
    assert not ctx_modul.gorunur(fm, ["web"])


def test_arsivlenmis_not_varsayilan_GELMEZ(ctx_modul):
    fm = {"status": "archived"}
    assert not ctx_modul.gorunur(fm, ["v2"])
    assert ctx_modul.gorunur(fm, ["v2"], arsiv=True)


def test_kapsam_tanimsizsa_filtre_UYGULANMAZ(ctx_modul):
    """Tek cagli projede kimse scope yazmak zorunda kalmasin."""
    assert ctx_modul.gorunur({"scope": "v1"}, [])


# --------------------------------------- eski `lifecycle` geriye uyumlulugu

def test_eski_product_evrensel_olur(ctx_modul):
    assert ctx_modul._gecerlilik({"lifecycle": "product"}) == ("active", [])
    assert ctx_modul.gorunur({"lifecycle": "product"}, ["legacy"])


def test_eski_current_ve_legacy_scope_etiketine_doner(ctx_modul):
    assert ctx_modul._gecerlilik({"lifecycle": "current"}) == ("active", ["current"])
    assert ctx_modul._gecerlilik({"lifecycle": "legacy"}) == ("active", ["legacy"])
    # eski davranis birebir korunur
    assert not ctx_modul.gorunur({"lifecycle": "legacy"}, ["current"])
    assert not ctx_modul.gorunur({"lifecycle": "current"}, ["legacy"])


def test_eski_historical_arsive_doner(ctx_modul):
    assert ctx_modul._gecerlilik({"lifecycle": "historical"}) == ("archived", [])
    assert not ctx_modul.gorunur({"lifecycle": "historical"}, ["current"])


def test_yeni_alanlar_eskisini_EZER(ctx_modul):
    fm = {"lifecycle": "legacy", "scope": "v2"}
    assert ctx_modul._gecerlilik(fm) == ("active", ["v2"])


# ------------------------------------------------- repo kapsami (context.yaml)

def test_repo_kapsami_scope_alanindan(ctx_modul):
    assert ctx_modul.repo_kapsami({"scope": ["server", "web"]}) == ["server", "web"]


def test_repo_kapsami_eski_role_alanindan_turetilir(ctx_modul):
    """Mevcut kurulumlar `role: current` tasiyor — bozulmamali."""
    assert ctx_modul.repo_kapsami({"role": "current"}) == ["current"]
    assert ctx_modul.repo_kapsami({}) == []


def test_state_klasorleri_taranmaz(ctx_modul, hafiza, yaz):
    """tasks/ ve sessions/ DURUM tutar, durable bilgi degil — gezinmeye girmez."""
    yaz(hafiza, "facts/gercek.md")
    (hafiza / "tasks" / "OPEN.md").write_text(
        "---\ntitle: sahte\n---\n", encoding="utf-8")
    yollar = [g.as_posix() for _p, g in ctx_modul.notlari_gez(hafiza)]
    assert "facts/gercek.md" in yollar
    assert not any(y.startswith(("tasks/", "sessions/")) for y in yollar)


# -------------------------------------------------------------- INDEX

def test_index_govdesi_yalniz_role_uygun_notlari_yazar(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/urun.md", lifecycle="product", description="urun gercegi")
    yaz(hafiza, "facts/yeni.md", lifecycle="current", description="bugunku mimari")
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
    yaz(hafiza, "facts/a.md", description="tek cumlelik ozet",
        govde="BU GOVDE HARITAYA GIRMEMELI cunku harita bir indekstir.")
    govde = ctx_modul.index_govde(veri(), hafiza)
    assert "tek cumlelik ozet" in govde
    assert "HARITAYA GIRMEMELI" not in govde


def test_index_not_sayisini_bildirir(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md")
    yaz(hafiza, "facts/b.md")
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


# ------------------------------------------- baglam butcesi

def test_butce_altinda_harita_degismez(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", description="kisa aciklama")
    veri = {"memory_path": str(hafiza)}
    assert ctx_modul.index_govde_butceli(veri, hafiza, 16000) == ctx_modul.index_govde(veri, hafiza)


def test_butce_asilinca_NOT_DUSMEZ_aciklama_kisalir(ctx_modul, hafiza, yaz):
    """Not dusurmek sessiz kapsam kaybidir: harita 'hepsi bu' diye okunur ve
    gorunmeyen not hic aranmaz. Bu yuzden satir sayisi KORUNUR."""
    for i in range(12):
        yaz(hafiza, f"facts/n{i}.md", description="uzun bir aciklama " * 12)
    veri = {"memory_path": str(hafiza)}
    tam = ctx_modul.index_govde(veri, hafiza)
    kisa = ctx_modul.index_govde_butceli(veri, hafiza, len(tam) // 2)
    assert len(kisa) < len(tam)
    assert kisa.count("- `n") == tam.count("- `n") == 12      # tek not dusmedi
    assert "kisaltildi" in kisa                                # ve bunu SOYLUYOR


def test_en_kisa_bicimde_bile_asiyorsa_yuksek_sesle_soyler(ctx_modul, hafiza, yaz):
    for i in range(40):
        yaz(hafiza, f"facts/n{i}.md", description="aciklama")
    veri = {"memory_path": str(hafiza)}
    kisa = ctx_modul.index_govde_butceli(veri, hafiza, 200)
    assert "tavani asiyor" in kisa and kisa.count("- `n") == 40
