# Agent Context Standard — SPEC V0

Bu belge, bir ürünün **harness-bağımsız kalıcı bilgi deposunun** nasıl kurulacağını
tarif eder. Depo ayrı bir Git reposudur; varsayılan yeri projenin içindeki
`.gitignore`'lu `.memory/` klasörüdür. Ürünün birden çok reposu (`<repo>`) ortak
hafıza kullanacaksa depo onların yanında `<product>-memory` olarak durur ve her
repo `context.yaml → memory_path` ile ona bakar. Bilgi Markdown + Git'te yaşar; Claude
auto-memory gibi harness hafızaları canonical DEĞİLDİR,
yalnız önbellek/adapter'dır.

🔴 **TALİMATSIZ COMMIT VE PUSH YASAK.** Kullanıcının açık talimatı olmadan hiçbir
repoda (hafıza reposu dahil) commit atılmaz — dosyalar hazırlanır, `git status`/`git diff`
gösterilir, karar kullanıcınındır. Remote push ayrıca PII taraması kapanmadan
gündeme bile gelmez.

## Geçerlilik: iki bağımsız eksen

| alan | değerler | anlamı |
|---|---|---|
| `status` | `active` · `archived` | Not bugünü tarif ediyor mu (varsayılan `active`) |
| `scope` | serbest etiket listesi | Hangi bağlamda geçerli. **Boşsa evrensel** |

`scope` etiketleri **projenin kendi sözlüğüdür**, araca gömülü değildir: tek çağlı
bir proje hiç `scope` yazmaz ve her not evrensel olur; çok çağlı bir proje
`[v1]`/`[v2]` ya da `[server]`/`[mobile]` gibi kendi etiketlerini kullanır.

Bir repo kendi kapsamını `context.yaml → scope` ile bildirir. Getirme kuralı iki
cümle: **arşivlenmiş not istenmedikçe gelmez; `scope`'u olmayan not her kapsamda
geçerlidir, `scope`'u olan yalnız kesişme varsa gelir.** Repo kapsamı tanımsızsa
filtre uygulanmaz. **Bütün korpus asla topluca okunmaz.**

### Eski `lifecycle` alanı

Önceki sürüm tek eksenliydi (`product|current|legacy|historical`) ve
`current`/`legacy` ikiliği bu aracı doğuran projenin kendi hikâyesinden
geliyordu. Eski notlar okunmaya devam eder; karşılıkları:

| eski | yeni |
|---|---|
| `product` | `status: active`, scope yok (evrensel) |
| `current` | `status: active`, `scope: [current]` |
| `legacy` | `status: active`, `scope: [legacy]` |
| `historical` | `status: archived` |

Dönüştürmek için: `ctx migrate` (`--dry-run` ile önce göster).

## Not formatı (frontmatter standardı)

```yaml
---
title: Kısa başlık
description: Tek cümlelik özet (arama bununla çalışır)
status: active | archived         # varsayılan active
scope: [v2, server]               # opsiyonel; boş = evrensel
type: domain | architecture | decision | pitfall | reference
tags: [etiket1, etiket2]
components: [server, client]      # opsiyonel
source: <geldiği eski kaynak>     # varsa
updated: YYYY-MM-DD
superseded_by: null               # yerine geçen not varsa göreli yolu
---
```

## Not içi düzen: Geçerli ve Tarihçe

Bir not **bugün** geçerli olanı, oraya **nasıl gelindiğinden** ayrı tutar:

```markdown
## Geçerli

(yalnız bugün geçerli olan; değişince BAŞTAN yazılır, üstüne eklenmez)

## Tarihçe

- 2026-10-13 · aynı seviye talebi ENGEL → serbest · ürün sahibi
```

- **Karar ya da gerçek değişince:** "Geçerli" yeniden yazılır, eski hâl
  Tarihçe'ye tek satır olarak iner (tarih · eski → yeni · kimin kararı).
  Yeni kararı eskisinin yanına parantez ya da yeni bölüm olarak eklemek yasaktır.
- **Arama ve dosya hatırlatması Tarihçe'yi saymaz.** Eski kararı anlatan bir
  paragraf güncel bilgi gibi bulunmaz. "Bu neden değişti?" sorusu için
  `ctx search --history` kullanılır.
