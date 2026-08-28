"""Python ↔ Go PARITE testi — iki implementasyon ayni ciktiyi mi veriyor?

Altin vektorler (tests/vectors) fonksiyon seviyesinde davranisi kilitler; bu
dosya UCTAN UCA olani kilitler: ayni hafizada, ayni komut, BAYT BAYT ayni cikti.
README'deki bitirme olcutu tam olarak budur.

Neden ayri bir katman: vektorler `Fold`, `ScoreNote`, `Visible` gibi saf
parcalari kanitlar ama bir komutun ciktisi bunlarin BILESIMIDIR — siralama,
kirpma, kapsam metni, arsiv bolumu, tasma satiri. 2026-08-13'te olculdu ki
frontmatter ayristiricisi Go'da hic YOKTU ve vektorlerin `parse` bolumu
sessizce kosmuyordu; boyle bir boslugu ancak uctan uca karsilastirma yakalar.

Go kurulu degilse ya da derleme basarisizsa test ATLANIR: Python surumu hala
kanoniktir, Go'nun yoklugu bir hata degildir.
"""
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

KOK = Path(__file__).resolve().parent.parent
PY_CTX = KOK / "bin" / "ctx.py"
GO_DIZIN = KOK / "go"


def _kos(komut, cwd):
    r = subprocess.run(komut, cwd=str(cwd), capture_output=True, text=True,
                       encoding="utf-8", errors="replace", timeout=60)
    # Satir sonu bicimi platformun isi, davranisin degil: Python Windows'ta
    # metin modunda \r\n yazar, Go dogrudan \n. Karsilastirilan sey ICERIK.
    return r.stdout.replace("\r\n", "\n"), r.returncode


@pytest.fixture(scope="module")
def go_ctx(tmp_path_factory):
    """Go binary'sini derler; Go yoksa butun modulu atlar."""
    if shutil.which("go") is None:
        pytest.skip("go kurulu degil — parite testi atlandi")
    hedef = tmp_path_factory.mktemp("gobin") / ("ctx.exe" if os.name == "nt" else "ctx")
    r = subprocess.run(["go", "build", "-o", str(hedef), "./cmd/ctx"],
                       cwd=str(GO_DIZIN), capture_output=True, text=True, timeout=300)
    if r.returncode != 0:
        pytest.skip(f"go build basarisiz: {r.stderr.strip()[:400]}")
    return hedef


@pytest.fixture(scope="module")
def parite_hafizasi(tmp_path_factory):
    """Kenar durumlari BILEREK iceren kucuk bir hafiza + tuketici repo.

    Her dosya bir seyi kanitlamak icin var: arsivlenmis not, eski `lifecycle`
    alani, kapsamsiz (evrensel) not, frontmatter'i bozuk not, devralinan arsiv
    indeksi, skip_dirs, ve arama normalizasyonunu zorlayan Turkce metin.
    """
    kok = tmp_path_factory.mktemp("parite")
    proje, hafiza = kok / "proje", kok / "mem"
    (proje / "src" / "derin").mkdir(parents=True)
    for d in ("facts", "pitfalls", "gecis", "tasks", "sessions"):
        (hafiza / d).mkdir(parents=True)

    (proje / "context.yaml").write_text(
        "version: 1\nproject: parite\nmemory_path: ../mem\n"
        "scope: [current, server]\narchive_index: gecis/legacy-index.md\n",
        encoding="utf-8")
    (hafiza / "ctx.yaml").write_text("skip_dirs: [gecis]\npii_profiles: [tr]\n",
                                     encoding="utf-8")
    (hafiza / "README.md").write_text("kok dosyasi — not degil\n", encoding="utf-8")
    (hafiza / "tasks" / "OPEN.md").write_text(
        "acik isler\n- [Fatura task](billing-task.md)\n", encoding="utf-8")
    (hafiza / "sessions" / "CURRENT.md").write_text("oturum\n", encoding="utf-8")
    (hafiza / "tasks" / "billing-task.md").write_text(
        "---\ntitle: Fatura task\ndescription: Acik fatura isi\n"
        "type: task\nstatus: active\nupdated: 2026-08-13\n---\n\n"
        "Fatura isi.\n", encoding="utf-8")

    (hafiza / "facts" / "billing.md").write_text(
        '---\ntitle: "Fatura akışı"\n'
        "description: Fatura ↔ hakediş eşlemesi ve kesinti kalemleri\n"
        "type: domain\nupdated: 2026-08-01\nscope: [current]\n"
        "tags: fatura, hakedis\n---\n\n"
        "Gövdede `distinct_lab_names` geçiyor ve İstanbul şubesi.\n",
        encoding="utf-8")
    (hafiza / "facts" / "arsivli.md").write_text(
        "---\ntitle: Arsivli not\ndescription: Arsivlenmis fatura notu\n"
        "type: domain\nupdated: 2025-01-01\nstatus: archived\n---\n\n"
        "Fatura hakkinda eski bilgi.\n", encoding="utf-8")
    (hafiza / "facts" / "eski-eksen.md").write_text(
        "---\ntitle: Eski eksen\ndescription: Legacy kapsamli fatura notu\n"
        "type: domain\nupdated: 2026-01-01\nlifecycle: legacy\n---\n\n"
        "Fatura ve legacy.\n", encoding="utf-8")
    (hafiza / "pitfalls" / "evrensel.md").write_text(
        "---\ntitle: Evrensel ders\n"
        "description: Kapsamsiz not her yerde gecerli — fatura\n"
        "type: pitfall\nupdated: 2026-08-10\n---\n\nBu not evrenseldir.\n",
        encoding="utf-8")
    (hafiza / "pitfalls" / "bozuk.md").write_text(
        "title: kapanmamis frontmatter\ndescription: frontmatter'siz fatura notu\n",
        encoding="utf-8")
    (hafiza / "gecis" / "legacy-index.md").write_text(
        "# Devralinan indeks\n"
        "- Fatura kesintileri eski sistemde ayri tabloda tutuluyordu\n"
        "- Fatura ve hakedis iliskisi elle giriliyordu\n"
        "- Hakedis yil sonu devri\n"
        "- Fatura numaralari ardisik degil\n"
        "- Fatura iptali kaydi\n"
        "- Fatura arsivi CD'de\n"
        "- Alakasiz satir\n", encoding="utf-8")
    return proje


