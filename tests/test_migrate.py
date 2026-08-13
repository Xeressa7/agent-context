"""`ctx migrate` — eski `lifecycle` alanini `status` + `scope` ikilisine cevirir.

Bu komutun sozu: **satir bazli ve yerinde**. Frontmatter'in geri kalanina,
govdeye ve dosyanin bicimine DOKUNMAZ. Asagidaki testler o sozu kilitler;
komutun kendisi 23 gercek notu donusturdu ve o gune kadar testi yoktu.
"""
import pytest


class _Args:
    def __init__(self, repo, kuru=False):
        self.repo = repo
        self.kuru = kuru


@pytest.fixture
def kurulum(hafiza, tmp_path):
    """Hafiza + ona isaret eden bir tuketici repo."""
    proje = tmp_path / "proje"
    proje.mkdir()
    (proje / "context.yaml").write_text(
        f"version: 1\nproject: t\nmemory_path: {hafiza.as_posix()}\nrole: current\n",
        encoding="utf-8")
    return proje, hafiza


def _yaz_ham(hafiza, goreli, metin):
    p = hafiza / goreli
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_bytes(metin.encode("utf-8"))
    return p


def test_dort_eski_deger_de_cevrilir(ctx_modul, kurulum, capsys):
    proje, hafiza = kurulum
    for ad, eski in (("a", "product"), ("b", "current"), ("c", "legacy"), ("d", "historical")):
        _yaz_ham(hafiza, f"domain/{ad}.md",
                 f"---\ntitle: {ad.upper()}\nlifecycle: {eski}\n---\n\ngovde\n")

    assert ctx_modul.komut_migrate(_Args(str(proje))) == 0
    capsys.readouterr()

    def fm(ad):
        return ctx_modul.frontmatter((hafiza / f"domain/{ad}.md").read_text(encoding="utf-8"))

    assert fm("a") == {"title": "A", "status": "active"}                    # evrensel
    assert fm("b") == {"title": "B", "status": "active", "scope": "current"}
    assert fm("c") == {"title": "C", "status": "active", "scope": "legacy"}
    assert fm("d") == {"title": "D", "status": "archived"}


def test_bilinmeyen_deger_atlanir_dosya_degismez(ctx_modul, kurulum, capsys):
    proje, hafiza = kurulum
    metin = "---\ntitle: E\nlifecycle: uydurma\n---\n\ngovde\n"
    p = _yaz_ham(hafiza, "domain/e.md", metin)

    ctx_modul.komut_migrate(_Args(str(proje)))
    cikti = capsys.readouterr().out

    assert p.read_bytes().decode("utf-8") == metin  # DOKUNULMADI
    assert "bilinmeyen lifecycle" in cikti
    assert "0 not cevrildi" in cikti


def test_kuru_kosum_yazmaz(ctx_modul, kurulum, capsys):
    proje, hafiza = kurulum
    metin = "---\ntitle: B\nlifecycle: current\n---\n\ngovde\n"
    p = _yaz_ham(hafiza, "domain/b.md", metin)

    ctx_modul.komut_migrate(_Args(str(proje), kuru=True))
    cikti = capsys.readouterr().out

    assert p.read_bytes().decode("utf-8") == metin
    assert "KURU KOSUM" in cikti


def test_satir_sonu_KORUNUR(ctx_modul, kurulum, capsys):
    """REGRESYON: migrate, Windows'ta her notu CRLF'e ceviriyordu.

    Iki ayri hata ust uste binmisti: `read_text` evrensel satir sonu cevirisi
    yapip dosyanin gercek ekini gizliyordu, `write_text` de metin modunda
    yazip "\\n"i "\\r\\n"e ceviriyordu. Sonuc: tek satirlik bir degisiklik,
    diff'te dosyanin TAMAMI olarak goruluyordu — komutun "gerisine dokunmaz"
    sozunun tam tersi. Ayni kural `_blok_degistir`de zaten yaziliydi.
    """
    proje, hafiza = kurulum
    lf = "---\ntitle: LF\nlifecycle: current\n---\n\ngovde\n"
    crlf = "---\r\ntitle: CRLF\r\nlifecycle: current\r\n---\r\n\r\ngovde\r\n"
    p_lf = _yaz_ham(hafiza, "domain/lf.md", lf)
    p_crlf = _yaz_ham(hafiza, "domain/crlf.md", crlf)

    ctx_modul.komut_migrate(_Args(str(proje)))
    capsys.readouterr()

    sonuc_lf = p_lf.read_bytes()
    sonuc_crlf = p_crlf.read_bytes()

    assert b"\r\n" not in sonuc_lf, "LF dosya CRLF'e cevrilmis"
    assert sonuc_crlf.count(b"\r\n") == sonuc_crlf.count(b"\n"), "CRLF dosya LF'e cevrilmis"
    # Icerik gercekten donustu mu (bos gecis olmasin)
    assert b"status: active" in sonuc_lf and b"status: active" in sonuc_crlf


def test_son_newline_yoksa_eklenmez(ctx_modul, kurulum, capsys):
    proje, hafiza = kurulum
    p = _yaz_ham(hafiza, "domain/son.md",
                 "---\ntitle: S\nlifecycle: current\n---\n\nson newline yok")

    ctx_modul.komut_migrate(_Args(str(proje)))
    capsys.readouterr()

    assert not p.read_bytes().endswith(b"\n")


def test_govde_ve_diger_alanlar_bozulmaz(ctx_modul, kurulum, capsys):
    proje, hafiza = kurulum
    p = _yaz_ham(hafiza, "domain/z.md",
                 "---\ntitle: Z\nlifecycle: current\ndescription: aciklama\n"
                 "updated: 2026-08-13\n---\n\n"
                 "Govde `kod` ve [[link]] ve — tire.\nIkinci satir.\n")

    ctx_modul.komut_migrate(_Args(str(proje)))
    capsys.readouterr()

    sonuc = p.read_text(encoding="utf-8")
    assert "description: aciklama" in sonuc
    assert "updated: 2026-08-13" in sonuc
    assert "Govde `kod` ve [[link]] ve — tire.\nIkinci satir.\n" in sonuc
    assert "lifecycle:" not in sonuc
