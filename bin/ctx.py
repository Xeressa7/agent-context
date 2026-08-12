"""ctx — agent context aracı (search · index · validate · close · doctor · init · bootstrap).

Yalnız Python stdlib (>= 3.10). context.yaml basit bir satir ayristiricisiyla okunur;
PyYAML bagimliligi YOKTUR.

search/doctor SALT-OKUNUR. init hicbir mevcut dosyanin ustune YAZMAZ.

Turkce: lower()/casefold() KULLANILMAZ — asagidaki INDIRME tablosu kullanilir.
"""

from __future__ import annotations

import argparse
import json
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


# ---------------------------------------------------------------- index
#
# HER-ZAMAN-YUKLU HARITA. 2026-08-12'de olculdu: 21 notluk korpus koca bir
# oturum boyunca HIC acilmadi — cunku `ctx search`'u tetikleyen hicbir sey yok.
# Eski harness auto-memory'sinin tek gercek ustunlugu indeksinin (MEMORY.md)
# bedavaya her oturuma yuklenmesiydi; bu komut onu harness-BAGIMSIZ geri takar.
#
# Uretici TEK (bu komut), tuketici COK:
#   • Claude Code       -> SessionStart hook ciktiyi baglama basar (dosyaya dokunmaz)
#   • Codex/Cursor/...  -> AGENTS.md icindeki isaretli blok (`ctx index --yaz`)
# Harness'tan bagimsiz yol DAIMA dosyadir; hook yalnizca tazelik kolayligidir.
#
# Blok notun KENDISI DEGILDIR, yalnizca "ne var, nerede" haritasidir (not basina
# tek satir). Boylece "butun korpusu yukleme" yasagi korunur: ajan haritayi gorur,
# ilgili notu ACAR.

INDEX_BAS = "<!-- ctx:index:basla -->"
INDEX_SON = "<!-- ctx:index:bitir -->"
INDEX_ACIKLAMA_SINIRI = 95
# Okuma onceligi: once urun gercegi, sonra mimari/karar, sonra ders.
INDEX_KLASOR_SIRASI = ("domain", "architecture", "decisions", "pitfalls", "legacy", "historical")


def _kisalt(metin: str, sinir: int = INDEX_ACIKLAMA_SINIRI) -> str:
    metin = " ".join((metin or "").split())
    if len(metin) <= sinir:
        return metin
    kesik = metin[:sinir].rsplit(" ", 1)[0]
    return (kesik or metin[:sinir]) + "…"


def index_govde(veri: dict, hafiza: Path, tum: bool = False) -> str:
    """Rol filtresine gore not haritasi (markdown). Deterministik: notlari_gez
    yol sirali, klasor sirasi sabit."""
    rol = str(veri.get("role", "current"))
    izin = None if tum else FILTRE.get(rol, FILTRE["current"])

    gruplar: dict[str, list[str]] = {}
    sayi = 0
    for p, goreli in notlari_gez(hafiza):
        fm = frontmatter(p.read_text(encoding="utf-8", errors="replace"))
        if izin is not None and fm.get("lifecycle", "") not in izin:
            continue
        grup, ad = goreli.parts[0], "/".join(goreli.parts[1:])
        gruplar.setdefault(grup, []).append(f"- `{ad}` — {_kisalt(fm.get('description', ''))}")
        sayi += 1

    hafiza_adi = Path(str(veri.get("memory_path", hafiza))).name
    satirlar = [
        f"### Hafıza haritası — {hafiza_adi} · rol: {rol} · {sayi} not",
        "",
        "Bu bir **harita**, notun kendisi değil: ilgili satırı görünce **o notu aç**. "
        'Hedefli arama `ctx search "<sorgu>"`; bütün korpusu topluca yükleme.',
        f"Açık işler `{veri.get('task_state', 'tasks/OPEN.md')}` · "
        f"oturum devamı `{veri.get('session_state', 'sessions/CURRENT.md')}`.",
    ]
    sirali = [g for g in INDEX_KLASOR_SIRASI if g in gruplar]
    sirali += sorted(g for g in gruplar if g not in INDEX_KLASOR_SIRASI)
    for grup in sirali:
        satirlar += ["", f"**{grup}/**", *gruplar[grup]]
    return "\n".join(satirlar)