# Her vaka bir davranisi zorlar; "why" yorumda degil, vakanin kendisinde.
SEARCH_VAKALARI = [
    (["fatura"], "kapsam filtresi: arsivli ve legacy notlar gelmemeli"),
    (["--all", "fatura"], "--all: filtre kapali, hepsi gelmeli"),
    (["fatura hakedis kesinti"], "cok kelime: kapsama carpani + n/m gosterimi"),
    (["distinct_lab_names"], "backtick icindeki tanimlayici kod alanindan"),
    (["billing"], "dosya adi kokune birebir esitlik en yuksek agirlik"),
    (["İstanbul"], "Turkce buyuk I — casefold tuzagi"),
    (["istanbul subesi"], "aksansiz sorgu aksanli metne eslesmeli"),
    (["hakediş"], "aksanli sorgu"),
    (["--top", "1", "fatura"], "tasma satiri: (+N sonuç daha)"),
    (["--top", "100", "fatura"], "tasma yok"),
    (["--min", "40", "fatura"], "puan esigi"),
    (["i"], "arsiv bolumu 5 satirla sinirli + tasma satiri"),
    (["olmayan-kelime-xyz"], "eslesme yok"),
    (["gecis"], "skip_dirs: atlanan klasor notu aramada cikmamali"),
]


@pytest.mark.parametrize("argv,neden", SEARCH_VAKALARI,
                         ids=[" ".join(a) for a, _ in SEARCH_VAKALARI])
def test_search_paritesi(go_ctx, parite_hafizasi, argv, neden):
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "search", *argv], parite_hafizasi)
    go_cikti, go_kod = _kos([str(go_ctx), "search", *argv], parite_hafizasi)
    assert go_cikti == py_cikti, f"cikti farkli ({neden})"
    assert go_kod == py_kod, f"cikis kodu farkli ({neden})"


def test_manifest_yukari_arama_paritesi(go_ctx, parite_hafizasi):
    """Ic ice dizinden kosuldugunda ikisi de ayni manifesti bulmali."""
    derin = parite_hafizasi / "src" / "derin"
    py_cikti, _ = _kos([sys.executable, str(PY_CTX), "search", "fatura"], derin)
    go_cikti, _ = _kos([str(go_ctx), "search", "fatura"], derin)
    assert go_cikti == py_cikti
    assert "billing.md" in go_cikti  # bos ciktiyla sahte gecis olmasin


INDEX_VAKALARI = [
    ([], "haritayi bas"),
    (["--all"], "--all: filtre kapali"),
]


@pytest.mark.parametrize("argv,neden", INDEX_VAKALARI,
                         ids=["index", "index --all"])
def test_index_paritesi(go_ctx, parite_hafizasi, argv, neden):
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "index", *argv], parite_hafizasi)
    go_cikti, go_kod = _kos([str(go_ctx), "index", *argv], parite_hafizasi)
    assert go_cikti == py_cikti, neden
    assert go_kod == py_kod
    assert "Hafıza haritası" in go_cikti  # bos ciktiyla sahte gecis olmasin


# Blok yazma vakalari: baslangic icerigi, argv, ne kanitliyor.
BLOK = ("# Proje\n\n<!-- ctx:index:basla -->\neski icerik\n"
        "<!-- ctx:index:bitir -->\n\nson.\n")
YAZMA_VAKALARI = [
    (BLOK, ["--kontrol"], "bayat blok bildirilir, YAZILMAZ"),
    (BLOK, ["--yaz"], "blok tazelenir"),
    ("isaret yok\n", ["--yaz"], "isaretsiz hedef: ne yapilacagi soylenir"),
    (BLOK.replace("\n", "\r\n"), ["--yaz"], "CRLF dosyanin satir sonu KORUNUR"),
    (None, ["--yaz"], "hedef dosya hic yok"),
]


@pytest.mark.parametrize("baslangic,argv,neden", YAZMA_VAKALARI,
                         ids=[n for _, _, n in YAZMA_VAKALARI])
def test_index_yazma_paritesi(go_ctx, parite_hafizasi, baslangic, argv, neden):
    """Iki implementasyon ayni ciktiyi VE ayni dosya icerigini uretmeli.

    Ayni dizinde sirayla kosulur: yol adlari ciktida gectigi icin iki ayri
    kopya kullanmak sahte fark uretir.
    """
    hedef = parite_hafizasi / "AGENTS.md"

    def kur():
        if baslangic is None:
            hedef.unlink(missing_ok=True)
        else:
            hedef.write_bytes(baslangic.encode("utf-8"))

    kur()
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "index", *argv], parite_hafizasi)
    py_dosya = hedef.read_bytes() if hedef.exists() else None

    kur()
    go_cikti, go_kod = _kos([str(go_ctx), "index", *argv], parite_hafizasi)
    go_dosya = hedef.read_bytes() if hedef.exists() else None

    assert go_cikti == py_cikti, f"cikti farkli ({neden})"
    assert go_kod == py_kod, f"cikis kodu farkli ({neden})"
    assert go_dosya == py_dosya, f"dosya icerigi farkli — BAYT BAYT ({neden})"
    hedef.unlink(missing_ok=True)