- **Başlık katlanarak tanınır:** `## Geçerli`/`## Current` ve
  `## Tarihçe`/`## History`. Yalnız ikinci seviye başlık bölüm sayılır.
- **Zorunlu değildir.** Tek paragraflık bir not bölümsüz kalabilir. Değişmiş
  ve büyümüş notlar ise `ctx audit` raporunda listelenir.

Ölçüldü (yapit, 2026-10-13, 204 not): 111 notta tarih ya da durum sözcüğü
taşıyan 315 başlık vardı ("UYGULANDI 2026-09-08"). `superseded_by` alanı hiç
kullanılmamıştı; kararlar eski notun üstüne yeni bölüm eklenerek değişiyordu.
Bir örnekte geri alınan karar, eski kararı söyleyen satırın ortasında parantez
olarak duruyordu.

**`ctx audit`** salt okunur bir rapordur. Hangi notun bu düzene geçmeye en çok
ihtiyacı olduğunu sıralar. Sinyaller Tarihçe dışında sayılır:

- tarihli ya da durum bildiren başlık,
- geri alma ifadesi ("kaldırıldı", "artık", "no longer"; genel "geçersiz" bilerek yok — "neyle geçersiz olur" tablosu yanlış alarm veriyordu),
- boyut,
- karar notunda "Geçerli" bölümünün olmaması.

Sözcük listeleri hafızanın `ctx.yaml`'ında `status_markers` ve
`override_markers` ile genişletilir. Rapor not yazmaz. Bir karar notunda hangi
ifadenin geçerli olduğunu ürün sahibi söyler, bir fact notunda kod söyler. Araç
yalnız adayları sıralar.

## Klasörler

Bir notun tek anlamlı sorusu **"bu ne tür bir iddia ve onu ne geçersiz kılar?"**
Klasör şeması bu soruya göre kurulur ve üç kutudan ibarettir:

| Klasör | İddia | Neyle geçersiz olur |
|---|---|---|
| `facts/` | sistem/ürün **nasıl** çalışıyor | gerçeklik değişince |
| `decisions/` | **neden** öyle | yeni karar eskisini geçersiz kılınca |
| `pitfalls/` | ne **ısırdı**, kural ne | tuzak kökten kapanınca |

Hafıza olmayan, bilerek ayrı duran iki durum dosyası:

- `tasks/OPEN.md` — TÜM açık işlerin tek adresi (durable memory DEĞİL)
- `sessions/CURRENT.md` — son 1-2 çalışma turu (durable memory DEĞİL)

**Arşiv ve kapsam KLASÖR DEĞİLDİR, frontmatter'dır.** `status: archived` notu
aramadan ve haritadan zaten çıkarır; `scope: [legacy]` kapsam filtresini zaten
uygular. Ayrıca bir `historical/` ya da `legacy/` klasörü tutmak aynı bilgiyi
ikinci kez kodlar ve iki kayıt er geç ayrışır — tek mekanizma frontmatter'dır.

Klasör adları **iskeledir, İngilizce kalır**; içerik projenin dilindedir.
`domain/` bilerek kullanılmaz: DDD'de "iş alanı" demektir ve bir altyapı
reposunda karşılığı yoktur; ayrıca aynı kelimeyi "ürün gerçeği" anlamında
kullanan proje sözleşmeleriyle çakışır.

Notun türü **klasörden okunur**, `type:` alanından değil. `type` zorunlu
değildir; yalnız gerçek bir ALT-TÜR taşıyorsa yazılır (`type: runbook`).

Task lifecycle sözleşmesi: `tasks/OPEN.md` hariç `tasks/` altındaki her Markdown
dosyası açık bir iş olarak kabul edilir ve OPEN indeksinde yer alır; yalnızca
`status: archived` veya eski `lifecycle: historical` taşıyan dosya indeksten
çıkarılabilir. Bağımsız iş kapanınca dosya ve OPEN satırı silinir. Karma task
dosyasında yalnız kapanan madde silinir; dosya başka açık maddeler kaldığı için
yaşar. `sessions/CURRENT.md` indeks değil, son turun kanıt ve devam kaydıdır.

