"""`dogrula()` — tek dogrulama motoru. `validate`, `doctor` ve pre-commit
kancasi AYNI fonksiyonu cagirir; mantik tekrari yok. Bu testler o motorun
sozlerini kilitler.

Onemli ayrim: 'hata' cikis kodunu etkiler, 'uyari' etkilemez. review-due
bilerek UYARIDIR — 90 gununu doldurmus not "yanlis" degildir, bakilmasi
gerekendir.
"""


def seviyeler(bulgular):
    return {(s, m.split(":")[0]) for s, _yol, m in bulgular}


def mesajlar(bulgular):
    return " | ".join(m for _s, _y, m in bulgular)


# ------------------------------------------------------- zorunlu alanlar

def test_frontmattersiz_dosya_hata(ctx_modul, hafiza):
    (hafiza / "domain" / "bos.md").write_text("frontmatter yok\n", encoding="utf-8")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(s == "hata" and "frontmatter" in m for s, _y, m in bulgular)


def test_gecersiz_lifecycle_hata(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/a.md", lifecycle="uydurma")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(s == "hata" and "lifecycle" in m for s, _y, m in bulgular)


def test_eksik_zorunlu_alan_hata(ctx_modul, hafiza):
    (hafiza / "domain" / "a.md").write_text(
        "---\ntitle: T\nlifecycle: current\ntype: domain\n---\n\ngovde\n", encoding="utf-8")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(s == "hata" and "description" in m for s, _y, m in bulgular)


def test_saglam_not_bulgu_uretmez(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/saglam.md")
    assert ctx_modul.dogrula(hafiza) == []


# ------------------------------------------------------------- linkler

def test_kirik_wiki_link_yakalanir(ctx_modul, hafiza, yaz):
    """12 Agustos'ta validator ilk kosumunda gercek bir kirik wiki-link yakaladi;
    bu test o davranisi kalici kilar."""
    yaz(hafiza, "domain/a.md", govde="Bkz [[olmayan-not]] detayda.")
    bulgular = ctx_modul.dogrula(hafiza)
    assert "olmayan-not" in mesajlar(bulgular)


def test_var_olan_wiki_link_temiz(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/hedef.md", title="Hedef not")
    yaz(hafiza, "domain/a.md", title="Kaynak not", govde="Bkz [[hedef]] detayda.")
    assert ctx_modul.dogrula(hafiza) == []


def test_ayni_baslikli_iki_not_uyarir(ctx_modul, hafiza, yaz):
    """Ayni basligi tasiyan iki not, haritada ayirt edilemez satir uretir."""
    yaz(hafiza, "domain/a.md", title="Ayni Baslik")
    yaz(hafiza, "domain/b.md", title="Ayni Baslik")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any("baslik tekrari" in m for _s, _y, m in bulgular)


def test_kirik_markdown_link_yakalanir(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/a.md", govde="Bkz [su](../domain/yok.md) detayda.")
    assert "yok.md" in mesajlar(ctx_modul.dogrula(hafiza))


# ---------------------------------------------------------- superseded_by

def test_superseded_by_hedefi_yoksa_bulgu(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/a.md", superseded_by="domain/olmayan.md")
    assert "olmayan.md" in mesajlar(ctx_modul.dogrula(hafiza))


# ------------------------------------------------------------ PII / secret

def test_tam_bayragi_olmadan_pii_taranmaz(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/a.md", govde="TC: 12345678901")
    assert ctx_modul.dogrula(hafiza, tam=False) == []


def test_tam_bayragi_ile_evrensel_pii_yakalanir(ctx_modul, hafiza, yaz):
    """pre-commit kancasi `--tam` ile kosar: PII'li commit DURDURULUR.
    E-posta her projede PII'dir — profil gerektirmez."""
    yaz(hafiza, "domain/a.md", govde="Iletisim: ornek.kisi@ornek.com")
    assert ctx_modul.dogrula(hafiza, tam=True), "evrensel PII taranmadi"


def test_ulkeye_ozel_desen_PROFILSIZ_kosmaz(ctx_modul, hafiza, yaz):
    """11 haneli sayi Turkiye'de kimlik no'dur ama baska projede siparis
    numarasi olabilir — profil acilmadan yanlis alarm uretmemeli."""
    yaz(hafiza, "domain/a.md", govde="Siparis numarasi 12345678901 olarak kayitli.")
    assert ctx_modul.dogrula(hafiza, tam=True) == []


def test_ulkeye_ozel_desen_PROFILLE_kosar(ctx_modul, hafiza, yaz):
    (hafiza / "ctx.yaml").write_text("pii_profiles: [tr]\n", encoding="utf-8")
    yaz(hafiza, "domain/a.md", govde="TC: 12345678901")
    bulgular = ctx_modul.dogrula(hafiza, tam=True)
    assert any("TC kimlik" in m for _s, _y, m in bulgular)


def test_bilinmeyen_profil_sessizce_yok_sayilir(ctx_modul, hafiza, yaz):
    (hafiza / "ctx.yaml").write_text("pii_profiles: [olmayan_ulke]\n", encoding="utf-8")
    yaz(hafiza, "domain/a.md", govde="duz metin")
    assert ctx_modul.dogrula(hafiza, tam=True) == []


def test_review_due_HATA_degil_UYARI(ctx_modul, hafiza, yaz):
    """Bilincli karar: bayat not commit'i engellemez. Bu testi kirmadan
    review-due'yu hataya cevirmeyin — kalite kapisi sevimsizlesir."""
    yaz(hafiza, "domain/eski.md", updated="2020-01-01")
    bulgular = ctx_modul.dogrula(hafiza)
    assert bulgular, "review-due hic uyarmadi"
    assert all(s == "uyari" for s, _y, _m in bulgular)


# ------------------------------------------------- state dosyalari not degil

def test_tasks_ve_sessions_dogrulamaya_girmez(ctx_modul, hafiza):
    """OPEN.md / CURRENT.md durable NOT degil, DURUM. Frontmatter'lari yok ve
    olmamali; dogrulama onlari gormemeli (ATLA kumesi)."""
    assert ctx_modul.dogrula(hafiza) == []


# ------------------------------------------- hafizanin kendi ayar dosyasi

def test_skip_dirs_ek_dizini_not_saymaz(ctx_modul, hafiza, yaz):
    """Projeye ozel dizin adlari KODA gomulmez; hafiza kendi ayarinda soyler."""
    yaz(hafiza, "gecis/devralinan.md", lifecycle="uydurma")  # gecerli olsa hata verirdi
    assert any("lifecycle" in m for _s, _y, m in ctx_modul.dogrula(hafiza))

    (hafiza / "ctx.yaml").write_text("skip_dirs: [gecis]\n", encoding="utf-8")
    assert ctx_modul.dogrula(hafiza) == []


def test_review_days_ayarlanabilir(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/a.md", updated="2026-07-01")   # ~43 gun once
    assert ctx_modul.dogrula(hafiza) == [], "90 gun varsayilaniyla uyarmamali"

    (hafiza / "ctx.yaml").write_text("review_days: 7\n", encoding="utf-8")
    assert any("review-due" in m for _s, _y, m in ctx_modul.dogrula(hafiza))


def test_ayar_dosyasi_yoksa_guvenli_varsayilan(ctx_modul, hafiza, yaz):
    yaz(hafiza, "domain/a.md")
    assert ctx_modul.dogrula(hafiza, tam=True) == []