@pytest.fixture(scope="module")
def bozuk_hafiza(tmp_path_factory):
    """Her bulgu tipini BILEREK tetikleyen hafiza.

    Gercek korpus temizdir — dolayisiyla `validate`in hata yollarinin hicbiri
    orada kosmaz. Iki implementasyonun ayni RAPORU verdigini kanitlamak icin
    hatalarin uretilmesi gerekir.
    """
    kok = tmp_path_factory.mktemp("bozuk")
    proje, hafiza = kok / "proje", kok / "mem"
    proje.mkdir()
    for d in ("facts", "pitfalls", "tasks", "sessions"):
        (hafiza / d).mkdir(parents=True)
    (proje / "context.yaml").write_text(
        "version: 1\nproject: bozuk\nmemory_path: ../mem\nscope: [current]\n", encoding="utf-8")
    # Uc profil birden: profile-ozel desenlerin sirasi da karsilastirilsin.
    (hafiza / "ctx.yaml").write_text("pii_profiles: [tr, us, de]\nreview_days: 30\n",
                                     encoding="utf-8")
    (hafiza / "README.md").write_text("# hafiza\n", encoding="utf-8")
    (hafiza / "tasks" / "OPEN.md").write_text("x\n", encoding="utf-8")
    (hafiza / "sessions" / "CURRENT.md").write_text("x\n", encoding="utf-8")

    y = lambda ad, metin: (hafiza / ad).write_text(metin, encoding="utf-8")
    y("facts/a-frontmatter-yok.md", "frontmatter yok bu notta\n")
    y("facts/b-gecersiz-lifecycle.md",
      "---\nlifecycle: uydurma\ntitle: B\n---\n\ngovde\n")
    y("facts/c-eski-lifecycle.md",
      "---\nlifecycle: current\ntitle: C\ndescription: D\ntype: domain\n"
      "updated: 2026-08-01\n---\n\ngovde\n")
    # Tirnak iceren deger: mesajlar Python repr() ile uretiliyor.
    y("facts/d-gecersiz-status.md",
      "---\nstatus: yarim\ntitle: D'li baslik\ndescription: D\ntype: domain\n"
      "updated: 2020-01-01\n---\n\ngovde\n")
    y("facts/e-linkler.md",
      "---\ntitle: C\ndescription: D\ntype: domain\nupdated: 2026-08-13\n"
      "superseded_by: facts/olmayan.md\n---\n\n"
      "[link](./yok.md) ve [dis](https://a.b/c.md) ve [[cozumlenmez]] "
      "ve [[c-eski-lifecycle]]\n")
    y("facts/f-tarih.md",
      "---\ntitle: F\ndescription: D\ntype: domain\nupdated: dun\n---\n\ngovde\n")
    y("facts/i-review-due.md",
      "---\ntitle: Bayat\ndescription: D\ntype: domain\nupdated: 2026-01-01\n---\n\ngovde\n")
    # PII: gercek olanlar YAKALANMALI, saglama toplamindan gecmeyen sahte kart
    # (yil dizisi) ve degeri olmayan ADMIN_SECRET YAKALANMAMALI.
    y("pitfalls/g-pii.md",
      "---\ntitle: G\ndescription: D\ntype: domain\nupdated: 2026-08-13\n---\n\n"
      "mail: kisi@ornek.com\nkart: 4111 1111 1111 1111\n"
      "sahte kart: 2024 2025 2026 2027\nIBAN: TR330006100519786457841326\n"
      "TC: 12345678950\ntel: 0532 123 45 67\nSSN: 123-45-6789\n"
      'api_key: "abcdef0123456789abcdef"\n'
      "ADMIN_SECRET isteniyor, degeri disarida\n")
    # Arsivlenmis + cok eski: review-due KOSMAMALI.
    y("pitfalls/h-arsiv.md",
      "---\ntitle: H\ndescription: D\ntype: pitfall\nupdated: 2019-01-01\n"
      "status: archived\n---\n\ngovde\n")
    y("tasks/unutulmus-brief.md",
      "---\ntitle: Unutulmus brief\ndescription: D\ntype: task\n"
      "status: active\nupdated: 2026-08-13\n---\n\nacik is.\n")
    return proje, hafiza


@pytest.mark.parametrize("argv", [[], ["--tam"]], ids=["validate", "validate --tam"])
def test_validate_paritesi(go_ctx, bozuk_hafiza, argv):
    proje, _ = bozuk_hafiza
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "validate", *argv], proje)
    go_cikti, go_kod = _kos([str(go_ctx), "validate", *argv], proje)
    assert go_cikti == py_cikti
    assert go_kod == py_kod
    # Vakanin gercekten bulgu urettiginden emin ol: bos raporlar sessizce esit olur.
    assert "hata ·" in go_cikti


def test_validate_memory_bayragi_paritesi(go_ctx, bozuk_hafiza, tmp_path):
    """`--memory` yolu: pre-commit kancasi manifest OLMADAN bunu cagirir."""
    proje, hafiza = bozuk_hafiza
    for arg in (str(hafiza), str(hafiza) + "-olmayan", str(proje)):
        py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "validate", "--memory", arg, "--tam"],
                                tmp_path)
        go_cikti, go_kod = _kos([str(go_ctx), "validate", "--memory", arg, "--tam"], tmp_path)
        assert go_cikti == py_cikti, f"--memory {arg}"
        assert go_kod == py_kod, f"--memory {arg}"


def test_manifestsiz_dizin_paritesi(go_ctx, tmp_path):
    """Manifest bulunamayinca ikisi de ayni sekilde ve ayni kodla yakinmali."""
    for komut in (["search", "x"], ["index"], ["validate"]):
        py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), *komut], tmp_path)
        go_cikti, go_kod = _kos([str(go_ctx), *komut], tmp_path)
        assert go_kod == py_kod == 2, komut
        assert go_cikti == py_cikti, komut


