"""ctx — agent context aracı V0 (search · doctor · init).

Yalnız Python stdlib (>= 3.10). context.yaml basit bir satir ayristiricisiyla okunur;
PyYAML bagimliligi YOKTUR.

search/doctor SALT-OKUNUR. init hicbir mevcut dosyanin ustune YAZMAZ.

Turkce: lower()/casefold() KULLANILMAZ — asagidaki INDIRME tablosu kullanilir.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")

KOK = Path(__file__).resolve().parent.parent  # ~/.agent-context
SABLON = KOK / "templates"

GECERLI_LIFECYCLE = {"product", "current", "legacy", "historical"}
FILTRE = {"current": {"product", "current"}, "legacy": {"product", "legacy"}}
ATLA = {"tasks", "sessions", "gecis", ".git"}

# Turkce-guvenli indirme: buyuk harf -> kucuk karsilik (I -> ı, İ -> i).
INDIRME = str.maketrans(
    "İIŞĞÜÖÇÂÎÛABCDEFGHJKLMNOPQRSTUVWXYZ",
    "iışğüöçâîûabcdefghjklmnopqrstuvwxyz",
)

# ARAMA KATLAMASI (yalniz arama icin, dil donusumu DEGIL): Turkce aksanli harfler ASCII
# transliterasyonlarina indirilir (ı->i, ş->s, ğ->g, ü->u, ö->o, ç->c, â->a, î->i, û->u).
# Gerekce: proje konvansiyonu — yollar/tanimlayicilar Turkce karaktersiz transliterasyonla
# yazilir (CONVENTIONS.md: "hakedis", "hakediş" degil), yani Turkce sorgu bu katlama
# olmadan dosya yoluna/tanimlayiciya HICBIR ZAMAN eslesemez. Arama normalizasyonu
# aksan-duyarsizdir; metin uretiminde ASLA kullanilmaz.
ARAMA_KATLAMA = str.maketrans("ışğüöçâîû", "isguocaiu")


def indir(metin: str) -> str:
    """Karsilastirma icin normalize et. .lower() KULLANMA — Turkce'de yanlis uretir."""
    return metin.translate(INDIRME).translate(ARAMA_KATLAMA)


# ---------------------------------------------------------------- manifest

def manifest_bul(baslangic: Path) -> Path | None:
    """cwd'den yukari cikarak context.yaml ara."""
    d = baslangic.resolve()
    for aday in [d, *d.parents]:
        y = aday / "context.yaml"
        if y.is_file():
            return y
    return None


def manifest_oku(yol: Path) -> dict:
    """Duz `anahtar: deger` + iki seviyeli girinti icin basit ayristirici."""
    veri: dict = {}
    ust = None
    for ham in yol.read_text(encoding="utf-8").splitlines():
        satir = ham.split("#", 1)[0].rstrip()
        if not satir.strip():
            continue
        girintili = satir[:1] in (" ", "\t")
        if ":" not in satir:
            continue
        anahtar, _, deger = satir.strip().partition(":")
        anahtar, deger = anahtar.strip(), deger.strip()
        if deger.startswith("[") and deger.endswith("]"):
            deger = [p.strip() for p in deger[1:-1].split(",") if p.strip()]
        if girintili and ust is not None:
            veri.setdefault(ust, {})
            if isinstance(veri[ust], dict):
                veri[ust][anahtar] = deger
        else:
            ust = anahtar
            veri[anahtar] = deger if deger != "" else {}
    return veri


def baglam_yukle(repo_arg: str | None):
    """(manifest_yolu, veri, memory_path) dondurur; bulunamazsa hata basip cikar."""
    baslangic = Path(repo_arg) if repo_arg else Path.cwd()
    yol = manifest_bul(baslangic)
    if yol is None:
        print(f"HATA: context.yaml bulunamadi ({baslangic}'den yukari arandi).")
        raise SystemExit(2)
    veri = manifest_oku(yol)
    mp = veri.get("memory_path")
    if not isinstance(mp, str):
        print(f"HATA: {yol} icinde memory_path yok.")
        raise SystemExit(2)
    return yol, veri, (yol.parent / mp).resolve()