def _index_hedefleri(manifest: Path, veri: dict) -> list[Path]:
    """context.yaml `index_targets` (yoksa AGENTS.md). Hedef yalnız AGENTS.md
    olmak zorunda degil — isaretli blogu tasiyan her dosya (Cursor kurali,
    copilot-instructions.md, ...) tuketici olabilir."""
    ham = veri.get("index_targets") or ["AGENTS.md"]
    if isinstance(ham, str):
        ham = [ham]
    return [(manifest.parent / str(h)).resolve() for h in ham]


def _blok_degistir(metin: str, govde: str) -> str | None:
    """Isaretli blogun ICINI degistirir; isaret yoksa None. Dosyanin satir sonu
    bicimi KORUNUR (LF dosyayi CRLF'e cevirmek butun dosyayi diff'e sokardi)."""
    i, j = metin.find(INDEX_BAS), metin.find(INDEX_SON)
    if i == -1 or j == -1 or j < i:
        return None
    nl = "\r\n" if "\r\n" in metin else "\n"
    blok = nl.join([INDEX_BAS, *govde.split("\n"), INDEX_SON])
    return metin[:i] + blok + metin[j + len(INDEX_SON):]


def _oku_ham(yol: Path) -> str:
    with open(yol, encoding="utf-8", newline="") as f:
        return f.read()


def komut_index(args) -> int:
    manifest, veri, hafiza = baglam_yukle(args.repo)
    govde = index_govde(veri, hafiza, tum=args.all)

    if not (args.yaz or args.kontrol):
        print(govde)
        return 0

    sorunlu = 0
    for hedef in _index_hedefleri(manifest, veri):
        if not hedef.is_file():
            print(f"✗ {hedef} · dosya yok")
            sorunlu += 1
            continue
        metin = _oku_ham(hedef)
        yeni = _blok_degistir(metin, govde)
        if yeni is None:
            print(f"✗ {hedef} · ctx:index isareti yok — hedef dosyaya su iki satiri "
                  f"komsu satirlarda ekle:\n    {INDEX_BAS}\n    {INDEX_SON}")
            sorunlu += 1
        elif yeni == metin:
            print(f"✓ {hedef.name} · guncel")
        elif args.kontrol:
            print(f"⚠ {hedef} · index blogu BAYAT — `ctx index --yaz`")
            sorunlu += 1
        else:
            with open(hedef, "w", encoding="utf-8", newline="") as f:
                f.write(yeni)
            print(f"✓ {hedef} · yazildi")
    return 1 if sorunlu else 0


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
    # Hafiza reposunun KENDISINDE context.yaml yoktur (manifest tuketici repolarda
    # yasar), bu yuzden pre-commit gibi cagiranlar icin dogrudan yol verilebilir.
    if args.memory:
        hafiza = Path(args.memory).resolve()
        if not hafiza.is_dir():
            print(f"HATA: --memory dizini yok: {hafiza}")
            return 2
        if not (hafiza / "README.md").is_file():
            print(f"HATA: {hafiza} bir hafiza deposuna benzemiyor (README.md yok).")
            return 2
    else:
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


# ---------------------------------------------------------------- close
#
# YAPISAL CAPTURE KARARI. Cozdugu problem: hafizaya yazmak bugune kadar tamamen
# gonulluydu — ajan unutursa hicbir sey olmuyordu. 2026-08-12 denetiminde bulunan
# uc bosluktan UCU DE bu yuzden olustu.
#
# Ilke: KARAR MUHAKEMEDE, DISIPLIN TOOLING'DE. Bu komut "sunu hafizaya yaz"
# DEMEZ (o muhakeme isi); "bu karar verilmeden oturum kapanmasin" der.
#
#   ctx close --yok ["gerekce"]        durable bulgu yok — GECERLI bir karardir
#   ctx close --not <yol> [--not ...]  su not(lar) yazildi
#   ctx close                          durum + karar bekliyor (cikis 1)
#   ctx close --kontrol                onceki oturum kararsiz mi? (hook icin)
#
# Karar `<hafiza>/sessions/capture-log.jsonl`e yazilir. Capture Decision Coverage
# metrigi bu logdan BEDAVA turetilir; ayri bir olcum sistemi gerekmez.