def test_doctor_paritesi(go_ctx, parite_hafizasi, bozuk_hafiza, tmp_path):
    """Saglikli kurulum, bulgulu kurulum ve manifestsiz dizin."""
    bozuk_proje, _ = bozuk_hafiza
    for dizin in (parite_hafizasi, bozuk_proje, tmp_path):
        py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "doctor"], dizin)
        go_cikti, go_kod = _kos([str(go_ctx), "doctor"], dizin)
        assert go_cikti == py_cikti, f"doctor @ {dizin}"
        assert go_kod == py_kod, f"doctor @ {dizin}"


# --- close ------------------------------------------------------------------
#
# `ctx close` zamana ve git durumuna bagli calisir: zaman damgasi her kosumda
# degisir, dolayisiyla karsilastirmadan once MASKELENIR. Maskelenen tek sey
# odur — kalan her sey (log satirinin bicimi dahil) bayt bayt karsilastirilir.

TS_DESENI = re.compile(r'"ts": "[^"]*"')
SON_KARAR_DESENI = re.compile(r"son karar: \S+ ")


def _maskele(metin):
    metin = TS_DESENI.sub('"ts": "TS"', metin)
    return SON_KARAR_DESENI.sub("son karar: TS ", metin)


@pytest.fixture(scope="module")
def kapanis_alani(tmp_path_factory):
    """Git reposu olan bir proje + tek notluk hafiza + isaretli AGENTS.md."""
    if shutil.which("git") is None:
        pytest.skip("git yok — close paritesi atlandi")
    kok = tmp_path_factory.mktemp("kapanis")
    proje, hafiza = kok / "proj", kok / "mem"
    proje.mkdir()
    for d in ("pitfalls", "tasks", "sessions"):
        (hafiza / d).mkdir(parents=True)
    (proje / "context.yaml").write_text(
        "version: 1\nproject: kapanis\nmemory_path: ../mem\nscope: [current]\n", encoding="utf-8")
    (hafiza / "README.md").write_text("# hafiza\n", encoding="utf-8")
    (hafiza / "tasks" / "OPEN.md").write_text("x\n", encoding="utf-8")
    (hafiza / "sessions" / "CURRENT.md").write_text("x\n", encoding="utf-8")
    (hafiza / "pitfalls" / "ornek.md").write_text(
        "---\ntitle: Ornek\ndescription: D\ntype: pitfall\nupdated: 2026-08-13\n---\n\ngovde\n",
        encoding="utf-8")
    for komut in (["git", "init", "-q", "."],
                  ["git", "add", "-A"],
                  ["git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "ilk"]):
        subprocess.run(komut, cwd=str(proje), capture_output=True, timeout=60)
    # Kirli dosya = "bu oturumda is yapildi" sinyali; close'un tetiklendigi hal.
    (proje / "kirli.txt").write_text("kirli\n", encoding="utf-8")
    return proje, hafiza


BLOK_ILK = "# Proje\n\n<!-- ctx:index:basla -->\neski\n<!-- ctx:index:bitir -->\n"
ESKI_KAYIT = ('{"ts": "2026-08-12T10:00:00", "repo": "proj", "head": "eskihead", '
              '"kirli": 2, "karar": "none"}\n')

CLOSE_VAKALARI = [
    ("", [], "karar bekleniyor — cikis 1, 'karar VERMEMEK gecersizdir'"),
    (ESKI_KAYIT, [], "onceki karar satiri gosterilir"),
    ("", ["--kontrol"], "kirli is + kayit yok = uyari"),
    (ESKI_KAYIT, ["--kontrol"], "kayit baska HEAD'e ait: yine uyari"),
    ("", ["--yok"], "gerekcesiz 'bulgu yok' karari"),
    ("", ["--yok", "kalici bulgu cikmadi"], "gerekceli karar"),
    ("", ["--yok", 'ölçüldü: "bulgu" yok & bitti'], "tirnak + unicode + & (JSON kacislari)"),
    ("", ["--not", "pitfalls/yok.md"], "olmayan not: HATA, cikis 2"),
    ("", ["--not", "pitfalls/ornek.md"], "not kaydi + harita tazeleme"),
    ("", ["--not", "pitfalls/ornek.md", "--not", "tasks/OPEN.md"], "birden cok not"),
    (ESKI_KAYIT * 2, ["--yok"], "Capture Decision Coverage sayaci"),
]


@pytest.mark.parametrize("onceki,argv,neden", CLOSE_VAKALARI,
                         ids=[n for _, _, n in CLOSE_VAKALARI])
def test_close_paritesi(go_ctx, kapanis_alani, onceki, argv, neden):
    """Cikti, YAZILAN LOG SATIRI ve tazelenen AGENTS.md — ucu birden."""
    proje, hafiza = kapanis_alani
    log = hafiza / "sessions" / "capture-log.jsonl"
    agents = proje / "AGENTS.md"

    def kur():
        log.unlink(missing_ok=True)
        if onceki:
            log.write_text(onceki, encoding="utf-8")
        agents.write_text(BLOK_ILK, encoding="utf-8")

    def topla():
        return (_maskele(log.read_text(encoding="utf-8")) if log.exists() else None,
                agents.read_text(encoding="utf-8"))

    kur()
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "close", *argv], proje)
    py_log, py_agents = topla()

    kur()
    go_cikti, go_kod = _kos([str(go_ctx), "close", *argv], proje)
    go_log, go_agents = topla()

    assert _maskele(go_cikti) == _maskele(py_cikti), f"cikti ({neden})"
    assert go_kod == py_kod, f"cikis kodu ({neden})"
    assert go_log == py_log, f"capture-log satiri ({neden})"
    assert go_agents == py_agents, f"AGENTS.md ({neden})"


