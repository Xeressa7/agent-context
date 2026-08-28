"""`dogrula()` — tek dogrulama motoru. `validate`, `doctor` ve pre-commit
kancasi AYNI fonksiyonu cagirir; mantik tekrari yok. Bu testler o motorun
sozlerini kilitler.

Onemli ayrim: 'hata' cikis kodunu etkiler, 'uyari' etkilemez. review-due
bilerek UYARIDIR — 90 gununu doldurmus not "yanlis" degildir, bakilmasi
gerekendir.
"""
import os


def seviyeler(bulgular):
    return {(s, m.split(":")[0]) for s, _yol, m in bulgular}


def mesajlar(bulgular):
    return " | ".join(m for _s, _y, m in bulgular)


# ------------------------------------------------------- zorunlu alanlar

def test_frontmattersiz_dosya_hata(ctx_modul, hafiza):
    (hafiza / "facts" / "bos.md").write_text("frontmatter yok\n", encoding="utf-8")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(s == "hata" and "frontmatter" in m for s, _y, m in bulgular)


def test_gecersiz_lifecycle_hata(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", lifecycle="uydurma")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(s == "hata" and "lifecycle" in m for s, _y, m in bulgular)


def test_eksik_zorunlu_alan_hata(ctx_modul, hafiza):
    (hafiza / "facts" / "a.md").write_text(
        "---\ntitle: T\nlifecycle: current\ntype: domain\n---\n\ngovde\n", encoding="utf-8")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(s == "hata" and "description" in m for s, _y, m in bulgular)


def test_saglam_not_bulgu_uretmez(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/saglam.md")
    assert ctx_modul.dogrula(hafiza) == []


# ------------------------------------------------------------- linkler

def test_kirik_wiki_link_yakalanir(ctx_modul, hafiza, yaz):
    """12 Agustos'ta validator ilk kosumunda gercek bir kirik wiki-link yakaladi;
    bu test o davranisi kalici kilar."""
    yaz(hafiza, "facts/a.md", govde="Bkz [[olmayan-not]] detayda.")
    bulgular = ctx_modul.dogrula(hafiza)
    assert "olmayan-not" in mesajlar(bulgular)


def test_var_olan_wiki_link_temiz(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/hedef.md", title="Hedef not")
    yaz(hafiza, "facts/a.md", title="Kaynak not", govde="Bkz [[hedef]] detayda.")
    assert ctx_modul.dogrula(hafiza) == []


def test_ayni_baslikli_iki_not_uyarir(ctx_modul, hafiza, yaz):
    """Ayni basligi tasiyan iki not, haritada ayirt edilemez satir uretir."""
    yaz(hafiza, "facts/a.md", title="Ayni Baslik")
    yaz(hafiza, "facts/b.md", title="Ayni Baslik")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any("baslik tekrari" in m for _s, _y, m in bulgular)


def test_kirik_markdown_link_yakalanir(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", govde="Bkz [su](../facts/yok.md) detayda.")
    assert "yok.md" in mesajlar(ctx_modul.dogrula(hafiza))


# ---------------------------------------------------------- superseded_by

def test_superseded_by_hedefi_yoksa_bulgu(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", superseded_by="facts/olmayan.md")
    assert "olmayan.md" in mesajlar(ctx_modul.dogrula(hafiza))


# ------------------------------------------------------------ PII / secret

def test_tam_bayragi_olmadan_pii_taranmaz(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", govde="TC: 12345678901")
    assert ctx_modul.dogrula(hafiza, tam=False) == []


def test_tam_bayragi_ile_evrensel_pii_yakalanir(ctx_modul, hafiza, yaz):
    """pre-commit kancasi `--tam` ile kosar: PII'li commit DURDURULUR.
    E-posta her projede PII'dir — profil gerektirmez."""
    yaz(hafiza, "facts/a.md", govde="Iletisim: ornek.kisi@ornek.com")
    assert ctx_modul.dogrula(hafiza, tam=True), "evrensel PII taranmadi"


def test_ulkeye_ozel_desen_PROFILSIZ_kosmaz(ctx_modul, hafiza, yaz):
    """11 haneli sayi Turkiye'de kimlik no'dur ama baska projede siparis
    numarasi olabilir — profil acilmadan yanlis alarm uretmemeli."""
    yaz(hafiza, "facts/a.md", govde="Siparis numarasi 12345678901 olarak kayitli.")
    assert ctx_modul.dogrula(hafiza, tam=True) == []


def test_ulkeye_ozel_desen_PROFILLE_kosar(ctx_modul, hafiza, yaz):
    (hafiza / "ctx.yaml").write_text("pii_profiles: [tr]\n", encoding="utf-8")
    yaz(hafiza, "facts/a.md", govde="TC: 12345678901")
    bulgular = ctx_modul.dogrula(hafiza, tam=True)
    assert any("TC kimlik" in m for _s, _y, m in bulgular)


def test_bilinmeyen_profil_sessizce_yok_sayilir(ctx_modul, hafiza, yaz):
    (hafiza / "ctx.yaml").write_text("pii_profiles: [olmayan_ulke]\n", encoding="utf-8")
    yaz(hafiza, "facts/a.md", govde="duz metin")
    assert ctx_modul.dogrula(hafiza, tam=True) == []


def test_review_due_HATA_degil_UYARI(ctx_modul, hafiza, yaz):
    """Bilincli karar: bayat not commit'i engellemez. Bu testi kirmadan
    review-due'yu hataya cevirmeyin — kalite kapisi sevimsizlesir."""
    yaz(hafiza, "facts/eski.md", updated="2020-01-01")
    bulgular = ctx_modul.dogrula(hafiza)
    assert bulgular, "review-due hic uyarmadi"
    assert all(s == "uyari" for s, _y, _m in bulgular)


# ------------------------------------------------- state dosyalari not degil

def test_tasks_ve_sessions_dogrulamaya_girmez(ctx_modul, hafiza):
    """OPEN.md / CURRENT.md durable NOT degil, DURUM. Frontmatter'lari yok ve
    olmamali; dogrulama onlari gormemeli (ATLA kumesi)."""
    assert ctx_modul.dogrula(hafiza) == []


def test_aktif_task_open_indeksinde_olmali(ctx_modul, hafiza, yaz):
    """Indekssiz aktif task = gorunmez is; bir sonraki oturum onu hic bulmaz."""
    yaz(hafiza, "tasks/unutulmus-brief.md", type_="task")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(
        s == "hata" and y == "tasks/unutulmus-brief.md" and
        "OPEN.md'de listelenmiyor" in m
        for s, y, m in bulgular
    )


def test_aktif_task_ve_open_linki_birlikte_temiz(ctx_modul, hafiza, yaz):
    yaz(hafiza, "tasks/brief.md", type_="task")
    (hafiza / "tasks" / "OPEN.md").write_text(
        "# Acik isler\n\n- [Brief](brief.md)\n", encoding="utf-8")
    bulgular = ctx_modul.dogrula(hafiza)
    assert not any(y.startswith("tasks/") for _s, y, _m in bulgular)


def test_open_kirik_task_linki_hata(ctx_modul, hafiza):
    """Hedefi silinmis satir, kapanmis isi acik gosterir."""
    (hafiza / "tasks" / "OPEN.md").write_text(
        "# Acik isler\n\n- [Yok](yok-boyle-bir-dosya.md)\n", encoding="utf-8")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(s == "hata" and "kirik task linki" in m for s, _y, m in bulgular)


def test_arsiv_task_open_indeksinden_cikarilmali(ctx_modul, hafiza, yaz):
    yaz(hafiza, "tasks/eski-brief.md", status="archived", type_="task")
    (hafiza / "tasks" / "OPEN.md").write_text(
        "# Acik isler\n\n- [Eski](eski-brief.md)\n", encoding="utf-8")
    bulgular = ctx_modul.dogrula(hafiza)
    assert any(
        s == "hata" and y == "tasks/eski-brief.md" and
        "arsiv task" in m
        for s, y, m in bulgular
    )


# ------------------------------------------- kapanmis madde AKTIF dosyada

def _acik_task(hafiza, yaz, govde):
    """OPEN.md'de listelenen, temiz bir aktif task dosyasi kurar."""
    yaz(hafiza, "tasks/brief.md", type_="task", govde=govde)
    (hafiza / "tasks" / "OPEN.md").write_text(
        "# Acik isler\n\n- [Brief](brief.md)\n", encoding="utf-8")


def test_isaretli_onay_kutusu_hata(ctx_modul, hafiza, yaz):
    """Evrensel sinyal: `- [x]`. Kapanan is SILINIR, isaretlenmez — isaretli
    birakilan madde bir sonraki oturumda yeniden alinir."""
    _acik_task(hafiza, yaz, "- [ ] acik is\n- [x] biten is\n")
    bulgular = ctx_modul.dogrula(hafiza)
    # Satir numarasi DOSYANIN kendi numarasidir (frontmatter dahil), govdenin
    # degil: bulguyu okuyan dogrudan o satira gider.
    assert any(
        s == "hata" and y.startswith("tasks/brief.md:") and
        "kapanmis madde" in m and "biten is" in m
        for s, y, m in bulgular
    )


def test_acik_kutu_temiz(ctx_modul, hafiza, yaz):
    _acik_task(hafiza, yaz, "- [ ] acik is\n- [ ] baska acik is\n")
    assert not any("kapanmis madde" in m for _s, _y, m in ctx_modul.dogrula(hafiza))


def test_projeye_ozgu_isaret_ayardan_gelir(ctx_modul, hafiza, yaz):
    """Arac projenin DILINI varsaymaz: 'KAPANDI' koda gomulu degil, hafizanin
    kendi ctx.yaml'indan gelir."""
    _acik_task(hafiza, yaz, "- ilk madde KAPANDI (a1b2c3)\n")
    assert not any("kapanmis madde" in m for _s, _y, m in ctx_modul.dogrula(hafiza))
    (hafiza / "ctx.yaml").write_text("closed_markers: [KAPANDI]\n", encoding="utf-8")
    assert any("kapanmis madde" in m for _s, _y, m in ctx_modul.dogrula(hafiza))


def test_muafiyet_satiri_bulgu_uretmez(ctx_modul, hafiza, yaz):
    """Kapanis bicimini ANLATAN prose madde degildir; `ctx:acik` muaf tutar."""
    (hafiza / "ctx.yaml").write_text("closed_markers: [KAPANDI]\n", encoding="utf-8")
    _acik_task(hafiza, yaz, "Biten madde KAPANDI yazilmaz, SILINIR. <!-- ctx:acik -->\n")
    assert not any("kapanmis madde" in m for _s, _y, m in ctx_modul.dogrula(hafiza))


def test_arsiv_task_kapanis_kapisindan_muaf(ctx_modul, hafiza, yaz):
    """Arsiv dosyasinin isi zaten kapanmis maddeleri TASIMAKTIR."""
    yaz(hafiza, "tasks/eski.md", status="archived", type_="task",
        govde="- [x] coktan bitti\n")
    (hafiza / "tasks" / "OPEN.md").write_text("# Acik isler\n", encoding="utf-8")
    assert not any("kapanmis madde" in m for _s, _y, m in ctx_modul.dogrula(hafiza))


def test_cok_sayida_kapanis_tavanla_ozetlenir(ctx_modul, hafiza, yaz):
    """Bulgu duvari kapiyi kullanilamaz yapar; tavan uzeri tek satirda ozetlenir."""
    _acik_task(hafiza, yaz, "".join(f"- [x] is {i}\n" for i in range(8)))
    bulgular = ctx_modul.dogrula(hafiza)
    tekil = [m for _s, _y, m in bulgular if "kapanmis madde aktif" in m]
    assert len(tekil) == ctx_modul._KAPANIS_BULGU_TAVANI
    assert any("kapanmis madde daha var" in m for _s, _y, m in bulgular)


# ------------------------------------------- hafizanin kendi ayar dosyasi

def test_skip_dirs_ek_dizini_not_saymaz(ctx_modul, hafiza, yaz):
    """Projeye ozel dizin adlari KODA gomulmez; hafiza kendi ayarinda soyler."""
    yaz(hafiza, "gecis/devralinan.md", lifecycle="uydurma")  # gecerli olsa hata verirdi
    assert any("lifecycle" in m for _s, _y, m in ctx_modul.dogrula(hafiza))

    (hafiza / "ctx.yaml").write_text("skip_dirs: [gecis]\n", encoding="utf-8")
    assert ctx_modul.dogrula(hafiza) == []


def test_review_days_ayarlanabilir(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", updated="2026-07-01")   # ~43 gun once
    assert ctx_modul.dogrula(hafiza) == [], "90 gun varsayilaniyla uyarmamali"

    (hafiza / "ctx.yaml").write_text("review_days: 7\n", encoding="utf-8")
    assert any("review-due" in m for _s, _y, m in ctx_modul.dogrula(hafiza))


def test_ayar_dosyasi_yoksa_guvenli_varsayilan(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md")
    assert ctx_modul.dogrula(hafiza, tam=True) == []


# ------------------------------------ saglama toplami (yanlis alarm onleme)

def test_yil_dizisi_kredi_karti_SAYILMAZ(ctx_modul, hafiza, yaz):
    """Olculmus regresyon: salt bicime bakan desen "2024 2025 2026 2027"u kart
    saniyordu. Yanlis alarm veren kapi `--no-verify` aliskanligi yaratir."""
    yaz(hafiza, "facts/a.md", govde="Olcumler 2024 2025 2026 2027 yillarinda alindi.")
    assert ctx_modul.dogrula(hafiza, tam=True) == []


def test_anahtar_gorunumlu_dizi_IBAN_sayilmaz(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", govde="Anahtar AB12CDEF1234GHIJ5678 olarak uretildi.")
    assert ctx_modul.dogrula(hafiza, tam=True) == []


def test_gercek_kart_LUHN_ile_yakalanir(ctx_modul, hafiza, yaz):
    yaz(hafiza, "facts/a.md", govde="Kart: 4539 1488 0343 6467")
    assert any("kredi karti" in m for _s, _y, m in ctx_modul.dogrula(hafiza, tam=True))


def test_gercek_iban_mod97_ile_yakalanir(ctx_modul, hafiza, yaz):
    """Ulkeden bagimsiz: mod-97 saglamasi TR de DE de gecerli olani ayirir."""
    yaz(hafiza, "facts/a.md", govde="Konto: DE89 3704 0044 0532 0130 00")
    assert any("IBAN" in m for _s, _y, m in ctx_modul.dogrula(hafiza, tam=True))


def test_luhn_ve_mod97_dogrudan(ctx_modul):
    assert ctx_modul._luhn_gecerli("4539148803436467")
    assert not ctx_modul._luhn_gecerli("2024202520262027")
    assert ctx_modul._iban_gecerli("TR330006100519786457841326")
    assert not ctx_modul._iban_gecerli("AB12CDEF1234GHIJ5678")


def test_doctor_yapisal_hata_varken_cokmez(ctx_modul, hafiza, yaz, tmp_path, capsys):
    """REGRESYON: `ctx doctor` bulgu yazdirdiktan sonra manifest yolunu kaybediyordu.

    `for _, yol, mesaj in hatalar` dongusu, manifest'in Path'ini tutan `yol`
    degiskeninin ustune yaziyordu; asagidaki `_index_hedefleri(yol, ...)` cagrisi
    bir str alip AttributeError ile coküyordu. Temiz korpusta HIC gorunmez —
    yalnizca hafizada en az bir YAPISAL HATA varken tetiklenir.
    """
    (hafiza / "facts" / "bozuk.md").write_text("frontmatter yok\n", encoding="utf-8")
    yaz(hafiza, "facts/saglam.md")

    proje = tmp_path / "proje"
    proje.mkdir()
    (proje / "context.yaml").write_text(
        f"version: 1\nproject: t\nmemory_path: {hafiza.as_posix()}\nrole: current\n",
        encoding="utf-8")

    class _Args:
        repo = str(proje)

    kod = ctx_modul.komut_doctor(_Args())  # cokmemeli
    cikti = capsys.readouterr().out
    assert kod == 1                        # yapisal hata cikis kodunu ETKILER
    assert "bozuk.md" in cikti
    assert "index hedefi yok" in cikti     # manifest yolu hala Path: blok kontrolu kostu


def test_bayat_binary_uyarisi(ctx_modul, tmp_path, monkeypatch):
    """Sarmalayici binary'yi SESSIZCE tercih eder → bayatlik gorunmez bir arizadir.

    Kaynagi degistirip yeniden derlemeyi unutan herkes eski kodu kosturur ve
    cikti "dogru" gorunur; yalnizca YANLIS SURUMDUR. Tek gorunur yer doctor.
    """
    kok = tmp_path / ".agent-context"
    (kok / "bin").mkdir(parents=True)
    (kok / "go").mkdir()
    (kok / "templates").mkdir()
    monkeypatch.setattr(ctx_modul.Path, "home", staticmethod(lambda: tmp_path))

    # binary yok → bayatlik kavrami da yok
    (kok / "bin" / "ctx.py").write_text("x\n", encoding="utf-8")
    assert ctx_modul._binary_tazeligi() == []

    # binary kaynaktan YENI → sessiz
    binary = kok / "bin" / "ctx.exe"
    binary.write_bytes(b"binary")
    os.utime(binary, (2_000_000_000, 2_000_000_000))
    os.utime(kok / "bin" / "ctx.py", (1_000_000_000, 1_000_000_000))
    assert ctx_modul._binary_tazeligi() == []

    # kaynak binary'den yeni → UYARI
    os.utime(kok / "bin" / "ctx.py", (2_100_000_000, 2_100_000_000))
    uyari = ctx_modul._binary_tazeligi()
    assert len(uyari) == 1 and "BAYAT" in uyari[0] and "ctx.py" in uyari[0]

    # gomulu SABLON degisikligi de bayatlatir (binary sablonlari tasiyor)
    os.utime(kok / "bin" / "ctx.py", (1_000_000_000, 1_000_000_000))
    sablon = kok / "templates" / "CLAUDE.md"
    sablon.write_text("y\n", encoding="utf-8")
    os.utime(sablon, (2_100_000_000, 2_100_000_000))
    assert any("templates/CLAUDE.md" in s for s in ctx_modul._binary_tazeligi())

    # test dosyalari SAYILMAZ: binary'ye girmezler
    os.utime(sablon, (1_000_000_000, 1_000_000_000))
    test_go = kok / "go" / "bir_test.go"
    test_go.write_text("package x\n", encoding="utf-8")
    os.utime(test_go, (2_100_000_000, 2_100_000_000))
    assert ctx_modul._binary_tazeligi() == []


# ------------------------------------------- oturum dokumu (hook flush)

import json as _json
import types as _types


def _dokum(tmp_path, hafiza, satirlar):
    """Manifest + transcript kurar, flush icin hazir bir args dondurur."""
    (tmp_path / "context.yaml").write_text(
        "version: 1\nproject: t\nmemory_path: ./mem\n", encoding="utf-8")
    ts = tmp_path / "oturum.jsonl"
    ts.write_text("\n".join(_json.dumps(s) for s in satirlar), encoding="utf-8")
    return _types.SimpleNamespace(repo=str(tmp_path), transcript=str(ts),
                                  arac="claude", sebep="sessionend")


def _claude_turu(rol, metin):
    return {"type": rol, "message": {"role": rol,
            "content": metin if rol == "user" else [{"type": "text", "text": metin}]}}


def test_flush_dokumu_yazar(ctx_modul, tmp_path):
    hafiza = tmp_path / "mem"
    for k in ("facts", "tasks", "sessions"):
        (hafiza / k).mkdir(parents=True)
    args = _dokum(tmp_path, hafiza, [_claude_turu("user", "soru bir"),
                                     _claude_turu("assistant", "cevap bir")])
    assert ctx_modul._hook_flush(args) == 0
    dosyalar = list((hafiza / "sessions" / ".pending").glob("*.md"))
    assert len(dosyalar) == 1
    metin = dosyalar[0].read_text(encoding="utf-8")
    assert "**Kullanici:** soru bir" in metin and "**Ajan:** cevap bir" in metin


def test_flush_kisa_oturumda_yazmaz(ctx_modul, tmp_path):
    """Ozetlenecek kadar konusma yoksa hicbir sey yazilmaz — DOGRU davranis."""
    hafiza = tmp_path / "mem"
    (hafiza / "sessions").mkdir(parents=True)
    args = _dokum(tmp_path, hafiza, [_claude_turu("user", "tek satir")])
    assert ctx_modul._hook_flush(args) == 0
    assert not (hafiza / "sessions" / ".pending").exists()


def test_flush_ayni_dokumu_iki_kez_yazmaz(ctx_modul, tmp_path):
    """SessionEnd ve PreCompact arka arkaya tetiklenebilir."""
    hafiza = tmp_path / "mem"
    (hafiza / "sessions").mkdir(parents=True)
    args = _dokum(tmp_path, hafiza, [_claude_turu("user", "a"),
                                     _claude_turu("assistant", "b")])
    ctx_modul._hook_flush(args)
    args.sebep = "precompact"
    ctx_modul._hook_flush(args)
    assert len(list((hafiza / "sessions" / ".pending").glob("*.md"))) == 1


def test_flush_ozyinelemede_sessiz(ctx_modul, tmp_path, monkeypatch):
    """Kancanin tetikledigi surec ayni kancayi tekrar kosturmaz."""
    hafiza = tmp_path / "mem"
    (hafiza / "sessions").mkdir(parents=True)
    args = _dokum(tmp_path, hafiza, [_claude_turu("user", "a"),
                                     _claude_turu("assistant", "b")])
    monkeypatch.setenv(ctx_modul.FLUSH_OZYINELEME, "1")
    assert ctx_modul._hook_flush(args) == 0
    assert not (hafiza / "sessions" / ".pending").exists()


def test_flush_codex_bicimi_okunur(ctx_modul, tmp_path):
    """Olculdu 2026-08-28: Codex `input_text`/`output_text` yazar, `text` degil —
    yalniz `text` arayan okuyucu SIFIR tur cikarip sessizce hicbir sey yazmadi."""
    hafiza = tmp_path / "mem"
    (hafiza / "sessions").mkdir(parents=True)
    satirlar = [
        {"type": "response_item", "payload": {"type": "message", "role": "user",
         "content": [{"type": "input_text", "text": "codex sorusu"}]}},
        {"type": "response_item", "payload": {"type": "message", "role": "assistant",
         "content": [{"type": "output_text", "text": "codex cevabi"}]}},
        {"type": "response_item", "payload": {"type": "reasoning", "content": "atlanir"}},
    ]
    args = _dokum(tmp_path, hafiza, satirlar)
    args.arac = "codex"
    ctx_modul._hook_flush(args)
    metin = next((hafiza / "sessions" / ".pending").glob("*.md")).read_text(encoding="utf-8")
    assert "codex sorusu" in metin and "codex cevabi" in metin and "atlanir" not in metin


def test_flush_tool_bloklari_dokume_girmez(ctx_modul, tmp_path):
    """Tool cagrisi/dusunme konusma degil, yurutmenin izidir; tavani yer."""
    hafiza = tmp_path / "mem"
    (hafiza / "sessions").mkdir(parents=True)
    satirlar = [
        _claude_turu("user", "soru"),
        {"type": "assistant", "message": {"role": "assistant", "content": [
            {"type": "thinking", "thinking": "GIZLI"},
            {"type": "tool_use", "name": "Bash", "input": {"command": "ls"}},
            {"type": "text", "text": "gorunur cevap"}]}},
    ]
    args = _dokum(tmp_path, hafiza, satirlar)
    metin = (ctx_modul._hook_flush(args),
             next((hafiza / "sessions" / ".pending").glob("*.md")).read_text(encoding="utf-8"))[1]
    assert "gorunur cevap" in metin
    assert "GIZLI" not in metin and "tool_use" not in metin
