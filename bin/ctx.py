"""ctx — agent context aracı (search · index · validate · close · doctor · init · bootstrap).

Yalnız Python stdlib (>= 3.10). context.yaml basit bir satir ayristiricisiyla okunur;
PyYAML bagimliligi YOKTUR.

search/doctor SALT-OKUNUR. init hicbir mevcut dosyanin ustune YAZMAZ.

Arama normalizasyonu dile bagimli DEGIL: Unicode NFKD + casefold (bkz. indir()).
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import unicodedata
import sys
from pathlib import Path

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")

KOK = Path(__file__).resolve().parent.parent  # ~/.agent-context
SABLON = KOK / "templates"

# ---------------------------------------------------------------- taksonomi
#
# Notun gecerlilik alani IKI BAGIMSIZ eksenle tarif edilir:
#
#   status : active | archived     — bugunu tarif ediyor mu (varsayilan active)
#   scope  : [etiket, ...]         — hangi baglamda gecerli; BOSSA EVRENSEL
#
# Onceki model tek eksenliydi (`lifecycle: product|current|legacy|historical`) ve
# `current`/`legacy` ikiligi bu araci dogurun projenin kendi hikayesinden
# geliyordu ("eski surum + yeni surum"). Tek cagli bir projede `legacy` olu bir
# kavram, uc cagli bir projede yetersizdi. Iki eksende:
#
#   product    -> scope yok (evrensel), status active
#   current    -> scope: [current]      \ etiketler artik PROJENIN sozluğu,
#   legacy     -> scope: [legacy]       / koda gomulu sabit degil
#   historical -> status: archived
#
# Bu esleme sayesinde eski notlar OLDUGU GIBI calisir (asagidaki _gecerlilik).
GECERLI_STATUS = {"active", "archived"}

# Geriye uyumluluk: eski tek-eksenli deger -> (status, scope)
_ESKI_LIFECYCLE = {
    "product":    ("active", []),
    "historical": ("archived", []),
    "current":    ("active", ["current"]),
    "legacy":     ("active", ["legacy"]),
}
# Not SAYILMAYAN dizinler. Burada yalnizca SPEC'in tanimladigi YAPISAL olanlar
# durur: `tasks/` ve `sessions/` DURUM tutar (durable bilgi degil), `.git` altyapi.
# Projeye ozel dizinler buraya YAZILMAZ → hafiza deposunun kendi ayar dosyasina.
ATLA = {"tasks", "sessions", ".git"}

# Hafiza deposunun KENDI ayarlari (opsiyonel, deponun kokunde).
#
#   skip_dirs:    [gecis]      not sayilmayan ek dizinler
#   pii_profiles: [tr]         ulkeye ozel PII desenleri (bkz. _PII_PROFILLERI)
#   review_days:  90           `current` notun gozden gecirme araligi
#   archive_index: gecis/legacy-index.md   devralinan, kanonik OLMAYAN indeks
#
# Neden tuketici repodaki context.yaml'da degil: bunlar HAFIZANIN ozellikleridir
# (hangi klasor not degil, hangi ulkenin verisi tutuluyor). Ayrica pre-commit
# kancasi hafiza deposunun icinden, manifest OLMADAN kosar — ayari orada bulmali.
HAFIZA_AYAR = "ctx.yaml"


def hafiza_ayarlari(hafiza: Path) -> dict:
    p = hafiza / HAFIZA_AYAR
    if not p.is_file():
        return {}
    try:
        return manifest_oku(p)
    except OSError:
        return {}


def _liste(deger) -> list[str]:
    """Liste alanini normalize et.

    Iki ayristirici iki bicim uretiyor: manifest_oku `[a, b]`yi gercek list,
    frontmatter ise "a, b" string'i yapiyor. Virgulle bolmek ikisini de dogru
    okur — aksi halde "server, worker" TEK etiket sanilir ve kapsam eslesmesi
    sessizce basarisiz olur.
    """
    if isinstance(deger, list):
        parcalar = deger
    elif isinstance(deger, str):
        parcalar = deger.split(",")
    else:
        return []
    return [str(d).strip() for d in parcalar if str(d).strip()]

# ARAMA NORMALIZASYONU — dile bagimli DEGIL, Unicode'un kendi kurallariyla.
#
# Iki ayri isi yapar; ikisi de yalniz ARAMA icindir, metin uretiminde ASLA kullanilmaz:
#
#   1) Aksan/isaret dusurme (NFKD + combining atma): "Ş"->"S", "é"->"e", "ü"->"u".
#      Gerekce: yollar ve tanimlayicilar cogu projede ASCII transliterasyonla yazilir
#      ("odeme", "ödeme" degil; "creche", "crèche" degil), yani aksanli sorgu bu
#      katlama olmadan dosya yoluna HICBIR ZAMAN eslesemez.
#   2) NFKD'nin AYRISTIRMADIGI harfler (asagidaki tablo): "ı", "ł", "ø", "ß"…
#      Bunlarin ASCII karsiligi bir isaret degil, ayri bir harftir.
#
# Buyuk/kucuk katlama en SONA birakilir ve `casefold()` ile yapilir — cunku aksanlar
# dustukten sonra "İ" zaten "I"dir ve Python'un "İ".lower() = "i" + U+0307 (combining
# dot) tuzagi olusamaz. Onceki surum bunun icin Turkce'ye ozel bir tablo tasiyordu;
# artik gerekmiyor, ve ayni kod Fransizca/Almanca/Ispanyolca/Lehce icin de calisiyor.
_AYRISMAYAN = str.maketrans({
    "ı": "i", "İ": "I",          # tr/az
    "ł": "l", "Ł": "L",          # pl
    "ø": "o", "Ø": "O",          # da/no
    "đ": "dj", "Đ": "DJ",        # hr/sr/bs — "Đakovo" ve "Djakovo" ayni anahtara dussun
    "ð": "d", "Ð": "D", "þ": "th", "Þ": "TH",   # is
    "ß": "ss", "æ": "ae", "Æ": "AE", "œ": "oe", "Œ": "OE",
})


def indir(metin: str) -> str:
    """Karsilastirma icin normalize et: aksan dusur, ASCII'ye katla, kucult.

    `.lower()` TEK BASINA KULLANILMAZ — "İ".lower() iki karakter (i + U+0307)
    uretir ve arama sessizce eslesmez.
    """
    ayrik = unicodedata.normalize("NFKD", metin.translate(_AYRISMAYAN))
    isaretsiz = "".join(c for c in ayrik if not unicodedata.combining(c))
    return isaretsiz.casefold()


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


def _gecerlilik(fm: dict) -> tuple[str, list[str]]:
    """Notun (status, scope) ikilisi. Eski `lifecycle` alani varsa cevrilir."""
    status = str(fm.get("status") or "").strip()
    scope = _liste(fm.get("scope"))
    if status or scope:
        return (status or "active"), scope
    eski = str(fm.get("lifecycle") or "").strip()
    return _ESKI_LIFECYCLE.get(eski, ("active", []))


def repo_kapsami(veri: dict) -> list[str]:
    """Bu reponun kapsami: `scope`, yoksa eski `role` alanindan turetilir."""
    kapsam = _liste(veri.get("scope"))
    if kapsam:
        return kapsam
    rol = str(veri.get("role") or "").strip()
    return [rol] if rol else []


def gorunur(fm: dict, kapsam: list[str], arsiv: bool = False) -> bool:
    """Not bu kapsamda okunmali mi?

    Kural iki cumle: (1) arsivlenmis not istenmedikce gelmez; (2) scope'u
    olmayan not EVRENSELDIR (her kapsamda gecerli), scope'u olan yalnizca
    kesisme varsa gelir. Kapsam tanimsizsa filtre uygulanmaz — tek cagli
    projede kimse scope yazmak zorunda kalmasin.
    """
    status, not_kapsami = _gecerlilik(fm)
    if status == "archived" and not arsiv:
        return False
    if not not_kapsami or not kapsam:
        return True
    return bool(set(not_kapsami) & set(kapsam))


def _atlanacaklar(hafiza: Path) -> set[str]:
    """Yapisal ATLA + hafizanin kendi `skip_dirs` girisleri."""
    return ATLA | {a.strip("/") for a in _liste(hafiza_ayarlari(hafiza).get("skip_dirs"))}


def notlari_gez(hafiza: Path):
    atla = _atlanacaklar(hafiza)
    for p in sorted(hafiza.rglob("*.md")):
        goreli = p.relative_to(hafiza)
        if atla & set(goreli.parts):
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
# 100, kelime yolun herhangi bir yerinde (kismi) geciyorsa 70. Boylece "billing" sorgusu
# domain/billing.md'yi (birebir stem esit) domain/billing-webhooks.md'nin (kismi yol
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
    (orn. `Aylik Rapor.xlsx` — bosluklu; `Fatura` — ozel karaktersiz +
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
    kapsam = repo_kapsami(veri)

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
        if not args.all and not gorunur(fm, kapsam):
            continue
        status, not_kapsami = _gecerlilik(fm)
        lc = ",".join(not_kapsami) or ("arsiv" if status == "archived" else "evrensel")
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

    # ESKI ARSIV (opsiyonel): baska bir sistemden devralinmis, kanonik olmayan bir
    # indeks dosyasi. Yolu context.yaml'daki `archive_index` belirler; tanimli
    # degilse bu bolum HIC calismaz. Kanonik notlardan SONRA, ayri baslik altinda,
    # en fazla 5 satir — devralinan kayit kanonik notla ayni gorunmemeli.
    arsiv_yolu = veri.get("archive_index")
    eski = (hafiza / str(arsiv_yolu)) if isinstance(arsiv_yolu, str) and arsiv_yolu else None
    if eski is not None and eski.is_file():
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
            print(f"\n--- eski arşiv ({arsiv_yolu}) ---")
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
    kapsam = repo_kapsami(veri)

    gruplar: dict[str, list[str]] = {}
    sayi = 0
    for p, goreli in notlari_gez(hafiza):
        fm = frontmatter(p.read_text(encoding="utf-8", errors="replace"))
        if not tum and not gorunur(fm, kapsam):
            continue
        grup, ad = goreli.parts[0], "/".join(goreli.parts[1:])
        gruplar.setdefault(grup, []).append(f"- `{ad}` — {_kisalt(fm.get('description', ''))}")
        sayi += 1

    hafiza_adi = Path(str(veri.get("memory_path", hafiza))).name
    satirlar = [
        f"### Hafıza haritası — {hafiza_adi} · kapsam: "
        f"{', '.join(kapsam) if kapsam else 'tümü'} · {sayi} not",
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


def tuketici_manifestleri(hafiza: Path, mevcut: Path) -> list[Path]:
    """Bu hafizayi okuyan BUTUN repolarin manifest yollari (mevcut olan dahil).

    Neden gerekli: bir hafiza, COK tuketici. Tazeleme yalnizca bulunulan reponun
    hedeflerine yazsaydi -- ki 2026-08-13'e kadar oyleydi -- digerleri sessizce
    kayardi. Olculdu: `yapit-server` ve `yapit-client` bloklari, iki notun
    aciklamasi degistikten 9 gun sonra hala eskisini gosteriyordu; o repolarda
    calisan harness'siz araclar bayat korpus okuyordu. SPEC "bayatlik yapisal
    olarak kapatilir" diyor, ama soz repo BASINA gecerliydi.

    Liste hafizanin kendi `ctx.yaml`'indadir (`consumers:`), cunku "beni kimler
    okuyor" hafizanin ozelligidir; tek tuketici yoksa alan bos birakilir.
    """
    yollar = [mevcut.resolve()]
    for ham in _liste(hafiza_ayarlari(hafiza).get("consumers")):
        aday = (hafiza / ham).resolve() / "context.yaml"
        if aday.is_file() and aday not in yollar:
            yollar.append(aday)
    return yollar


def _kisa_hedef(hedef: Path) -> str:
    """Hedefi reposuyla adlandirir: dort tuketici de duz 'AGENTS.md' yazmasin."""
    return f"{hedef.parent.name}/{hedef.name}"


def _haritalari_tazele(manifest: Path) -> list[str]:
    """Tek tuketicinin bloklarini SESSIZCE tazeler; yazilanlarin adini dondurur."""
    veri = manifest_oku(manifest)
    mp = veri.get("memory_path")
    if not isinstance(mp, str):
        return []
    hafiza = (manifest.parent / mp).resolve()
    govde = index_govde(veri, hafiza)
    yazilan = []
    for hedef in _index_hedefleri(manifest, veri):
        if not hedef.is_file():
            continue
        metin = _oku_ham(hedef)
        yeni = _blok_degistir(metin, govde)
        if yeni is not None and yeni != metin:
            with open(hedef, "w", encoding="utf-8", newline="") as f:
                f.write(yeni)
            yazilan.append(_kisa_hedef(hedef))
    return yazilan


def _tuketiciyi_isle(manifest: Path, yaz: bool, tum: bool, cikti) -> int:
    """Tek bir tuketicinin index bloklarini kontrol eder / tazeler.

    Her tuketici KENDI manifest'iyle hesaplanir: kapsam filtresi repodan repoya
    degisir (legacy reposu legacy notlarini gorur, current repo gormez).
    Sorunlu hedef sayisini dondurur.
    """
    veri = manifest_oku(manifest)
    mp = veri.get("memory_path")
    if not isinstance(mp, str):
        return 0
    hafiza = (manifest.parent / mp).resolve()
    govde = index_govde(veri, hafiza, tum=tum)

    sorunlu = 0
    for hedef in _index_hedefleri(manifest, veri):
        if not hedef.is_file():
            cikti(f"✗ {hedef} · dosya yok")
            sorunlu += 1
            continue
        metin = _oku_ham(hedef)
        guncellenmis = _blok_degistir(metin, govde)
        if guncellenmis is None:
            cikti(f"✗ {hedef} · ctx:index isareti yok — hedef dosyaya su iki satiri "
                  f"komsu satirlarda ekle:\n    {INDEX_BAS}\n    {INDEX_SON}")
            sorunlu += 1
        elif guncellenmis == metin:
            cikti(f"✓ {_kisa_hedef(hedef)} · guncel")
        elif not yaz:
            cikti(f"⚠ {hedef} · index blogu BAYAT — `ctx index --yaz`")
            sorunlu += 1
        else:
            with open(hedef, "w", encoding="utf-8", newline="") as f:
                f.write(guncellenmis)
            cikti(f"✓ {hedef} · yazildi")
    return sorunlu


def komut_index(args) -> int:
    manifest, veri, hafiza = baglam_yukle(args.repo)

    if not (args.yaz or args.kontrol):
        print(index_govde(veri, hafiza, tum=args.all))
        return 0

    sorunlu = 0
    for m in tuketici_manifestleri(hafiza, manifest):
        sorunlu += _tuketiciyi_isle(m, yaz=args.yaz, tum=args.all, cikti=print)
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

ZORUNLU_ALANLAR = ("title", "description", "type", "updated")
REVIEW_GUN = 90

_MD_LINK = re.compile(r"\[[^\]]*\]\(([^)#]+?)(?:#[^)]*)?\)")
_WIKI_LINK = re.compile(r"\[\[([^\]|]+)")

# Deger ATANMIS gizli anahtar. Yalniz adin gecmesi bulgu DEGILDIR — SPEC'in
# onerdigi "ADMIN_SECRET ister, degeri disarida yasar" bicimi mesru.
_SECRET_DESENI = re.compile(
    r"(?i)\b(api[_-]?key|secret|token|password|passwd|pwd)\b\s*[:=]\s*['\"]?([A-Za-z0-9+/=_.-]{16,})"
)
# PII desenleri iki kumede: EVRENSEL olanlar her projede kosar; ULKEYE OZEL
# olanlar yalnizca context.yaml'daki `pii_profiles` listesinde adi gecerse kosar
# (orn. `pii_profiles: [tr]`). Gerekce: "11 haneli sayi" Turkiye'de kimlik
# numarasidir ama baska bir projede sipariş/olcum numarasi olabilir — her yerde
# kosarsa yanlis alarm uretir; hic kosmazsa gercek PII kacar.
# BICIM TAHMINI YETMEZ, SAGLAMA TOPLAMI GEREKIR.
#
# Olculdu (0.5.0 gelistirme turu): salt bicime bakan desenler gunluk metni PII
# saniyordu — "2024 2025 2026 2027" (yil dizisi) kredi kartina, "AB12CDEF1234..."
# (anahtar/hash) IBAN'a benziyordu. Yanlis alarm veren kapi kapidan SAYILMAZ:
# kullanici `--no-verify` aliskanligi edinir ve gercek PII de gecer.
#
# Cozum: bu iki desende bicim yalnizca ADAY uretir; karar Luhn (kart) ve
# ISO 7064 mod-97 (IBAN) saglamasiyla verilir. Rastgele rakam dizisinin Luhn'dan
# gecme olasiligi ~1/10, mod-97'den ~1/97'dir.

def _luhn_gecerli(hane: str) -> bool:
    toplam, ikili = 0, False
    for c in reversed(hane):
        d = ord(c) - 48
        if ikili:
            d *= 2
            if d > 9:
                d -= 9
        toplam += d
        ikili = not ikili
    return toplam % 10 == 0


def _iban_gecerli(ham: str) -> bool:
    s = re.sub(r"[\s-]", "", ham).upper()
    if not (15 <= len(s) <= 34):
        return False
    tasinmis = s[4:] + s[:4]
    sayisal = "".join(str(ord(c) - 55) if c.isalpha() else c for c in tasinmis)
    try:
        return int(sayisal) % 97 == 1
    except ValueError:
        return False


def _saglamali(desen: str, dogrulayici):
    """Bicimsel aday uretir, karari `dogrulayici`ya birakir."""
    derli = re.compile(desen)

    class _Saglamali:
        @staticmethod
        def search(metin: str):
            for m in derli.finditer(metin):
                if dogrulayici(m.group(0)):
                    return m
            return None

    return _Saglamali


_PII_EVRENSEL = [
    ("e-posta", re.compile(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}")),
    ("IBAN", _saglamali(r"\b[A-Z]{2}\d{2}(?:[ -]?[A-Za-z0-9]{4}){2,7}[ -]?[A-Za-z0-9]{1,4}\b",
                        _iban_gecerli)),
    ("kredi karti", _saglamali(r"(?<!\d)(?:\d{4}[ -]?){3}\d{4}(?!\d)",
                               lambda s: _luhn_gecerli(re.sub(r"[ -]", "", s)))),
]

_PII_PROFILLERI = {
    "tr": [
        ("TC kimlik gorunumlu 11 hane", re.compile(r"(?<!\d)[1-9]\d{10}(?!\d)")),
        ("telefon (TR cep)", re.compile(r"(?<!\d)0?5\d{2}[\s.-]?\d{3}[\s.-]?\d{2}[\s.-]?\d{2}(?!\d)")),
    ],
    "us": [
        ("SSN gorunumlu", re.compile(r"(?<!\d)\d{3}-\d{2}-\d{4}(?!\d)")),
        ("telefon (US)", re.compile(r"(?<!\d)\(?\d{3}\)?[\s.-]\d{3}[\s.-]\d{4}(?!\d)")),
    ],
    "de": [
        ("Steuer-ID gorunumlu 11 hane", re.compile(r"(?<!\d)\d{11}(?!\d)")),
    ],
}


def pii_desenleri(profiller=()) -> list:
    """Evrensel desenler + istenen ulke profilleri."""
    secili = list(_PII_EVRENSEL)
    for ad in profiller or ():
        secili += _PII_PROFILLERI.get(str(ad).strip().casefold(), [])
    return secili


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
    """(seviye, dosya, mesaj) listesi dondurur. seviye: 'hata' | 'uyari'.

    Ulkeye ozel PII desenleri ve gozden gecirme araligi hafizanin kendi
    `ctx.yaml`indan gelir (yoksa: yalniz evrensel desenler, 90 gun).
    """
    ayar = hafiza_ayarlari(hafiza)
    desenler = pii_desenleri(_liste(ayar.get("pii_profiles")))
    try:
        review_gun = int(str(ayar.get("review_days") or REVIEW_GUN))
    except ValueError:
        review_gun = REVIEW_GUN
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

        # Gecerlilik ekseni: yeni `status`/`scope` ya da eski `lifecycle`.
        # Ikisi de yoksa not evrensel+aktif sayilir — tek cagli projede kimse
        # kapsam yazmak zorunda degil.
        eski_lc = str(fm.get("lifecycle") or "").strip()
        if eski_lc and eski_lc not in _ESKI_LIFECYCLE:
            bulgular.append(("hata", yol, f"gecersiz lifecycle: {eski_lc!r}"))
        elif eski_lc:
            bulgular.append(("uyari", yol,
                             f"`lifecycle: {eski_lc}` eskidi → `ctx migrate` "
                             "(status/scope ikilisine cevirir)"))
        status = str(fm.get("status") or "").strip()
        if status and status not in GECERLI_STATUS:
            bulgular.append(("hata", yol, f"gecersiz status: {status!r} "
                                          f"(gecerli: {', '.join(sorted(GECERLI_STATUS))})"))
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

        # review-due YALNIZ aktif notlar icin. Onceki model bunu sadece
        # `lifecycle: current`e uyguluyordu; iki eksenli modelde "bugunu tarif
        # eden her not" dogal karsiligi — arsivlenmis not zaten bugunu tarif
        # etmedigini soyluyor, ona bayatlik sormak anlamsiz.
        if _gecerlilik(fm)[0] == "active":
            t = _tarih_ayristir(fm.get("updated", ""))
            if t is None:
                bulgular.append(("hata", yol, f"updated tarihi okunamadi: {fm.get('updated')!r}"))
            else:
                gun = (_bugun() - t).days
                if gun > review_gun:
                    bulgular.append(("uyari", yol,
                                     f"review-due: {gun} gundur dogrulanmadi (stale DEGIL)"))

        if tam:
            for m in _SECRET_DESENI.finditer(metin):
                bulgular.append(("hata", yol,
                                 f"deger atanmis gizli anahtar gorunumu: {m.group(1)}=***"))
            for ad, desen in desenler:
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

    # Korumanin KAPSAMI her zaman soylenir. Aksi halde profil tanimsizken tarama
    # sessizce daralir ve "✓ bulgu yok" ciktisi, aslinda hic BAKILMAMIS bir PII
    # sinifi icin de temiz izlenimi verir — sessizce yanlis olmanin ta kendisi.
    if args.tam:
        profiller = _liste(hafiza_ayarlari(hafiza).get("pii_profiles"))
        if profiller:
            print(f"  PII kapsami: evrensel + profil({', '.join(profiller)})")
        else:
            print("  PII kapsami: YALNIZ EVRENSEL (e-posta · IBAN · kart). Ulkeye ozel "
                  "kimlik/telefon desenleri KOSMADI —\n  acmak icin: "
                  f"{HAFIZA_AYAR} -> pii_profiles: [tr]  (secenekler: "
                  f"{', '.join(sorted(_PII_PROFILLERI))})")
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


def _icinde_mi(alt: Path, ust: Path) -> bool:
    try:
        alt.resolve().relative_to(ust.resolve())
        return True
    except (ValueError, OSError):
        return False


def _repo_durumu(repo: Path) -> tuple[str, int]:
    """Tek bir git reposunun (HEAD, kirli dosya sayisi) ikilisi."""
    kirli = [s for s in _git(repo, "status", "--porcelain").splitlines() if s.strip()]
    return _git(repo, "rev-parse", "--short", "HEAD"), len(kirli)


def _calisma_durumu(repo: Path, sinir: Path | None = None) -> dict:
    """Bulunulan yerdeki 'anlamli is' sinyalleri. Kirli dosya varsa is yapilmistir.

    SINIR manifest'in dizinidir ve git aramasi ONU ASLA ASMAZ. Olculdu
    (2026-08-13): `yapit/` catisi bir git reposu DEGIL, dolayisiyla
    `rev-parse --show-toplevel` yukari tirmanip kullanicinin EV DIZININI repo
    saniyordu — "repo: onur5 · HEAD: ? · degisik dosya: 63". Kayitlar da ayni
    ada dustugu icin `--kontrol` her seferinde "guncel" deyip susuyordu: sistemin
    butun disiplini olan kapi, oturumlarin basladigi yerde KAPALIYDI.

    Cati bir repo degilse altindaki repolar TOPLANIR. Boylece kapi cok repolu
    duzende de calisir; HEAD parmak izi alt repolarin HEAD'lerinden turetilir ki
    herhangi birindeki commit kapiyi yeniden kursun.
    """
    kok_str = _git(repo, "rev-parse", "--show-toplevel")
    kok = Path(kok_str) if kok_str else None

    if kok is not None and (sinir is None or kok.resolve() == sinir.resolve()
                            or _icinde_mi(kok, sinir)):
        head, kirli = _repo_durumu(kok)
        return {"repo": kok.name, "head": head, "kirli": kirli}

    if sinir is None:
        return {"repo": repo.name, "head": "", "kirli": 0}

    parcalar, toplam = [], 0
    for d in sorted(p for p in sinir.iterdir() if (p / ".git").exists()):
        head, kirli = _repo_durumu(d)
        parcalar.append(f"{d.name}@{head or '?'}")
        toplam += kirli
    return {"repo": sinir.name, "head": ",".join(parcalar), "kirli": toplam}


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

    manifest, _, hafiza = baglam_yukle(args.repo)
    # Sinir manifest'in dizinidir: git aramasi projeden disari cikamaz.
    durum = _calisma_durumu(Path(args.repo) if args.repo else Path.cwd(), manifest.parent)
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
        # BUTUN tuketiciler, yalnizca bu repo degil: yeni not hepsinin blogunu
        # ayni anda bayatlatir ve `ctx close`u kostugun repo ozel degildir.
        # Yalnizca mevcut repoyu tazelemek, iki reponun dokuz gun geride
        # kalmasina yol acan seydi.
        manifest, _veri, _ = baglam_yukle(args.repo)
        for m in tuketici_manifestleri(hafiza, manifest):
            for ad in _haritalari_tazele(m):
                print(f"  ↻ {ad} · hafiza haritasi tazelendi")
    else:
        print("✓ capture karari kaydedildi: durable bulgu yok")

    kayitlar = _log_satirlari(hafiza)
    kapali = sum(1 for k in kayitlar if k.get("karar") in ("none", "captured"))
    if kayitlar:
        print(f"  Capture Decision Coverage: {kapali}/{len(kayitlar)} oturum kararla kapandi")
    return 0


# ---------------------------------------------------------------- hook
#
# SessionStart kancalari ARACIN KENDISINDE. Onceden `templates/hooks/*.py`
# altinda ayri script'lerdi ve her biri ince bir sarmalayiciydi: bir ctx komutu
# kos, ciktisini harness'in JSON zarfina koy, her hatada sessizce cik. Aracin
# icine alinca hem tek kaynak olur hem de kurulumun ayri dosyalara bagimliligi
# biter (Go binary'si de ayni komutu tasiyor).
#
# ILK KURAL SESSIZLIK: bu kod HER oturum baslangicinda kosar. Bir kez cop
# basarsa, biri fark edene kadar butun oturumlari kirletir. Bu yuzden her hata
# yolu sessizce 0 ile doner — hafiza yok, manifest yok, korpus okunmuyor: hepsi
# sessiz.

def _hook_zarf(icerik: str) -> str:
    """Harness'in bekledigi SessionStart zarfi."""
    return json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "SessionStart",
            "additionalContext": icerik,
        }
    })