# ---------------------------------------------------------------- frontmatter

def frontmatter(metin: str) -> dict:
    if not metin.startswith("---"):
        return {}
    son = metin.find("\n---", 3)
    if son == -1:
        return {}
    alanlar = {}
    for satir in metin[3:son].splitlines():
        if ":" not in satir or satir[:1] in (" ", "\t", "-"):
            continue
        a, _, d = satir.partition(":")
        d = d.strip().strip('"').strip("'")
        if d.startswith("[") and d.endswith("]"):
            d = ", ".join(p.strip() for p in d[1:-1].split(","))
        alanlar[a.strip()] = d
    return alanlar


def notlari_gez(hafiza: Path):
    for p in sorted(hafiza.rglob("*.md")):
        goreli = p.relative_to(hafiza)
        if ATLA & set(goreli.parts):
            continue
        if goreli.parts[0] == "README.md" or len(goreli.parts) == 1:
            continue
        yield p, goreli


# ---------------------------------------------------------------- search
#
# Puanlama (V3, 2026-08-12): aday uretimi VEYA mantigidir — en az BIR kelime
# eslesen not adaydir. Eski VE mantigi olculmus bir ariza uretiyordu: "tenant
# izolasyonu" sorgusu, tam o konuyu anlatan notu dondurmuyordu ("izolasyonu"
# kelimesi notta gecmedigi icin not tamamen eleniyordu).
#
# Her kelime en yuksek agirlikli alandan puan alir (toplama degil, kelime basina
# TEK alan). Ham puan sonra KAPSAMA carpaniyla olceklenir: butun kelimeleri
# karsilayan not tam puan alir, yarisini karsilayan ~%70 alir. Boylece VE'nin
# siralama kalitesi korunur ama eleme yapilmaz.
#
# Cok kelimeli sorguda, sorgunun tamami bir alanda birebir geciyorsa o alanin
# agirligi kadar TAM IFADE BONUSU eklenir.
#
# Alan agirliklari + sirasi asagida sabit: sira ayni zamanda "en yuksek alan"
# aramasinda kullanilir (ilk eslesen = en yuksek agirlikli).

# Kapsama carpani: puan * (TABAN + (1 - TABAN) * eslesen/toplam).
# TABAN=0.4 -> tek kelimesi eslesen 2 kelimelik sorguda not ham puanin %70'ini alir.
KAPSAMA_TABANI = 0.4

_KOD_PARCA_DESENI = re.compile(r"`([^`\n]+)`")

# "yol" ikiye ayrilir: dosya adi koku (uzantisiz stem) sorgu kelimesine BIREBIR esitse
# 100, kelime yolun herhangi bir yerinde (kismi) geciyorsa 70. Boylece "hakediş" sorgusu
# domain/hakedis.md'yi (birebir stem esit) domain/fatura-hakedis.md'nin (kismi yol
# eslesmesi) onune gecirir. Her giris: (goruntu adi, agirlik, mod) — mod "esit" ise
# tam esitlik, "icerir" ise alt-dize arandigi anlamina gelir. Liste agirlik-azalan
# sirada: sira ayni zamanda "en yuksek alan" aramasinda VE gerekce siralamasinda
# kullanilir (ilk eslesen = en yuksek agirlikli).
_ALAN_TANIM = [
    ("yol", "yol-tam", 100, "esit"),
    ("yol", "yol-kismi", 70, "icerir"),
    ("kod", "kod", 60, "icerir"),
    ("başlık", "başlık", 40, "icerir"),
    ("etiket", "etiket", 30, "icerir"),
    ("bileşen", "bileşen", 20, "icerir"),
    ("açıklama", "açıklama", 12, "icerir"),
    ("gövde", "gövde", 3, "icerir"),
]


def _govde_ayir(metin: str) -> str:
    """Frontmatter'i atlayip gövde metnini dondurur (frontmatter yoksa oldugu gibi)."""
    if not metin.startswith("---"):
        return metin
    son = metin.find("\n---", 3)
    if son == -1:
        return metin
    satir_sonu = metin.find("\n", son + 1)
    if satir_sonu == -1:
        return ""
    return metin[satir_sonu + 1:]


