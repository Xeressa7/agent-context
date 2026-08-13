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
    assert "hook index" in komutlar
    assert "hook capture-check" in komutlar
    # Kancalar ARACIN ICINDE: ayrica kopyalanan script dosyasi YOK.
    assert not (sahte_claude / "hooks").exists()


def test_eski_script_kaydi_YERINDE_guncellenir(ctx_modul, sahte_claude):
    """Gecis: 2026-08-13 oncesi kayitlar `python ".../ctx_index.py"` biciminde.

    Taninmazsa ayni kanca hem eski hem yeni bicimle kayitli kalir ve her oturum
    baslangicinda IKI KEZ kosardi.
    """
    (sahte_claude / "settings.json").write_text(json.dumps({
        "hooks": {"SessionStart": [{"hooks": [
            {"type": "command", "command": 'python "/eski/ctx_index.py"',
             "statusMessage": "benim mesajim"},
        ]}]},
    }), encoding="utf-8")

    rapor = ctx_modul.hook_kur()
    ayarlar = ayarlari_oku(sahte_claude)
    komutlar = kayitli_komutlar(ayarlar)

    assert komutlar.count("hook index") == 1, "kanca iki kez kayitli"
    assert "ctx_index.py" not in komutlar, "eski komut hala duruyor"
    assert any("guncellendi" in s for s in rapor)
    girdi = ayarlar["hooks"]["SessionStart"][0]["hooks"][0]
    assert girdi["statusMessage"] == "benim mesajim", "kullanicinin alani silinmis"


def test_ikinci_kosum_kayit_TEKRARLAMAZ(ctx_modul, sahte_claude):
    ctx_modul.hook_kur()
    ilk = ayarlari_oku(sahte_claude)
    ctx_modul.hook_kur()
    ikinci = ayarlari_oku(sahte_claude)
    assert ilk == ikinci, "idempotent degil: ikinci kosum ayarlari degistirdi"
    assert kayitli_komutlar(ikinci).count("hook index") == 1


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
    assert "hook index" in komutlar


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


# ------------------------------------------------------- capture kapisi sinir

def _git(*arg, cwd):
    import subprocess
    subprocess.run(["git", *arg], cwd=str(cwd), capture_output=True, timeout=60)


def test_capture_kapisi_EV_DIZININE_TIRMANMAZ(ctx_modul, tmp_path):
    """REGRESYON: cati bir git reposu degilse git aramasi yukari tirmaniyordu.

    Olculdu (2026-08-13): `yapit/` catisi repo olmadigi icin
    `rev-parse --show-toplevel` kullanicinin EV DIZININI buluyor, `ctx close`
    "repo: onur5 · degisik dosya: 63" diyordu. Kayitlar da ayni ada dustugu icin
    `--kontrol` hep "guncel" deyip susuyordu — kapi, oturumlarin basladigi yerde
    fiilen KAPALIYDI.
    """
    disarisi = tmp_path / "ev"        # catinin USTUNDEKI repo (ev dizini taklidi)
    cati = disarisi / "proje"         # cati: repo DEGIL
    (cati / "alt-a").mkdir(parents=True)
    (cati / "alt-b").mkdir()
    _git("init", "--quiet", ".", cwd=disarisi)
    (disarisi / "ev-copu.txt").write_text("ev dizininde kirli dosya\n", encoding="utf-8")

    for alt in ("alt-a", "alt-b"):
        d = cati / alt
        _git("init", "--quiet", ".", cwd=d)
        (d / "dosya.txt").write_text("x\n", encoding="utf-8")
        _git("add", "-A", cwd=d)
        _git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "ilk", cwd=d)
        (d / "kirli.txt").write_text("is yapildi\n", encoding="utf-8")

    durum = ctx_modul._calisma_durumu(cati, cati)

    assert durum["repo"] == "proje", f"ev dizinine tirmandi: {durum['repo']}"
    assert durum["kirli"] == 2, "alt repolarin kirli dosyalari toplanmadi"
    assert "ev-copu" not in str(durum), "ev dizininin kirliligi sayildi"
    # HEAD parmak izi alt repolardan gelir → birinde commit olunca kapi yeniden kurulur
    assert durum["head"].startswith("alt-a@") and "alt-b@" in durum["head"]


def test_capture_kapisi_gercek_repoda_degismedi(ctx_modul, tmp_path):
    """Sinir yalniz DISARIYI keser; reponun kendi icinde davranis aynidir."""
    repo = tmp_path / "repo"
    (repo / "ic" / "derin").mkdir(parents=True)
    _git("init", "--quiet", ".", cwd=repo)
    (repo / "a.txt").write_text("x\n", encoding="utf-8")
    _git("add", "-A", cwd=repo)
    _git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "ilk", cwd=repo)
    (repo / "kirli.txt").write_text("y\n", encoding="utf-8")

    # Alt dizinden kosulsa da repo koku bulunur (sinir icinde kaldigi icin).
    durum = ctx_modul._calisma_durumu(repo / "ic" / "derin", repo)
    assert durum["repo"] == "repo"
    assert durum["kirli"] == 1
    assert durum["head"] and "@" not in durum["head"], "tek repoda parmak izi degil gercek HEAD"