**Kapanan madde SİLİNİR, işaretlenmez.** İşaretlenip bırakılan madde iki maliyet
üretir: bir sonraki oturum onu okumak için token harcar ve iş listesinden iş
alırken aynı işi yeniden alabilir. Neyin kapandığı git geçmişindedir; task
dosyası yalnız AÇIK işi taşır. Kapı bunu ölçer: aktif (arşiv olmayan) bir task
dosyasında işaretli onay kutusu (`- [x]`) bulunması **hatadır**. Projeye özgü
kapanış sözcükleri araca gömülü değildir, hafızanın kendi `ctx.yaml`'ındaki
`closed_markers` listesinden gelir; kapanış biçimini ANLATAN satır `ctx:acik`
ile muaf tutulur.

Aynı tarama `tasks/OPEN.md`'ye de uygulanır. İndeks satırının tipik çürümesi
şudur: her oturum durum hücresini "şu an açık olan" diye yeniden yazmak yerine
sonuna "X kapandı (hash)" ekler ve hücre bir değişiklik günlüğüne döner.
Hafıza isterse `ctx.yaml`'da `open_row_max_chars` ile indeks hücresine uzunluk
sınırı koyar (0/yok = kapalı): yalnız açık kalanı söyleyen hücre kendiliğinden
kısa kalır.

## Yazma politikası

- Canonical nota yalnız **lider ajan** yazar. Alt ajan/ucuz model bulguyu raporlar,
  lider kalıcılaştırır.
- Kalıcılaştırma testi: *"Bu bilgi üç ay sonra başka bir oturumda yeniden keşif
  maliyetini anlamlı azaltır mı?"* Hayırsa buraya girmez.
- Task/session durumu ("push bekliyor", "test kırık") nota girmez → `tasks/OPEN.md`.
- Secret/PII asla girmez. Doğru biçim: "Deployment ADMIN_SECRET ister; değer bu
  reponun dışında yaşar."
- Kapanış kapısı `ctx validate`'dır: OPEN içindeki yerel linkler mevcut olmalı;
  aktif task dosyası indekssiz, arşiv taskı indeksli bırakılamaz.

## Talimat önceliği — kime uyarım

1. Kullanıcının güncel açık talimatı
2. Daha dar kapsamlı repo talimatı (alt klasördeki AGENTS.md)
3. Repo kökündeki AGENTS.md
4. Global SPEC varsayılanları

Global sert güvenlik/iş akışı kuralları (commit onayı, PII, doğrulama) proje tarafından
GEVŞETİLEMEZ; yalnız daraltılabilir.

## Doğruluk önceliği — neye inanırım

1. Kullanıcının verdiği güncel olgu/gereksinim
2. Canlı kod + git + runtime kanıtı
3. Normatif spec / ADR / CONVENTIONS.md
4. Kanonik hafıza (bu depo)
5. Historical / legacy notlar
6. Harness'ın kendi oturum hafızası

Implementation iddiaları kaynakta doğrulanır. "Talimat" ile "gerçeklik kaynağı" aynı
hiyerarşi değildir.

## Sert kurallar

<!-- ctx:hard-rules:start -->
- **Oturuma git durumuyla başla.** `git status` + son commit'ler; canlı durum hafızadan çıkarılmaz.
- **Onaysız commit/push yok.** Değişiklik uygulanır, test koşulur, diff gösterilir; commit
  kararı kullanıcınındır. "Acil" veya "bu zaten devamı" onay değildir.
- **Sorunu çözen en az değişiklik.** İlgisiz refactor yok; ilgisiz ölü kod **söylenir, silinmez**.
- **Kod hafızadan otoriterdir.** Dosya/fonksiyon/bayrak adı geçen her iddia kaynakta doğrulanır.
- **PII ve secret commit edilmez.** İsim, kimlik/vergi numarası, adres, telefon; notlarda da gerçek isim
  değil "örnek mal sahibi" yazılır. Secret'ın varlığı kaydedilir, değeri kaydedilmez.
- **Alt ajan raporu kanıt değildir**, kaynakta doğrulanacak girdidir.
<!-- ctx:hard-rules:end -->

## Retrieval

