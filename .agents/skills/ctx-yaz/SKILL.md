---
name: ctx-yaz
description: Kalıcı bulguyu kanonik hafızaya not olarak yaz (doğru klasör + frontmatter). Kalıcılaştırma testini geçen bilgi için kullan.
---

# ctx-yaz — hafızaya yaz

Yalnız **lider ajan** yazar. Önce kalıcılaştırma testi: *bu bilgi üç ay sonra
başka bir oturumda yeniden keşif maliyetini anlamlı azaltır mı?* Hayırsa
yazma.

## Nereye

Klasör iddiayı söyler — **yeni klasör açma**:

| Klasör | İddia |
|---|---|
| `facts/` | sistem/ürün nasıl çalışıyor |
| `decisions/` | karar + gerekçe |
| `pitfalls/` | bir kez düşülmüş tuzak + kural |

Arşiv ve kapsam klasör DEĞİL, frontmatter'dır (`status: archived`,
`scope: [...]`).

## Biçim

```yaml
---
title: Kısa başlık
description: Tek cümlelik özet (arama bununla çalışır)
status: active | archived
scope: [etiket]            # boşsa evrensel
tags: [etiket1, etiket2]
updated: YYYY-MM-DD        # bugünün tarihi
superseded_by: null
---

## Geçerli

(yalnız bugün geçerli olan: kural + ölçüldüğü gerçek iş)

## Tarihçe

- YYYY-MM-DD · eski → yeni · kimin kararı
```

## Önce ara, sonra yaz

Yazmadan önce `ctx search "<konu>"` çalıştır. Aynı konuda not varsa
**yeni not açma, o notu güncelle**:

- Karar ya da gerçek değiştiyse "Geçerli" bölümünü BAŞTAN yaz. Eski hâli
  Tarihçe'ye tek satır olarak indir (tarih · eski → yeni · kimin kararı).
- Eski kararın yanına parantez ya da yeni bölüm olarak ekleme yapma. Arama
  bütün gövdeyi okur ve iki çelişkili ifade yan yana kalır.
- Ürün kararında hangisinin geçerli olduğunu kullanıcı söyler, emin değilsen
  sor. Sistemin nasıl çalıştığını anlatan notta (fact) kodu oku.
- Bölümsüz eski bir nota dokunuyorsan onu bu düzene geçir.

- "Şöyle olmalı" tek başına yetmez; kaç örnekte, hangi gerçek işte
  ölçüldüğü yazılır. Ölçülmemiş şey "varsayım" diye işaretlenir.
- Not = kural + ölçüm; rapor gömülmez, kanıt gösterilir.

## Neler nota GİRMEZ

- İş durumu (`tasks/OPEN.md`) ve oturum devamı (`sessions/CURRENT.md`) —
  bunlar durum dosyasıdır, durable memory değil.
- Secret/PII: değer değil varlık yazılır ("... ADMIN_SECRET ister; değer bu
  reponun dışında yaşar"). Gerçek isim yerine "örnek mal sahibi".
- Kapanan iş: dosya + `OPEN.md` satırı SİLİNİR, `✅`/`[x]` bırakılmaz.

Yazdıktan sonra `ctx validate` koş; bulgu varsa kapatmadan düzelt.
