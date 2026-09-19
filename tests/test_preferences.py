"""`ctx preferences` — oturum bağlam tercihi.

Harita bütçesi (`index_budget`) + otomatik harita (`auto_index`) tüketici
`context.yaml`'ında yaşar. Düzenleme SATIR BAZLIDIR: anahtar sırası, yorumlar
ve satır sonu biçimi korunur — tam bir YAML yazıcı iki dilde aynı baytları
üretmezdi, satır editi üretir.
"""
import types

VARSAYILAN = ("version: 1\nproject: deneme\nmemory_path: ../mem\n"
              "role: current\ncomponents: [app]\n")


def _args(**kw):
    return types.SimpleNamespace(repo=None, profile=kw.get("profile"),
                                 index_budget=kw.get("index_budget"))


def _kur(tmp_path, metin=VARSAYILAN):
    (tmp_path / "context.yaml").write_text(metin, encoding="utf-8")


def test_goster_varsayilan(ctx_modul, tmp_path, monkeypatch, capsys):
    _kur(tmp_path)
    monkeypatch.chdir(tmp_path)
    assert ctx_modul.komut_preferences(_args()) == 0
    out = capsys.readouterr().out
    assert "profil: normal" in out and "16000" in out and "açık" in out


def test_ekonomik_yazar_ve_gosterir(ctx_modul, tmp_path, monkeypatch, capsys):
    _kur(tmp_path)
    monkeypatch.chdir(tmp_path)
    assert ctx_modul.komut_preferences(_args(profile="ekonomik")) == 0
    metin = (tmp_path / "context.yaml").read_text(encoding="utf-8")
    assert "index_budget: 6000" in metin and "auto_index" not in metin
    assert ctx_modul.komut_preferences(_args()) == 0
    assert "profil: ekonomik" in capsys.readouterr().out


def test_manuel_kancayi_sessizlestirir(ctx_modul, tmp_path, monkeypatch, capsys):
    """Manuel profilde `hook index` tek bayt basmaz — oturum haritasız açılır."""
    _kur(tmp_path)
    (tmp_path / "mem").mkdir()
    monkeypatch.chdir(tmp_path)
    ctx_modul.komut_preferences(_args(profile="manuel"))
    capsys.readouterr()  # ayar çıktısını tüket; aşağıda yalnız kanca konuşabilir
    assert ctx_modul._hook_index(types.SimpleNamespace(repo=None)) == 0
    assert capsys.readouterr().out == "", "manuel profilde kanca konuşmamalı"


def test_normal_satirlari_siler(ctx_modul, tmp_path, monkeypatch):
    _kur(tmp_path, VARSAYILAN + "index_budget: 6000\nauto_index: false\n")
    monkeypatch.chdir(tmp_path)
    assert ctx_modul.komut_preferences(_args(profile="normal")) == 0
    assert (tmp_path / "context.yaml").read_text(encoding="utf-8") == VARSAYILAN


def test_ozel_butce_profil_ozel(ctx_modul, tmp_path, monkeypatch, capsys):
    _kur(tmp_path)
    monkeypatch.chdir(tmp_path)
    assert ctx_modul.komut_preferences(_args(index_budget=4000)) == 0
    assert ctx_modul.komut_preferences(_args()) == 0
    out = capsys.readouterr().out
    assert "profil: özel" in out and "4000" in out


def test_gecersiz_butce_yazmaz(ctx_modul, tmp_path, monkeypatch, capsys):
    _kur(tmp_path)
    monkeypatch.chdir(tmp_path)
    assert ctx_modul.komut_preferences(_args(index_budget=0)) == 2
    assert "HATA" in capsys.readouterr().out
    assert (tmp_path / "context.yaml").read_text(encoding="utf-8") == VARSAYILAN


def test_okunamayan_butce_varsayilana_duser(ctx_modul, tmp_path, monkeypatch, capsys):
    _kur(tmp_path, VARSAYILAN + "index_budget: abc\n")
    monkeypatch.chdir(tmp_path)
    assert ctx_modul.komut_preferences(_args()) == 0
    out = capsys.readouterr().out
    assert "profil: normal" in out and "16000" in out


def test_yorum_ve_sira_korunur(ctx_modul, tmp_path, monkeypatch):
    """Satır editi yalnız hedef satıra dokunur; yorum ve sıra aynen kalır."""
    giris = ("# proje manifesti\nversion: 1\nproject: deneme  # ad\n"
             "memory_path: ../mem\nrole: current\n")
    _kur(tmp_path, giris)
    monkeypatch.chdir(tmp_path)
    ctx_modul.komut_preferences(_args(profile="ekonomik"))
    assert (tmp_path / "context.yaml").read_text(encoding="utf-8") == \
        giris + "index_budget: 6000\n"


def test_girintili_anahtar_eslesmez(ctx_modul):
    """Blok-içi aynı adlı satır bizimki değildir; üste eklenir, içteki durur."""
    giris = "retrieval:\n  index_budget: 1\nrole: current\n"
    cikti = ctx_modul._manifest_satir_ayarla(giris, "index_budget", "6000")
    assert cikti == giris + "index_budget: 6000\n"


def test_yorum_satiri_eslesmez(ctx_modul):
    giris = "# index_budget: 5\nrole: current\n"
    assert ctx_modul._manifest_satir_ayarla(giris, "index_budget", "6000") == \
        giris + "index_budget: 6000\n"


def test_satir_sonu_korunur(ctx_modul):
    """CRLF manifest CRLF kalır; LF'ye çevrilince diff bütün dosyaya bulaşırdı."""
    giris = "role: current\r\nindex_budget: 5\r\n"
    assert ctx_modul._manifest_satir_ayarla(giris, "index_budget", "6000") == \
        "role: current\r\nindex_budget: 6000\r\n"
    assert ctx_modul._manifest_satir_ayarla(giris, "index_budget", None) == \
        "role: current\r\n"
    assert ctx_modul._manifest_satir_ayarla("role: current\n", "index_budget", None) == \
        "role: current\n"
