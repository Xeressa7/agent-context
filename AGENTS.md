## Hafıza

Canonical ürün hafızası: `.memory` (Git + Markdown; ayrıntı: oradaki README).
Bu repo için retrieval filtresi **product + current**. Göreve başlamadan görev
kavramları/dosya yolları ile orada ara (`ctx search` varsa onu kullan, yoksa rg);
bütün korpusu asla yükleme. Harness-native hafızalar (ajan/editör gömülü hafızası)
canonical DEĞİLDİR. Alt ajanlara ilgili notlar prompt'a gömülerek verilir —
ajanın gönüllü aramasına güvenilmez.

Hafıza canlı kodun yerine geçmez: `git log` ve `git diff` her ikisinden de otoriterdir.
Oturum sonunda capture kararı ver: kalıcı bulgu varsa nota yazıp `ctx close --note <yol>`,
yoksa `ctx close --none "<gerekçe>"` — karar vermemek geçersizdir.

<!-- ctx:index:basla -->
### Hafıza haritası — agent-context · kapsam: current · 1 not

Bu bir **harita**, notun kendisi değil: ilgili satırı görünce **o notu aç**. Hedefli arama `ctx search "<sorgu>"`; bütün korpusu topluca yükleme.
Açık işler `.memory/tasks/OPEN.md` · oturum devamı `.memory/sessions/CURRENT.md`.

**decisions/**
- `hatirlatma-tetigi.md` — Notları bağlama otomatik getiren kanca kullanıcı mesajıyla değil ajanın dokunduğu dosyayla…
<!-- ctx:index:bitir -->