def _kod_parca_mi(parca: str) -> bool:
    """Backtick icerigi identifier/yol GORUNUMLU mu (kod proxy'si), yoksa duz vurgu mu.

    Kural: icinde bosluk YOKSA VE (. _ / \\ ( : - karakterlerinden biri VAR YA DA
    camelCase (kucuk harfi buyuk harf izliyor) VARSA -> kod sayilir. Aksi halde
    (orn. `GELMEYEN HAKEDİŞLER.xlsx` — bosluklu; `Hakediş` — ozel karaktersiz +
    camelCase degil) normal govde metni sayilir.
    """
    if not parca or any(c.isspace() for c in parca):
        return False
    if any(c in parca for c in ".:_/\\(-"):
        return True
    for onceki, sonraki in zip(parca, parca[1:]):
        if onceki.islower() and sonraki.isupper():
            return True
    return False


def _kod_parcalari(govde: str) -> str:
    """Backtick icinden yalniz identifier/yol-gorunumlu parcalari cikarir."""
    return " ".join(p for p in _KOD_PARCA_DESENI.findall(govde) if _kod_parca_mi(p))


def _not_puanla(goreli: Path, fm: dict, metin: str, kelimeler: list[str], ifade: str = ""):
    """Bir notun sorguya gore (puan, gerekce_alanlari, eslesen_kelime_sayisi) uclusunu hesaplar.

    Hicbir kelime eslesmezse None doner (aday degil). Bir kelimenin eslesmemesi
    notu ELEMEZ — yalnizca kapsama carpanini dusurur.
    """
    govde = _govde_ayir(metin)[:100_000]
    kod = _kod_parcalari(govde)
    alan_metni = {
        "yol-tam": indir(Path(goreli).stem),
        "yol-kismi": indir(goreli.as_posix()),
        "kod": indir(kod),
        "başlık": indir(fm.get("title", "")),
        "etiket": indir(fm.get("tags", "")),
        "bileşen": indir(fm.get("components", "")),
        "açıklama": indir(fm.get("description", "")),
        "gövde": indir(govde),
    }

    ham = 0
    eslesen = 0
    gerekce_gorulen: set[str] = set()
    gerekce_sirasi: list[str] = []
    for k in kelimeler:
        eslesen_goruntu = None
        for goruntu, anahtar, agirlik, mod in _ALAN_TANIM:
            deger = alan_metni[anahtar]
            eslesti = (k == deger) if mod == "esit" else (k in deger)
            if eslesti:
                eslesen_goruntu = goruntu
                ham += agirlik
                eslesen += 1
                break
        if eslesen_goruntu is None:
            continue  # bu kelime yok -> eleme yok, kapsama duser
        if eslesen_goruntu not in gerekce_gorulen:
            gerekce_gorulen.add(eslesen_goruntu)
            gerekce_sirasi.append(eslesen_goruntu)

    if eslesen == 0:
        return None  # hicbir kelime eslesmedi -> aday degil

    # kapsama carpani: butun kelimeler eslesince 1.0, yarisi eslesince ~0.7
    kapsama = eslesen / len(kelimeler)
    puan = ham * (KAPSAMA_TABANI + (1 - KAPSAMA_TABANI) * kapsama)

    # tam ifade bonusu: cok kelimeli sorgunun tamami bir alanda birebir geciyorsa
    if ifade and len(kelimeler) > 1:
        for goruntu, anahtar, agirlik, mod in _ALAN_TANIM:
            deger = alan_metni[anahtar]
            if (ifade == deger) if mod == "esit" else (ifade in deger):
                puan += agirlik
                if "ifade" not in gerekce_gorulen:
                    gerekce_gorulen.add("ifade")
                    gerekce_sirasi.insert(0, "ifade")
                break

    # gerekce siralamasi: ilk-gorulen agirliga gore (ifade > yol > kod > ... > govde)
    sira_indeks: dict[str, int] = {"ifade": -1}
    for i, (goruntu, _anahtar, _agirlik, _mod) in enumerate(_ALAN_TANIM):
        sira_indeks.setdefault(goruntu, i)
    gerekce_sirasi.sort(key=lambda a: sira_indeks[a])
    return round(puan, 1), gerekce_sirasi[:3], eslesen


