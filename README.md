# agent-context

Bir ürünün **harness-bağımsız kalıcı bilgi deposu** ve onu yöneten `ctx` aracı.

Bilgi Markdown + Git'te yaşar. Claude auto-memory, editör gömülü hafızası ve
benzerleri **kanonik değildir** — yalnız önbellek/adapter'dır. Amaç: harness
değişince bilginin ölmemesi.

Standart: [`SPEC.md`](SPEC.md) · Sürüm: `VERSION` · Bağımlılık: **yok** (Python ≥ 3.10 stdlib)

## Kurulum

```sh
git clone <bu-repo> ~/.agent-context
# PATH'e: ~/.agent-context/bin
```

Yeni bir projede, proje kökünde:

```sh
ctx bootstrap --proje myapp --components "api,web"
ctx bootstrap --proje myapp --kuru      # önce ne yapacağını göster
```

Dört adım: hafıza iskeleti + adapter dosyaları → `SessionStart` kancaları
(`settings.json`'a kaydedilir) → hafıza deposunda pre-commit kalite kapısı →
harita bloğu + sağlık kontrolü.

**İdempotent ve yıkıcı değildir**: ikinci koşum hiçbir şeyi değiştirmez;
mevcut kancaların, ilgisiz ayarların ve projenin kendi `core.hooksPath`'inin
üstüne yazmaz. İkisi de teste bağlı (`tests/test_bootstrap.py`).

## Komutlar

| Komut | Ne yapar |
|---|---|
| `ctx search "<sorgu>"` | Kapsama göre filtreli arama; alan ağırlıklı, aksan-duyarsız |
| `ctx index [--yaz]` | Her-zaman-yüklü not haritası (hook + `AGENTS.md` bloğu) |
| `ctx validate [--tam]` | Not kalitesi; `--tam` ile PII/secret taraması |
| `ctx close --not <yol>` / `--yok "<gerekçe>"` | Oturumu capture kararıyla kapatır |
| `ctx migrate` | Eski `lifecycle` alanını `status` + `scope`'a çevirir |
| `ctx doctor` | Kurulum sağlığı |
| `ctx init` / `ctx bootstrap` | Yeni proje kurulumu |

## Neden bu tasarım

**Aramanın tetikleyicisi yoktur.** Ölçüldü: 21 notluk bir korpus koca bir oturum
boyunca hiç açılmadı, çünkü ajan aramayı çalıştırmayı düşünmedi. Bu yüzden
korpusun **haritası** (not başına tek satır) her oturuma yüklenir; ajan haritada
isabet görünce o notu açar. Harita notun kendisi değildir — "korpusu topluca
yükleme" yasağı bozulmaz.

**Karar muhakemede, disiplin tooling'de.** `ctx close` "şunu hafızaya yaz"
demez; "bu karar verilmeden oturum kapanmasın" der. *Durable bulgu yok* geçerli
bir karardır; karar vermemek geçersizdir.

**Kod hafızadan otoriterdir.** Hafıza canlı kodun yerine geçmez; `git log` ve
`git diff` ondan üstündür.

## Projeye özel her şey `<memory>/ctx.yaml`'da

Araç hiçbir projeye özel varsayım taşımaz:

```yaml
skip_dirs:     [gecis]                  # not sayılmayan ek dizinler
archive_index: gecis/legacy-index.md    # devralınan, KANONİK OLMAYAN indeks
pii_profiles:  [tr]                     # ülkeye özel PII desenleri (tr · us · de)
review_days:   90                       # gözden geçirme aralığı
```

Dosya yoksa güvenli varsayılan: yalnız `tasks/` + `sessions/` atlanır, yalnız
evrensel PII desenleri taranır (e-posta · IBAN · kart), 90 gün.

**PII kararı biçimle değil sağlamayla verilir** — kart için Luhn, IBAN için
ISO 7064 mod-97. Ölçüldü: salt biçime bakan desen `2024 2025 2026 2027`'yi kart
sanıyordu. Yanlış alarm veren kapı kapı değildir; `--no-verify` alışkanlığı
yaratır ve gerçek PII de geçer.

**Arama dile bağlı değildir** — Unicode NFKD + combining düşürme + `casefold()`.
`şablon`/`sablon`, `crèche`/`creche`, `Łódź`/`lodz` aynı anahtara düşer.
`.lower()` tek başına kullanılmaz: `"İ".lower()` iki karakter üretir
(`i` + U+0307) ve arama sessizce eşleşmez.

## Test

```sh
python -m pytest tests/ -q
```

**Altın vektörler** (`tests/vectors/*.json`) davranışı **dilden bağımsız** tarif
eder: normalizasyon, skorlama (mutlak puanlarla), görünürlük, sağlama toplamları,
frontmatter. Python bunları koşar; planlanan Go implementasyonu **aynı dosyaları**
okuyup aynı sonucu üretecek — "doğru mu taşıdım" sorusu kanaat değil, geçen/kalan
sayısı olur.

Her vaka bir `why` alanı taşır: vektör hangi arızayı ya da kararı kilitliyor.
Bir vaka değiştirilecekse önce o cümle okunur (bu zorunluluk da testli).

## Bilinen sınırlar

- Harita **O(N)** büyür (~115 karakter/not): ~150–200 nota kadar rahat, ötesinde
  hiyerarşik harita gerekir.
- `bootstrap` kancaları yalnız Claude Code'a kurar; diğer araçlar dosyadan okur
  (`AGENTS.md` bloğu) ama otomatik kurulumları yoktur.
- Çok yazarlı/ekip senaryosu tanımsız.
