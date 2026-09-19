"""Tüketici kaydı: keşif + bootstrap kaydı + ölü girdi.

`consumers` listesi elle tutulursa yeni repo unutulur ve haritası sessizce
bayatlar (2026-08-13 dersi). Kayıt kurulumun işidir (`ctx bootstrap`),
kayıtsız ve ölü girdi `ctx doctor` uyarısıdır.
"""
import pytest

MEM_CTX = ("# hafıza ayarları\npii_profiles: [tr]\n"
           "consumers: [.., ../sunc]\n")


def _repo(kok, ad, mem_goreli="../mem"):
    d = kok / ad
    d.mkdir(parents=True)
    (d / "context.yaml").write_text(
        f"version: 1\nproject: {ad}\nmemory_path: {mem_goreli}\nrole: current\n",
        encoding="utf-8")
    return d


def _alan(tmp_path):
    """Çatı düzeni: roof/ altında mem + iki repo (biri listede, biri değil)."""
    roof = tmp_path / "roof"
    mem = roof / "mem"
    mem.mkdir(parents=True)
    (mem / "ctx.yaml").write_text(MEM_CTX, encoding="utf-8")
    _repo(roof, "cati", "./mem")
    _repo(roof, "sunc", "../mem")
    _repo(roof, "yeni", "../mem")
    (roof / "baska").mkdir()  # manifest yok — tarama atlar
    return roof, mem


def test_kesif_listede_olmayani_bulur(ctx_modul, tmp_path):
    roof, mem = _alan(tmp_path)
    # consumers: [.., ../sunc] → cati (..) ve sunc kayıtlı, yeni kayıtsız.
    bulunan = ctx_modul.kayitsiz_tuketiciler(mem, roof / "cati" / "context.yaml")
    assert [p.parent.name for p in bulunan] == ["yeni"]


def test_kesif_hepsi_listedeyken_bos(ctx_modul, tmp_path):
    roof, mem = _alan(tmp_path)
    (mem / "ctx.yaml").write_text(
        MEM_CTX.replace("[.., ../sunc]", "[.., ../sunc, ../yeni]"), encoding="utf-8")
    assert ctx_modul.kayitsiz_tuketiciler(mem, roof / "cati" / "context.yaml") == []


def test_kesif_baska_hafizayi_katmaz(ctx_modul, tmp_path):
    roof, mem = _alan(tmp_path)
    _repo(roof, "yabanci", "../baska-mem")
    bulunan = ctx_modul.kayitsiz_tuketiciler(mem, roof / "cati" / "context.yaml")
    assert [p.parent.name for p in bulunan] == ["yeni"]


def test_olu_girdi_cati_goreli_yol(ctx_modul, tmp_path):
    """Hafızaya göreli olmayan girdi ölü doğar: okuma onu sessizce atlar."""
    roof, mem = _alan(tmp_path)
    (roof / "context.yaml").write_text(
        "version: 1\nproject: cati\nmemory_path: mem\nrole: current\n",
        encoding="utf-8")
    (mem / "ctx.yaml").write_text(
        "consumers: [.., ../sunc, server]\n", encoding="utf-8")
    assert ctx_modul.olu_tuketici_girdileri(mem) == ["server"]


def test_kayit_yeni_ctx_yaml_olusturur(ctx_modul, tmp_path):
    roof, mem = _alan(tmp_path)
    (mem / "ctx.yaml").unlink()
    rapor = ctx_modul.tuketici_kaydet(mem, roof / "yeni")
    assert rapor == ["+ consumers: [../yeni] (ctx.yaml oluşturuldu)"]
    metin = (mem / "ctx.yaml").read_text(encoding="utf-8")
    assert "consumers: [../yeni]" in metin


def test_kayit_kose_parantezi_genisletir(ctx_modul, tmp_path):
    roof, mem = _alan(tmp_path)
    rapor = ctx_modul.tuketici_kaydet(mem, roof / "yeni")
    assert rapor == ["+ consumers: [../yeni] eklendi"]
    metin = (mem / "ctx.yaml").read_text(encoding="utf-8")
    assert "consumers: [.., ../sunc, ../yeni]" in metin
    assert metin.startswith("# hafıza ayarları\npii_profiles: [tr]\n")


def test_kayit_ikinci_kosumda_kayitli_der(ctx_modul, tmp_path):
    roof, mem = _alan(tmp_path)
    ctx_modul.tuketici_kaydet(mem, roof / "yeni")
    once = (mem / "ctx.yaml").read_bytes()
    assert ctx_modul.tuketici_kaydet(mem, roof / "yeni") == ["= consumers (kayıtlı)"]
    assert (mem / "ctx.yaml").read_bytes() == once


def test_kayit_cati_ustu_yolu(ctx_modul, tmp_path):
    """Repo hafızanın üstündeyse girdi `..` olur (çatı düzeni) — zaten listede."""
    roof, mem = _alan(tmp_path)
    assert ctx_modul.tuketici_kaydet(mem, roof) == ["= consumers (kayıtlı)"]


def test_kayit_blok_bicimine_dokunmaz(ctx_modul, tmp_path):
    """Köşeli-parantez dışı biçim tanınmaz ve dosya aynen kalır."""
    roof, mem = _alan(tmp_path)
    blok = "consumers:\n  - ..\n  - ../sunc\n"
    (mem / "ctx.yaml").write_text("pii_profiles: [tr]\n" + blok, encoding="utf-8")
    rapor = ctx_modul.tuketici_kaydet(mem, roof / "yeni")
    assert rapor == ["! consumers biçimi tanınmadı — elle ekleyin: ../yeni"]
    assert (mem / "ctx.yaml").read_text(encoding="utf-8") == \
        "pii_profiles: [tr]\n" + blok


def test_kayit_kuru_yazmaz(ctx_modul, tmp_path):
    roof, mem = _alan(tmp_path)
    rapor = ctx_modul.tuketici_kaydet(mem, roof / "yeni", yaz=False)
    assert rapor == ["+ consumers: [../yeni] eklendi"]
    assert (mem / "ctx.yaml").read_text(encoding="utf-8") == MEM_CTX


def test_kayit_hafiza_yoksa_atlar(ctx_modul, tmp_path):
    assert ctx_modul.tuketici_kaydet(tmp_path / "yok", tmp_path) == \
        ["! hafıza dizini yok — kayıt atlandı"]


def test_kayit_crlf_korur(ctx_modul):
    giris = "pii_profiles: [tr]\r\nconsumers: [..]\r\n"
    yeni, durum = ctx_modul._consumers_satir_ekle(giris, "../yeni")
    assert durum == "eklendi"
    assert yeni == "pii_profiles: [tr]\r\nconsumers: [.., ../yeni]\r\n"
    ayni, durum = ctx_modul._consumers_satir_ekle(yeni, "../yeni")
    assert (ayni, durum) == (yeni, "kayitli")
