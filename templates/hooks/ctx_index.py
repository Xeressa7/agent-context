#!/usr/bin/env python
"""SessionStart hook: kanonik hafizanin HARITASINI baglama bas.

Cozdugu problem 2026-08-12'de olculdu: 21 notluk korpus koca bir oturum boyunca
hic acilmadi, cunku `ctx search` calistirmayi tetikleyen hicbir sey yoktu. Eski
harness auto-memory'sinin tek gercek ustunlugu indeksinin bedavaya her oturuma
yuklenmesiydi. Bu hook onu geri takar.

Harita `ctx index`ten gelir — AGENTS.md'deki `ctx:index` blogunun AYNI kaynagi.
Yani harness'siz araclar (Codex/Cursor) dosyadan, Claude Code buradan okur; iki
tuketici, tek uretici. Hook dosyaya YAZMAZ (salt-okunur), yalniz basar.

Hafiza kurulu olmayan projelerde (context.yaml yok -> cikis 2) SESSIZCE cikar.
"""
import json
import os
import subprocess
import sys

CTX = os.path.join(os.path.expanduser("~"), ".agent-context", "bin", "ctx.py")


def main():
    if not os.path.isfile(CTX):
        return

    try:
        # encoding acikca utf-8: Windows varsayilani (cp1254) ctx'in utf-8
        # ciktisini mojibake'e ceviriyordu.
        r = subprocess.run(
            [sys.executable, CTX, "index"],
            capture_output=True, text=True, timeout=15, cwd=os.getcwd(),
            encoding="utf-8", errors="replace",
        )
    except Exception:
        return  # hook asla oturumu bozmaz

    harita = (r.stdout or "").strip()
    if r.returncode != 0 or not harita:
        return

    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "SessionStart",
            "additionalContext": (
                "KANONIK HAFIZA HARITASI (her oturumda otomatik yuklenir — "
                "harness auto-memory'si DEGIL, `context.yaml -> memory_path`):\n\n"
                + harita
                + "\n\nKullanim: gorevin konusu yukaridaki bir satirla ortusuyorsa O NOTU AC "
                  "(tam metin orada; bu yalniz harita). Ortusme belirsizse "
                  '`ctx search "<sorgu>"`. Butun korpusu topluca yukleme. '
                  "Hafiza canli kodun yerine gecmez: `git log`/`git diff` otoriterdir."
            ),
        }
    }))


if __name__ == "__main__":
    main()
