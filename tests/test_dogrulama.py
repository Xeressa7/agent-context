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


def test_tam_bayragi_ile_tc_no_yakalanir(ctx_modul, hafiza, yaz):
    """pre-commit kancasi `--tam` ile kosar: PII'li commit DURDURULUR."""
    yaz(hafiza, "domain/a.md", govde="TC: 12345678901")
    bulgular = ctx_modul.dogrula(hafiza, tam=True)
    assert bulgular, "PII taranmadi"


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