def _hook_index(args) -> int:
    yol = manifest_bul(Path(args.repo) if args.repo else Path.cwd())
    if yol is None:
        return 0
    veri = manifest_oku(yol)
    mp = veri.get("memory_path")
    if not isinstance(mp, str):
        return 0
    hafiza = (yol.parent / mp).resolve()
    if not hafiza.is_dir():
        return 0

    harita = index_govde(veri, hafiza).strip()
    if not harita:
        return 0

    print(_hook_zarf(
        "KANONIK HAFIZA HARITASI (her oturumda otomatik yuklenir — "
        "harness auto-memory'si DEGIL, `context.yaml -> memory_path`):\n\n"
        + harita
        + "\n\nKullanim: gorevin konusu yukaridaki bir satirla ortusuyorsa O NOTU AC "
          "(tam metin orada; bu yalniz harita). Ortusme belirsizse "
          '`ctx search "<sorgu>"`. Butun korpusu topluca yukleme. '
          "Hafiza canli kodun yerine gecmez: `git log`/`git diff` otoriterdir."))
    return 0


def _hook_capture_check(args) -> int:
    import contextlib
    import io

    class _K:
        repo = args.repo
        kontrol = True
        yok = False
        not_ = None

    # Kullanicinin kosacagi KOMUTUN KENDISI cagrilir; boylece kanca, haber
    # verdigi komuttan hicbir zaman ayrisamaz.
    tampon = io.StringIO()
    with contextlib.redirect_stdout(tampon):
        kod = komut_close(_K())
    # 0 = karar guncel · 2 = bu projede hafiza yok → ikisinde de sessiz
    if kod != 1:
        return 0
    uyari = tampon.getvalue().strip()
    if not uyari:
        return 0

    print(_hook_zarf(
        "CAPTURE KARARI BEKLIYOR — onceki oturum kalici bulgu karari "
        "verilmeden kapanmis:\n\n" + uyari + "\n\n"
        "Bu oturumun SONUNDA karar ver: kalici bir bulgu varsa once "
        "hafizaya yaz, sonra `ctx close --not <yol>`; "
        "yoksa `ctx close --yok \"gerekce\"`. Karar VERMEMEK gecersizdir. "
        "Kullaniciyi simdi bununla mesgul etme — kapanista hallet."))
    return 0