def test_close_temiz_repoda_sessiz(go_ctx, kapanis_alani):
    """Kirli dosya yoksa `--kontrol` uyarmamali: kapi yalniz IS YAPILINCA kapanir."""
    proje, hafiza = kapanis_alani
    kirli = proje / "kirli.txt"
    yedek = kirli.read_text(encoding="utf-8") if kirli.exists() else None
    kirli.unlink(missing_ok=True)
    subprocess.run(["git", "add", "-A"], cwd=str(proje), capture_output=True, timeout=60)
    subprocess.run(["git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "temiz"],
                   cwd=str(proje), capture_output=True, timeout=60)
    try:
        py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "close", "--kontrol"], proje)
        go_cikti, go_kod = _kos([str(go_ctx), "close", "--kontrol"], proje)
        assert go_cikti == py_cikti == "✓ capture karari guncel\n"
        assert go_kod == py_kod == 0
    finally:
        if yedek is not None:
            kirli.write_text(yedek, encoding="utf-8")


@pytest.fixture(scope="module")
def cok_tuketici(tmp_path_factory):
    """Bir hafiza, UC tuketici repo — biri legacy kapsamli.

    Kapsam filtresi repodan repoya degistigi icin her tuketici KENDI
    manifest'iyle hesaplanmali; tek gövde uretip hepsine yazmak legacy repoyu
    yanlis doldururdu.
    """
    kok = tmp_path_factory.mktemp("cok")
    hafiza = kok / "mem"
    for d in ("facts", "tasks", "sessions"):
        (hafiza / d).mkdir(parents=True)
    (hafiza / "README.md").write_text("# h\n", encoding="utf-8")
    (hafiza / "tasks" / "OPEN.md").write_text("x\n", encoding="utf-8")
    (hafiza / "sessions" / "CURRENT.md").write_text("x\n", encoding="utf-8")
    (hafiza / "ctx.yaml").write_text(
        "consumers: [../a, ../b, ../c]\n", encoding="utf-8")
    (hafiza / "facts" / "ortak.md").write_text(
        "---\ntitle: Ortak\ndescription: Her kapsamda gecerli\ntype: domain\n"
        "updated: 2026-08-13\n---\n\ngovde\n", encoding="utf-8")
    (hafiza / "facts" / "eski.md").write_text(
        "---\ntitle: Eski\ndescription: Yalniz legacy\ntype: domain\n"
        "updated: 2026-08-13\nscope: [legacy]\n---\n\ngovde\n", encoding="utf-8")

    blok = "# R\n\n<!-- ctx:index:basla -->\nESKI\n<!-- ctx:index:bitir -->\n"
    for ad, kapsam in (("a", "current"), ("b", "current"), ("c", "legacy")):
        d = kok / ad
        d.mkdir()
        (d / "context.yaml").write_text(
            f"version: 1\nproject: {ad}\nmemory_path: ../mem\nscope: [{kapsam}]\n",
            encoding="utf-8")
        (d / "AGENTS.md").write_text(blok, encoding="utf-8")
    return kok


def test_cok_tuketici_tazeleme_paritesi(go_ctx, cok_tuketici):
    """Bir repodan kosulan `index --yaz` UC repoyu birden tazelemeli."""
    kok = cok_tuketici
    blok = "# R\n\n<!-- ctx:index:basla -->\nESKI\n<!-- ctx:index:bitir -->\n"

    def kur():
        for ad in ("a", "b", "c"):
            (kok / ad / "AGENTS.md").write_text(blok, encoding="utf-8")

    def topla():
        return {ad: (kok / ad / "AGENTS.md").read_text(encoding="utf-8") for ad in ("a", "b", "c")}

    kur()
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "index", "--yaz"], kok / "a")
    py_dosyalar = topla()

    kur()
    go_cikti, go_kod = _kos([str(go_ctx), "index", "--yaz"], kok / "a")
    go_dosyalar = topla()

    assert go_cikti == py_cikti
    assert go_kod == py_kod
    assert go_dosyalar == py_dosyalar
    # Uc repo da tazelendi mi — ve legacy repo KENDI kapsamiyla mi?
    for ad in ("a", "b", "c"):
        assert "ESKI" not in go_dosyalar[ad], f"{ad} tazelenmedi"
        assert "ortak.md" in go_dosyalar[ad]
    assert "eski.md" in go_dosyalar["c"], "legacy repo legacy notu gormeli"
    assert "eski.md" not in go_dosyalar["a"], "current repo legacy notu GORMEMELI"


def test_cok_tuketici_bayatlik_paritesi(go_ctx, cok_tuketici):
    """Baska bir repodaki bayat blok, buradan kosulan --kontrol ile gorulmeli."""
    kok = cok_tuketici
    _kos([sys.executable, str(PY_CTX), "index", "--yaz"], kok / "a")   # hepsi guncel
    hedef = kok / "c" / "AGENTS.md"
    hedef.write_text(hedef.read_text(encoding="utf-8").replace("ortak.md", "X.md", 1),
                     encoding="utf-8")

    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "index", "--kontrol"], kok / "a")
    go_cikti, go_kod = _kos([str(go_ctx), "index", "--kontrol"], kok / "a")

    assert go_cikti == py_cikti
    assert go_kod == py_kod == 1, "baska repodaki bayatlik cikis kodunu etkilemeli"
    assert "BAYAT" in go_cikti


