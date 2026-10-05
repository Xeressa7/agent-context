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

## Güncelleme

```sh
ctx self-update --check   # yalnız bakar: güncel mi, kaç commit geride
ctx self-update           # fetch + fast-forward; kirli ağaçta DURUR
```

Kurallar: kirli çalışma ağacında güncellenmez (önce commit/stash), derleme
otomatik yapılmaz — `go build -o bin/ctx.exe ./go/cmd/ctx` (yoksa Python
sürümü koşar; `ctx doctor` bayatlığı gösterir). Bakımcı sürüm paketi için
`python scripts/release.py` (zip + SHA-256, `dist/` altına).

Yeni bir projede, proje kökünde:

```sh
ctx bootstrap --proje myapp --components "api,web"
ctx bootstrap --proje myapp --kuru      # önce ne yapacağını göster
```

Hafıza varsayılan olarak proje içinde, `.gitignore`'lu `.memory/` klasörüne
kurulur (kendi Git reposu olarak; projenin reposuna girmez). Bir ürünün birden
çok reposu tek hafızayı paylaşacaksa dışarıda kurun:
`--memory-path ../myapp-memory` — bu durumda `.gitignore`'a dokunulmaz.

Dört adım: hafıza iskeleti + adapter dosyaları (`context.yaml`, AGENTS router,
`CLAUDE.md`, `.agents/skills/` altında `ctx-ara`/`ctx-yaz`/`ctx-kapat`) +
bulunulan reponun hafızanın `consumers` listesine kaydı →
`SessionStart` kancaları
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
| `ctx validate [--tam]` | Not + task/OPEN.md tutarlılığı; `--tam` ile PII/secret taraması |
| `ctx close --not <yol>` / `--yok "<gerekçe>"` | Oturumu capture kararıyla kapatır |
| `ctx migrate` | Eski `lifecycle` alanını `status` + `scope`'a çevirir |
| `ctx doctor` | Kurulum sağlığı |
| `ctx init` / `ctx bootstrap` | Yeni proje kurulumu |
| `ctx hook index` / `hook capture-check` | SessionStart kancaları (harness çağırır) |
| `ctx hook flush` | SessionEnd/PreCompact kancası: ham oturum dökümünü `sessions/.pending/`e düşürür |
| `ctx preferences [--profile normal\\|ekonomik\\|manuel] [--index-budget N]` | Oturum bağlam tercihi: harita bütçesi + otomatik harita |

## Neden bu tasarım
**Kapanış disiplin değil mekanizmadır.** Kapanışta not yazmayı modelin hatırlaması
güvenilmez: kapanış tam da dikkatin ve bütçenin bittiği andır. `ctx hook flush`
transcript'i okuyup ham dökümü diske bırakır (model ÇAĞRILMAZ — kota yok, özyineleme
yok, her sağlayıcıda çalışır); bir sonraki oturumun `capture-check`'i onu yüzeye
çıkarır ve notu yine lider ajan yazar.

**Kapanan iş silinir, işaretlenmez.** İşaretli bırakılan madde iki maliyet üretir:
sonraki oturum onu okumak için token harcar ve iş listesinden aynı işi yeniden
alabilir. Neyin kapandığı git geçmişindedir. `ctx validate` bunu ölçer.

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
go test ./...              # hepsi: vektörler + komutlar (süreçsiz, milisaniyelik)
go test -count=1 ./...     # vektör değişikliğinden sonra (test önbelleği modül
                           # dışındaki JSON'u fark etmez)
