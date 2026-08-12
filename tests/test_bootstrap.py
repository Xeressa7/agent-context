"""`ctx bootstrap` — kurulumun iki sozu:

  IDEMPOTENT   : ikinci kosum hicbir sey degistirmez
  YIKICI DEGIL : kullanicinin settings.json'undaki kendi kancalari korunur

Ikincisi kritik: settings.json kullanicinin butun harness ayarlarini tasir.
Bootstrap oraya YAZMAK zorunda (hook kaydi asil is), ama ezmek FELAKET olur.
"""
import json

import pytest


@pytest.fixture
def sahte_claude(tmp_path, ctx_modul, monkeypatch):
    """`~/.claude` yerine gecici dizin — testler gercek ayarlara dokunmaz."""
    d = tmp_path / "claude"
    d.mkdir()
    monkeypatch.setattr(ctx_modul, "_claude_dizini", lambda: d)
    return d


def ayarlari_oku(dizin):
    return json.loads((dizin / "settings.json").read_text(encoding="utf-8"))


def kayitli_komutlar(ayarlar):
    liste = ayarlar["hooks"]["SessionStart"][0]["hooks"]
    return " ".join(k.get("command", "") for k in liste)


# --------------------------------------------------------- hook kurulumu

def test_bos_kurulumda_iki_kanca_da_kaydedilir(ctx_modul, sahte_claude):
    ctx_modul.hook_kur()
    komutlar = kayitli_komutlar(ayarlari_oku(sahte_claude))
    assert "ctx_index.py" in komutlar
    assert "ctx_capture_check.py" in komutlar
    assert (sahte_claude / "hooks" / "ctx_index.py").is_file()


def test_ikinci_kosum_kayit_TEKRARLAMAZ(ctx_modul, sahte_claude):
    ctx_modul.hook_kur()
    ilk = ayarlari_oku(sahte_claude)
    ctx_modul.hook_kur()
    ikinci = ayarlari_oku(sahte_claude)
    assert ilk == ikinci, "idempotent degil: ikinci kosum ayarlari degistirdi"
    assert kayitli_komutlar(ikinci).count("ctx_index.py") == 1


def test_kullanicinin_kendi_kancasi_KORUNUR(ctx_modul, sahte_claude):
    """Bootstrap'in en tehlikeli anı: mevcut settings.json'a dokunmak."""
    (sahte_claude / "settings.json").write_text(json.dumps({
        "theme": "dark",
        "hooks": {"SessionStart": [{"hooks": [
            {"type": "command", "command": "python benim_kancam.py"},
        ]}]},
    }), encoding="utf-8")

    ctx_modul.hook_kur()
    ayarlar = ayarlari_oku(sahte_claude)

    assert ayarlar["theme"] == "dark", "ilgisiz ayar kayboldu"
    komutlar = kayitli_komutlar(ayarlar)
    assert "benim_kancam.py" in komutlar, "kullanicinin kancasi silindi"
    assert "ctx_index.py" in komutlar


def test_bozuk_settings_json_ezilmez(ctx_modul, sahte_claude):
    """Okunamayan ayar dosyasi uzerine yazmak kullanicinin ayarlarini yok ederdi;
    dogru davranis: dokunma, elle ekle de."""
    bozuk = "{ bu gecerli json degil"
    (sahte_claude / "settings.json").write_text(bozuk, encoding="utf-8")
    rapor = ctx_modul.hook_kur()
    assert (sahte_claude / "settings.json").read_text(encoding="utf-8") == bozuk
    assert any("okunamadi" in s for s in rapor)


def test_kuru_kosum_hicbir_sey_yazmaz(ctx_modul, sahte_claude):
    ctx_modul.hook_kur(yaz=False)
    assert not (sahte_claude / "settings.json").exists()
    assert not (sahte_claude / "hooks" / "ctx_index.py").exists()


# ------------------------------------------------------- kalite kapisi

def test_git_olmayan_dizinde_kapi_atlanir(ctx_modul, tmp_path):
    rapor = ctx_modul.kalite_kapisi_kur(tmp_path)
    assert any("git reposu degil" in s for s in rapor)


def test_kapi_kurulur_ve_ikinci_kosumda_guncel_der(ctx_modul, tmp_path):
    import subprocess
    subprocess.run(["git", "init", "--quiet", str(tmp_path)], check=True)

    ilk = ctx_modul.kalite_kapisi_kur(tmp_path)
    assert (tmp_path / ".githooks" / "pre-commit").is_file()
    assert any("core.hooksPath" in s for s in ilk)

    ikinci = ctx_modul.kalite_kapisi_kur(tmp_path)
    assert all(not s.startswith("+") for s in ikinci), "idempotent degil"


def test_baska_hooksPath_ayari_EZILMEZ(ctx_modul, tmp_path):
    """Projenin kendi kanca dizini varsa ustune yazmak sessizce kancalarini
    devre disi birakirdi."""
    import subprocess
    subprocess.run(["git", "init", "--quiet", str(tmp_path)], check=True)
    subprocess.run(["git", "-C", str(tmp_path), "config", "core.hooksPath", ".benim-kancalarim"],
                   check=True)

    rapor = ctx_modul.kalite_kapisi_kur(tmp_path)
    mevcut = subprocess.run(["git", "-C", str(tmp_path), "config", "--get", "core.hooksPath"],
                            capture_output=True, text=True).stdout.strip()
    assert mevcut == ".benim-kancalarim"
    assert any("DOKUNULMADI" in s for s in rapor)
