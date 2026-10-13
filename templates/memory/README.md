# {project}-memory — {project} ürün hafızası (canonical)

Bu repo, {project} ürününün **harness-bağımsız kalıcı bilgi deposudur**. Ürünün bütün
repoları bu hafızayı ortak kullanır. Bilgi Markdown + Git'te yaşar; Claude auto-memory,
ajanın/editörün gömülü hafızası canonical DEĞİLDİR, yalnız
önbellek/adapter'dır. Standart: `~/.agent-context/SPEC.md`.

🔴 **TALİMATSIZ COMMIT VE PUSH YASAK.** Kullanıcının açık talimatı olmadan hiçbir
repoda (bu repo dahil) commit atılmaz — dosyalar hazırlanır, `git status`/`git diff`
gösterilir, karar kullanıcınındır.

## Geçerlilik — iki bağımsız eksen

| alan | soru | değerler |
|---|---|---|
| `status` | bu not BUGÜNÜ tarif ediyor mu? | `active` · `archived` |
| `scope` | NEREDE geçerli? | projenin kendi etiketleri; **boş = evrensel** |

`archived` not aramaya ve haritaya girmez; `scope` kesişmeyen not o repoda
görünmez. **Bütün korpus asla topluca okunmaz.**

## Not formatı (frontmatter standardı)

```yaml
---
title: Kısa başlık
description: Tek cümlelik özet (arama bununla çalışır)
status: active                    # active | archived
scope: []                         # boş = her repoda geçerli
tags: [etiket1, etiket2]
components: []                    # opsiyonel
updated: YYYY-MM-DD
---
```

`type` ZORUNLU DEĞİLDİR: notun türünü klasör zaten söyler. Yalnız gerçek bir
alt-tür taşıyorsa yazılır (`type: runbook`).

Gövde iki bölümlüdür: en üstte `## Geçerli` (yalnız bugün geçerli olan), en altta
`## Tarihçe` (değişiklik başına tek satır: tarih · eski → yeni · kimin kararı). Karar
değişince Geçerli BAŞTAN yazılır; yeni karar eskisinin yanına eklenmez. Arama Tarihçe'yi
saymaz (`ctx search --history` sayar). Düzene geçmesi gereken notlar: `ctx audit`.

## Klasörler

Soru şu: **bu ne tür bir iddia ve onu ne geçersiz kılar?**

| Klasör | İddia | Neyle geçersiz olur |
|---|---|---|
| `facts/` | sistem/ürün **nasıl** çalışıyor | gerçeklik değişince |
| `decisions/` | **neden** öyle | yeni karar eskisini geçersiz kılınca |
| `pitfalls/` | ne **ısırdı**, kural ne | tuzak kökten kapanınca |

Hafıza olmayan iki durum dosyası:

- `tasks/OPEN.md` — TÜM açık işlerin tek adresi (durable memory DEĞİL)
- `sessions/CURRENT.md` — son 1-2 çalışma turu (durable memory DEĞİL)

Arşiv ve kapsam için AYRI KLASÖR AÇILMAZ — `status: archived` ve `scope:` bunu
zaten yapar; ikinci bir kayıt yeri iki kaydın sessizce ayrışması demektir.

## Yazma politikası

- Canonical nota yalnız **lider ajan** yazar; alt ajan bulguyu raporlar.
- Kalıcılaştırma testi: *"Bu bilgi üç ay sonra yeniden keşif maliyetini anlamlı
  azaltır mı?"* Hayırsa buraya girmez.
- Task/session durumu nota girmez → `tasks/OPEN.md`.
- Secret/PII asla girmez.
- Kapanan task maddesi SİLİNİR, işaretlenmez — neyin kapandığı git geçmişindedir.
- Çelişkide üstünlük: kullanıcı talimatı → canlı kod/git → normative spec → bu repo →
  arşiv/dar kapsamlı notlar → harness hafızası.