KAPANIS_LOG = "sessions/capture-log.jsonl"


def _git(repo: Path, *arg) -> str:
    try:
        r = subprocess.run(["git", "-C", str(repo), *arg],
                           capture_output=True, text=True, timeout=15)
        return r.stdout.strip() if r.returncode == 0 else ""
    except Exception:
        return ""


def _calisma_durumu(repo: Path) -> dict:
    """Bulunulan repodaki 'anlamli is' sinyalleri. Kirli dosya varsa is yapilmistir."""
    kok = _git(repo, "rev-parse", "--show-toplevel") or str(repo)
    kirli = [s for s in _git(repo, "status", "--porcelain").splitlines() if s.strip()]
    return {
        "repo": Path(kok).name,
        "head": _git(repo, "rev-parse", "--short", "HEAD"),
        "kirli": len(kirli),
    }


def _log_satirlari(hafiza: Path) -> list[dict]:
    import json
    p = hafiza / KAPANIS_LOG
    if not p.is_file():
        return []
    kayitlar = []
    for satir in p.read_text(encoding="utf-8", errors="replace").splitlines():
        satir = satir.strip()
        if not satir:
            continue
        try:
            kayitlar.append(json.loads(satir))
        except Exception:
            continue
    return kayitlar


def komut_close(args) -> int:
    import json
    from datetime import datetime

    _, _, hafiza = baglam_yukle(args.repo)
    durum = _calisma_durumu(Path(args.repo) if args.repo else Path.cwd())
    log_yolu = hafiza / KAPANIS_LOG

    if args.kontrol:
        # Hook icin: son kayit ne zaman? Kirli is var ama hic karar yoksa uyar.
        kayitlar = _log_satirlari(hafiza)
        son = kayitlar[-1] if kayitlar else None
        if durum["kirli"] and (son is None or son.get("head") != durum["head"]
                               or son.get("repo") != durum["repo"]):
            print(f"⚠ {durum['repo']}: {durum['kirli']} degisik dosya var, bu HEAD icin "
                  f"capture karari YOK.\n  `ctx close --yok \"...\"` ya da "
                  f"`ctx close --not <yol>` ile kapat.")
            return 1
        print("✓ capture karari guncel")
        return 0

    if not args.yok and not args.not_:
        print(f"repo: {durum['repo']} · HEAD: {durum['head'] or '?'} · "
              f"degisik dosya: {durum['kirli']}")
        kayitlar = _log_satirlari(hafiza)
        if kayitlar:
            son = kayitlar[-1]
            print(f"son karar: {son.get('ts', '?')} · {son.get('karar', '?')}")
        print("\nKARAR BEKLENIYOR — bu oturumda kalici bir bulgu var mi?")
        print("  yok  : ctx close --yok \"gerekce\"")
        print("  var  : ctx close --not pitfalls/ornek.md [--not ...]")
        print("\n'Durable bulgu yok' GECERLI bir karardir; karar VERMEMEK gecersizdir.")
        return 1

    kayit = {
        "ts": datetime.now().isoformat(timespec="seconds"),
        "repo": durum["repo"],
        "head": durum["head"],
        "kirli": durum["kirli"],
    }

    if args.not_:
        eksik = [n for n in args.not_ if not (hafiza / n).is_file()]
        if eksik:
            print("HATA: hafizada bulunmayan not(lar): " + ", ".join(eksik))
            return 2
        kayit["karar"] = "captured"
        kayit["notlar"] = list(args.not_)
    else:
        kayit["karar"] = "none"
        if isinstance(args.yok, str) and args.yok:
            kayit["gerekce"] = args.yok

    log_yolu.parent.mkdir(parents=True, exist_ok=True)
    with log_yolu.open("a", encoding="utf-8", newline="\n") as f:
        f.write(json.dumps(kayit, ensure_ascii=False) + "\n")

    if kayit["karar"] == "captured":
        print(f"✓ capture karari kaydedildi: {len(args.not_)} not "
              f"({', '.join(args.not_)})")
        # Yeni not yazildi -> her-zaman-yuklu harita BAYATLADI. Tazelemeyi ayri bir
        # disipline birakmak, tam da bu sistemin bir kez batirdigi sey: hook'u olan
        # arac (Claude Code) haritayi canli uretir ama Codex/Cursor dosyadan okur ve
        # yeni notu HIC gormezdi. Kapanisin dogal parcasi olarak burada tazelenir.
        manifest, veri, _ = baglam_yukle(args.repo)
        govde = index_govde(veri, hafiza)
        for hedef in _index_hedefleri(manifest, veri):
            if not hedef.is_file():
                continue
            metin = _oku_ham(hedef)
            yeni = _blok_degistir(metin, govde)
            if yeni is not None and yeni != metin:
                with open(hedef, "w", encoding="utf-8", newline="") as f:
                    f.write(yeni)
                print(f"  ↻ {hedef.name} · hafiza haritasi tazelendi")
    else:
        print("✓ capture karari kaydedildi: durable bulgu yok")

    kayitlar = _log_satirlari(hafiza)
    kapali = sum(1 for k in kayitlar if k.get("karar") in ("none", "captured"))
    if kayitlar:
        print(f"  Capture Decision Coverage: {kapali}/{len(kayitlar)} oturum kararla kapandi")
    return 0


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

    # Her-zaman-yuklu harita: hedef dosyalardaki blok bayatsa harness'siz araclar
    # (Codex/Cursor) eski korpusu gorur. UYARI seviyesinde — cikis kodunu etkilemez.
    if hafiza.is_dir():
        govde = index_govde(veri, hafiza)
        for hedef in _index_hedefleri(yol, veri):
            if not hedef.is_file():
                uyarilar.append(("uyari", str(hedef), "index hedefi yok"))
            elif _blok_degistir(_oku_ham(hedef), govde) is None:
                uyarilar.append(("uyari", hedef.name, "ctx:index isareti yok — blok kurulmamis"))
            elif _blok_degistir(_oku_ham(hedef), govde) != _oku_ham(hedef):
                uyarilar.append(("uyari", hedef.name, "index blogu bayat — `ctx index --yaz`"))

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


