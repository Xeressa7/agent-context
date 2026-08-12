# Agent Context Standard — SPEC V0

Bu belge, bir ürünün **harness-bağımsız kalıcı bilgi deposunun** nasıl kurulacağını
tarif eder. Depo `<product>-memory` adlı bir Git reposudur; ürünün bütün repoları
(`<repo>`) bu hafızayı ortak kullanır. Bilgi Markdown + Git'te yaşar; Claude
auto-memory gibi harness hafızaları canonical DEĞİLDİR,
yalnız önbellek/adapter'dır.

🔴 **TALİMATSIZ COMMIT VE PUSH YASAK.** Kullanıcının açık talimatı olmadan hiçbir
repoda (hafıza reposu dahil) commit atılmaz — dosyalar hazırlanır, `git status`/`git diff`
gösterilir, karar kullanıcınındır. Remote push ayrıca PII taraması kapanmadan
gündeme bile gelmez.

## Lifecycle sınıfları

| lifecycle | anlamı | kim okur |
|---|---|---|
| `product` | teknoloji-bağımsız ürün/domain gerçeği (dış sistem davranışı, iş kuralı, mevzuat) | herkes |
| `current` | bugünkü mimariye ait karar/bilgi | güncel repolar |
| `legacy` | yalnız eski/bakım modundaki repo için geçerli implementasyon bilgisi | eski ağaç bakımı |
| `historical` | bugünü tarif etmeyen ama karar/hata geçmişi için değerli kayıt | ihtiyaç hâlinde |

Varsayılan retrieval filtresi: güncel repolarda `product + current`,
eski ağaçta `product + legacy`. **Bütün korpus asla topluca okunmaz.**

## Not formatı (frontmatter standardı)

```yaml
---
title: Kısa başlık
description: Tek cümlelik özet (arama bununla çalışır)
lifecycle: product | current | legacy | historical
type: domain | architecture | decision | pitfall | reference
tags: [etiket1, etiket2]
components: [server, client]      # opsiyonel
source: <geldiği eski kaynak>     # varsa
updated: YYYY-MM-DD
superseded_by: null               # yerine geçen not varsa göreli yolu
---
```

## Klasörler

- `domain/` — ürün/domain gerçekleri (product)
- `architecture/` — güncel mimari sınırlar (current)
- `decisions/` — verilmiş kararlar ve gerekçeleri
- `pitfalls/` — pahalıya mal olmuş dersler
- `legacy/<repo>/` — yalnız eski ağaç için geçerli bilgi
- `historical/` — geçmiş deney/postmortem
- `tasks/OPEN.md` — TÜM açık işlerin tek adresi (durable memory DEĞİL)
- `sessions/CURRENT.md` — son 1-2 çalışma turu (durable memory DEĞİL)

## Yazma politikası

- Canonical nota yalnız **lider ajan** yazar. Alt ajan/ucuz model bulguyu raporlar,
  lider kalıcılaştırır.
- Kalıcılaştırma testi: *"Bu bilgi üç ay sonra başka bir oturumda yeniden keşif
  maliyetini anlamlı azaltır mı?"* Hayırsa buraya girmez.
- Task/session durumu ("push bekliyor", "test kırık") nota girmez → `tasks/OPEN.md`.
- Secret/PII asla girmez. Doğru biçim: "Deployment ADMIN_SECRET ister; değer bu
  reponun dışında yaşar."

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
`rg`); isabet eden 1-3 notu oku. Bütün korpusu asla yükleme — filtre `role`'den gelir
(`current` → product+current, `legacy` → product+legacy). Alt ajanlara ilgili notlar
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
| Codex / Cursor / Copilot | `AGENTS.md` içindeki `ctx:index` bloğu | `ctx index --yaz` |
| harness'sız / CI | `ctx index` çıktısını okur | anında |

**Harness'a bağımlı olmayan yol daima dosyadır**; hook yalnızca kolaylıktır. Blok
`<!-- ctx:index:basla -->` / `<!-- ctx:index:bitir -->` işaretleri arasında yaşar,
hedefleri `context.yaml → index_targets` belirler (varsayılan `AGENTS.md`; Cursor
kuralı veya `copilot-instructions.md` de hedef olabilir — işaretleri taşıyan her
dosya tüketicidir).

Bayatlık yapısal olarak kapatılır: `ctx close --not <yol>` yeni notu kaydettiği anda
hedef blokları **tazeler**; `ctx index --kontrol` bayatsa 1 döner; `ctx doctor` bunu
uyarı olarak gösterir. Yani "hook'u olan araç yeni notu görür, dosyadan okuyan araç
görmez" ayrışması oluşamaz.

## Kurulum

Yeni bir projede sistem **tek komutla** kurulur (proje kökünde):

```
ctx bootstrap --proje <ad> [--role current|legacy] [--components "api,web"]
ctx bootstrap --proje <ad> --kuru     # hiçbir şey yazmadan ne yapacağını göster
```

Dört adım: (1) hafıza iskeleti + `context.yaml` + AGENTS router + CLAUDE.md,
(2) `SessionStart` kancaları — dosyaya kopyalanır **ve** `settings.json`'a kaydedilir,
(3) hafıza reposunda pre-commit kalite kapısı (`core.hooksPath .githooks`),
(4) harita bloğu yazılır + `ctx doctor`.

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

## Adapter'lar

Her repo kökünde iki dosya durur:

- `context.yaml` — hafızanın yeri (`memory_path`), bu reponun `role`'ü ve
  `components`'ı, retrieval filtreleri, `write_policy`, task/session dosyalarının yolu.
- `AGENTS.md` — içinde bir **router bloğu**: canonical hafızanın nerede olduğu, bu
  repo için hangi filtrenin geçerli olduğu, korpusun topluca yüklenmeyeceği.

`CLAUDE.md` yalnız `@AGENTS.md` satırını taşır (harness adapter'ı, içerik tutmaz).
Harness-native hafızalar canonical DEĞİLDİR; eski depolar
READ-ONLY arşive dönüştürülür ve yeni durable kayıt almaz.