def komut_search(args) -> int:
    _, veri, hafiza = baglam_yukle(args.repo)
    rol = veri.get("role", "current")
    izin = None if args.all else FILTRE.get(rol, FILTRE["current"])

    kelimeler = [indir(k) for k in args.sorgu.split() if k.strip()]
    if not kelimeler:
        print("HATA: bos sorgu.")
        return 2
    ifade = indir(" ".join(args.sorgu.split()))

    top_k = args.top

    adaylar = []  # (puan, yol, lifecycle, aciklama, gerekce, eslesen)
    for p, goreli in notlari_gez(hafiza):
        metin = p.read_text(encoding="utf-8", errors="replace")
        fm = frontmatter(metin)
        lc = fm.get("lifecycle", "")
        if izin is not None and lc not in izin:
            continue
        sonuc = _not_puanla(goreli, fm, metin, kelimeler, ifade)
        if sonuc is None:
            continue
        puan, gerekce, eslesen = sonuc
        if puan < args.min:
            continue
        adaylar.append((puan, goreli.as_posix(), lc, fm.get("description", ""), gerekce, eslesen))

    # puan azalan, esitlikte yol alfabetik -> determinizm
    adaylar.sort(key=lambda t: (-t[0], t[1]))

    toplam = len(adaylar)
    gosterilecek = adaylar[:top_k]
    cok = len(kelimeler) > 1
    for puan, yol_str, lc, aciklama, gerekce, eslesen in gosterilecek:
        gerekce_metni = "+".join(gerekce)
        kapsama = f" · {eslesen}/{len(kelimeler)}" if cok else ""
        print(f"{yol_str} · {lc or '?'} · [{gerekce_metni}]{kapsama} · {aciklama[:120]}")

    if toplam > top_k:
        print(f'(+{toplam - top_k} sonuç daha — "--top N" ile artır)')

    if toplam == 0:
        print("(eslesme yok)")

    # eski arsiv: HER ZAMAN kanonik notlardan sonra, ayri baslik, en fazla 5 satir.
    eski = hafiza / "gecis" / "legacy-index.md"
    if eski.is_file():
        # kanonik tarafla ayni mantik: VEYA adaylik + eslesen kelime sayisina gore siralama
        eski_puanli = []
        for satir in eski.read_text(encoding="utf-8", errors="replace").splitlines():
            if not satir.startswith("- "):
                continue
            ind = indir(satir)
            n = sum(1 for k in kelimeler if k in ind)
            if n:
                eski_puanli.append((n, ifade in ind, satir))
        eski_puanli.sort(key=lambda t: (-t[0], not t[1]))
        eski_satirlar = [s for _, _, s in eski_puanli]
        if eski_satirlar:
            print("\n--- eski arşiv (gecis/legacy-index.md) ---")
            for satir in eski_satirlar[:5]:
                print(f"[eski arşiv] {satir[2:][:160]}")
            if len(eski_satirlar) > 5:
                print(f"(+{len(eski_satirlar) - 5} eski arşiv sonucu daha)")

    return 0


# ---------------------------------------------------------------- validate
#
# TEK dogrulama motoru. Tuketiciler: `ctx doctor`, `ctx validate`, pre-commit hook.
# Ayni mantik iki yerde yazilmaz.
#
# Iki mod:
#   hizli (varsayilan) — yapisal: frontmatter, zorunlu alanlar, linkler, review-due.
#                        search'in de karsilayabilecegi ucuzlukta.
#   --tam              — ustune PII/secret taramasi (pahali, pre-commit ve elle kosum icin).
#
# Seviye ayrimi onemli: 90 gunluk not STALE DEGILDIR, yalnizca REVIEW-DUE'dur.
# Gercek stale tespiti makine-dogrulanabilir anchor'larla yapilir (henuz yok).