# ---------------------------------------------------------- bootstrap

HOOK_KAYITLARI = (
    ("ctx_capture_check.py", "Capture karari kontrol ediliyor...", 20),
    ("ctx_index.py", "Hafiza haritasi yukleniyor...", 20),
)


def _claude_dizini() -> Path:
    return Path.home() / ".claude"


def hook_kur(yaz: bool = True) -> list[str]:
    """SessionStart kancalarini yerlestirir ve settings.json'a KAYDEDER.

    Iki sey ayri: dosyayi kopyalamak (kolay) ve harness'a tanitmak (asil is).
    Ikincisi 2026-08-12'de elle yapildi; bu fonksiyon onu tekrarlanabilir kilar.

    settings.json kullanicinin KENDI ayarlarini tasir → uzerine YAZILMAZ, yalniz
    eksik kayit eklenir (idempotent: ikinci kosumda hicbir sey degismez).
    """
    rapor: list[str] = []
    hedef_dizin = _claude_dizini() / "hooks"
    kaynak_dizin = SABLON / "hooks"
    if yaz:
        hedef_dizin.mkdir(parents=True, exist_ok=True)

    for ad, _mesaj, _sure in HOOK_KAYITLARI:
        kaynak = kaynak_dizin / ad
        hedef = hedef_dizin / ad
        if not kaynak.is_file():
            rapor.append(f"! sablon yok: {kaynak}")
            continue
        yeni = kaynak.read_text(encoding="utf-8")
        if hedef.is_file() and hedef.read_text(encoding="utf-8") == yeni:
            rapor.append(f"= {ad} (guncel)")
            continue
        if yaz:
            hedef.write_text(yeni, encoding="utf-8")
        rapor.append(f"+ {ad} ({'yazildi' if yaz else 'yazilacak'})")

    ayar_yolu = _claude_dizini() / "settings.json"
    try:
        ayarlar = json.loads(ayar_yolu.read_text(encoding="utf-8")) if ayar_yolu.is_file() else {}
    except (json.JSONDecodeError, OSError) as e:
        rapor.append(f"! settings.json okunamadi ({e}) — hook KAYDEDILMEDI, elle ekleyin")
        return rapor

    kancalar = ayarlar.setdefault("hooks", {}).setdefault("SessionStart", [])
    if not kancalar:
        kancalar.append({"hooks": []})
    liste = kancalar[0].setdefault("hooks", [])
    mevcut = " ".join(str(k.get("command", "")) for k in liste)

    eklendi = False
    for ad, mesaj, sure in HOOK_KAYITLARI:
        if ad in mevcut:
            rapor.append(f"= settings.json: {ad} kayitli")
            continue
        liste.append({
            "type": "command",
            "command": f'python "{hedef_dizin / ad}"',
            "shell": "bash",
            "timeout": sure,
            "statusMessage": mesaj,
        })
        eklendi = True
        rapor.append(f"+ settings.json: {ad} eklendi")

    if eklendi and yaz:
        ayar_yolu.write_text(json.dumps(ayarlar, ensure_ascii=False, indent=2), encoding="utf-8")
    return rapor