Göreve başlamadan **görev kavramları ve dosya yolları** ile ara (`ctx search`, yoksa
`rg`); isabet eden 1-3 notu oku. Bütün korpusu asla yükleme — filtre reponun
`scope`'undan gelir. Alt ajanlara ilgili notlar
**prompt'a gömülerek** verilir; ajanın gönüllü aramasına güvenilmez.

### Her-zaman-yüklü harita (index)

Aramanın **tetikleyicisi yoktur**: 2026-08-12'de ölçüldü, 21 notluk korpus koca bir
oturum boyunca hiç açılmadı çünkü ajan `ctx search` çalıştırmayı düşünmedi. Bu yüzden
korpusun **haritası** (not başına tek satır: yol + `description`) her oturuma
yüklenir. Harita notun kendisi değildir; ajan haritada isabet görünce **o notu açar**,
"bütün korpusu yükleme" yasağı bozulmaz.

Üretici tektir (`ctx index`), tüketici çoktur:

| Araç | Nasıl okur | Tazelik |
|---|---|---|
| Claude Code | `SessionStart` hook `ctx index` çıktısını bağlama basar | her oturum, canlı |
| Claude Code / Hermes | `SessionEnd` + `PreCompact` hook `ctx hook flush` ile ham oturum dökümünü diske düşürür | her kapanış |
| Claude Code | `PostToolUse` hook `ctx hook recall` dokunulan dosyayı anan notları ekler | her dosya çağrısı |
| Codex / Cursor / Copilot | `AGENTS.md` içindeki `ctx:index` bloğu | `ctx index --write` |
| harness'sız / CI | `ctx index` çıktısını okur | anında |

**Harness'a bağımlı olmayan yol daima dosyadır**; hook yalnızca kolaylıktır. Blok
`<!-- ctx:index:basla -->` / `<!-- ctx:index:bitir -->` işaretleri arasında yaşar,
hedefleri `context.yaml → index_targets` belirler (varsayılan `AGENTS.md`; Cursor
kuralı veya `copilot-instructions.md` de hedef olabilir — işaretleri taşıyan her
dosya tüketicidir).

**Yalnız haritayı taşıyan dosya üretilmiş çıktıdır, git'e girmez.** `index_targets`'ta
adı açıkça verilen hedef (ör. `MEMORY-INDEX.md`) yoksa `ctx index --write` ve
`ctx close` onu işaretleriyle birlikte **sıfırdan üretir**; `--check` ve `ctx doctor`
yalnız söyler. Böylece dosya tüketici repoda `.gitignore`'lanabilir ve taze klonda ilk
tazelemeyle geri gelir. Ölçüldü (yapit, 2026-10-13): haritayı commit eden üç repoda
326 commit yalnız harita tazelemesiydi — her yeni not, tüketici başına bir commit.
Varsayılan `AGENTS.md` sıfırdan **üretilmez**: router bloğunu `ctx bootstrap` yazar,
yalnız harita taşıyan bir `AGENTS.md` kurulu repo gibi görünür ama router'sızdır.

**Bileşen süzgeci (isteğe bağlı).** `context.yaml → index_components: true` haritayı
reponun `components` listesiyle daraltır; kural kapsamla aynıdır (`components`'ı olmayan
not evrenseldir, kesişme varsa gelir). Yalnız **harita** daralır, arama daralmaz:
bileşen notun nerede doğru olduğunu değil nerede en çok işe yaradığını söyler — istemci
reposundaki ajan sunucunun sözleşmesini yine arayıp bulabilmelidir. Daralma sessiz
değildir; haritada "Başka bileşenlerin N notu bu haritada yok" satırı yazar. Varsayılan
kapalıdır, çünkü `ctx init` yer tutucu bir bileşen (`app`) yazar ve mevcut notlar onu
taşımaz.

Bayatlık yapısal olarak kapatılır: `ctx close --note <yol>` yeni notu kaydettiği anda
hedef blokları **tazeler**; `ctx index --check` bayatsa 1 döner; `ctx doctor` bunu
uyarı olarak gösterir. Yani "hook'u olan araç yeni notu görür, dosyadan okuyan araç
görmez" ayrışması oluşamaz.