ZORUNLU_ALANLAR = ("title", "description", "lifecycle", "type", "updated")
REVIEW_GUN = 90

_MD_LINK = re.compile(r"\[[^\]]*\]\(([^)#]+?)(?:#[^)]*)?\)")
_WIKI_LINK = re.compile(r"\[\[([^\]|]+)")

# Deger ATANMIS gizli anahtar. Yalniz adin gecmesi bulgu DEGILDIR — SPEC'in
# onerdigi "ADMIN_SECRET ister, degeri disarida yasar" bicimi mesru.
_SECRET_DESENI = re.compile(
    r"(?i)\b(api[_-]?key|secret|token|password|passwd|pwd)\b\s*[:=]\s*['\"]?([A-Za-z0-9+/=_.-]{16,})"
)
_PII_DESENLERI = [
    ("TC kimlik gorunumlu 11 hane", re.compile(r"(?<!\d)[1-9]\d{10}(?!\d)")),
    ("IBAN", re.compile(r"(?i)\bTR\d{2}[\s]?(?:\d{4}[\s]?){5}\d{2}\b")),
    ("e-posta", re.compile(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}")),
    ("telefon", re.compile(r"(?<!\d)0?5\d{2}[\s.-]?\d{3}[\s.-]?\d{2}[\s.-]?\d{2}(?!\d)")),
]


def _bugun():
    from datetime import date
    return date.today()


def _tarih_ayristir(s: str):
    from datetime import date
    try:
        y, a, g = (int(x) for x in str(s).strip()[:10].split("-"))
        return date(y, a, g)
    except Exception:
        return None


def dogrula(hafiza: Path, tam: bool = False) -> list[tuple[str, str, str]]:
    """(seviye, dosya, mesaj) listesi dondurur. seviye: 'hata' | 'uyari'."""
    bulgular: list[tuple[str, str, str]] = []
    notlar = list(notlari_gez(hafiza))
    stemler = {p.stem for p, _ in notlar}
    basliklar: dict[str, str] = {}

    for p, goreli in notlar:
        yol = goreli.as_posix()
        metin = p.read_text(encoding="utf-8", errors="replace")
        fm = frontmatter(metin)

        if not fm:
            bulgular.append(("hata", yol, "frontmatter yok ya da ayristirilamadi"))
            continue

        lc = fm.get("lifecycle", "")
        if lc not in GECERLI_LIFECYCLE:
            bulgular.append(("hata", yol, f"gecersiz lifecycle: {lc!r}"))
        for alan in ZORUNLU_ALANLAR:
            if not fm.get(alan):
                bulgular.append(("hata", yol, f"zorunlu alan eksik: {alan}"))

        baslik = fm.get("title", "")
        if baslik:
            if baslik in basliklar:
                bulgular.append(("uyari", yol, f"baslik tekrari: {basliklar[baslik]} ile ayni"))
            else:
                basliklar[baslik] = yol

        sup = fm.get("superseded_by", "")
        if sup and sup not in ("null", "None", ""):
            if not (hafiza / sup).exists():
                bulgular.append(("hata", yol, f"superseded_by hedefi yok: {sup}"))

        govde = _govde_ayir(metin)

        for hedef in _MD_LINK.findall(govde):
            h = hedef.strip()
            if h.startswith(("http://", "https://", "mailto:")) or not h.endswith(".md"):
                continue
            if not (p.parent / h).resolve().exists():
                bulgular.append(("hata", yol, f"kirik markdown link: {h}"))

        for ad in _WIKI_LINK.findall(govde):
            if ad.strip() not in stemler:
                bulgular.append(("uyari", yol,
                                 f"cozumlenmeyen wiki-link: [[{ad.strip()}]] "
                                 f"(eski arsive isaret ediyor olabilir)"))

        if lc == "current":
            t = _tarih_ayristir(fm.get("updated", ""))
            if t is None:
                bulgular.append(("hata", yol, f"updated tarihi okunamadi: {fm.get('updated')!r}"))
            else:
                gun = (_bugun() - t).days
                if gun > REVIEW_GUN:
                    bulgular.append(("uyari", yol,
                                     f"review-due: {gun} gundur dogrulanmadi (stale DEGIL)"))

        if tam:
            for m in _SECRET_DESENI.finditer(metin):
                bulgular.append(("hata", yol,
                                 f"deger atanmis gizli anahtar gorunumu: {m.group(1)}=***"))
            for ad, desen in _PII_DESENLERI:
                if desen.search(metin):
                    bulgular.append(("hata", yol, f"olasi PII: {ad}"))

    return bulgular


def komut_validate(args) -> int:
    _, _, hafiza = baglam_yukle(args.repo)
    bulgular = dogrula(hafiza, tam=args.tam)
    hatalar = [b for b in bulgular if b[0] == "hata"]
    uyarilar = [b for b in bulgular if b[0] == "uyari"]

    for seviye, yol, mesaj in hatalar + uyarilar:
        isaret = "✗" if seviye == "hata" else "⚠"
        print(f"{isaret} {yol} · {mesaj}")

    if not bulgular:
        print("✓ bulgu yok" + ("" if args.tam else " (yapisal; PII taramasi icin --tam)"))
    else:
        print(f"\n{len(hatalar)} hata · {len(uyarilar)} uyari"
              + ("" if args.tam else " · PII taramasi KOSULMADI (--tam)"))
    return 1 if hatalar else 0


# ---------------------------------------------------------------- doctor

def komut_doctor(args) -> int:
    yol = manifest_bul(Path(args.repo) if args.repo else Path.cwd())
    hata = 0

    def kontrol(tamam: bool, metin: str):
        nonlocal hata
        print(("✓ " if tamam else "✗ ") + metin)
        if not tamam:
            hata = 1

    kontrol(yol is not None, f"context.yaml bulundu ({yol})")
    if yol is None:
        return 1

    veri = manifest_oku(yol)
    for alan in ("project", "memory_path", "role"):
        kontrol(bool(veri.get(alan)), f"context.yaml · {alan}: {veri.get(alan)!r}")

    hafiza = (yol.parent / str(veri.get("memory_path", ""))).resolve()
    kontrol(hafiza.is_dir(), f"memory_path erisilebilir ({hafiza})")
    kontrol((hafiza / "README.md").is_file(), "memory/README.md var")

    # not dogrulamasi TEK motordan gelir (ctx validate ile ayni mantik)
    sayi = 0
    bulgular: list[tuple[str, str, str]] = []
    if hafiza.is_dir():
        sayi = sum(1 for _ in notlari_gez(hafiza))
        bulgular = dogrula(hafiza, tam=False)
    hatalar = [b for b in bulgular if b[0] == "hata"]
    uyarilar = [b for b in bulgular if b[0] == "uyari"]

    kontrol(not hatalar, f"{sayi} not yapisal dogrulamadan gecti"
            + (f" — {len(hatalar)} HATA" if hatalar else ""))
    for _, yol, mesaj in hatalar[:5]:
        print(f"    ✗ {yol} · {mesaj}")
    if len(hatalar) > 5:
        print(f"    (+{len(hatalar) - 5} hata daha — 'ctx validate' ile tamamini gor)")

    kontrol((hafiza / "tasks" / "OPEN.md").is_file(), "tasks/OPEN.md var")
    kontrol((hafiza / "sessions" / "CURRENT.md").is_file(), "sessions/CURRENT.md var")

    # uyarilar cikis kodunu ETKILEMEZ — doctor'un yesil kalmasi gerekir
    if uyarilar:
        print(f"\n{len(uyarilar)} uyari (cikis kodunu etkilemez):")
        for _, yol, mesaj in uyarilar[:8]:
            print(f"  ⚠ {yol} · {mesaj}")
        if len(uyarilar) > 8:
            print(f"  (+{len(uyarilar) - 8} uyari daha)")
    return hata


# ---------------------------------------------------------------- init

def komut_init(args) -> int:
    kok = Path.cwd()
    proje = args.proje
    mp = args.memory_path or f"../{proje}-memory"
    hafiza = (kok / mp).resolve()
    olusan, atlanan = [], []

    if hafiza.exists():
        atlanan.append(f"{hafiza} (mevcut — dokunulmadi)")
    else:
        (hafiza / "tasks").mkdir(parents=True)
        (hafiza / "sessions").mkdir(parents=True)
        for klasor in ("domain", "architecture", "decisions", "pitfalls",
                       "legacy", "historical"):
            d = hafiza / klasor
            d.mkdir()
            (d / ".gitkeep").write_text("", encoding="utf-8")
        for kaynak, hedef in (
            ("memory/README.md", "README.md"),
            ("memory/tasks/OPEN.md", "tasks/OPEN.md"),
            ("memory/sessions/CURRENT.md", "sessions/CURRENT.md"),
        ):
            metin = (SABLON / kaynak).read_text(encoding="utf-8")
            (hafiza / hedef).write_text(metin.replace("{project}", proje), encoding="utf-8")
        subprocess.run(["git", "init", "--quiet", str(hafiza)], check=False)
        olusan.append(f"{hafiza} (iskelet + git init, COMMIT YOK)")

    hedef = kok / "context.yaml"
    if hedef.exists():
        atlanan.append("context.yaml (mevcut)")
    else:
        metin = (SABLON / "context.yaml").read_text(encoding="utf-8")
        hedef.write_text(
            metin.replace("{project}", proje)
                 .replace("{memory_path}", mp)
                 .replace("{role}", args.role)
                 .replace("{components}", args.components),
            encoding="utf-8")
        olusan.append("context.yaml")

    router = (SABLON / "AGENTS-router.md").read_text(encoding="utf-8") \
        .replace("{memory_path}", mp) \
        .replace("{filter}", "product + " + args.role)
    agents = kok / "AGENTS.md"
    if agents.exists():
        atlanan.append("AGENTS.md (MEVCUT — DOKUNULMADI)")
        print("AGENTS.md mevcut; router blogunu elle ekleyin:\n")
        print(router)
    else:
        agents.write_text(router, encoding="utf-8")
        olusan.append("AGENTS.md (router)")

    claude = kok / "CLAUDE.md"
    if claude.exists():
        atlanan.append("CLAUDE.md (mevcut)")
    else:
        claude.write_text((SABLON / "CLAUDE.md").read_text(encoding="utf-8"), encoding="utf-8")
        olusan.append("CLAUDE.md")

    print("\n--- ozet ---")
    for o in olusan:
        print("  olusturuldu:", o)
    for a in atlanan:
        print("  atlandi    :", a)
    return 0


# ---------------------------------------------------------------- cli

def main() -> int:
    ap = argparse.ArgumentParser(prog="ctx", description="agent context araci V0")
    alt = ap.add_subparsers(dest="komut", required=True)

    s = alt.add_parser("search", help="hafizada ara")
    s.add_argument("sorgu")
    s.add_argument("--repo")
    s.add_argument("--all", action="store_true", help="lifecycle filtresi uygulama")
    s.add_argument("--top", type=int, default=8, help="en fazla kac kanonik sonuc gosterilsin")
    s.add_argument("--min", type=float, default=0.0,
                   help="bu puanin altindaki sonuclari gizle (varsayilan 0 = kapali)")
    s.set_defaults(fn=komut_search)

    d = alt.add_parser("doctor", help="kurulum saglik kontrolu")
    d.add_argument("--repo")
    d.set_defaults(fn=komut_doctor)

    v = alt.add_parser("validate", help="not kalitesi dogrulamasi (doctor + pre-commit ayni motoru kullanir)")
    v.add_argument("--repo")
    v.add_argument("--tam", action="store_true", help="PII/secret taramasini da kos (pahali)")
    v.set_defaults(fn=komut_validate)

    i = alt.add_parser("init", help="yeni projede hafiza iskeleti kur")
    i.add_argument("proje")
    i.add_argument("--memory-path", dest="memory_path")
    i.add_argument("--role", default="current", choices=["current", "legacy"])
    i.add_argument("--components", default="app")
    i.set_defaults(fn=komut_init)

    args = ap.parse_args()
    return args.fn(args)


if __name__ == "__main__":
    raise SystemExit(main())