def test_close_tum_tuketicileri_tazeler_paritesi(go_ctx, cok_tuketici):
    """`ctx close --not` yeni notu kaydederken UC repoyu birden tazelemeli.

    Yeni not hepsinin blogunu ayni anda bayatlatir; `close`u kostugun repo ozel
    degildir. Yalnizca mevcut repoyu tazelemek, iki reponun dokuz gun geride
    kalmasina yol acan seydi.
    """
    if shutil.which("git") is None:
        pytest.skip("git yok")
    kok = cok_tuketici
    blok = "# R\n\n<!-- ctx:index:basla -->\nESKI\n<!-- ctx:index:bitir -->\n"
    log = kok / "mem" / "sessions" / "capture-log.jsonl"
    # `close` git sinyali okur; tuketici 'a' bir repo olsun.
    for komut in (["git", "init", "-q", "."],):
        subprocess.run(komut, cwd=str(kok / "a"), capture_output=True, timeout=60)
    (kok / "a" / "kirli.txt").write_text("is\n", encoding="utf-8")

    def kur():
        log.unlink(missing_ok=True)
        for ad in ("a", "b", "c"):
            (kok / ad / "AGENTS.md").write_text(blok, encoding="utf-8")

    def topla():
        return {ad: (kok / ad / "AGENTS.md").read_text(encoding="utf-8") for ad in ("a", "b", "c")}

    kur()
    py_cikti, py_kod = _kos(
        [sys.executable, str(PY_CTX), "close", "--not", "facts/ortak.md"], kok / "a")
    py_dosyalar = topla()

    kur()
    go_cikti, go_kod = _kos([str(go_ctx), "close", "--not", "facts/ortak.md"], kok / "a")
    go_dosyalar = topla()

    assert _maskele(go_cikti) == _maskele(py_cikti)
    assert go_kod == py_kod == 0
    assert go_dosyalar == py_dosyalar
    for ad in ("a", "b", "c"):
        assert "ESKI" not in go_dosyalar[ad], f"{ad} tazelenmedi — close yalniz kendi repoosunu gordu"
    assert go_cikti.count("hafiza haritasi tazelendi") == 3


# --- init ve migrate ---------------------------------------------------------
#
# Bu iki komut DOSYA URETIR. Dolayisiyla karsilastirma ciktiyla bitmez: uretilen
# dosya agaci ve her dosyanin BAYTLARI da karsilastirilir. Ayni dizinde sirayla
# kosulur — yol adlari ciktida gectigi icin iki ayri kopya sahte fark uretir.

def _agac(kok):
    """Dizin altindaki her dosyanin (goreli yol -> bayt) haritasi. .git haric:
    git'in kendi ic dosyalari zaman ve hash tasiyor, davranisin parcasi degil."""
    cikti = {}
    for p in sorted(Path(kok).rglob("*")):
        if p.is_file() and ".git" not in p.relative_to(kok).parts:
            cikti[p.relative_to(kok).as_posix()] = p.read_bytes()
    return cikti


INIT_VAKALARI = [
    ([], None, "bos dizin: hafiza + context.yaml + AGENTS.md + CLAUDE.md"),
    (["--role", "legacy", "--components", "api, ui"], None, "bayraklar proje adindan SONRA"),
    (["--memory-path", "./hafiza"], None, "hafiza yolu elle verilir"),
    ([], {"AGENTS.md": "eski icerik\n"}, "AGENTS.md MEVCUT: dokunulmaz, router basilir"),
    ([], {"context.yaml": "v: 1\n"}, "context.yaml mevcut"),
    ([], {"AGENTS.md": "a\n", "CLAUDE.md": "b\n", "context.yaml": "c\n"}, "hepsi mevcut"),
]


@pytest.mark.parametrize("argv,onceden,neden", INIT_VAKALARI,
                         ids=[n for _, _, n in INIT_VAKALARI])
def test_init_paritesi(go_ctx, tmp_path, argv, onceden, neden):
    alan = tmp_path / "alan"

    def kur():
        if alan.exists():
            shutil.rmtree(alan, ignore_errors=True)
        (alan / "proj").mkdir(parents=True)
        for ad, metin in (onceden or {}).items():
            (alan / "proj" / ad).write_text(metin, encoding="utf-8")
        return alan / "proj"

    proj = kur()
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "init", "deneme", *argv], proj)
    py_agac = _agac(alan)

    proj = kur()
    go_cikti, go_kod = _kos([str(go_ctx), "init", "deneme", *argv], proj)
    go_agac = _agac(alan)

    assert go_cikti == py_cikti, f"cikti ({neden})"
    assert go_kod == py_kod, f"cikis kodu ({neden})"
    assert sorted(go_agac) == sorted(py_agac), f"uretilen dosya agaci ({neden})"
    for yol in py_agac:
        assert go_agac[yol] == py_agac[yol], f"{yol} icerigi BAYT BAYT farkli ({neden})"


MIGRATE_NOTLARI = {
    "facts/a.md": "---\ntitle: A\nlifecycle: product\ndescription: D\n---\n\ngovde\n",
    "facts/b.md": "---\ntitle: B\nlifecycle: current\n---\n\ngovde\n",
    "facts/c.md": "---\ntitle: C\nlifecycle: legacy\n---\n\ngovde\n",
    "facts/d.md": "---\ntitle: D\nlifecycle: historical\n---\n\ngovde\n",
    "facts/e.md": "---\ntitle: E\nlifecycle: uydurma\n---\n\ngovde\n",
    "facts/f.md": "---\ntitle: F\nstatus: active\n---\n\nzaten yeni\n",
    # Satir sonu ve son-newline kenar durumlari: migrate bunlari KORUMALI.
    "pitfalls/g-crlf.md": "---\r\ntitle: G\r\nlifecycle: current\r\n---\r\n\r\nCRLF\r\n",
    "pitfalls/h-son.md": "---\ntitle: H\nlifecycle: current\n---\n\nson newline yok",
}