def kalite_kapisi_kur(hafiza: Path, yaz: bool = True) -> list[str]:
    """Hafiza reposuna pre-commit kalite kapisini takar (`core.hooksPath`).

    Kanca dogrulama MANTIGI TASIMAZ; tek motor `ctx validate`'i cagirir.
    """
    rapor: list[str] = []
    if not (hafiza / ".git").exists():
        rapor.append("! hafiza bir git reposu degil — kalite kapisi atlandi")
        return rapor

    kanca_dizin = hafiza / ".githooks"
    kaynak = SABLON / "hooks" / "pre-commit"
    if not kaynak.is_file():
        rapor.append("! pre-commit sablonu yok")
        return rapor

    hedef = kanca_dizin / "pre-commit"
    yeni = kaynak.read_text(encoding="utf-8")
    if hedef.is_file() and hedef.read_text(encoding="utf-8") == yeni:
        rapor.append("= .githooks/pre-commit (guncel)")
    else:
        if yaz:
            kanca_dizin.mkdir(parents=True, exist_ok=True)
            hedef.write_text(yeni, encoding="utf-8")
        rapor.append("+ .githooks/pre-commit")

    mevcut_yol = _git(hafiza, "config", "--get", "core.hooksPath")
    if mevcut_yol == ".githooks":
        rapor.append("= core.hooksPath (.githooks)")
    elif mevcut_yol:
        rapor.append(f"! core.hooksPath baska bir degerde ({mevcut_yol}) — DOKUNULMADI")
    else:
        if yaz:
            _git(hafiza, "config", "core.hooksPath", ".githooks")
        rapor.append("+ core.hooksPath = .githooks")
    return rapor