```

Testler Go'dadır ve koşan binary'yi test eder (`cli.Env` enjeksiyonuyla tam
komut satırı dahil). Eskiden Python'da koşan diller-arası parite (`pytest`)
görevini tamamlayıp emekli oldu: taşınacak davranış kalmayınca hakeme gerek
kalmadı.

**Altın vektörler** (`tests/vectors/*.json`) davranışı **dilden bağımsız** tarif
eder: normalizasyon, skorlama (mutlak puanlarla), görünürlük, sağlama toplamları,
frontmatter. Go bunları doğrudan okur.

Her vaka bir `why` alanı taşır: vektör hangi arızayı ya da kararı kilitliyor.
Bir vaka değiştirilecekse önce o cümle okunur (bu zorunluluk da testli).

## Bilinen sınırlar

- Harita **O(N)** büyür (~115 karakter/not): ~150–200 nota kadar rahat, ötesinde
  hiyerarşik harita gerekir.
- `bootstrap` kancaları yalnız Claude Code'a kurar; diğer araçlar dosyadan okur
  (`AGENTS.md` bloğu) ama otomatik kurulumları yoktur.
- Çok yazarlı/ekip senaryosu tanımsız.
- Keşif derinliği 1: daha derindeki yerleşimler ne uyarılır ne yazılır
  (`ctx doctor` yalnız komşuları tarar).

## Go geçişi — durum

Kaynaklar `go/` altında, `go.mod` **repo kökünde**. Kökte olmasının tek sebebi
`embed.go`: `go:embed` kendi dizininin dışına çıkamaz, `templates/` ise Python
sürümünün de kullandığı yerde duruyor. Modülü yukarı almak import yollarını
değiştirmedi ve şablonları taşıyan tek dosyalık bir binary kazandırdı — ikinci
bir şablon kopyası tutmak, bu aracın önlemek için var olduğu şeyin ta kendisi
olurdu.

**Her şey Go'da:** on bir komut (`search` · `index` · `validate` · `doctor` ·
`close` · `init` · `migrate` · `bootstrap` · `preferences` · `self-update`) ve
oturum kancaları (`ctx hook index` · `ctx hook capture-check` · `ctx hook flush`).
Go **kanoniktir**: değişiklik oraya yazılır, `go test ./...` güncellenir.
`bin/ctx.py` (Python) duruyor ama referans değil yedek — silinmesi ayrı bir karar.

### Tek giriş noktası: `bin/ctx`

Çağıran taraf detaya girmez: `bin/ctx` (ve Windows'ta `bin/ctx.bat`) derlenmiş
`bin/ctx.exe`'yi koşar. Kancalar, `pre-commit` ve `settings.json` hep
sarmalayıcıyı çağırır.

```sh
go build -o bin/ctx.exe ./go/cmd/ctx    # kurulumun parçası; binary repoda durmaz (gitignore)
```

**Kancayı KABUK üzerinden çalıştırmayan bir ana makine `ctx.exe`'yi doğrudan
çağırmalıdır.** Sarmalayıcı bir POSIX shell script'idir; Windows'ta kabuk
araya girmeden `CreateProcess` ile açılırsa `WinError 193 — geçerli bir Win32
uygulaması değil` verir, `.bat` ise `cmd.exe` ister. Ölçüldü 2026-08-28: Hermes
`on_session_end` kancası tam olarak buna düştü. Claude Code kancaları bash'ten
koştuğu için sarmalayıcıyı kullanmaya devam eder.

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

Beş vektör grubunun tamamı Go'da geçiyor (`tests/vectors/*.json` — dilden
bağımsız davranış sözleşmesi).

```sh
go test ./...                          # vektörler + komutlar (kökten)
go build -o bin/ctx.exe ./go/cmd/ctx    # tek dosya binary (şablonlar gömülü)
```

> Vektör dosyalarını düzenledikten sonra `go test -count=1` — vektörler Go
> modülünün dışında yaşadığı için test önbelleği değişikliği fark etmez.

**Bitirme ölçütü karşılandı** (2026-08-13): iki sürüm aynı hafızada yan yana
koşuyor ve komutların tamamında **bayt bayt** aynı sonucu veriyordu — yalnız
çıktı değil, *yazdıkları da*. Kanıt `tests/test_parite.py` idi (gerçek korpus +
kenar durumlarını bilerek içeren sentetik hafızalar); parite görevini tamamlayıp
emekli oldu, davranış şimdi Go testlerinde kilitli (`go/.../*_test.go`).
Çıktı akımı da davranışın parçasıdır — manifest hatası **stdout**'a gider
(hook'lar çıktıyı oradan okur).

Parite çalışmasının asıl getirisi hata bulmak oldu: hepsi Python tarafında
düzeltildi:

 1. **`ctx doctor` çöküyordu** — hafızada en az bir yapısal hata varken
    `AttributeError`. Bulgu döngüsü manifest yolunu tutan değişkenin üzerine
    yazıyordu; temiz korpusta hiç görünmez. Regresyon o gün `test_dogrulama.py`'ye,
    bugün `validate_test.go`'ya kilitli.
 2. **`ctx migrate` satır sonlarını bozuyordu** — Windows'ta her notu CRLF'e
    çeviriyordu, yani "satır bazlı, gerisine dokunmaz" sözünün tersine tek
    satırlık değişiklik diff'te bütün dosya olarak görünüyordu. İki hata üst
    üste binmişti: `read_text` gerçek satır sonunu gizliyor, `write_text` metin
    modunda yazıyordu. Regresyon bugün `migrate_test.go`'da.
3. **`capture-log.jsonl`** iki dilde farklı boşlukla yazılıyordu; Python'un
   `json.dumps` biçimi kanonik oldu.

4. **Kanca hafızasız projede 2 ile çıkıyordu** — `SystemExit`, `Exception`ın
   altında olmadığı için `komut_hook`un koruması onu kaçırıyordu ve harness
   açısından bu "kanca patladı" demek. Kancanın ilk kuralı buydu ve ihlal
   ediliyordu.

**Python emekli edildi:** `bin/ctx.py` silindi, parite testleri görevini tamamlayıp
kaldırıldı, davranış Go testlerinde kilitli. Kurulum `go` gerektirir
(`go build` yukarıda); sarmalayıcı binary yoksa temiz hata verir.