Tazeleme **bütün tüketicileri** kapsar, yalnız bulunulan repoyu değil: bir hafızayı
birden çok repo okur ve yeni not hepsinin bloğunu aynı anda bayatlatır. Liste
hafızanın kendi `ctx.yaml`'ındadır (`consumers:`) — "beni kimler okuyor" hafızanın
özelliğidir. Yollar hafızaya GÖRELİ tutulur (`..`, `../../sunucu`); çatıya göreli
yazılan girdi ölü doğar — okuma onu sessizce atlar, `ctx doctor` uyarır.
Her tüketici KENDİ manifest'iyle hesaplanır; kapsam filtresi repodan
repoya değişir. (2026-08-13'e kadar söz repo başına geçerliydi ve ölçüldü ki iki repo
dokuz gün geride kalmıştı.)

Kayıt kurulumun işidir: `ctx bootstrap` bulunulan repoyu listeye kendisi ekler
(satır bazlı, yıkıcı değil). Listede OLMAYAN ama hafızayı okuyan repo `ctx doctor`
uyarısı olur; keşif derinliği 1'dir (hafızanın üst dizini + reponun üst dizini +
hafızanın üst-üst dizini) — daha derindeki yerleşim ne uyarılır ne yazılır.

## Oturum kapanışı — disiplin değil mekanizma

Kapanışta bir şey yazmayı **modelin hatırlaması** güvenilmez bir sözleşmedir:
kapanış tam da modelin dikkatinin ve bütçesinin bittiği andır. Bu yüzden kapanış
kancası (`ctx hook flush`) transcript'i okur ve ham dökümü `sessions/.pending/`
altına bırakır; bir sonraki oturumun `capture-check`'i onu yüzeye çıkarır ve notu
**yine lider ajan** yazar.

**Model ÇAĞRILMAZ.** Kapanış kancası her araçta saniyeler içinde dönmek zorundadır;
arka planda bir özetleyici çağırmak (a) kota yakar, (b) aynı deponun kancalarını
tetikleyip özyinelemeye girer, (c) başka bir sağlayıcıda karşılığı yoktur. Ham
döküm bedava ve deterministiktir; özetlemeyi bir sonraki oturum zaten açık olan
bağlamıyla yapar. Özyineleme kapısı: `CTX_HOOK_ICINDE`.

Okuyucular saplanmaz, araç başına ayrışır: Claude `text`, Codex ve Hermes
`input_text`/`output_text` yazar (ölçüldü 2026-08-28; yalnız `text` arayan okuyucu
Codex dökümünden SIFIR tur çıkarıp sessizce hiçbir şey yazmadı). Hermes dosya yolu
değil `session_id` verir; oturum SQLite'ta yaşar ve aracın kendi
`sessions export --redact` arayüzüyle çıkarılır — şema ayrıştırılmaz.

**PII:** transcript gerçek ad/adres taşıyabilir. `sessions/.pending/` gitignore'ludur,
makine-yereldir ve hafızaya asla commit edilmez. Kanca her koşuda 7 günden eski dökümleri siler (`<memory>/ctx.yaml → pending_keep_days`, 0 = silme): döküm kararsız kapanmış oturum için bir emniyet ağıdır, ama kanca kararlı kapanan oturum için de yazar. Ölçüldü (yapit, 2026-10-14): bir ayda 94 döküm, 1,2 MB birikti; taramada 77'sinin bulgusu zaten nottaydı, birinde düz metin parola vardı.

## Bağlam bütçesi

Kancayla ENJEKTE edilen haritanın karakter tavanı vardır (varsayılan 16.000);
dosyaya yazılan blok tavansızdır, çünkü dosya isteğe bağlı okunur ama enjeksiyon
her oturumun bağlamını yer. Tavan tüketici `context.yaml`'daki `index_budget`
ile değişir (`ctx preferences --profile ekonomik` 6.000 yazar, `--index-budget N`
açık değer verir); `auto_index: false` enjeksiyonu kapatır (`ctx preferences
--profile manuel`), `ctx doctor` bunu uyarı olarak gösterir.

Tavan aşılınca **not düşürülmez** — açıklama kısaltılır (95 → 80 → 55), sonra
harita **yalnız dosya adlarına** iner; kısaltma çıktıda açıkça yazılır. Not
düşürmek sessiz kapsam kaybıdır: harita "hepsi bu" diye okunur ve görünmeyen not
hiç aranmaz. Dosya adları bile sığmıyorsa bu da yüksek sesle söylenir.
`index_style: adlar` bütçeden bağımsız olarak doğrudan ad kademesini seçer.

