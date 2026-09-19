#!/usr/bin/env python3
"""Sürüm paketi: VERSION'daki sürüm için zip + SHA-256 üretir.

Kullanim (repo kökünden):
    python scripts/release.py [--cikti dist/]

Paket (zip kökünde, repo düzeniyle): bin/ctx, bin/ctx.bat, bin/ctx.py,
(+ varsa bin/ctx.exe), templates/**, README.md, SPEC.md, VERSION, LICENSE.

Notlar:
- `bin/ctx.exe` yoksa paketlenmez; sarmalayıcı o makinede Python'a düşer.
  Varsa KAYNAKTAN YENİ olmalı — değilse betik uyarır (`ctx doctor` kuralı).
- Yayın akışı: VERSION'u güncelle → bu betik → GitHub'da `v<VERSION>` etiketi
  + zip + .sha256 dosyasını release'e ekle. Geliştirme dalından paket çıkmaz.
- İmza yok, SHA-256 var; kurulum tarafı checksum'u el ile doğrular.
"""
import argparse
import hashlib
import sys
import zipfile
from pathlib import Path

KOK = Path(__file__).resolve().parent.parent

CALISMA_ANI = ["bin/ctx", "bin/ctx.bat", "bin/ctx.py", "README.md", "SPEC.md",
               "VERSION", "LICENSE"]
# Derlenmis binary opsiyonel ama bayatsa sessizce ESKI kod dagitilir.
DERLENMIS = "bin/ctx.exe"


def tazelik() -> list[str]:
    """ctx.exe kaynaklardan eski mi? (doctor'daki kuralın bakımcı sürümü)."""
    exe = KOK / DERLENMIS
    if not exe.is_file():
        return ["NOT: bin/ctx.exe yok — paket Python'a düşer"]
    yapim = exe.stat().st_mtime
    yeniler = [str(p.relative_to(KOK)) for p in
               [KOK / "bin" / "ctx.py", KOK / "go.mod", KOK / "embed.go",
                *sorted((KOK / "go").rglob("*.go")),
                *sorted((KOK / "templates").rglob("*"))]
               if p.is_file() and p.stat().st_mtime > yapim
               and not p.name.endswith("_test.go")]
    if yeniler:
        return [f"UYARI: ctx.exe {len(yeniler)} kaynaktan eski "
                f"({', '.join(yeniler[:3])}) — önce derleyin"]
    return []


def topla() -> list[Path]:
    dosyalar = []
    for goreceli in CALISMA_ANI:
        p = KOK / goreceli
        if not p.is_file():
            sys.exit(f"HATA: beklenen dosya yok: {goreceli}")
        dosyalar.append(p)
    for p in sorted((KOK / "templates").rglob("*")):
        if p.is_file():
            dosyalar.append(p)
    exe = KOK / DERLENMIS
    if exe.is_file():
        dosyalar.append(exe)
    return dosyalar


def main() -> int:
    ap = argparse.ArgumentParser(description="ctx sürüm paketi üretir")
    ap.add_argument("--cikti", default="dist",
                    help="çıktı dizini (repo köküne göreli)")
    args = ap.parse_args()

    surum = (KOK / "VERSION").read_text(encoding="utf-8").strip()
    for uyari in tazelik():
        print(uyari)
    dosyalar = topla()

    cikti = KOK / args.cikti
    cikti.mkdir(parents=True, exist_ok=True)
    zip_adi = f"ctx-{surum}.zip"
    zip_yolu = cikti / zip_adi
    with zipfile.ZipFile(zip_yolu, "w", zipfile.ZIP_DEFLATED) as z:
        for p in dosyalar:
            z.write(p, p.relative_to(KOK).as_posix())

    ozet = hashlib.sha256(zip_yolu.read_bytes()).hexdigest()
    (cikti / f"{zip_adi}.sha256").write_text(f"{ozet}  {zip_adi}\n",
                                             encoding="utf-8")
    print(f"paket: {zip_yolu} ({len(dosyalar)} dosya)")
    print(f"sha256: {ozet}")
    print(f"etiket önerisi: v{surum}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