def komut_hook(args) -> int:
    try:
        if args.ad == "index":
            return _hook_index(args)
        return _hook_capture_check(args)
    except (Exception, SystemExit):
        # Kanca ASLA oturumu bozmaz. `SystemExit` ayrica yakalanir cunku
        # `Exception`in altinda DEGILDIR: `baglam_yukle` hafiza kurulu olmayan
        # projede SystemExit(2) firlatir ve bu, kancanin cikis kodu olarak
        # disari sizardi — harness acisindan "kanca patladi" demek.
        return 0


# ---------------------------------------------------------------- doctor

def _binary_tazeligi() -> list[str]:
    """Derlenmis ctx, kaynagindan eski mi?

    Sarmalayici (`bin/ctx`) binary varsa onu SESSIZCE secer. Bu, kurulumun en
    sessiz ariza bicimini dogurur: kaynagi degistirir, yeniden derlemeyi
    unutursun, ve komutlar eski kodla kosmaya devam eder — cikti dogru gorunur,
    yalnizca YANLIS SURUMDUR. Kurulum saglik kontrolu bunu soylemek zorunda.

    Kurulum yeri sabittir (`~/.agent-context`): doctor calisan binary'yi degil
    KURULUMU denetler, ve iki implementasyon ayni cevabi vermelidir.
    """
    kok = Path.home() / ".agent-context"
    binary = next((p for p in (kok / "bin" / "ctx.exe", kok / "bin" / "ctx.bin")
                   if p.is_file()), None)
    if binary is None:
        return []  # binary yok -> sarmalayici Python'u secer, bayatlik kavrami yok

    try:
        binary_zaman = binary.stat().st_mtime
    except OSError:
        return []

    kaynaklar = [kok / "bin" / "ctx.py", kok / "go.mod"]
    kaynaklar += [p for p in (kok / "go").rglob("*.go") if not p.name.endswith("_test.go")]
    kaynaklar.append(kok / "embed.go")
    kaynaklar += list((kok / "templates").rglob("*"))  # sablonlar binary'ye GOMULU

    yeniler = []
    for p in kaynaklar:
        try:
            if p.is_file() and p.stat().st_mtime > binary_zaman:
                yeniler.append(p.relative_to(kok).as_posix())
        except OSError:
            continue
    if not yeniler:
        return []

    ornek = ", ".join(sorted(yeniler)[:3])
    if len(yeniler) > 3:
        ornek += f" (+{len(yeniler) - 3})"
    return [f"derlenmis ctx BAYAT — {len(yeniler)} kaynak daha yeni ({ornek}); "
            f"`go build -o bin/ctx.exe ./go/cmd/ctx`"]


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
    # DIKKAT: dongu degiskeni `yol` OLAMAZ — `yol` manifest'in Path'ini tutuyor ve
    # asagida `_index_hedefleri(yol, ...)` ona ihtiyac duyuyor. Ustune yazilinca
    # `ctx doctor`, hafizada YAPISAL HATA VARKEN AttributeError ile coker; temiz
    # korpusta hic gorunmez. 2026-08-13'te Go paritesi sirasinda yakalandi.
    for _, bulgu_yolu, mesaj in hatalar[:5]:
        print(f"    ✗ {bulgu_yolu} · {mesaj}")
    if len(hatalar) > 5:
        print(f"    (+{len(hatalar) - 5} hata daha — 'ctx validate' ile tamamini gor)")

    kontrol((hafiza / "tasks" / "OPEN.md").is_file(), "tasks/OPEN.md var")
    kontrol((hafiza / "sessions" / "CURRENT.md").is_file(), "sessions/CURRENT.md var")

    # Derlenmis binary BAYAT mi? `bin/ctx` sarmalayicisi varsa binary'yi SESSIZCE
    # tercih eder — bu, kaynagi degistirip yeniden derlemeyi unutan herkesin eski
    # kodu kosturmasi demektir ve hicbir yerde belli olmaz. Tek gorunur yer burasi.
    for satir in _binary_tazeligi():
        uyarilar.append(("uyari", "bin/ctx", satir))

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
        for _, bulgu_yolu, mesaj in uyarilar[:8]:
            print(f"  ⚠ {bulgu_yolu} · {mesaj}")
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