35 karakterlik kademe 2026-10-13'te kaldırıldı. Ölçüldü (yapit, 204 not): o
uzunlukta açıklamadan geriye bilgi kalmıyordu (`fatura-hakedis.md` — F). Buna
karşılık açıklayıcı dosya adları 204 not için yaklaşık 2.800 karakter tutuyordu;
kırpılmış liste ise 16.000 karakterdi.

## Dosyaya dokununca hatırlatma

Harita "ne var" sorusunu cevaplar; ama ajan çoğu zaman notu ancak işine yarayacağı
anda arar. Claude Code'da `PostToolUse` kancası (`ctx hook recall`, eşleyici
`Read|Edit|MultiEdit|Write|NotebookEdit`) ajanın okuduğu ya da değiştirdiği dosyayı
**adıyla anan notları** bağlama ekler (en çok 2). Aynı not aynı oturumda ikinci
kez eklenmez; `auto_recall: false` kancayı kapatır.

Tetik mesaj değil dosyadır. İkisi de ölçüldü (yapit, 2026-10-13):

- **Mesaja göre arama:** kullanıcının 268 gerçek mesajında sözcük eşleştirmesi
  76 mesaja not ekledi ve eklenen notların çoğu ilgisizdi. Mesajlar sohbet diliyle
  yazılıyor ("bunu da commitle") ve konuyu nadiren adıyla söylüyor.
- **Dosyaya göre arama:** dosya yolu bulanık değildir. 204 notun 174'ü en az bir
  kod dosyası anıyor ve anılan 593 dosyanın 416'sını tek bir not anıyor. Ajanların
  1.976 gerçek dosya çağrısının 572'si bir not getirdi.

Gürültü kuralları:

- Dört ya da daha fazla notta geçen bir ad hiçbir yeri göstermez.
- Genel adlar (`conftest.py`, `main.ts` …) yalnız yolla eşleşir.
- 50'den fazla dosya anan katalog notu yalnız yolla bulunur.

Bu aynı zamanda bayatlığa karşı en ucuz önlemdir. Bir dosyayı anlatan not tam o
dosya okunurken ya da değiştirilirken ajanın önüne gelir: bayat iddianın zarar
verebileceği an da, düzeltilebileceği an da budur.

## Kurulum

Yeni bir projede sistem **tek komutla** kurulur (proje kökünde):

```
ctx bootstrap --project <ad> [--role <kapsam-etiketi>] [--components "api,web"]
ctx bootstrap --project <ad> --dry-run     # hiçbir şey yazmadan ne yapacağını göster
```

Dört adım: (1) hafıza iskeleti + `context.yaml` + AGENTS router + CLAUDE.md +
`.agents/skills/` ve `.claude/skills/` (`ctx-search` · `ctx-write` · `ctx-close` —
modelin çağırdığı fiiller, harness'lar dosyadan keşfeder; Claude Code ikincisini
yükler). Şablondan farklı mevcut skill dosyası raporlanır, yalnız
`--update-skills` ile ezilir; eski adlı dizinler (`ctx-ara`/`ctx-yaz`/`ctx-kapat`)
yeni adına taşınır,
(2) `SessionStart` kancaları `settings.json`'a kaydedilir — kancalar **aracın içindedir**
(`ctx hook index` · `ctx hook capture-check`), ayrı script dosyası kopyalanmaz,
(3) hafıza reposunda pre-commit kalite kapısı (`core.hooksPath .githooks`),
(4) harita bloğu yazılır + `ctx doctor`.

Bayrak adları İngilizcedir (`--write`, `--dry-run`, `--check`, `--full`, `--note`,
`--none`, `--history`, `--project`, `--tool`, `--reason`). Eski Türkçe adlar
(`--yaz`, `--kuru`, `--kontrol`, `--tam`, `--not`, `--yok`, `--tarihce`, `--proje`,
`--arac`, `--sebep`) aynı değişkene bağlı gizli alias olarak kabul edilir; kullanıcıların
`settings.json`'ındaki eski kanca kayıtları bozulmaz, `bootstrap` yeniden koşunca
yerinde İngilizce bayraklıya çevrilir.