@pytest.mark.parametrize("argv", [["--kuru"], []], ids=["migrate --kuru", "migrate"])
def test_migrate_paritesi(go_ctx, tmp_path, argv):
    alan = tmp_path / "m"

    def kur():
        if alan.exists():
            shutil.rmtree(alan, ignore_errors=True)
        proje, hafiza = alan / "proj", alan / "mem"
        proje.mkdir(parents=True)
        for d in ("tasks", "sessions"):
            (hafiza / d).mkdir(parents=True)
        (proje / "context.yaml").write_text(
            "version: 1\nproject: m\nmemory_path: ../mem\nrole: current\n", encoding="utf-8")
        (hafiza / "README.md").write_text("# h\n", encoding="utf-8")
        (hafiza / "tasks" / "OPEN.md").write_text("x\n", encoding="utf-8")
        (hafiza / "sessions" / "CURRENT.md").write_text("x\n", encoding="utf-8")
        for yol, metin in MIGRATE_NOTLARI.items():
            p = hafiza / yol
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_bytes(metin.encode("utf-8"))
        return proje, hafiza

    proje, hafiza = kur()
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "migrate", *argv], proje)
    py_agac = _agac(hafiza)

    proje, hafiza = kur()
    go_cikti, go_kod = _kos([str(go_ctx), "migrate", *argv], proje)
    go_agac = _agac(hafiza)

    assert go_cikti == py_cikti
    assert go_kod == py_kod
    for yol in py_agac:
        assert go_agac[yol] == py_agac[yol], f"{yol} BAYT BAYT farkli"


# --- bootstrap ---------------------------------------------------------------
#
# Bu komut kullanicinin KENDI ~/.claude/settings.json'ini duzenler. Test bunu
# ASLA gercek dosyada yapmaz: HOME/USERPROFILE sahte bir dizine cevrilir.
# Python (ntpath.expanduser) ve Go (os.UserHomeDir) Windows'ta ikisi de
# USERPROFILE'a bakar, POSIX'te ikisi de HOME'a — ikisi birden ayarlanir.

def _sahte_ev_ile(kok):
    ortam = dict(os.environ)
    ortam["HOME"] = str(kok)
    ortam["USERPROFILE"] = str(kok)
    return ortam


def _kos_ortamli(komut, cwd, ortam):
    r = subprocess.run(komut, cwd=str(cwd), env=ortam, capture_output=True, text=True,
                       encoding="utf-8", errors="replace", timeout=120)
    return r.stdout.replace("\r\n", "\n"), r.returncode


KULLANICI_AYARLARI = """{
  "theme": "dark",
  "model": "opus",
  "permissions": {
    "allow": ["Bash(git:*)", "Read"],
    "deny": []
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "echo x"}]
      }
    ]
  },
  "zzz_son_anahtar": true
}"""

# 2026-08-13 oncesi bicim: kanca ayri bir Python script'iydi. Bootstrap bunu
# TANIYIP yerinde guncellemeli — tanimazsa ayni kanca iki kez kosar.
ESKI_BICIM = """{
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "python \\"/x/ctx_index.py\\"", "shell": "bash", "timeout": 20, "statusMessage": "eski"}]}
    ]
  }
}"""

BOOTSTRAP_VAKALARI = [
    (["--kuru"], None, "kuru kosum: hicbir sey yazilmaz"),
    ([], None, "settings.json yok: sifirdan kurulur"),
    ([], KULLANICI_AYARLARI, "kullanicinin ayarlari + ANAHTAR SIRASI korunur"),
    ([], ESKI_BICIM, "eski script kaydi: YERINDE guncellenir, ikinci kez eklenmez"),
    (["--role", "legacy", "--components", "api"], None, "bayraklar"),
]


@pytest.mark.parametrize("argv,ayarlar,neden", BOOTSTRAP_VAKALARI,
                         ids=[n for _, _, n in BOOTSTRAP_VAKALARI])
def test_bootstrap_paritesi(go_ctx, tmp_path, argv, ayarlar, neden):
    alan = tmp_path / "b"

    def kur():
        if alan.exists():
            shutil.rmtree(alan, ignore_errors=True)
        (alan / "home" / ".claude").mkdir(parents=True)
        (alan / "proj").mkdir(parents=True)
        if ayarlar:
            (alan / "home" / ".claude" / "settings.json").write_text(ayarlar, encoding="utf-8")
        return alan / "proj", _sahte_ev_ile(alan / "home")

    def topla():
        ayar = alan / "home" / ".claude" / "settings.json"
        return (_agac(alan),
                ayar.read_text(encoding="utf-8").replace("\r\n", "\n") if ayar.exists() else None)

    proj, ortam = kur()
    py_cikti, py_kod = _kos_ortamli(
        [sys.executable, str(PY_CTX), "bootstrap", "--proje", "deneme", *argv], proj, ortam)
    py_agac, py_ayar = topla()

    proj, ortam = kur()
    go_cikti, go_kod = _kos_ortamli(
        [str(go_ctx), "bootstrap", "--proje", "deneme", *argv], proj, ortam)
    go_agac, go_ayar = topla()

    assert go_cikti == py_cikti, f"cikti ({neden})"
    assert go_kod == py_kod, f"cikis kodu ({neden})"
    assert go_ayar == py_ayar, f"settings.json ({neden}) — anahtar sirasi da dahil"
    assert sorted(go_agac) == sorted(py_agac), f"uretilen dosyalar ({neden})"
    for yol in py_agac:
        assert go_agac[yol] == py_agac[yol], f"{yol} BAYT BAYT farkli ({neden})"


