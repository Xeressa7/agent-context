# {project}-memory — {project} ürün hafızası (canonical)

Bu repo, {project} ürününün **harness-bağımsız kalıcı bilgi deposudur**. Ürünün bütün
repoları bu hafızayı ortak kullanır. Bilgi Markdown + Git'te yaşar; Claude auto-memory,
WrongStack SAGE ve benzeri harness hafızaları canonical DEĞİLDİR, yalnız
önbellek/adapter'dır. Standart: `~/.agent-context/SPEC.md`.

🔴 **TALİMATSIZ COMMIT VE PUSH YASAK.** Kullanıcının açık talimatı olmadan hiçbir
repoda (bu repo dahil) commit atılmaz — dosyalar hazırlanır, `git status`/`git diff`
gösterilir, karar kullanıcınındır.

## Lifecycle sınıfları

| lifecycle | anlamı | kim okur |
|---|---|---|
| `product` | teknoloji-bağımsız ürün/domain gerçeği | herkes |
| `current` | bugünkü mimariye ait karar/bilgi | güncel repolar |
| `legacy` | yalnız eski/bakım modundaki repo için geçerli bilgi | eski ağaç bakımı |
| `historical` | bugünü tarif etmeyen ama karar/hata geçmişi için değerli kayıt | ihtiyaç hâlinde |

Varsayılan retrieval filtresi: güncel repolarda `product + current`, eski ağaçta
`product + legacy`. **Bütün korpus asla topluca okunmaz.**

## Not formatı (frontmatter standardı)

```yaml
---
title: Kısa başlık
description: Tek cümlelik özet (arama bununla çalışır)
lifecycle: product | current | legacy | historical
type: domain | architecture | decision | pitfall | reference
tags: [etiket1, etiket2]
components: []                    # opsiyonel
source: <geldiği eski kaynak>     # varsa
updated: YYYY-MM-DD
superseded_by: null
---
```

## Klasörler

- `domain/` · `architecture/` · `decisions/` · `pitfalls/` · `legacy/<repo>/` · `historical/`
- `tasks/OPEN.md` — TÜM açık işlerin tek adresi (durable memory DEĞİL)
- `sessions/CURRENT.md` — son 1-2 çalışma turu (durable memory DEĞİL)

## Yazma politikası

- Canonical nota yalnız **lider ajan** yazar; alt ajan bulguyu raporlar.
- Kalıcılaştırma testi: *"Bu bilgi üç ay sonra yeniden keşif maliyetini anlamlı
  azaltır mı?"* Hayırsa buraya girmez.
- Task/session durumu nota girmez → `tasks/OPEN.md`.
- Secret/PII asla girmez.
- Çelişkide üstünlük: kullanıcı talimatı → canlı kod/git → normative spec → bu repo →
  historical/legacy → harness hafızası.