def komut_bootstrap(args) -> int:
    """Yeni projede tek komutla kurulum: hafiza + adapter + kancalar + kalite kapisi.

    Tasarim sozu: IDEMPOTENT ve YIKICI DEGIL. Mevcut hicbir dosyanin uzerine
    yazmaz (settings.json'a yalniz eksik kaydi ekler), ikinci kosumda ciktisi
    "hepsi guncel" olur. --kuru ile hicbir sey yazmadan ne yapacagini gosterir.
    """
    yaz = not args.kuru
    if args.kuru:
        print("KURU KOSUM — hicbir dosya yazilmayacak\n")

    kok = Path.cwd()
    print(f"proje kokku: {kok}")

    # 1) hafiza iskeleti + adapter dosyalari (mevcut init'i aynen kullanir)
    print("\n[1/4] hafiza + adapter")
    if yaz:
        komut_init(args)
    else:
        mp = args.memory_path or f"../{args.proje}-memory"
        print(f"  ctx init -> {(kok / mp).resolve()} + context.yaml + AGENTS.md + CLAUDE.md")

    # 2) SessionStart kancalari (harita + capture karari)
    print("\n[2/4] SessionStart kancalari")
    for satir in hook_kur(yaz=yaz):
        print("  ", satir)

    # 3) hafiza reposunda kalite kapisi
    print("\n[3/4] kalite kapisi (pre-commit)")
    mp = args.memory_path or f"../{args.proje}-memory"
    for satir in kalite_kapisi_kur((kok / mp).resolve(), yaz=yaz):
        print("  ", satir)

    # 4) harita blogunu yaz + saglik
    print("\n[4/4] harita + saglik")
    if args.kuru:
        print("   (kuru kosum — index/doctor atlandi)")
        return 0

    class _Index:
        repo = None
        all = False
        yaz = True
        kontrol = False
    # Bos korpusta bile blok YAZILIR: tuketici dosyalarda isaretler bastan dursun,
    # ilk not eklendiginde tazeleme yapacak yer hazir olsun.
    komut_index(_Index())

    class _D:
        repo = None
    kod = komut_doctor(_D())

    print("\nKALAN ELLE ADIM:")
    print("  · hafiza reposu icin uzak depo (gh repo create <ad>-memory --private) — PII taramasi sonrasi")
    print("  · `ctx` PATH'te degilse: bin/ dizinini PATH'e ekleyin")
    return kod


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

    x = alt.add_parser("index", help="her-zaman-yuklu not haritasi (hook + AGENTS.md blogu)")
    x.add_argument("--repo")
    x.add_argument("--all", action="store_true", help="lifecycle filtresi uygulama")
    x.add_argument("--yaz", action="store_true",
                   help="hedef dosyalardaki ctx:index blogunu tazele (index_targets)")
    x.add_argument("--kontrol", action="store_true",
                   help="bloklar bayat mi? yazmaz; bayatsa cikis 1 (doctor/pre-commit icin)")
    x.set_defaults(fn=komut_index)

    d = alt.add_parser("doctor", help="kurulum saglik kontrolu")
    d.add_argument("--repo")
    d.set_defaults(fn=komut_doctor)

    c = alt.add_parser("close", help="oturumu capture karariyla kapat (yapisal kapi)")
    c.add_argument("--repo")
    c.add_argument("--yok", nargs="?", const=True, default=False,
                   metavar="GEREKCE", help="durable bulgu yok (gecerli karar)")
    c.add_argument("--not", dest="not_", action="append", metavar="YOL",
                   help="yazilan notun hafizaya goreli yolu (birden cok kez verilebilir)")
    c.add_argument("--kontrol", action="store_true",
                   help="karar bekleyen oturum var mi? (hook icin; varsa cikis 1)")
    c.set_defaults(fn=komut_close)

    v = alt.add_parser("validate", help="not kalitesi dogrulamasi (doctor + pre-commit ayni motoru kullanir)")
    v.add_argument("--repo")
    v.add_argument("--memory", help="hafiza deposunun yolu (manifest aramayi atlar; pre-commit icin)")
    v.add_argument("--tam", action="store_true", help="PII/secret taramasini da kos (pahali)")
    v.set_defaults(fn=komut_validate)

    i = alt.add_parser("init", help="yeni projede hafiza iskeleti kur")
    i.add_argument("proje")
    i.add_argument("--memory-path", dest="memory_path")
    i.add_argument("--role", default="current", choices=["current", "legacy"])
    i.add_argument("--components", default="app")
    i.set_defaults(fn=komut_init)

    b = alt.add_parser("bootstrap",
                       help="yeni projede TEK KOMUTLA kurulum (init + kancalar + kalite kapisi)")
    b.add_argument("--proje", required=True)
    b.add_argument("--memory-path", dest="memory_path")
    b.add_argument("--role", default="current", choices=["current", "legacy"])
    b.add_argument("--components", default="")
    b.add_argument("--kuru", action="store_true",
                   help="hicbir sey yazma, ne yapacagini goster")
    b.set_defaults(fn=komut_bootstrap)

    args = ap.parse_args()
    return args.fn(args)


if __name__ == "__main__":
    raise SystemExit(main())