def test_bootstrap_eski_kaydi_yerinde_gunceller(go_ctx, tmp_path):
    """Gecis: eski script kaydi TANINIR, kopyalanmaz.

    Tanimasa ne olurdu: ayni kanca hem eski hem yeni bicimle kayitli kalir ve
    her oturum baslangicinda IKI KEZ kosardi — harita da uyari da cift basilir.
    """
    alan = tmp_path / "goc"
    (alan / "home" / ".claude").mkdir(parents=True)
    (alan / "proj").mkdir(parents=True)
    (alan / "home" / ".claude" / "settings.json").write_text(ESKI_BICIM, encoding="utf-8")
    ortam = _sahte_ev_ile(alan / "home")

    cikti, _ = _kos_ortamli([str(go_ctx), "bootstrap", "--proje", "deneme"], alan / "proj", ortam)
    ayar = (alan / "home" / ".claude" / "settings.json").read_text(encoding="utf-8")

    assert "↻ settings.json[SessionStart]: index" in cikti, "eski kayit taninmadi"
    assert ayar.count("hook index") == 1, "kanca iki kez kayitli"
    assert "ctx_index.py" not in ayar, "eski komut hala duruyor"
    assert '"statusMessage": "eski"' in ayar, "kullanicinin diger alanlari silinmis"


def test_bootstrap_ikinci_kosumda_hicbir_sey_degismez(go_ctx, tmp_path):
    """Tasarim sozu: IDEMPOTENT. Ikinci kosum 'hepsi guncel' demeli."""
    alan = tmp_path / "bi"

    def iki_kez(binary):
        if alan.exists():
            shutil.rmtree(alan, ignore_errors=True)
        (alan / "home" / ".claude").mkdir(parents=True)
        (alan / "proj").mkdir(parents=True)
        ortam = _sahte_ev_ile(alan / "home")
        _kos_ortamli(binary + ["bootstrap", "--proje", "deneme"], alan / "proj", ortam)
        cikti, kod = _kos_ortamli(binary + ["bootstrap", "--proje", "deneme"], alan / "proj", ortam)
        ayar = (alan / "home" / ".claude" / "settings.json").read_text(encoding="utf-8")
        return cikti, kod, ayar.replace("\r\n", "\n")

    py = iki_kez([sys.executable, str(PY_CTX)])
    go = iki_kez([str(go_ctx)])
    assert go == py
    assert "= settings.json[SessionStart]: index kayitli" in go[0]      # tekrar eklenmedi
    assert "eklendi" not in go[0] and "↻" not in go[0]    # ne ekleme ne guncelleme


# --- hook ---------------------------------------------------------------------
#
# Kancalar HER oturum baslangicinda kosar. Ilk kural sessizlik: hafiza kurulu
# olmayan bir projede tek bayt bile basmamalilar, aksi halde ilgisiz her oturumu
# kirletirler. Ciktilari harness'in JSON zarfi oldugu icin karsilastirma
# bayt bayt yapilir — `json.dumps` varsayilanlari (kompakt + \\uXXXX kacislari)
# dahil.

@pytest.mark.parametrize("ad", ["index", "capture-check"])
def test_hook_paritesi(go_ctx, parite_hafizasi, ad):
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "hook", ad], parite_hafizasi)
    go_cikti, go_kod = _kos([str(go_ctx), "hook", ad], parite_hafizasi)
    assert go_cikti == py_cikti
    assert go_kod == py_kod


def test_hook_index_gercekten_harita_basiyor(go_ctx, parite_hafizasi):
    """Bos ciktiyla sahte gecis olmasin: zarf ve harita gercekten uretiliyor."""
    cikti, kod = _kos([str(go_ctx), "hook", "index"], parite_hafizasi)
    assert kod == 0
    veri = json.loads(cikti)
    icerik = veri["hookSpecificOutput"]["additionalContext"]
    assert veri["hookSpecificOutput"]["hookEventName"] == "SessionStart"
    assert "Hafıza haritası" in icerik and "billing.md" in icerik
    # ensure_ascii=True: Turkce karakterler kacisla yazilmali
    assert "\\u" in cikti


@pytest.mark.parametrize("ad", ["index", "capture-check"])
def test_hook_hafizasiz_projede_SESSIZ(go_ctx, tmp_path, ad):
    py_cikti, py_kod = _kos([sys.executable, str(PY_CTX), "hook", ad], tmp_path)
    go_cikti, go_kod = _kos([str(go_ctx), "hook", ad], tmp_path)
    assert go_cikti == py_cikti == "", "hafizasiz projede kanca konusmamali"
    assert go_kod == py_kod == 0, "kanca oturumu bozacak bir kodla cikmamali"


def test_hook_capture_check_karar_bekleyince_uyarir(go_ctx, kapanis_alani):
    """Kirli is + karar yok = uyari; ikisi de ayni zarfi uretmeli."""
    proje, hafiza = kapanis_alani
    (hafiza / "sessions" / "capture-log.jsonl").unlink(missing_ok=True)
    kirli = proje / "kirli.txt"
    kirli.write_text("kirli\n", encoding="utf-8")

    py_cikti, _ = _kos([sys.executable, str(PY_CTX), "hook", "capture-check"], proje)
    go_cikti, _ = _kos([str(go_ctx), "hook", "capture-check"], proje)
    assert go_cikti == py_cikti
    assert "CAPTURE KARARI BEKLIYOR" in json.loads(go_cikti)["hookSpecificOutput"]["additionalContext"]


def test_gercek_hafiza_paritesi(go_ctx):
    """Sentetik degil, GERCEK korpus: fixture'in gozden kacirdigi seyi yakalar."""
    manifest = None
    for aday in (Path.home() / "Desktop" / "Dev" / "yapit",):
        if (aday / "context.yaml").is_file():
            manifest = aday
            break
    if manifest is None:
        pytest.skip("gercek hafiza bu makinede yok")
    for sorgu in ("hakedis", "yds token", "tenant izolasyonu", "render sunucuda"):
        py_cikti, _ = _kos([sys.executable, str(PY_CTX), "search", sorgu], manifest)
        go_cikti, _ = _kos([str(go_ctx), "search", sorgu], manifest)
        assert go_cikti == py_cikti, f"gercek korpusta fark: {sorgu!r}"