# ---------------------------------------------------------------- migrate

def komut_migrate(args) -> int:
    """Eski tek-eksenli `lifecycle` alanini `status` + `scope` ikilisine cevirir.

    Satir bazli, yerinde: frontmatter'in geri kalanina ve govdeye DOKUNMAZ.
    `--kuru` ile yalniz ne yapacagini gosterir.
    """
    _, _veri, hafiza = baglam_yukle(args.repo)
    yaz = not args.kuru
    degisen = atlanan = 0

    for p, goreli in notlari_gez(hafiza):
        # HAM okuma (newline=""): `read_text` evrensel satir sonu cevirisi yapar
        # ve "\r\n"i "\n"e dondurur — yani dosyanin GERCEK satir sonu gorunmez
        # olur ve korunamaz. Ayristirma bundan etkilenmez: `splitlines()` \r'yi
        # zaten dusurur, frontmatter degerleri `.strip()`ten gecer.
        metin = _oku_ham(p)
        fm = frontmatter(metin)
        eski = str(fm.get("lifecycle") or "").strip()
        if not eski:
            atlanan += 1
            continue
        if eski not in _ESKI_LIFECYCLE:
            print(f"  ! {goreli.as_posix()} · bilinmeyen lifecycle: {eski!r} — ATLANDI")
            atlanan += 1
            continue

        status, kapsam = _ESKI_LIFECYCLE[eski]
        yeni_satirlar = [f"status: {status}"]
        if kapsam:
            yeni_satirlar.append(f"scope: [{', '.join(kapsam)}]")

        cikti, degisti = [], False
        for satir in metin.splitlines():
            if not degisti and satir.strip().startswith("lifecycle:"):
                cikti.extend(yeni_satirlar)
                degisti = True
            else:
                cikti.append(satir)
        if not degisti:
            atlanan += 1
            continue

        print(f"  {goreli.as_posix()} · lifecycle: {eski} → "
              f"status: {status}" + (f" · scope: [{', '.join(kapsam)}]" if kapsam else " (evrensel)"))
        if yaz:
            # Satir sonu KORUNUR — `_blok_degistir` ile ayni kural, ayni gerekce.
            # Onceki surum `write_text`i metin modunda cagiriyordu; Windows'ta bu,
            # LF olan her notu CRLF'e ceviriyordu ve migrate'in kendi sozunu
            # ("satir bazli, gerisine DOKUNMAZ") bozuyordu: tek satirlik degisiklik
            # diff'te butun dosya olarak goruluyordu. splitlines() zaten \r'yi
            # dusurdugu icin birlestirme dosyanin KENDI ekiyle yapilir.
            nl = "\r\n" if "\r\n" in metin else "\n"
            son = nl.join(cikti)
            if metin.endswith("\n"):
                son += nl
            p.write_text(son, encoding="utf-8", newline="")
        degisen += 1

    print(f"\n{degisen} not cevrildi, {atlanan} atlandi"
          + (" (KURU KOSUM — dosya yazilmadi)" if args.kuru else ""))
    if degisen and yaz:
        print("Not: tuketici repolarin context.yaml'inda `role:` yerine `scope:` "
              "kullanabilirsiniz (role hala okunuyor).")
    return 0


