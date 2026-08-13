# agent-context

Bir ürünün **harness-bağımsız kalıcı bilgi deposu** ve onu yöneten `ctx` aracı.

Bilgi Markdown + Git'te yaşar. Claude auto-memory, editör gömülü hafızası ve
benzerleri **kanonik değildir** — yalnız önbellek/adapter'dır. Amaç: harness
değişince bilginin ölmemesi.

Standart: [`SPEC.md`](SPEC.md) · Sürüm: `VERSION` · Bağımlılık: **yok**
(Python ≥ 3.10 stdlib · Go tarafında yalnız `x/text`)

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
| `ctx hook index` / `hook capture-check` | SessionStart kancaları (harness çağırır) |

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
- `consumers:` listesi elle tutulur; bir repo eklenip listeye yazılmazsa bloğu
  sessizce bayatlar (`ctx doctor` yalnız listedekileri kontrol eder).

## Go geçişi — durum

Kaynaklar `go/` altında, `go.mod` **repo kökünde**. Kökte olmasının tek sebebi
`embed.go`: `go:embed` kendi dizininin dışına çıkamaz, `templates/` ise Python
sürümünün de kullandığı yerde duruyor. Modülü yukarı almak import yollarını
değiştirmedi ve şablonları taşıyan tek dosyalık bir binary kazandırdı — ikinci
bir şablon kopyası tutmak, bu aracın önlemek için var olduğu şeyin ta kendisi
olurdu.

**Her şey Go'da:** sekiz komut (`search` · `index` · `validate` · `doctor` ·
`close` · `init` · `migrate` · `bootstrap`) ve SessionStart kancaları
(`ctx hook index` · `ctx hook capture-check`). Python sürümü hâlâ
**kanoniktir**: değişiklik önce oraya yazılır, vektör ve parite testleri
güncellenir, sonra Go devralır.

### Tek giriş noktası: `bin/ctx`

Çağıran taraf hangi implementasyonun koştuğunu **bilmez**. `bin/ctx` (ve
Windows'ta `bin/ctx.bat`) önce derlenmiş `bin/ctx.exe`'yi, yoksa `ctx.py`'yi
seçer. Kancalar, `pre-commit` ve `settings.json` hep sarmalayıcıyı çağırır —
böylece binary'nin o makinede derlenmiş olup olmaması hiçbir yeri ilgilendirmez.

```sh
go build -o bin/ctx.exe ./go/cmd/ctx    # bundan sonra ctx = Go
rm bin/ctx.exe                          # geri dönüş: yine Python
```

### Kancalar araçta

Eskiden `~/.claude/hooks/` altına iki Python script'i kopyalanırdı; ikisi de
ince sarmalayıcıydı (bir ctx komutu koş, çıktısını JSON zarfına koy) ve ayrı
birer kopya olarak bakım istiyordu. Artık kancalar aracın içinde ve
`settings.json` doğrudan sarmalayıcıyı çağırıyor. **Eski kayıtlar tanınıp
yerinde güncellenir** — tanınmasaydı aynı kanca her oturumda iki kez koşardı.
Geçiş için ilgili projede `ctx bootstrap --proje <ad>` yeter (idempotent ve
yıkıcı değil; önce `--kuru` ile bakılabilir).

Kancaların ilk kuralı **sessizlik**: her oturum başlangıcında koştukları için
hafıza kurulu olmayan bir projede tek bayt bile basmazlar ve asla sıfırdan
farklı bir kodla çıkmazlar.

Beş vektör grubunun tamamı Go'da geçiyor; Go testi Python'un okuduğu **aynı**
JSON dosyalarını okur, böylece iki implementasyon karşılaştırılabilir kalır.

```sh
go test ./...                          # vektörler + store + jsonx  (kökten)
python -m pytest tests/test_parite.py   # uçtan uca: iki sürüm aynı çıktıyı mı veriyor
go build -o bin/ctx.exe ./go/cmd/ctx    # tek dosya binary (şablonlar gömülü)
```

> Vektör dosyalarını düzenledikten sonra `go test -count=1` — vektörler Go
> modülünün dışında yaşadığı için test önbelleği değişikliği fark etmez.

**Bitirme ölçütü karşılandı** (2026-08-13): iki sürüm aynı hafızada yan yana
koşuyor ve sekiz komutun tamamında **bayt bayt** aynı sonucu veriyor — yalnız
çıktı değil, *yazdıkları da*: `ctx:index` bloğu, `capture-log.jsonl` satırı,
`init`in ürettiği dosya ağacı, migrate edilen notun baytları ve kullanıcının
`settings.json`'ı (anahtar sırası dahil). Kanıt bir kanaat değil,
`tests/test_parite.py`: gerçek korpus + kenar durumlarını bilerek içeren
sentetik hafızalar. Çıktı akımı da davranışın parçasıdır — manifest hatası
Python'da **stdout**'a gider, Go da oraya yazar.

**Bilerek eşlenmeyen iki şey** (davranış değil, kütüphane metni): `argparse`in
eksik argüman için bastığı `usage:` bloğu ve `json` modülünün ayrıştırma hatası
cümlesi. İkisinde de karar, çıkış kodu ve dosya etkisi aynıdır.

Parite çalışmasının asıl getirisi hata bulmak oldu; hepsi Python tarafında
düzeltildi:

1. **`ctx doctor` çöküyordu** — hafızada en az bir yapısal hata varken
   `AttributeError`. Bulgu döngüsü manifest yolunu tutan değişkenin üzerine
   yazıyordu; temiz korpusta hiç görünmez. Regresyon: `tests/test_dogrulama.py`.
2. **`ctx migrate` satır sonlarını bozuyordu** — Windows'ta her notu CRLF'e
   çeviriyordu, yani "satır bazlı, gerisine dokunmaz" sözünün tersine tek
   satırlık değişiklik diff'te bütün dosya olarak görünüyordu. İki hata üst
   üste binmişti: `read_text` gerçek satır sonunu gizliyor, `write_text` metin
   modunda yazıyordu. Artık `_blok_degistir` ile aynı kural geçerli.
   Regresyon: `tests/test_migrate.py` (bu komutun ilk testleri).
3. **`capture-log.jsonl`** iki dilde farklı boşlukla yazılıyordu; Python'un
   `json.dumps` biçimi kanonik oldu.

4. **Kanca hafızasız projede 2 ile çıkıyordu** — `SystemExit`, `Exception`ın
   altında olmadığı için `komut_hook`un koruması onu kaçırıyordu ve harness
   açısından bu "kanca patladı" demek. Kancanın ilk kuralı buydu ve ihlal
   ediliyordu.

**Python'un emekliliği için kalan:** teknik engel yok — `bin/ctx.exe` derliyken
her şey Go'dan koşuyor. `bin/ctx.py` hâlâ kanonik referans ve parite testlerinin
karşı tarafı olduğu için duruyor; silinmesi ayrı bir karar.

Python sürümü hâlâ kanoniktir: değişiklik önce oraya yazılır, vektör ve parite
testleri güncellenir, sonra Go devralır.