İki sert söz — ikisi de teste bağlıdır (`tests/test_bootstrap.py`):

- **İdempotent:** ikinci koşum hiçbir şeyi değiştirmez.
- **Yıkıcı değil:** `settings.json`'daki kullanıcı kancaları ve ilgisiz ayarlar
  korunur; dosya okunamıyorsa **dokunulmaz**, elle ekleme talimatı verilir. Projenin
  kendi `core.hooksPath` ayarı varsa ezilmez.

Elle kalan tek adım bilinçlidir: **hafıza reposunun uzak deposu**. Push, PII taraması
kapanmadan gündeme gelmez (bkz. üstteki sert kural) — bu yüzden otomatikleştirilmez.

## Hafızanın kendi ayarları — `<memory>/ctx.yaml`

Standart hiçbir projeye özel varsayım taşımaz. Projeye özgü olan her şey, hafıza
deposunun kökündeki **opsiyonel** `ctx.yaml` dosyasında yaşar:

```yaml
skip_dirs:     [gecis]                  # not sayılmayan ek dizinler
archive_index: gecis/legacy-index.md    # devralınan, KANONİK OLMAYAN indeks
pii_profiles:  [tr]                     # ülkeye özel PII desenleri (tr · us · de)
review_days:   90                       # `current` notun gözden geçirme aralığı
closed_markers: [KAPANDI]               # projeye özgü "iş bitti" sözcükleri
open_row_max_chars: 300                 # OPEN.md hücre sınırı (0 = kapalı)
consumers:     [.., ../sunucu]          # hafızayı okuyan repolar (HAFIZAYA göreli)
```

Dosya yoksa güvenli varsayılanlar geçerlidir: yalnız `tasks/` + `sessions/` atlanır,
yalnız **evrensel** PII desenleri (e-posta, IBAN, kart numarası) taranır, 90 gün.

Neden tüketici repodaki `context.yaml`'da değil: bunlar **hafızanın** özellikleridir
(hangi klasör not değil, hangi ülkenin verisi tutuluyor), tüketici reponun değil.
Ayrıca pre-commit kancası hafıza deposunun içinden, manifest **olmadan** koşar.

**Ülkeye özel PII bir profil arkasındadır**, çünkü desen her yerde doğru değildir:
11 haneli sayı Türkiye'de kimlik numarasıdır, başka bir projede sipariş numarası —
profilsiz koşarsa yanlış alarm üretir, hiç koşmazsa gerçek PII kaçar.

**Arama normalizasyonu dile bağlı değildir:** Unicode NFKD + combining işaretlerin
düşürülmesi + `casefold()`. Böylece `şablon`/`sablon`, `crèche`/`creche`,
`Łódź`/`lodz` aynı anahtara düşer. `.lower()` tek başına **kullanılmaz** —
`"İ".lower()` iki karakter (`i` + U+0307) üretir ve arama sessizce eşleşmez.

## Davranış sözleşmesi

Standardın davranışsal kısmı `tests/vectors/*.json` altında **dilden bağımsız**
altın vektörlerle tanımlıdır: arama normalizasyonu, alan ağırlıklı skorlama
(mutlak puanlar), görünürlük kuralı, PII sağlama toplamları, frontmatter
ayrıştırma. Bir implementasyon bu vektörleri geçiyorsa uyumludur.

## Adapter'lar

Her repo kökünde iki dosya durur:

- `context.yaml` — hafızanın yeri (`memory_path`), bu reponun `scope`'u ve
  `components`'ı, `write_policy`, task/session dosyalarının yolu. (Eski `role`
  alanı hâlâ okunur ve tek elemanlı kapsam olarak yorumlanır.)
- `AGENTS.md` — içinde bir **router bloğu**: canonical hafızanın nerede olduğu, bu
  repo için hangi filtrenin geçerli olduğu, korpusun topluca yüklenmeyeceği.

`CLAUDE.md` yalnız `@AGENTS.md` satırını taşır (harness adapter'ı, içerik tutmaz).
Harness-native hafızalar canonical DEĞİLDİR; eski depolar
READ-ONLY arşive dönüştürülür ve yeni durable kayıt almaz.