# ---------------------------------------------------------- bootstrap

# SessionStart kancalari: (hook adi, eski script adi, durum mesaji, zaman asimi).
#
# Eski script adi yalnizca GECIS icin durur: 2026-08-13'ten once settings.json'a
# `python ".../ctx_index.py"` diye kaydedilmisti. Kayit artik dogrudan `ctx`
# sarmalayicisini cagirir (bkz. `_ctx_sarmalayici`), ama eski kayit taninip
# YERINDE guncellenmeli — aksi halde ayni kanca iki kez kosardi.
HOOK_KAYITLARI = (
    ("capture-check", "ctx_capture_check.py", "Capture karari kontrol ediliyor...", 20),
    ("index", "ctx_index.py", "Hafiza haritasi yukleniyor...", 20),
)


def _claude_dizini() -> Path:
    return Path.home() / ".claude"


def _ctx_sarmalayici() -> Path:
    """Kancalarin cagiracagi tek giris noktasi.

    Implementasyon DEGIL, sarmalayici kaydedilir: `bin/ctx` once derlenmis
    binary'yi, yoksa Python'u secer. Boylece settings.json, Go gecisinden ve
    binary'nin o makinede derlenmis olup olmamasindan etkilenmez.
    """
    return Path.home() / ".agent-context" / "bin" / "ctx"


