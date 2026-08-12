# Altın vektörler — dilden bağımsız davranış sözleşmesi

Bu klasördeki JSON dosyaları `ctx`'in **davranışını dile bağlı olmadan** tarif
eder. Python implementasyonu (`tests/test_vektorler.py`) ve planlanan Go
implementasyonu **aynı dosyaları** okuyup aynı sonucu üretmek zorundadır.

Amaç: "Go'ya doğru taşıdım mı?" sorusunun bir kanaat değil, **geçen/kalan
sayısı** olması.

## Dosyalar

| Dosya | Neyi kilitler |
|---|---|
| `normalize.json` | Arama normalizasyonu — Unicode katlama, aksan düşürme, çok dillilik |
| `scoring.json` | Alan ağırlıkları, VEYA adaylığı, kapsama çarpanı, tam ifade bonusu |
| `visibility.json` | `status` + `scope` görünürlük kuralı ve eski `lifecycle` çevrimi |
| `checksum.json` | Luhn (kart) ve ISO 7064 mod-97 (IBAN) sağlamaları |
| `frontmatter.json` | Frontmatter ayrıştırma ve liste normalizasyonu |

## Biçim

Her dosya `{version, description, cases[]}` taşır. Her vaka bir `why` alanı
içerir: **vektör neden var**, hangi arızayı ya da kararı kilitliyor. Bir vaka
değiştirilecekse önce o cümle okunmalı.

## Mutlak puanlar hakkında

`scoring.json` mutlak puan bekler (sıralama değil). Ağırlık tablosu bilinçli
olarak değişirse vektörler de değişir — bu istenen davranıştır: sessiz bir
skorlama kayması test kırarak görünür olur.
