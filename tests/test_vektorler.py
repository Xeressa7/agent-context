"""Altin vektorler — dilden bagimsiz davranis sozlesmesi.

`tests/vectors/*.json` dosyalari `ctx`'in davranisini implementasyondan bagimsiz
tarif eder. Bu modul onlari PYTHON'a kosar; planlanan Go implementasyonu AYNI
dosyalari okuyup ayni sonucu uretecek.

Vektor dosyalari testin girdisidir, ciktisi degil — buradan uretilmezler. Aksi
halde "kendi kendini onaylayan" bir sozlesme olurdu.
"""
import json
from pathlib import Path

import pytest

VEKTOR = Path(__file__).parent / "vectors"


def yukle(ad: str) -> dict:
    return json.loads((VEKTOR / ad).read_text(encoding="utf-8"))


def _kimlik(vaka: dict, *anahtarlar) -> str:
    """Test kimligi: hangi vaka duserse dogrudan gorunsun."""
    return " · ".join(str(vaka.get(a, "")) for a in anahtarlar)[:60]


# --------------------------------------------------------- normalize.json

NORMALIZE = yukle("normalize.json")["cases"]


@pytest.mark.parametrize("vaka", NORMALIZE, ids=[v["input"][:20] or "bos" for v in NORMALIZE])
def test_normalizasyon(ctx_modul, vaka):
    assert ctx_modul.indir(vaka["input"]) == vaka["expect"], vaka["why"]


# ---------------------------------------------------------- checksum.json

CHECKSUM = yukle("checksum.json")


@pytest.mark.parametrize("vaka", CHECKSUM["luhn"], ids=[v["input"][:18] for v in CHECKSUM["luhn"]])
def test_luhn(ctx_modul, vaka):
    assert ctx_modul._luhn_gecerli(vaka["input"]) is vaka["expect"], vaka["why"]


@pytest.mark.parametrize("vaka", CHECKSUM["iban"], ids=[v["input"][:18] for v in CHECKSUM["iban"]])
def test_iban_mod97(ctx_modul, vaka):
    assert ctx_modul._iban_gecerli(vaka["input"]) is vaka["expect"], vaka["why"]


# ----------------------------------------------------------- scoring.json

SCORING = yukle("scoring.json")


def test_agirlik_tablosu_vektorle_ayni(ctx_modul):
    """Vektorler mutlak puan bekliyor; agirliklar kayarsa once BURASI kirilsin."""
    kodda = {}
    for goruntu, anahtar, agirlik, _mod in ctx_modul._ALAN_TANIM:
        kodda[anahtar.replace("başlık", "baslik").replace("bileşen", "bilesen")
                     .replace("açıklama", "aciklama").replace("gövde", "govde")] = agirlik
    assert kodda == SCORING["weights"]
    assert ctx_modul.KAPSAMA_TABANI == SCORING["coverage_base"]


@pytest.mark.parametrize("vaka", SCORING["cases"],
                         ids=[f'{v["path"]}|{v["query"]}'[:40] for v in SCORING["cases"]])
def test_skorlama(ctx_modul, vaka):
    kelimeler = [ctx_modul.indir(k) for k in vaka["query"].split() if k.strip()]
    ifade = ctx_modul.indir(" ".join(vaka["query"].split()))
    fm = {"title": vaka["title"], "description": vaka["description"]}
    metin = "---\n---\n" + vaka["body"]

    sonuc = ctx_modul._not_puanla(Path(vaka["path"]), fm, metin, kelimeler, ifade)

    if vaka["score"] is None:
        assert sonuc is None, vaka["why"]
        return
    assert sonuc is not None, vaka["why"]
    puan, gerekce, eslesen = sonuc
    assert puan == vaka["score"], vaka["why"]
    assert gerekce == vaka["reasons"], vaka["why"]
    assert eslesen == vaka["matched"], vaka["why"]


# -------------------------------------------------------- visibility.json

VISIBILITY = yukle("visibility.json")


@pytest.mark.parametrize("vaka", VISIBILITY["cases"],
                         ids=[_kimlik(v, "note", "repo_scope") for v in VISIBILITY["cases"]])
def test_gorunurluk(ctx_modul, vaka):
    sonuc = ctx_modul.gorunur(vaka["note"], vaka["repo_scope"], vaka.get("archive", False))
    assert sonuc is vaka["visible"], vaka["why"]


@pytest.mark.parametrize("vaka", VISIBILITY["validity"],
                         ids=[str(v["note"])[:40] for v in VISIBILITY["validity"]])
def test_gecerlilik_cevrimi(ctx_modul, vaka):
    status, scope = ctx_modul._gecerlilik(vaka["note"])
    assert status == vaka["status"]
    assert scope == vaka["scope"]


# ------------------------------------------------------- frontmatter.json

FRONTMATTER = yukle("frontmatter.json")


@pytest.mark.parametrize("vaka", FRONTMATTER["parse"],
                         ids=[v["text"][:24].replace("\n", "\\n") for v in FRONTMATTER["parse"]])
def test_frontmatter_ayristirma(ctx_modul, vaka):
    sonuc = ctx_modul.frontmatter(vaka["text"])
    for anahtar, deger in vaka["expect"].items():
        assert sonuc.get(anahtar) == deger, vaka["why"]
    if not vaka["expect"]:
        assert sonuc == {}, vaka["why"]


@pytest.mark.parametrize("vaka", FRONTMATTER["list"],
                         ids=[str(v["input"])[:24] for v in FRONTMATTER["list"]])
def test_liste_normalizasyonu(ctx_modul, vaka):
    assert ctx_modul._liste(vaka["input"]) == vaka["expect"], vaka["why"]


# ------------------------------------------------------------- butunluk

def test_her_vektor_dosyasi_kosuluyor():
    """Yeni bir vektor dosyasi eklenip bu modulde unutulursa yakalansin."""
    beklenen = {"normalize.json", "checksum.json", "scoring.json",
                "visibility.json", "frontmatter.json"}
    diskte = {p.name for p in VEKTOR.glob("*.json")}
    assert diskte == beklenen, f"kosulmayan/eksik vektor dosyasi: {diskte ^ beklenen}"


def test_her_vakanin_gerekcesi_var():
    """`why` alani zorunlu: vektor degistirilecekse once NEDEN var oldugu okunmali."""
    for dosya in VEKTOR.glob("*.json"):
        veri = json.loads(dosya.read_text(encoding="utf-8"))
        for anahtar, deger in veri.items():
            if not isinstance(deger, list):
                continue
            for i, vaka in enumerate(deger):
                assert vaka.get("why"), f"{dosya.name} · {anahtar}[{i}] · `why` eksik"