def _hook_komutu(ad: str) -> str:
    return f'"{_ctx_sarmalayici()}" hook {ad}'


def hook_kur(yaz: bool = True) -> list[str]:
    """SessionStart kancalarini settings.json'a KAYDEDER.

    Artik dosya kopyalanmaz: kancalarin kendisi aracin icinde (`ctx hook ...`).
    Onceden `~/.claude/hooks/` altina iki Python script'i yazilirdi; onlar ince
    sarmalayicidan ibaretti ve ayri bir kopya olarak bakim istiyordu.

    settings.json kullanicinin KENDI ayarlarini tasir → uzerine YAZILMAZ, yalniz
    eksik kayit eklenir ya da eski kayit yerinde guncellenir (idempotent).
    """
    rapor: list[str] = []
    ayar_yolu = _claude_dizini() / "settings.json"
    if yaz:
        _claude_dizini().mkdir(parents=True, exist_ok=True)
    try:
        ayarlar = json.loads(ayar_yolu.read_text(encoding="utf-8")) if ayar_yolu.is_file() else {}
    except (json.JSONDecodeError, OSError) as e:
        rapor.append(f"! settings.json okunamadi ({e}) — hook KAYDEDILMEDI, elle ekleyin")
        return rapor

    kancalar = ayarlar.setdefault("hooks", {}).setdefault("SessionStart", [])
    if not kancalar:
        kancalar.append({"hooks": []})
    liste = kancalar[0].setdefault("hooks", [])

    degisti = False
    for ad, eski_script, mesaj, sure in HOOK_KAYITLARI:
        istenen = _hook_komutu(ad)
        # Kaydin BIZIM olup olmadigi iki isaretten anlasilir: yeni bicim
        # (`hook <ad>`) ya da eski script adi. Ikincisi olmadan gecis kaydi
        # taninmaz ve ayni kanca iki kez kosardi.
        girdi = next(
            (k for k in liste
             if eski_script in str(k.get("command", ""))
             or f"hook {ad}" in str(k.get("command", ""))),
            None,
        )
        if girdi is None:
            liste.append({
                "type": "command",
                "command": istenen,
                "shell": "bash",
                "timeout": sure,
                "statusMessage": mesaj,
            })
            degisti = True
            rapor.append(f"+ settings.json: {ad} eklendi")
        elif str(girdi.get("command", "")) != istenen:
            # YERINDE guncelleme: kullanicinin sirasi ve diger alanlari korunur.
            girdi["command"] = istenen
            degisti = True
            rapor.append(f"↻ settings.json: {ad} → `ctx hook {ad}` olarak guncellendi")
        else:
            rapor.append(f"= settings.json: {ad} kayitli")

    if degisti and yaz:
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

    mg = alt.add_parser("migrate", help="eski `lifecycle` alanini status+scope'a cevir")
    mg.add_argument("--repo")
    mg.add_argument("--kuru", action="store_true", help="hicbir sey yazma, ne yapacagini goster")
    mg.set_defaults(fn=komut_migrate)

    hk = alt.add_parser("hook", help="SessionStart kancalari (harness cagirir)")
    hk.add_argument("ad", choices=["index", "capture-check"])
    hk.add_argument("--repo")
    hk.set_defaults(fn=komut_hook)

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
