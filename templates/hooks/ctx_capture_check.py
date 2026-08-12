#!/usr/bin/env python
"""SessionStart hook: capture karari bekleyen bir onceki oturum var mi?

`ctx close --kontrol` calistirir. Amac: hafizaya yazmayi unutmayi YAPISAL olarak
yakalamak. 2026-08-12 denetiminde bulunan uc bosluktan ucu de "yazilmasi gereken
yazilmadi" sinifindaydi ve hicbir sey uyarmiyordu.

Ilke: KARAR MUHAKEMEDE, DISIPLIN TOOLING'DE. Bu hook "sunu yaz" demez; yalnizca
"onceki oturum karar vermeden kapandi" der. `durable bulgu yok` GECERLI bir karardir.

Hafiza kurulu olmayan projelerde (context.yaml yok) SESSIZCE cikar.
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
        # ciktisini mojibake'e ceviriyordu ("⚠" -> "âš ").
        r = subprocess.run(
            [sys.executable, CTX, "close", "--kontrol"],
            capture_output=True, text=True, timeout=15, cwd=os.getcwd(),
            encoding="utf-8", errors="replace",
        )
    except Exception:
        return  # hook asla oturumu bozmaz

    # 0 = karar guncel · 2 = bu projede hafiza yok → ikisinde de sessiz
    if r.returncode != 1:
        return

    uyari = (r.stdout or "").strip()
    if not uyari:
        return

    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "SessionStart",
            "additionalContext": (
                "CAPTURE KARARI BEKLIYOR — onceki oturum kalici bulgu karari "
                "verilmeden kapanmis:\n\n" + uyari + "\n\n"
                "Bu oturumun SONUNDA karar ver: kalici bir bulgu varsa once "
                "hafizaya yaz, sonra `ctx close --not <yol>`; "
                "yoksa `ctx close --yok \"gerekce\"`. Karar VERMEMEK gecersizdir. "
                "Kullaniciyi simdi bununla mesgul etme — kapanista hallet."
            ),
        }
    }))


if __name__ == "__main__":
    main()
